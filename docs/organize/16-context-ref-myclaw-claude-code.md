# 参考项目梳理：Claude Code 的上下文管理 / 自动压缩 / Token 用量展示

> 参考代码库：`C:\Users\ytq\work\ai\myclaw\claude-code`
> 梳理日期：2026-07-03
> 目的：为 agent-builder（Go + Wails3 + React）实现 1) 长对话自动压缩；2) 发送框 context 用量图标；3) 压缩中/压缩完成提示；4) 按 provider/model 配置 context window 与 auto-compact 阈值 提供实现参考。

## 0. 这个目录是什么

该目录**不是官方开发仓库**，而是一份 Claude Code 的**完整 TypeScript 源码快照 + 中文研究文档**（README.md 自述："围绕 claude-code 当前源码快照整理出来的中文研究文档"）。特点：

- `src/`：约 34MB 的可读 TS/TSX 源码（非混淆），带完整注释、内部实验开关（`feature('...')` / GrowthBook 灰度）、埋点。个别 UI 组件（如 `TokenWarning.tsx`、`Settings/Config.tsx`）是 React Compiler 编译产物形态（带 `_c(n)` memo cache），但底部 sourceMap 内嵌了原始源码，且逻辑仍可读。
- `docs/`：45 篇中文研究文档，其中与本主题直接相关：`docs/19-context-compression-and-history-management.md`（上下文压缩与历史管理）、`docs/38-read-file-state-and-context-cache-mechanics.md`、`docs/34-history-snip-and-replay-projection.md`。
- 技术栈：Bun + TypeScript + React(Ink 终端 UI)。UI 层是终端 TUI，但压缩/统计逻辑与 UI 完全分层，可直接映射到 Go 后端 + React 前端。

与本主题相关的核心目录：

| 位置 | 内容 |
|---|---|
| `src/services/compact/` | autoCompact.ts（阈值/触发）、compact.ts（执行压缩，1705 行）、prompt.ts（摘要 prompt）、microCompact.ts（裁剪旧 tool result）、sessionMemoryCompact.ts、apiMicrocompact.ts、compactWarningState.ts、postCompactCleanup.ts、grouping.ts |
| `src/utils/tokens.ts` | 当前上下文 token 用量计算（canonical） |
| `src/utils/context.ts` | 模型 context window 解析、用量百分比 |
| `src/utils/model/modelCapabilities.ts` | 从 `/v1/models` API 自动获取模型能力并缓存 |
| `src/utils/analyzeContext.ts` | `/context` 的分类统计（1382 行） |
| `src/commands/compact/`、`src/commands/context/` | `/compact`、`/context` 命令 |
| `src/components/TokenWarning.tsx`、`src/components/ContextVisualization.tsx`、`src/components/messages/CompactBoundaryMessage.tsx` | UI 展示 |
| `src/query.ts` (395-650 行) | 查询主循环中 snip → microcompact → collapse → autocompact → 阻断检查 的调用顺序 |

---

## 1. 模型 context window 的确定方式

### 1.1 解析优先级（`src/utils/context.ts:51` `getContextWindowForModel(model, betas)`）

默认值常量：`MODEL_CONTEXT_WINDOW_DEFAULT = 200_000`（context.ts:9）。解析顺序：

1. **环境变量硬覆盖**（仅内部用户）：`CLAUDE_CODE_MAX_CONTEXT_TOKENS`——"cap the effective context window for local decisions (auto-compact, etc.) while still using a 1M-capable endpoint"。
2. **模型名 `[1m]` 后缀**（context.ts:35 `has1mContext`，正则 `/\[1m\]/i`）→ 返回 `1_000_000`。用户通过 `/model sonnet[1m]` 显式选择 1M 窗口，这是最主要的 1M 入口。可被 `CLAUDE_CODE_DISABLE_1M_CONTEXT` 环境变量全局禁用（HIPAA 合规用途）。
3. **模型能力缓存**（context.ts:74）：`getModelCapability(model)?.max_input_tokens >= 100_000` 时直接使用该值（见 1.2）。若大于 200k 但 1M 被禁用，则钳回 200k。
4. **beta header**：请求 betas 中含 `CONTEXT_1M_BETA_HEADER` 且 `modelSupports1M(model)`（canonical 名含 `claude-sonnet-4` 或 `opus-4-6`）→ 1M。
5. **灰度实验**：`getSonnet1mExpTreatmentEnabled`——sonnet-4-6 且远端配置 `clientDataCache['coral_reef_sonnet'] === 'true'` → 1M。
6. **兜底**：`200_000`。

### 1.2 自动获取模型能力表（`src/utils/model/modelCapabilities.ts`）

- `refreshModelCapabilities()`（:85）调用 Anthropic SDK 的 `anthropic.models.list()` 分页拉取，用 zod schema `{id, max_input_tokens?, max_tokens?}` 解析，写入磁盘缓存 `~/.claude/cache/model-capabilities.json`（0600 权限，带 timestamp）。
- 读取端 `getModelCapability(model)`（:75）是**同步**的（memoized `readFileSync`），先精确匹配 id，再做"model 字符串包含 capability.id"的子串匹配；缓存写入时按 **id 长度降序排序**（:54 `sortForMatching`），保证子串匹配命中最具体的条目。
- 仅在第一方 Anthropic API 时启用（`isModelCapabilitiesEligible`）。**结论：CC 采用"API 自动获取 + 磁盘缓存 + 硬编码默认值兜底"的三层方案。**

### 1.3 max output tokens 表（压缩时要预留）

`src/utils/context.ts:149` `getModelMaxOutputTokens(model)` 是一张按 canonical 模型名 substring 匹配的硬编码表：opus-4-6→64k/128k、sonnet-4-6→32k/128k、opus-4-5/sonnet-4/haiku-4→32k/64k、claude-3-opus→4096、3-5-sonnet→8192、3-7-sonnet→32k/64k，默认 32k/64k；同样会被 modelCapabilities 缓存里的 `max_tokens` 修正上限。

### 1.4 /model 的影响

`/model` 改变 `mainLoopModel`，所有阈值函数都以 model 字符串为参数实时重算；`[1m]` 后缀是模型选择的一部分。升级提示逻辑在 `src/utils/model/contextWindowUpgradeCheck.ts`：当用户当前是 `opus`/`sonnet` 且有 1M 权限时，在警告栏附加 `` `/model sonnet[1m]` `` 提示，或在 /compact 完成后附加 `Tip: You have access to Sonnet 1M with 5x more context`。

---

## 2. 当前上下文用量的计算

核心文件：`src/utils/tokens.ts`。**CC 不自己数消息 token，而是信任最近一次 API 响应的 usage 字段，再对其后新增的消息做粗估补偿。**

### 2.1 基础公式

```ts
// tokens.ts:46  一次 API 调用时的完整上下文大小
getTokenCountFromUsage(usage) =
  usage.input_tokens
  + (usage.cache_creation_input_tokens ?? 0)
  + (usage.cache_read_input_tokens ?? 0)
  + usage.output_tokens        // 注意：包含 output，因为 output 会成为下一轮的 input
```

`getTokenUsage(message)`（tokens.ts:7）只从**非合成**（非 SYNTHETIC_MODEL、非合成文本）的 assistant 消息取 usage。

### 2.2 canonical 函数：`tokenCountWithEstimation(messages)`（tokens.ts:226）

**所有阈值判断（autocompact、session memory）都必须用它**（注释明确警告不要用累计计数——会重复计算；不要只用 output_tokens）。算法：

1. 从尾部向前找到最近一个带真实 usage 的 assistant 消息；
2. **并行 tool call 修正**：流式返回时一次 API 响应会被拆成多条 assistant 记录（同 `message.id`），且 tool_result 穿插其间。若锚定在最后一条会漏算前面穿插的 tool_result，所以向前回溯到**同 message.id 的第一条**（:235-252）；
3. 返回 `getTokenCountFromUsage(usage) + roughTokenCountEstimationForMessages(其后的所有消息)`。
4. 整个数组都没有 usage 时，全量粗估。

粗估函数 `roughTokenCountEstimation`（`src/services/tokenEstimation.ts:203`）就是 **`content.length / 4`**（4 字节/token）。microCompact.ts:164 `estimateMessageTokens` 逐 block 估算后再 **×4/3 上浮**保守化；图片/文档按固定 2000 token（`IMAGE_MAX_TOKEN_SIZE`，microCompact.ts:38）。

### 2.3 UI 用量的取数

- 输入框下方警告条（`src/components/PromptInput/Notifications.tsx:74-82`）：`tokenUsage = tokenCountFromLastAPIResponse(getMessagesAfterCompactBoundary(messages))`——只取 compact boundary 之后的消息、只取最近一次 usage（不做粗估补偿，UI 不需要那么准）。
- `/context` 的总量（analyzeContext.ts:1163-1174）：`getCurrentUsage()` 取最近 usage，总量 = `input_tokens + cache_creation + cache_read`（**不含 output**，与状态栏口径一致），无 API usage 时回退到估算总和。
- 百分比工具函数 `calculateContextPercentages`（context.ts:118）：`used = round((input+cache_creation+cache_read) / contextWindow * 100)`，clamp 到 0-100。

---

## 3. 自动压缩 autocompact

### 3.1 阈值体系（`src/services/compact/autoCompact.ts`，全部为具体数字）

```ts
const MAX_OUTPUT_TOKENS_FOR_SUMMARY = 20_000   // :30 为压缩摘要输出预留（p99.99 摘要=17,387 tok）
export const AUTOCOMPACT_BUFFER_TOKENS       = 13_000  // :62
export const WARNING_THRESHOLD_BUFFER_TOKENS = 20_000  // :63
export const ERROR_THRESHOLD_BUFFER_TOKENS   = 20_000  // :64
export const MANUAL_COMPACT_BUFFER_TOKENS    =  3_000  // :65
const MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES   = 3       // :70 熔断
```

层层推导（`autoCompact.ts:33/72/93`）：

```
effectiveContextWindow = contextWindow − min(modelMaxOutput, 20_000)
                         （可被 env CLAUDE_CODE_AUTO_COMPACT_WINDOW 进一步压小）
autoCompactThreshold   = effectiveContextWindow − 13_000
                         （env CLAUDE_AUTOCOMPACT_PCT_OVERRIDE 可按百分比覆盖，取更小值）
threshold(用于percentLeft) = autocompact启用 ? autoCompactThreshold : effectiveContextWindow
percentLeft            = max(0, round((threshold − tokenUsage)/threshold × 100))
warningThreshold       = threshold − 20_000     // 开始显示 "x% until auto-compact"
errorThreshold         = threshold − 20_000     // 同值；关闭autocompact时文案变红
blockingLimit          = effectiveContextWindow − 3_000
                         （env CLAUDE_CODE_BLOCKING_LIMIT_OVERRIDE 可覆盖）
```

以 200k 模型（max output 32k→预留 20k）为例：effective=180k，**autocompact 在 167k（约 83.5% 总窗口）触发**，警告条在 147k 出现，阻断线 177k。

### 3.2 触发判断（`shouldAutoCompact`，autoCompact.ts:160）

- 每轮查询循环、发 API 请求**之前**检查（`src/query.ts:453` `deps.autocompact(...)`，顺序：HISTORY_SNIP 裁剪 → microcompact → context-collapse → **autocompact** → blocking limit 检查 → callModel）。
- 递归护栏：`querySource === 'session_memory' || 'compact'` 直接 false（压缩 agent 自己不许再压缩，防死锁）。
- 开关：`isAutoCompactEnabled()`（:147）= `!DISABLE_COMPACT && !DISABLE_AUTO_COMPACT && globalConfig.autoCompactEnabled`。
- 判据：`tokenCountWithEstimation(messages) − snipTokensFreed >= autoCompactThreshold`。
- **熔断器**（autoCompactIfNeeded :257）：连续失败 ≥3 次后本 session 不再尝试（注释：曾有 1279 个 session 连续失败 50+ 次、全球日浪费 ~25 万次 API 调用）。成功后归零。

### 3.3 执行流程（`compactConversation`，compact.ts:387）

1. `preCompactTokenCount = tokenCountWithEstimation(messages)`；`setSDKStatus('compacting')`；`onCompactProgress({type:'hooks_start', hookType:'pre_compact'})` → 执行 PreCompact hooks（可注入自定义摘要指令）。
2. `onCompactProgress({type:'compact_start'})` → UI 显示 "Compacting conversation" spinner。
3. **调用摘要**（`streamCompactSummary` :1136）：
   - 首选 **prompt-cache 共享的 fork agent** 路径（`runForkedAgent`，maxTurns:1，`canUseTool` 永远 deny——"Tool use is not allowed during compaction"），复用主对话的缓存前缀，摘要请求作为一条 user message 附加；
   - 失败则回退到普通流式调用：system prompt 仅一句 `"You are a helpful AI assistant tasked with summarizing conversations."`，`thinking: disabled`，maxOutput = min(20_000, 模型上限)，发送前 `stripImagesFromMessages`（图片/文档替换为 `[image]`/`[document]` 文本占位，包括 tool_result 内嵌的）；
   - 压缩期间每 30s 发 keep-alive 防远程 WebSocket 超时（:1167）。
4. **压缩请求自身 prompt-too-long 的重试**（:450-491，CC-1180）：响应文本以 `'Prompt is too long'` 开头 → `truncateHeadForPTLRetry`（:243）按 **API round 分组**（grouping.ts）从**最旧的组**开始丢，丢够 tokenGap（从错误信息解析）或兜底丢 20% 的组，最多重试 `MAX_PTL_RETRIES = 3` 次；仍失败抛 `ERROR_MESSAGE_PROMPT_TOO_LONG = 'Conversation too long. Press esc twice to go up a few messages and try again.'`（:293）。
5. **重建上下文**：
   - 清空 `readFileState`（已读文件缓存）与 `loadedNestedMemoryPaths`；
   - 生成 boundary：`createCompactBoundaryMessage(trigger:'auto'|'manual', preTokens, lastUuid)`（messages.ts:4530）→ `{type:'system', subtype:'compact_boundary', content:'Conversation compacted', compactMetadata:{trigger, preTokens, ...}}`；
   - 摘要消息：`createUserMessage({content: getCompactUserSummaryMessage(...), isCompactSummary:true, isVisibleInTranscriptOnly:true})`；
   - 附件（attachments）重注入：**最近读过的文件**（`createPostCompactFileAttachments` :1415，最多 `POST_COMPACT_MAX_FILES_TO_RESTORE=5` 个文件、总预算 `POST_COMPACT_TOKEN_BUDGET=50_000`、单文件 `POST_COMPACT_MAX_TOKENS_PER_FILE=5_000`，按 timestamp 最近优先，排除 plan/CLAUDE.md 文件，并**去重**保留段里已有的 Read 结果）；**plan 文件**、**plan mode 指令**、**已调用 skills 内容**（单 skill 截断 5k、总预算 25k，截断标记提示可 Read 全文）、**后台异步 agent 状态**、tools/agents/MCP instructions delta 公告；
   - 执行 SessionStart(source='compact') hooks 与 PostCompact hooks。
6. 新消息数组顺序（`buildPostCompactMessages` :330）：`[boundaryMarker, ...summaryMessages, ...(messagesToKeep), ...attachments, ...hookResults]`，替换掉整个对话（REPL 保留旧消息用于滚动回看，发 API 时用 `getMessagesAfterCompactBoundary` 截取 boundary 之后）。
7. 收尾（finally）：`onCompactProgress({type:'compact_end'})`、`setSDKStatus(null)`；成功后 `suppressCompactWarning()`（见 4.1）、`runPostCompactCleanup()`（清各类模块级缓存，postCompactCleanup.ts:31）。
8. 埋点 `tengu_compact` 记录 pre/post token、是否会下一轮再次触发（`willRetriggerNextTurn: truePostCompactTokenCount >= threshold`）等。

### 3.4 Summarization prompt 关键内容（`src/services/compact/prompt.ts`）

- **禁工具前导**（:19，放最前面）：`CRITICAL: Respond with TEXT ONLY. Do NOT call any tools. ... Tool calls will be REJECTED and will waste your only turn`（针对 4.6 自适应思考模型爱调工具的问题，尾部还有 NO_TOOLS_TRAILER 重申）。
- **主 prompt**（BASE_COMPACT_PROMPT :61）：先在 `<analysis>` 标签里逐消息分析（用户显式请求、方案、关键决策、文件名/代码片段/函数签名/报错与修复、用户纠偏反馈），再输出 `<summary>`，固定 9 节：
  1. Primary Request and Intent；2. Key Technical Concepts；3. Files and Code Sections（含代码片段与重要性说明）；4. Errors and fixes（重点记用户反馈）；5. Problem Solving；6. **All user messages**（全部非工具结果的用户消息——防意图漂移的关键）；7. Pending Tasks；8. Current Work；9. Optional Next Step（必须与最近显式请求直接相关，**要求引用原文 verbatim** 防任务漂移）。
- 支持用户/hook 追加 `Additional Instructions`（/compact 参数即此）。
- **后处理**（:311 `formatCompactSummary`）：正则**剔除 `<analysis>` 草稿**（只为提升质量、无信息价值），`<summary>` 换成 `Summary:` 头。
- **注入回对话的包裹文案**（:337 `getCompactUserSummaryMessage`）：`This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.` + 摘要 + **完整 transcript 文件路径**（"If you need specific details from before compaction ... read the full transcript at: ..."）+ autocompact 时追加"直接继续干活、不要复述摘要、不要问用户问题"（`Resume directly — do not acknowledge the summary... Pick up the last task as if the break never happened.`）。
- 另有 partial compact 的两个变体（PARTIAL / PARTIAL_UP_TO，:145/:208）：用户在消息选择器中选中某条消息后可只摘要该点之后（保留前缀缓存）或之前（保留近期原文）。

### 3.5 microcompact（裁剪旧 tool result，`src/services/compact/microCompact.ts`）

三条路径，都只针对 `COMPACTABLE_TOOLS`（Read/Bash 系/Grep/Glob/WebSearch/WebFetch/Edit/Write，:41）：

1. **时间触发型**（:446 `maybeTimeBasedMicrocompact`，GrowthBook 配置 `tengu_slate_heron`，默认 `{enabled:false, gapThresholdMinutes:60, keepRecent:5}`，timeBasedMCConfig.ts:30）：距上次 assistant 消息超过 60 分钟（服务端 1h prompt cache 必已过期，反正要全量重写）→ 把除最近 5 个之外的可裁剪 tool_result 的 content 直接替换为 `'[Old tool result content cleared]'`，仅主线程。
2. **cached microcompact**（ant-only 实验，`feature('CACHED_MICROCOMPACT')`）：利用 API 的 cache-editing 能力，不改本地消息，在 API 层追加 `cache_edits` 块删除旧 tool result，同时保住缓存前缀。
3. **API 原生 context management**（apiMicrocompact.ts:64 `getAPIContextManagement`）：请求体带 `context_management.edits`，策略 `clear_tool_uses_20250919`（trigger: input_tokens 180_000，clear_at_least: 180k−40k=140k，即压回 40k）与 `clear_thinking_20251015`（保留策略；>1h 冷缓存时只留最后 1 轮 thinking）。工具清理部分 ant-only + 环境变量 `USE_API_CLEAR_TOOL_RESULTS`/`USE_API_CLEAR_TOOL_USES`/`API_MAX_INPUT_TOKENS`/`API_TARGET_INPUT_TOKENS` 控制。

microcompact 产生 `microcompact_boundary` 系统消息（messages.ts:4569，UI **不渲染**，Message.tsx:246 返回 null——微压缩对用户透明）。

### 3.6 session memory compaction（优先于摘要压缩的实验路径）

`autoCompactIfNeeded` 与 `/compact` 都**先尝试** `trySessionMemoryCompaction`（sessionMemoryCompact.ts:514）：若后台持续维护的 session memory 文档已有内容，则**不再调用模型做摘要**，直接用该文档充当摘要 + 保留 `lastSummarizedMessageId` 之后的消息（保留区间配置 `DEFAULT_SM_COMPACT_CONFIG = {minTokens:10_000, minTextBlockMessages:5, maxTokens:40_000}` :57，可被 GrowthBook `tengu_sm_compact_config` 覆盖）。压缩后若估算 token 仍超阈值则放弃回退传统压缩（:605）。`calculateMessagesToKeepIndex` + `adjustIndexToPreserveAPIInvariants`（:232）保证保留边界**不拆散 tool_use/tool_result 对与同 message.id 的 thinking 块**（否则 API 报 orphan tool_result 错）。

### 3.7 出错处理与保底

- 错误常量（compact.ts:225-297）：`'Not enough messages to compact.'`、`'Conversation too long. Press esc twice to go up a few messages and try again.'`、`'API Error: Request was aborted.'`、`'Compaction interrupted · This may be due to network issues — please try again.'`。
- autocompact 失败：**不弹错误通知**（下一轮会重试，:749-755），只累计熔断计数；手动 /compact 失败才 `addNotification({text:'Error compacting conversation', color:'error'})`。
- **阻断线**（query.ts:637）：未压缩且超过 blockingLimit 时不发请求，直接 yield 一条 `'Prompt is too long'` 的合成错误 assistant 消息。
- **reactive compact**（ant-only 实验，`feature('REACTIVE_COMPACT')`，源文件 reactiveCompact.ts 不在本快照中，仅有调用点）：完全不做事前压缩，等 API 返回 413/prompt-too-long 再压。
- **context collapse**（ant-only 实验，`feature('CONTEXT_COLLAPSE')`）：后台 ctx-agent 按 span 渐进摘要（90% 提交/95% 阻断），启用时 autocompact 被抑制（autoCompact.ts:215 有长注释解释两者会互相竞态）。

### 3.8 手动 /compact（`src/commands/compact/compact.ts`）

`/compact [自定义指令]`：参数即 customInstructions 附加到摘要 prompt。流程 = 先试 session memory（无自定义指令时）→ microcompact → `compactConversation(suppressFollowUpQuestions=false, isAutoCompact=false)`。完成后返回 `type:'compact'` 结果，UI 显示 `Compacted (ctrl+o to see full summary)`（buildDisplayText :230，可附升级 tip 与 hook 消息）。

---

## 4. UI / 展示

### 4.1 状态警告条 "x% until auto-compact"（`src/components/TokenWarning.tsx`）

- 挂在输入框下方 Notifications 区（`PromptInput/Notifications.tsx:321` `<TokenWarning tokenUsage={tokenUsage} model={mainLoopModel} />`）。
- `tokenUsage >= warningThreshold` 才显示；autocompact 开启时灰字 `` `${percentLeft}% until auto-compact` ``（percentLeft 是**相对 autocompact 阈值**而非总窗口的百分比，见 3.1 公式）；关闭时黄/红字 `Context low (x% remaining) · Run /compact to compact & continue`（超 errorThreshold 变 error 色）。可附 `· /model sonnet[1m]` 升级提示。
- **压缩成功后立即抑制**（compactWarningState.ts:8 一个全局 boolean store + `useSyncExternalStore` hook）：因为压缩后要到下一次 API 响应才有准确 usage，期间旧 tokenUsage 会误报，故 `suppressCompactWarning()`；下一次 microcompact 尝试开始时 `clearCompactWarningSuppression()`。

### 4.2 /context 分布明细

两个实现共享同一数据层 `collectContextData`（commands/context/context-noninteractive.ts:34）→ `analyzeContextUsage`（analyzeContext.ts:918）。**统计前先复现发 API 前的全部变换**（取 compact boundary 之后 + collapse 投影 + microcompact），保证显示的就是模型实际看到的。

分类统计（并行执行，:950-983）：
- **System prompt**：构建有效 system prompt 后用 **count_tokens API** 精确计数（`countMessagesTokensWithAPI`，失败回退 Haiku/粗估），并按 section 细分（ant-only）；
- **System tools**（内置工具 schema，逐工具计数并减去每次调用固定 ~500 token 的 API 工具前导开销 `TOOL_TOKEN_COUNT_OVERHEAD`，analyzeContext.ts:75）；
- **MCP tools**（bulk 一次 API 调用；tool search 启用时列为 "MCP tools (deferred)" 且**不计入用量**）；
- **Custom agents**、**Memory files**（各 CLAUDE.md 逐文件路径+token）、**Skills**（frontmatter 逐条）；
- **Messages**：microcompact 后的消息用 count_tokens API 计总量，另有 ant-only 细分（tool calls / tool results / attachments / assistant / user，及 Top Tools 表）；
- **Autocompact buffer**（= contextWindow − autoCompactThreshold，作为一个"预留"分类显示；autocompact 关闭时显示 3k 的 "Compact buffer"）与 **Free space**。
- 总量优先用最近 API usage（`input + cache_creation + cache_read`）对齐状态栏口径（:1163-1174）。

渲染两种形态：
- 非交互/SDK：markdown 表格（`formatContextAsMarkdownTable`，含 `**Tokens:** 45.3k / 200k (23%)` 头行）；SDK 还有 `get_context_usage` 控制请求走同一函数。
- 交互式 TUI（`ContextVisualization.tsx`）：**方格网格图**——200k 模型 10×10=100 格（窄屏 5×5）、1M 模型 20×10=200 格，每格代表 1% 左右，按分类着色；空心/实心棋子字符区分填充度（`⛀`/`⛁`，`⛝` 表示 Autocompact buffer 预留格，Free space 画空格），旁边图例列出每类名称+token 数。分类固定配色（analyzeContext.ts:1010-1096：System prompt/System tools/MCP tools/Custom agents/Memory files/Skills/Messages/预留/Free space）。

### 4.3 压缩进行中 / 完成提示

- 进行中：`onCompactProgress` 回调（Tool.ts:155 定义 `compact_start`/`compact_end`/`hooks_start` 事件），REPL.tsx:2497 映射为 spinner 文案：`Running PreCompact hooks…` → `Compacting conversation`（蓝色系统 spinner）→ 结束清空。远程/SDK 侧显示 `'Compacting conversation…'`（sdkMessageAdapter.ts:98），SDK status 置 `'compacting'`。
- 完成后对话流内：boundary 消息渲染为居中一行灰字 `✻ Conversation compacted (ctrl+o for history)`（CompactBoundaryMessage.tsx）；摘要正文标记 `isCompactSummary + isVisibleInTranscriptOnly`，默认视图折叠、仅在 transcript 模式（ctrl+o）展开（Message.tsx:159）。
- 手动 /compact 额外输出一行 `Compacted (ctrl+o to see full summary)`。

---

## 5. 设置项与环境变量

### 5.1 设置

| 项 | 位置 | 说明 |
|---|---|---|
| `autoCompactEnabled: boolean`，**默认 true** | 全局配置 `~/.claude.json`（utils/config.ts:234/594，GLOBAL_CONFIG_KEYS :638） | 唯一的用户级 autocompact 开关 |
| /config 面板 "Auto-compact" 布尔项 | components/Settings/Config.tsx:267 | 改动即 saveGlobalConfig + 埋点 `tengu_auto_compact_setting_changed` |

注意：**CC 没有用户可配的"阈值百分比"设置**——阈值全部由 contextWindow 推导 + 常量 buffer，只有环境变量能改。

### 5.2 环境变量（全部实测于源码）

| 变量 | 作用 | 位置 |
|---|---|---|
| `DISABLE_COMPACT` | 禁用一切压缩（含手动） | autoCompact.ts:148 |
| `DISABLE_AUTO_COMPACT` | 只禁自动压缩，/compact 仍可用 | autoCompact.ts:152 |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW` | 把 autocompact 计算用的窗口钳小 | autoCompact.ts:40 |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | 按 effectiveWindow 百分比设阈值（测试用，取与默认阈值的较小者） | autoCompact.ts:79 |
| `CLAUDE_CODE_BLOCKING_LIMIT_OVERRIDE` | 覆盖阻断线 | autoCompact.ts:127 |
| `CLAUDE_CODE_MAX_CONTEXT_TOKENS` | （ant）硬覆盖 context window | utils/context.ts:60 |
| `CLAUDE_CODE_DISABLE_1M_CONTEXT` | 禁用 1M 上下文（合规） | utils/context.ts:31 |
| `USE_API_CLEAR_TOOL_RESULTS` / `USE_API_CLEAR_TOOL_USES` / `API_MAX_INPUT_TOKENS`(180k) / `API_TARGET_INPUT_TOKENS`(40k) | API 原生 context_management 策略 | apiMicrocompact.ts:94-134 |

灰度（GrowthBook）项：`tengu_compact_cache_prefix`（缓存共享 fork，默认 true）、`tengu_slate_heron`（时间触发 MC 配置）、`tengu_sm_compact_config`、`tengu_cobalt_raccoon`（reactive-only）、`coral_reef_sonnet`（sonnet 1M 实验）。

---

## 6. 精华与糟粕（Go 后端 + React 前端桌面应用视角）

### 值得借鉴的精华

1. **"信 API usage、只估增量"的用量口径**（tokens.ts:226）：不要在客户端全量数 token。以最近一次响应的 `input + cache_creation + cache_read (+ output)` 为锚，仅对其后未发送的消息做 `len/4` 粗估。准确、零成本、任何 provider 通用。
2. **阈值 = 窗口 − 摘要输出预留 − 固定 buffer**，全链条推导而非拍脑袋百分比：`effective = window − min(maxOutput, 20k)`，`threshold = effective − 13k`，警告再提前 20k，手动兜底留 3k。数值可直接抄。
3. **压缩后上下文重建清单**：boundary 标记（含 preTokens 元数据，供 UI 与统计）→ 摘要（固定 9 节结构、`<analysis>` 草稿丢弃、"All user messages" 一节防意图漂移、Next Step 要求 verbatim 引用）→ 最近 5 个已读文件回填（50k 预算）→ plan/todo/已用 skills/后台任务状态回填 → transcript 路径兜底（"细节丢了可以去读原始记录"）。这份清单几乎可以原样搬进 agent-builder。
4. **三层防线而非单一压缩**：microcompact（廉价、透明地清旧 tool result）→ autocompact（阈值摘要）→ blocking limit（拒发请求）+ PTL 重试（压缩请求自己超长时按 API-round 分组丢头部）。加上**熔断器**（连续 3 次失败停手）和**压缩后抑制警告**两个小状态，鲁棒性都来自这些细节。
5. **UI 事件协议**：后端仅发 `compact_start / compact_end / hooks_start` 三个进度事件 + `compacting` 状态，前端自行决定 spinner/文案；boundary 作为一种消息类型持久化在对话流里渲染为分隔线，摘要正文默认折叠。对 Wails 事件桥是现成的设计。
6. **模型能力自动获取**：`models.list` → 磁盘 JSON 缓存（带时间戳、长 id 优先的子串匹配）→ 硬编码默认表兜底。agent-builder 的"按 provider/model 配 context window，能自动获取更好，有默认值"可完全照此三层结构；OpenAI 兼容 provider 拿不到时走用户配置 + 默认 200k/128k 表。
7. **/context 统计先复现发送前变换**（compact boundary 截取 + microcompact）再统计，保证"显示的就是模型看到的"；总量与状态栏用同一 usage 口径，避免两处数字对不上。

### 应避免的糟粕

1. **实验开关缠绕核心路径**：autoCompact.ts / TokenWarning.tsx 里 REACTIVE_COMPACT、CONTEXT_COLLAPSE、CACHED_MICROCOMPACT 等 feature/require 分支互相解释谁抑制谁，注释长过代码。agent-builder 应定义单一 `ContextStrategy` 接口，一次只挂一个策略。
2. **模块级全局可变状态**：cachedMCState、compactWarningStore、sentSkillNames 等模块级单例导致"子 agent 压缩会污染主线程状态"，postCompactCleanup.ts 整个文件都在为此擦屁股（querySource.startsWith('repl_main_thread') 判断谁能清什么）。Go 侧应把这些状态挂在 per-session/per-agent 结构体上。
3. **循环依赖 workaround**：大量 `require()` inline、"importing that file pulls in ... circular-deps loop" 式注释、靠测试断言两份复制常量不漂移（microCompact.ts:36）。分层要先想清楚：types → token 计量 → 压缩服务 → UI。
4. **多处近似重复的 token 估算器**（roughTokenCountEstimation / estimateMessageTokens ×4/3 / analyzeContext 的逐 block 版），口径差异只靠注释维系。应收敛为一个带选项的函数。
5. **同名字段语义漂移**：`postCompactTokenCount` 实际是"压缩 API 调用的总用量"而非压缩后上下文大小，为埋点连续性保留错名（compact.ts:626 注释）。新项目直接用 `truePostCompactTokenCount` 这种明确命名。
6. **UI 组件里做业务计算**：TokenWarning 组件内部直接调 `calculateTokenWarningState` 并 require 各策略模块。agent-builder 应由 Go 侧算好 `{percentLeft, level, label}` 通过事件推给 React，前端纯渲染。

---

## 7. 对 agent-builder 的借鉴要点（对应四个需求）

### 7.1 长对话自动压缩（Go 侧）

- 在 runtime service 的查询循环、每次调用 provider **之前**插入检查点：`usedTokens = lastUsage.input + cacheCreation + cacheRead (+ output) + estimate(newMessages)`（estimate = utf8len/4）。
- 阈值：`threshold = contextWindow − reserveOutput(≤20k) − buffer(13k)`；支持按 provider/model 覆盖 contextWindow 与 buffer；超过即触发压缩。
- 压缩实现：以现有消息 + 一条"摘要指令" user 消息调用当前模型（禁止工具、maxOutput 20k、thinking 关闭、图片替换为占位文本）；摘要 prompt 直接采用 3.4 的 9 节结构（含 NO_TOOLS 前导/尾注、`<analysis>` 草稿剔除）。
- 重建：`[compactBoundary 事件消息, 摘要(user, isCompactSummary), 最近读过的文件回填(≤5 个/50k), 未完成 todo/plan, 运行中后台任务状态]`；持久化 boundary 到会话存储，后续请求只发 boundary 之后。
- 保底：压缩请求超长 → 从最旧的"assistant 起始的 API 轮次组"开始丢再试（≤3 次）；连续失败 3 次熔断本会话；autocompact 失败静默重试、手动失败才报错；`usedTokens >= window − 3k` 时拒发并提示。
- 消息边界完整性：保留/丢弃切分点必须校验 tool_use/tool_result 配对（参考 adjustIndexToPreserveAPIInvariants，OpenAI 系同样有 tool_call/tool 消息配对要求）。

### 7.2 发送框右下角 context 用量图标（React 侧）

- Go 每轮响应后推送事件：`{contextWindow, usedTokens, percentUsed, percentLeftUntilCompact, breakdown:{systemPrompt, tools, skills, memory, messages, reserved, free}}`。breakdown 的 systemPrompt/tools 可在会话启动时算一次缓存（有 count-tokens API 就用，否则 len/4），messages 用 `usedTokens − 固定开销`反推即可，不必逐条精算。
- 图标交互参考 /context：点开显示 总量/已用/百分比 + 分类列表（或 10×10 方格图，1M 用 20×10）；"Autocompact buffer" 作为一个显式分类展示，让用户理解为什么不到 100% 就压缩。
- 警告分级照抄：`percentLeft`（相对 compact 阈值）≤ 某值时输入框下方出现灰字 "x% until auto-compact"；禁用 autocompact 时改为 warning/error 色的 "Context low (x%) · 手动压缩" 提示；**压缩成功后到下一次响应前抑制该提示**。

### 7.3 对话流压缩提示

- 复用事件协议：`compact_start` → 对话流插入进行中气泡/spinner "正在压缩对话…"；`compact_end` + 成功 → 持久化一条 boundary 消息，渲染为分隔线 "✻ 对话已压缩（点击查看完整摘要）"，摘要正文默认折叠；失败（仅手动）→ toast "压缩失败"。
- runtime_events.go 已有输出流事件机制，追加 `context_compaction` 事件类型即可。

### 7.4 设置：按 provider/model 配 context window 与阈值

- 三层解析（照抄 1.1/1.2）：用户显式配置 > provider API 自动获取（Anthropic `models.list` 的 `max_input_tokens`；OpenAI 兼容端点普遍拿不到则跳过）缓存到本地 JSON（带 timestamp，启动异步刷新，同步读缓存）> 内置默认表（Claude 200k/1M、GPT-4o 128k、…，兜底 128k 或 200k）。
- 设置面板：全局 `autoCompactEnabled`（默认 true）+ 每 provider/model 可选覆盖 `contextWindow`、`compactThresholdBuffer`（默认 13k）或百分比；不必暴露 warning buffer 等次级参数。
- 可选支持环境变量覆盖用于调试（对应 `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` 的作用：把阈值调低到 1% 即可快速人工验证整条压缩链路）。

### 7.5 建议的落地顺序

1. tokens 计量（usage 锚点 + 粗估）与 contextWindow 解析表 → 2. 用量事件 + 前端图标/警告条 → 3. 手动 /compact（摘要 prompt + boundary + 重建）→ 4. autocompact 阈值触发 + 熔断/保底 → 5. /context 式分布明细 → 6.（可选）microcompact 清旧 tool result 与 provider 原生 context_management（Anthropic `clear_tool_uses_20250919`）。

---

## 8. 深挖：多层压缩级联体系（逐层定位准确术语与编排）

> 本章逐一核对"tool result budget / snip / micro / collapse / auto / reactivate"六个层。源码中的准确术语以粗体给出；名称对不上的明确标注。研究文档佐证：`docs/19-context-compression-and-history-management.md`（明确说"当前快照里至少有四层治理"）、`docs/34-history-snip-and-replay-projection.md`。

### 8.0 层级总表（按查询循环中的实际执行顺序）

每轮 query、发 API 请求之前，`src/query.ts` 按以下顺序依次执行（行号为调用点）：

| # | 准确术语（源码） | 调用点 | 触发条件/阈值 | 处理对象 | 处理方式 | 是否改写持久化 |
|---|---|---|---|---|---|---|
| 0a | **per-tool result limit + persist-to-disk**（`maxResultSizeChars` / `toolResultStorage.ts`） | 工具结果写入管线 `processToolResultBlock` | 单条结果 > min(工具声明值, 50k chars)；Bash 自身另有 30k 截断 | 单条 tool_result | 全文写盘 `tool-results/<id>.txt`，上下文中替换为 `<persisted-output>` 路径+2k 预览 | 是（消息本身即预览态落盘） |
| 0b | **message-level aggregate tool result budget**（`enforceToolResultBudget`） | query.ts:379 `applyToolResultBudget` | 单个 API 级 user 消息内 tool_result 合计 > 200k chars | 一轮并行工具的结果集合 | 最大的"新"结果写盘换预览；决策冻结（seenIds）保证缓存前缀稳定 | 否（投影；决策记录另存 `ContentReplacementEntry`） |
| 1 | **snip compact**（`snipCompactIfNeeded`，feature `HISTORY_SNIP`，ant-only） | query.ts:403 | 未知（源文件不在快照） | 历史**中段**消息区间 | 从运行时消息数组真正移除，产生 snip boundary（含 `snipMetadata.removedUuids`） | 否（append-only；resume 时按 removedUuids 重放删除） |
| 2 | **microcompact**（`microcompactMessages`：time-based MC / cached MC / API context management） | query.ts:414 | time-based：距上次 assistant 消息 >60min；cached MC：按条数（GB 配置）；API 层：input>180k | 旧的可裁剪工具结果（Read/Bash/Grep/Glob/Web*/Edit/Write） | 替换为 `[Old tool result content cleared]` / API cache_edits 删除 | 否（只改发 API 的数组/请求体） |
| 3 | **context collapse**（`applyCollapsesIfNeeded`，feature `CONTEXT_COLLAPSE`，ant-only 实验） | query.ts:441 | 90% 开始提交、95% 阻断（autoCompact.ts:207 注释） | 按 span 分段的历史 | 后台 ctx-agent 渐进摘要 span，`projectView` 读时投影 | 否（commit log 独立存储，读时重放） |
| 4 | **autocompact**（`autoCompactIfNeeded`；内部先试 **session memory compaction**，再 `compactConversation`） | query.ts:454 | tokens ≥ effective−13k（200k 模型约 167k） | 整段对话 | 摘要替换 + boundary + 附件重建（见 §3.3） | 否（boundary/summary 追加写；旧消息保留，见 §9） |
| 5 | **blocking limit** 检查 | query.ts:637 | tokens ≥ effective−3k 且未压缩成功 | 本次请求 | 拒发，yield `'Prompt is too long'` 错误消息 | — |
| 6 | **reactive compact**（`tryReactiveCompact`，feature `REACTIVE_COMPACT`，ant-only） | query.ts:1120（API 出错**之后**） | API 返回 prompt-too-long(413) | 整段对话 | 事后压缩再重试本轮 | 否 |

编排要点（均为源码注释原文）：query.ts:369-372 "Enforce per-message budget ... Runs BEFORE microcompact — cached MC operates purely by tool_use_id (never inspects content), so content replacement is invisible to it and the two compose cleanly"；query.ts:396 "Apply snip before microcompact (both may run — they are not mutually exclusive)"；query.ts:428-431 "(collapse) Runs BEFORE autocompact so that if collapse gets us under the autocompact threshold, autocompact is a no-op and we keep granular context instead of a single summary"。总原则：**廉价、局部、无损的层先跑；昂贵、全局、有损的摘要层最后兜底**。snip 释放的 token 通过 `snipTokensFreed` 参数传给 autocompact 阈值判断（query.ts:397-399：幸存 assistant 消息的 usage 仍反映 snip 前上下文，`tokenCountWithEstimation` 看不到节省，必须显式扣减）。

### 8.1 第 0 层：tool result budget（写入时预算）

用户所称 "tool result budget" 在源码中对应**两级机制**，且 CC 的哲学是 **"persist to disk instead of truncating"**（`src/utils/toolResultStorage.ts:1` 文件头注释："Utility for persisting large tool results to disk instead of truncating them"）：

**(a) 工具自身生成时截断**：如 Bash 的 `formatOutput`（`src/tools/BashTool/utils.ts:133`）：超过 `BASH_MAX_OUTPUT_DEFAULT = 30_000` chars（上限 150k，env `BASH_MAX_OUTPUT_LENGTH` 可调，`src/utils/shell/outputLimits.ts`）截头保留并附 `... [N lines truncated] ...`。

**(b) 单条结果持久化阈值**（`src/constants/toolLimits.ts` + `toolResultStorage.ts`）：
- 每个工具声明 `maxResultSizeChars`（Tool.ts:466）：Bash=30_000（BashTool.tsx:424），**Read=Infinity 永不持久化**（FileReadTool.ts:342；注释：把 Read 结果写盘再让模型 Read 回来是循环）；
- 全局钳制 `DEFAULT_MAX_RESULT_SIZE_CHARS = 50_000`（`getPersistenceThreshold` 取 min，GB `tengu_satin_quoll` 可按工具覆盖）；绝对上限 `MAX_TOOL_RESULT_TOKENS = 100_000`（×4 字节 = 400KB）；
- 超限处理（`maybePersistLargeToolResult` :272）：全文写入 `<projectDir>/<sessionId>/tool-results/<tool_use_id>.txt|json`（`wx` 标志防重写），tool_result content 替换为：

```
<persisted-output>
Output too large (X MB). Full output saved to: <filepath>

Preview (first 2.0 kB):
<前 2000 字节、按换行边界截断的预览>
...
</persisted-output>
```

模型看到路径后可自行 Read 恢复——**这是"被压内容的恢复通道"之一**。
- 附带修复：空 tool_result 替换为 `(<toolName> completed with no output)`（:287，inc-4586：prompt 尾部空结果会诱发某些模型直接结束回合）。

**(c) 消息级聚合预算**（`enforceToolResultBudget` :769，GB 开关 `tengu_hawthorn_steeple`）：`MAX_TOOL_RESULTS_PER_MESSAGE_CHARS = 200_000`（toolLimits.ts，注释举例"10 × 40K = 400K in one turn's user message"）——防止 N 个并行工具各自低于单条阈值、合计爆炸。按 **API 级 user 消息分组**（连续 user 消息在 wire 上被 normalizeMessagesForAPI 合并，预算分组必须一致，:600 有长注释）；组内超预算时**按 size 降序**挑最大的"新"结果写盘换预览。关键设计：`ContentReplacementState {seenIds, replacements}`——**一旦某条结果以原文发过给模型，永远冻结不再替换；替换过的每轮字节级一致重放**（:749 注释），否则破坏 prompt cache 前缀。

相邻的读去重机制：**FILE_UNCHANGED_STUB**（`src/tools/FileReadTool/prompt.ts:7`）——同一文件重复 Read 且未变更时，结果替换为 `'File unchanged since last read. The content from the earlier Read tool_result in this conversation is still current — refer to that instead of re-reading.'`。

### 8.2 第 1 层：snip compact

**准确术语：snip / snipCompact**（feature `HISTORY_SNIP`，ant-only）。⚠️ **实现文件 `snipCompact.ts` / `snipProjection.ts` 不在本快照中**（条件 require + DCE 排除，query.ts:115；docs/34:19 明确说明"实际文件未出现在可见快照中，本章只依据调用点和注释解释其协议"）。从调用点可确认：

- 位置：query.ts:401-410，`snipCompactIfNeeded(messagesForQuery)` → `{messages, tokensFreed, boundaryMessage}`；boundary 会 yield 进对话流并有专门 UI 组件（`SnipBoundaryMessage`，Message.tsx:256）。
- 与 compact 的本质区别（sessionStorage.ts:1962 注释原文）："**Unlike compact_boundary which truncates a prefix, snip removes middle ranges**"——compact 砍前缀、snip 挖**中段**（如一次跑偏的长探索）。
- 存在一个**模型可调用的 snip 工具**：messages.ts:197 "Used for snip tool referencing — injected into API-bound messages as [id:...] tags"、:1618 "This lets Claude reference message IDs when calling the snip tool"——发给 API 的消息带 `[id:...]` 标签供模型引用要剪除的消息区间。
- snip boundary 携带 `snipMetadata.removedUuids`（被删消息 UUID 清单），是 resume 重放的依据（§9.3）。
- docs/34 定性：snip 是"运行时内存治理协议"——REPL 保留 full history 供 UI 回看，SDK/headless 路径真正缩短消息 store 以便 GC；曾发生 resume 未重放 snip 导致 "397K displayed → 1.65M actual" 直接 PTL 的事故（sessionStorage.ts:1966 注释）。

### 8.3 第 2 层：microcompact

**准确术语：microcompact**（`src/services/compact/microCompact.ts`，§3.5 已详述）。补充"是否可恢复"：

- 处理对象仅限 `COMPACTABLE_TOOLS`（Read/Bash 系/Grep/Glob/WebSearch/WebFetch/Edit/Write，:41）。
- 占位文本：`TIME_BASED_MC_CLEARED_MESSAGE = '[Old tool result content cleared]'`（microCompact.ts:36；与 toolResultStorage.ts:34 `TOOL_RESULT_CLEARED_MESSAGE` 同字符串，靠测试断言两处不漂移）。
- 阈值：time-based 路径 gap>60min、保留最近 5 个（GB `tengu_slate_heron`，默认 disabled）；cached MC 按条数触发/保留（GB 配置，ant-only，`cachedMicrocompact.ts` 亦不在快照）；API 原生路径（apiMicrocompact.ts）trigger=180k input tokens、压回 40k。
- 产生 `microcompact_boundary` 系统消息但 **UI 不渲染**（Message.tsx:246 返回 null）——对用户完全透明。
- **可恢复性**：time-based 清空只作用于发给 API 的数组副本，**内存 REPL 数组与 JSONL 原文均保留**（§9.4）；若结果曾被第 0 层写盘，模型仍可循 `<persisted-output>` 路径 Read 回全文。cached MC 根本不改本地消息（纯 API cache_edits）。

### 8.4 第 3 层：collapse

**准确术语：context collapse**（feature `CONTEXT_COLLAPSE`，querySource 代号 `marble_origami`，执行者叫 ctx-agent）。⚠️ **`src/services/contextCollapse/` 目录不在本快照中**（已验证不存在），但调用点/注释可确认：

- 机制：后台 **ctx-agent** 把历史按 **span（消息区段）** 渐进摘要——"staged"（已生成待提交）→"committed"（提交到 collapse store 的 commit log）；主循环每轮 `projectView(messages)` 做**读时投影**（query.ts:433 注释："the collapsed view is a read-time projection over the REPL's full history. Summary messages live in the collapse store, not the REPL array"）。
- 阈值梯度（autoCompact.ts:207 注释原文）："the 90% commit / 95% blocking-spawn flow owns the headroom problem"——90% 开始提交折叠、95% 阻断。
- 与 micro 的区别：micro 是**无损占位清除单条工具结果**；collapse 是**有损分段摘要**，但比 autocompact 的全量单一摘要**粒度细**（保留多段独立摘要）。collapse 启用时 autocompact 被显式抑制（autoCompact.ts:215：autocompact 阈值 ~93% 恰好落在 collapse 的 90%/95% 区间之内，会竞态并"nuking granular context that collapse was about to save"）。
- UI：TokenWarning 显示 `x / y summarized`（CollapseLabel）；/context 显示 `**Context strategy:** collapse (N spans summarized (M messages), K staged)`（context-noninteractive.ts:113-147）。

### 8.5 第 4 层：autocompact 的编排关系补充

已在 §3 详述，补充优先级：

- autocompact 内部：**session memory compaction 优先**（有后台记忆文档就不花钱调模型摘要，autoCompact.ts:288），不可用才 `compactConversation`。
- 手动 `/compact`：session memory →（reactive-only 模式走 reactive 路径）→ **先跑一遍 microcompact 再摘要**（commands/compact/compact.ts:97-99 "Run microcompact first to reduce tokens before summarization"）。
- 让位规则：reactive-only 模式（GB `tengu_cobalt_raccoon`）与 context collapse 启用时，`shouldAutoCompact` 直接返回 false。

### 8.6 第 5 层："reactivate compact" —— 源码中无此术语

**源码中不存在 "reactivate compact"。最接近的机制有两个，按所指分别对应：**

1. 若指"**事后响应式压缩**"：准确术语是 **reactive compact**（feature `REACTIVE_COMPACT`，ant-only；`reactiveCompact.ts` 不在快照，仅有调用点）。不做事前压缩，等 API 返回 prompt-too-long(413) 后在 query.ts:1119（`isWithheld413`）调 `tryReactiveCompact` 压缩再重试本轮；`/compact` 在 reactive-only 模式下改走 `reactiveCompactOnPromptTooLong`（commands/compact/compact.ts:175）。compact.ts:239 注释提到它有"proper retry loop that peels from the tail"（从尾部逐段剥离的重试循环）；它同时是 collapse 模式下的 413 兜底（autoCompact.ts:207 "keeps reactiveCompact alive as the 413 fallback"）。
2. 若指"**被压内容的重新激活/回填**"：无统一术语，是分散的多条恢复通道——
   - **压缩时主动回填**（§3.3 第 5 步）：最近 5 个已读文件重读注入、plan/todo、已调用 skills（截断 5k/条）、后台任务状态；
   - **transcript 路径兜底**：摘要里附完整 JSONL 路径，"If you need specific details from before compaction ... read the full transcript at: ..."（prompt.ts:349）；
   - **persisted-output 文件**：第 0 层写盘的工具结果全文永远可 Read 回来；
   - **FILE_UNCHANGED_STUB 反解**：压缩时发现保留段里的 Read 结果只是去重 stub，则重新注入真实文件内容（compact.ts:1602 注释："the stub points at an earlier full Read that may have been compacted away, so we want createPostCompactFileAttachments to re-inject the real content"）。

---

## 9. 深挖：压缩与会话持久化（JSONL transcript）的交互

> 核心结论先行：**Claude Code 选择"JSONL 永远 append-only、原始消息永不改写/删除；一切压缩都是发 API 时的投影 + 追加式 boundary 标记；resume 时靠 parentUuid 链 + boundary 元数据重放出压缩后的有效上下文"。** 这正是 agent-builder "SQLite 永远保留原始数据、压缩只影响 API 投影"方案的直接背书。

### 9.1 append-only 与 uuid 去重

- 每条消息作为一行 `TranscriptMessage` 追加写入 JSONL（`src/utils/sessionStorage.ts` `insertMessageChain` :1000-1069），带 `parentUuid` 形成链；`recordTranscript`（:1408）按 uuid 去重——**已写过的消息跳过，绝不重写**。
- 明确注释：sessionStorage.ts:1963 "**The JSONL is append-only, so removed messages stay on disk**"；:3230 "…JSONL forever"。compact/snip/microcompact 都不会回头修改已写入的行。

### 9.2 compact 相关内容的持久化标记

- **boundary**：`{type:'system', subtype:'compact_boundary', content:'Conversation compacted', compactMetadata:{trigger:'auto'|'manual', preTokens, preCompactDiscoveredTools?, preservedSegment?:{headUuid, anchorUuid, tailUuid}}}`（messages.ts:4530；preservedSegment 由 `annotateBoundaryWithPreservedSegment` 写入，compact.ts:349）。
- **写入时的链断点**（sessionStorage.ts:1040-1041，关键设计）：boundary 落盘时 `parentUuid: null`（**物理切断链**，`--resume` 的链回溯自然止于此），原 parent 移入 `logicalParentUuid`（保留逻辑链接，供查看压缩前历史；:699 注释：compact 时先写 metadata 再写 boundary，"this is what enables loadTranscriptFile's pre-compact …"）。
- **摘要消息**：普通 user 消息 + `isCompactSummary: true`（识别用；resume 选取会话标题时跳过它，:1751）+ `isVisibleInTranscriptOnly: true`（UI 默认折叠，仅 ctrl+o transcript 模式展开）。
- **messagesToKeep（保留段）**：因 uuid 去重**不重写**，磁盘上保留其压缩前的 parentUuid；靠 boundary 的 `preservedSegment` 三元组在读取时重接（§9.3）。`recordTranscript` 的"前缀跳过"规则（:1391-1407 长注释）保证压缩后新消息链到 summary 而非旧 uuid，否则"orphaning the compact boundary"。
- **snip boundary**：`snipMetadata.removedUuids` 记录被删消息 UUID 清单。
- **microcompact boundary**：`{subtype:'microcompact_boundary', microcompactMetadata:{trigger, preTokens, tokensSaved, compactedToolIds, clearedAttachmentUUIDs}}`（messages.ts:4569）。
- **tool result budget 决策**：独立的 `ContentReplacementEntry` 追加写入 transcript，内容 `{kind:'tool-result', toolUseId, replacement}`（query.ts:376-388，仅 `repl_main_thread*`/`agent:*` 持久化）；`replacement` 存**模型实际看到的完整替换字符串**——"stored rather than derived on resume so code changes to the preview template … can't silently break prompt cache"（toolResultStorage.ts:470）。

### 9.3 resume 如何重建"压缩后的有效上下文"

resume 不是"重放 boundary 之后的行"，而是**读全部 JSONL 行 → 从最新 leaf 沿 parentUuid 反向走链（`buildConversationChain`，sessionStorage.ts:2069）→ 应用重放修正**：

1. **`applyPreservedSegmentRelinks`**（:1839，处理 compact）：取最后一个带 `preservedSegment` 的 boundary，把保留段 head 的 parent 补接到 anchor（summary 或 boundary）、anchor 的其他子节点接到 tail；**先验证 tail→head 链可走通，走不通则放弃修剪、加载全量压缩前历史**并埋点 `tengu_relink_walk_broken`（:1888——宁可多花 token 也不丢消息）；然后从内存 Map **删除**绝对最后一个 boundary 之前的所有非保留消息。
   - **关键坑**（:1920-1939）：把保留段内 assistant 消息的 usage 四字段**全部清零**——磁盘上的 input_tokens 反映压缩前 ~190K 上下文，"Without this, resume → immediate autocompact spiral"（恢复后立即误触发自动压缩死循环）。"信 usage"计量方案在持久化侧必须配套这一步。
2. **`applySnipRemovals`**（:1982，处理 snip）：收集所有 snip boundary 的 `removedUuids` 从 Map 删除，并把 parentUuid 悬空的幸存者沿被删区段的 parent 链回溯重接（带路径压缩），埋点 `tengu_snip_resume_filtered`。
3. tool result budget 状态用 `reconstructContentReplacementState`（toolResultStorage.ts:960）从 ContentReplacementEntry 重建：替换字符串原样重放；transcript 里出现过的所有 tool_use_id 全部标 frozen（"在 transcript 里 = 发给过模型 = fate 已定"）。
4. `recoverOrphanedParallelToolResults` 后处理（:2118）：并行 tool call 的 DAG 拓扑在单亲链走链时会丢兄弟分支（同 message.id 的拆分 assistant + 各自 tool_result），读侧修复。

即：**有效上下文 = 全量行 + 走链 + 按 boundary 元数据重放压缩决策**。压缩语义完全编码在追加写入的元数据里，由加载器重放。

### 9.4 microcompact 对持久化的影响：完全不落盘、每轮重算

- time-based microcompact 只替换**本轮发给 API 的消息数组副本**（query.ts:414 `messagesForQuery`），REPL 内存数组与 JSONL 均保留原文；**resume 后旧 tool result 原样回来**，等待下一次 microcompact/预算再处理（无状态、幂等）。
- 直接佐证：toolResultStorage.ts:158 注释——"prevents re-writing the same content on every API turn **when microcompact replays the original messages**"（明确说 microcompact 每轮从原始消息重放）。
- cached MC 更彻底：本地消息一个字节不改，纯在 API 请求层加 cache_edits。
- 对比第 0b 层预算：同样不改写历史行，但**必须**把替换决策持久化为 ContentReplacementEntry——它要求跨 turn/跨 resume 的 prompt cache 字节级稳定；microcompact 不需要（触发时缓存本来就已冷或在缓存尾部之外）。

### 9.5 对 agent-builder 的结论

1. **采用"DB 永远保留原始数据、压缩只是投影 + 追加式事件"**——这就是 CC 的实际选择，理由在注释中反复出现：UI 回看要全量、恢复要全量、决策可重放、出错可回退（relink 走不通就加载全量）。SQLite 方案：`messages` 表永不 UPDATE 内容；新增 `compaction_events` 表存 boundary（trigger、preTokens、preserved 区间、removed ids）；加载会话时按 events 重放出 API 投影。
2. **必须复刻的两个坑**：a) 恢复时把保留段 assistant 的 usage 清零（或恢复后首轮改用估算），否则立即误触发 autocompact；b) 保留/删除切分点不能拆散 tool_call/tool_result 配对与同响应的拆分块。
3. **可大幅简化处**：agent-builder 用 SQLite 而非 parentUuid 链式 JSONL，可用显式 `active_from_message_id` 列/投影视图代替 CC 的"链断点 + logicalParentUuid + relink"机制，复杂度低一个量级；snip/collapse 这类 ant-only 实验层可不做。
4. **tool result budget 值得优先做**：单条 min(工具声明, 50k chars) 写盘换 2k 预览 + 单轮聚合 200k 预算 + Bash 生成端 30k 截断，是所有层里性价比最高、最无风险的（无损、可恢复、零模型调用），能显著推迟 autocompact 到来。若做预算层，注意"决策冻结 + 替换串持久化"以保 prompt cache 稳定；若不追求缓存极致，可简化为无状态每轮重算（即 microcompact 模式）。
