# cc-haha 上下文管理 / 自动压缩 / Token 用量展示 — 参考梳理

> 参考项目路径：`C:\Users\ytq\work\ai\cc-haha`
> 本文所有行号基于 2026-07 当前工作区快照，供直接跳转，不保证长期稳定。

## 0. 项目简介

cc-haha 是 Claude Code CLI 的本地化重建版（`package.json` name 为 `claude-code-local`，bin 为 `claude-haha`），语言栈：

- **核心 CLI**：TypeScript + Bun（`src/`），Ink 终端 UI（React 组件渲染到终端）。内含完整的 query 循环、工具系统、compact 服务、SDK 入口。
- **桌面端**：`desktop/`，Vite + React（Electron / Tauri 双壳），通过内置 HTTP/WS server（`src/server/`）与 CLI 会话（SDK 子进程）通信，UI 经 control request 向 CLI 拉取上下文快照。
- **多 Provider**：内置 DeepSeek / GLM / Kimi / MiniMax 等 preset（`src/server/config/providerPresets.json`），全部走 Anthropic 兼容协议，用环境变量向 CLI 注入 baseUrl、模型映射、context window 配置。

架构上和 agent-builder（Go 后端 + React 前端桌面应用）同构：**"引擎（CLI/后端）负责计量与压缩，前端只做展示 + 定时拉取"**，非常有参考价值。

---

## 1. 模型 Context Window 的来源

结论：**三层来源 —— 内置硬编码表（含正则模式）→ 配置（env JSON / provider 设置）→ API 自动获取（models.list，仅内部构建启用）**，默认兜底 200k。

### 1.1 解析优先级（核心函数）

`src/utils/context.ts:58` `getContextWindowForModel(model, betas)`，优先级从高到低：

1. env `CLAUDE_CODE_MAX_CONTEXT_TOKENS`（仅内部用户）——强制覆盖一切；
2. 模型名带 `[1m]` / `:1m` 后缀（`has1mContext`，context.ts:42）→ 直接 1,000,000（客户端显式 opt-in）；
3. `getConfiguredOrBuiltInModelContextWindow`（见 1.2）：用户配置优先于内置表；
4. OpenAI Codex 模型表（`src/services/openaiAuth/models.ts`）；
5. **模型能力缓存**（见 1.3）`cap.max_input_tokens`（要求 ≥100k 才采信）；
6. 1M beta header + `modelSupports1M`（sonnet-4 / opus-4-6）→ 1,000,000；
7. 兜底 `MODEL_CONTEXT_WINDOW_DEFAULT = 200_000`（context.ts:16）。

另有 `CLAUDE_CODE_DISABLE_1M_CONTEXT`（HIPAA 合规场景）会把所有 >200k 的结果压回 200k。

### 1.2 硬编码表 + env 配置

`src/utils/model/modelContextWindows.ts`：

- `DIRECT_MODEL_CONTEXT_WINDOWS`（第 5-34 行）精确表，示例数值：
  - `claude-opus-4-7: 1_000_000`、`claude-sonnet-4-6: 200_000`、`claude-haiku-4-5: 200_000`
  - `deepseek-v4-pro / deepseek-chat: 1_000_000`
  - `kimi-k2.x 系列: 262_144`、`minimax-m3: 1_000_000`、`minimax-m2.7: 204_800`
  - `glm-5.2: 1_000_000`、`glm-5/5.1: 200_000`、`glm-4.5: 128_000`
- `PATTERN_MODEL_CONTEXT_WINDOWS`（第 36-53 行）正则表：`gpt-5 → 400_000`、`gpt-4.1 → 1_047_576`、`gemini-2.x/3 → 1_048_576`、`qwen-long → 10_000_000` 等。
- **env 配置入口**：`CLAUDE_CODE_MODEL_CONTEXT_WINDOWS`（JSON，如 `{"my-model": 262144}`），键做归一化（去 `[1m]` 后缀、小写），支持 `xxx/model`、`xxx:model` 后缀匹配；值域校验 `16_000 ~ 10_000_000`（第 2-3 行）。**配置优先于内置表**（第 160-167 行）。

### 1.3 API 自动获取（可选增强）

`src/utils/model/modelCapabilities.ts`：

- `refreshModelCapabilities()`（第 85 行）调 `anthropic.models.list()` 拉全量模型的 `{id, max_input_tokens, max_tokens}`，写入 `~/.claude/cache/model-capabilities.json`（0600 权限，内容不变则跳过写入）；
- `getModelCapability()`（第 75 行）同步读缓存（memoize），先精确匹配 id 再子串匹配（列表按 id 长度降序，最长最特异优先）；
- 注意：该能力被 `isModelCapabilitiesEligible()` 限制为**内部用户 + 一方 API** 才启用 —— 对第三方 provider 它们不信任 models.list。

### 1.4 Provider 级配置（桌面端设置 → env 注入）

- preset 定义：`src/server/config/providerPresets.json` —— 每个 provider 带 `modelContextWindows`（如 deepseek 全系 1M、kimi 262144）和 `defaultEnv.CLAUDE_CODE_AUTO_COMPACT_WINDOW`（deepseek/GLM/minimax 都是 `"1000000"`）。
- schema：`src/server/types/provider.ts:53-54` —— `AutoCompactWindowSchema = z.number().int().min(16000).max(10000000)`、`ModelContextWindowsSchema = z.record(...)`；`SavedProvider` 上有 `autoCompactWindow?` 与 `modelContextWindows?`（第 71-72 行）。
- 注入：`src/server/services/providerRuntimeEnv.ts:320-344` —— 合并 `preset.modelContextWindows` 与用户覆盖后：
  ```ts
  ...(provider.autoCompactWindow !== undefined && {
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: String(provider.autoCompactWindow) }),
  ...(Object.keys(modelContextWindows).length > 0 && {
    [MODEL_CONTEXT_WINDOWS_ENV_KEY]: JSON.stringify(modelContextWindows) }),
  ```
  即：**配置存 DB/JSON，运行时以 env 传给 CLI 引擎** —— UI 层与引擎解耦。
- `CLAUDE_CODE_AUTO_COMPACT_WINDOW` 的语义（见 `autoCompact.ts:40-46`）：**强制覆盖 auto-compact 计算所用的 context window**，与 `modelContextWindows`（按模型识别）互补 —— 桌面端把它包装成"**未知模型兜底窗口**"（Settings 里已识别模型的窗口优先，见第 4.4 节）。

---

## 2. Token 用量统计

结论：**以 API 响应的 usage 字段为锚点 + 本地"字符/4"粗估增量，二者取 max，不使用本地 tokenizer**。

### 2.1 单次响应的上下文总量公式

`src/utils/tokens.ts:46` `getTokenCountFromUsage(usage)`：

```ts
input_tokens + cache_creation_input_tokens + cache_read_input_tokens + output_tokens
```

- 加 `output_tokens` 的理由（`context.ts:161-172` 注释）：本次输出会成为下一次请求的输入，不加会导致"回答完成后用量瞬间回落"的视觉跳变。
- `getCurrentUsage()`（tokens.ts:138）从消息列表**倒序**找最近一条带真实 usage 的 assistant 消息；全 0 usage 被视为"stale/unavailable 占位符"跳过（压缩后置 0 的约定，见 3.6）。

### 2.2 canonical 计量函数（阈值判断专用）

`src/utils/tokens.ts:244` `tokenCountWithEstimation(messages)` —— 注释明确标注"THE CANONICAL function"：

1. 倒序找到最近带 usage 的 assistant 消息；
2. **并行工具调用修正**：同一 API 响应会被拆成多条同 `message.id` 的 assistant 记录、与 tool_result 交错，需回退到同 id 的**第一条**兄弟记录，否则会漏算中间交错的 tool_result；
3. 返回 `getTokenCountFromUsage(usage) + roughTokenCountEstimationForMessages(其后的新消息)`。

注释同时列出三个**反模式**：累计计数（会重复计费上下文）、只看 output_tokens、只看最后一次 usage（漏掉新消息）。

### 2.3 本地粗估（无 tokenizer）

`src/services/tokenEstimation.ts`：

- `roughTokenCountEstimation(content, bytesPerToken=4)`（第 203 行）：**字符数 / 4**；JSON 类文件用 `/2`（第 215-224 行，dense JSON 单字符 token 多）；
- 按 block 类型精细化（第 409-453 行）：`image/document` 固定 **2000** tokens（防止 base64 PDF 被当文本估成 30 万）、`tool_use` 按 `name + JSON.stringify(input)` 长度、thinking 按文本长度、其余 block 按 stringify 长度。

### 2.4 精确计数（仅 /context 明细用）

`src/utils/analyzeContext.ts:84` `countTokensWithFallback`：先调官方 `countTokens` API（`tokenEstimation.ts:140`，含 Bedrock/Vertex 特判），失败则 **Haiku fallback**（`countTokensViaHaikuFallback`，第 269 行：发一条 `max_tokens: 1` 的真实请求，从响应 usage 读 `input + cache_creation + cache_read`）。有 `estimateOnly` 快速路径（纯本地估算）—— 桌面端拉取用量时用的就是 estimateOnly，避免每次刷新打 N 个计数 API。

### 2.5 多 Provider 的 usage 可信度

`src/utils/contextBudget.ts` —— 这是对"第三方 provider usage 不可靠"的防御层：

- `getProviderUsageTrust`（第 22 行）：一方 Anthropic = `high`，其余 = `low`；
- `calculateContextBudget`（第 115 行）：`usedTokens = min(max(本地估算, provider usage 总量), contextWindow)`——**取 max 再夹紧到窗口**；
- `shouldIgnoreLowTrustUsage`（第 99 行）：low-trust + 含图片/PDF + usage ≥ window 且比估算大 4 倍或多 5 万 → 判定 provider 把媒体 token 报炸了，忽略 usage 回退纯估算（返回 `ignoredUsageReason: 'low_trust_media_usage'`）；
- `calculateContextPercentagesFromTokens`（第 163 行）：百分比 = round(used/window*100)，clamp 0-100。

---

## 3. 自动压缩（auto-compact）

### 3.1 阈值：绝对 buffer 制，不是百分比

`src/services/compact/autoCompact.ts`：

```ts
const MAX_OUTPUT_TOKENS_FOR_SUMMARY = 20_000        // 行30：给摘要输出预留（基于 p99.99=17,387）
export const AUTOCOMPACT_BUFFER_TOKENS = 13_000      // 行62：自动压缩触发 buffer
export const WARNING_THRESHOLD_BUFFER_TOKENS = 20_000 // 行63：警告线
export const ERROR_THRESHOLD_BUFFER_TOKENS = 20_000   // 行64：错误线
export const MANUAL_COMPACT_BUFFER_TOKENS = 3_000     // 行65：手动压缩硬阻断线
```

- **有效窗口**（行 33）：`effectiveWindow = contextWindow − min(模型maxOutput, 20_000)`；`CLAUDE_CODE_AUTO_COMPACT_WINDOW` env 可覆盖 contextWindow。
- **自动压缩阈值**（行 72）：`threshold = effectiveWindow − 13_000`。以 200k 模型、32k 输出为例：threshold = 200k − 20k − 13k = 167k（约 83.5%）；1M 模型则约 96.7% —— **绝对值 buffer 天然适配不同窗口大小，比固定百分比合理**。
- 测试用 env：`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`（百分比覆盖，行 79）、`CLAUDE_CODE_BLOCKING_LIMIT_OVERRIDE`。
- `calculateTokenWarningState(tokenUsage, model)`（行 93）：一次性算出 `percentLeft`（相对阈值的剩余百分比）、`isAboveWarningThreshold`、`isAboveErrorThreshold`、`isAboveAutoCompactThreshold`、`isAtBlockingLimit`（= effectiveWindow − 3k，到这条线连手动 /compact 都发不出去）。

### 3.2 触发判断与调用位置

- `shouldAutoCompact(messages, model, querySource, snipTokensFreed)`（autoCompact.ts:160）：
  - **递归护栏**：`querySource === 'session_memory' || 'compact'` 直接 false（压缩 fork agent 自己不许再压缩，否则死锁）；
  - 开关：env `DISABLE_COMPACT`（全禁）、`DISABLE_AUTO_COMPACT`（只禁自动，保留手动）、用户配置 `autoCompactEnabled`（autoCompact.ts:147-158）；
  - 计量：`tokenCountWithEstimation(messages) − snipTokensFreed ≥ threshold`。
- **调用点**：query 主循环每轮发 API 前，`src/query.ts:456-470`，顺序为 `snip → microcompact → (context collapse) → autocompact`——先做廉价裁剪，压不下去才做昂贵的 LLM 摘要。

### 3.3 失败处理：熔断器

`autoCompactIfNeeded`（autoCompact.ts:241）：

- `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3`（行 70），注释给了血泪数据：曾有 1,279 个会话连续失败 50+ 次（最高 3,272 次），每天浪费约 25 万次 API 调用。连续失败 ≥3 次后本会话不再尝试自动压缩；成功后归零。
- 失败静默重试（下一轮再试），**只有手动 /compact 失败才弹错误通知**（compact.ts:781-788 注释：自动压缩失败弹窗只会造成困惑）。
- 跟踪状态 `AutoCompactTrackingState { compacted, turnCounter, turnId, consecutiveFailures }`（行 51），压缩后重置并把"距上次压缩的轮数/上次压缩 turnId"写进遥测（判断压缩打转）。

### 3.4 压缩流程（compactConversation）

`src/services/compact/compact.ts:419`，主线：

1. `preCompactTokenCount = tokenCountWithEstimation(messages)`；
2. 执行 PreCompact hooks（可注入额外摘要指令）；`setSDKStatus('compacting')` → 前端进入"压缩中"状态；`onCompactProgress({type:'compact_start'})`；
3. **摘要请求**：优先走 **forked agent 复用主对话 prompt cache**（`runForkedAgent`，行 1220：发送与主线程完全相同的 system/tools/messages 前缀 + 摘要指令，cache 命中率极高、便宜），失败则回退普通流式请求（system prompt 换成一句"You are a helpful AI assistant tasked with summarizing conversations."，`thinking: disabled`，只带 FileReadTool，maxOutput = min(20k, 模型上限)），流式失败重试 2 次（行 131）；
   - 压缩期间禁止一切工具调用：`createCompactCanUseTool`（行 1157）一律 deny；
   - 压缩前对消息做预处理：`stripImagesFromMessages`（行 145，图片/PDF 替换为 `[image]`/`[document]` 文本标记，防止压缩请求自己超长）；
4. **prompt-too-long 自救**（CC-1180，行 482-523）：若压缩请求本身 413，`truncateHeadForPTLRetry`（行 243）按 API round 分组丢弃最老的组（按报错里的 token 差额累计，解析不出就丢 20%），插入 `[earlier conversation truncated for compaction retry]` 合成 user 标记，最多重试 `MAX_PTL_RETRIES = 3` 次；
5. 校验摘要文本（无文本/以 API Error 前缀开头都算失败并打遥测事件 `tengu_compact_failed`）。

### 3.5 Summarization Prompt（关键摘录）

`src/services/compact/prompt.ts`。结构 = 反工具前导 + 9 段式摘要模板 + 自定义指令 + 反工具尾注：

```
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.
- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.
```

（行 19-26。注释：Sonnet 4.6 上没有这个前导会有 2.79% 概率尝试调工具浪费唯一一轮。）

主模板（行 61+）：
```
Your task is to create a detailed summary of the conversation so far, paying close
attention to the user's explicit requests and your previous actions. ...
Your summary should include the following sections:
1. Primary Request and Intent  2. Key Technical Concepts
3. Files and Code Sections (含完整代码片段)  4. Errors and fixes (含用户反馈)
5. Problem Solving  6. All user messages (列出所有非工具结果的用户消息)
7. Pending Tasks  8. Current Work (精确描述压缩前正在做什么)
9. Optional Next Step (必须直接对应最近的显式请求，带原文引用防漂移)
```

要求先在 `<analysis>` 里做草稿分析再输出 `<summary>` —— `formatCompactSummary`（行 311）会**把 `<analysis>` 整段剥掉**（只是提升质量的 scratchpad），`<summary>` 转为 `Summary:` 头。

摘要包装成的 user 消息（`getCompactUserSummaryMessage`，行 337）：

```
This session is being continued from a previous conversation that ran out of context.
The summary below covers the earlier portion of the conversation.
{formattedSummary}
If you need specific details from before compaction ... read the full transcript at: {transcriptPath}
[auto 模式追加] Continue the conversation from where it left off without asking the user
any further questions. Resume directly — do not acknowledge the summary ...
```

### 3.6 压缩后保留什么

`CompactionResult`（compact.ts:299）→ `buildPostCompactMessages`（行 362）拼装顺序：`boundaryMarker → summaryMessages → messagesToKeep → attachments → hookResults`。附件重建逻辑：

| 保留项 | 逻辑 | 预算 |
|---|---|---|
| 最近读过的文件 | `createPostCompactFileAttachments`（行 1447）：按 readFileState 时间戳取最近 5 个重新 Read 注入；已在保留尾部出现过的 Read 结果去重；排除 plan 文件与 CLAUDE.md 系 | 最多 5 个文件、总预算 50k、单文件 5k（行 122-124）|
| 已调用的 skills | `createSkillAttachmentIfNeeded`（行 1526）：按最近调用排序，**每个 skill 头部截断**（指令通常在文件开头）| 单 skill 5k、总 25k（行 129-130）|
| 计划文件 / plan mode | plan 内容附件 + plan-mode 指令附件（行 1502/1574，否则压缩后模型会"忘记"自己在 plan mode）| — |
| 后台 agent 状态 | `createAsyncAgentAttachmentsIfNeeded`（行 1600）：运行中/已完成未取结果的任务，防止重复 spawn | — |
| 工具/MCP/agent 列表增量 | 重新播报 deferred tools、agent listing、MCP instructions（行 599-617）| — |
| SessionStart hooks | `processSessionStartHooks('compact')` | — |

其他清理：`readFileState.clear()`、清 nested memory 路径；**不**重发 skill_listing（省 ~4k 纯 cache_creation）。

**边界记录**：`createCompactBoundaryMessage`（`src/utils/messages.ts:4647`）——

```ts
{ type: 'system', subtype: 'compact_boundary', content: 'Conversation compacted',
  compactMetadata: { trigger: 'auto'|'manual', preTokens, userContext?, messagesSummarized? },
  logicalParentUuid: <压缩前最后一条消息 uuid> }
```

持久化时保留段落用 `preservedSegment { headUuid, anchorUuid, tailUuid }` 标注（compact.ts:381），loader 靠它重建消息链。发送 API 时用 `getMessagesAfterCompactBoundary` 只取最后一个边界之后的消息（UI 滚动历史完整保留，API 视图被截断）。

**#743 关键坑**：保留下来的旧 assistant 消息里带着**压缩前的 usage**，会让 `getCurrentUsage → max(估算, usage)` 把用量表钉在压缩前水平。解法 `stripStaleUsageFromPreservedMessages`（compact.ts:336）：保留消息的 usage 全部置 0（getCurrentUsage 会跳过全 0），原始消息在 transcript 里不动。

### 3.7 手动 /compact 与多层压缩体系

- `/compact [自定义指令]`：`src/commands/compact/compact.ts:40` —— 先试 session-memory 压缩（无自定义指令时）→ 先跑 microcompact 再 `compactConversation(...)`；成功后 `suppressCompactWarning()`（压掉警告条），显示 `Compacted (ctrl+o to see full summary)`。
- **microcompact**（`src/services/compact/microCompact.ts:253`）：轻量级——对旧 tool_result 做"内容清空"（`[Old tool result content cleared]`），只处理 Read/Bash/Grep/Glob/WebSearch/WebFetch/Edit/Write 结果（行 41-50）；time-based 触发（距上次响应太久 = server cache 已过期，趁冷缓存清理）；cache-editing 路径为内部 feature。
- **snip compact**（`snipCompact.ts`，feature 门控）：裁剪历史但保留 UI 滚动，释放的 token 数 `snipTokensFreed` 传给 autocompact 修正计量（query.ts:399-413）。
- **session memory 压缩**（`sessionMemoryCompact.ts`）：实验路径，autocompact 会先尝试它（autoCompact.ts:288）。
- **reactive compact**（413 兜底）与 **context collapse**（渐进折叠）为内部 feature，外部构建是 stub（`reactiveCompact.ts` 开头即 `@generated stub`）——设计意图值得了解：reactive 模式干脆不做主动压缩，等 API 返回 prompt-too-long 再从头部剥离消息组重试。
- **partial compact**（compact.ts:804）：从消息选择器选一个 pivot，`from`（摘要 pivot 之后、保留之前，保 prompt cache）或 `up_to`（摘要 pivot 之前、保留之后）两个方向，各有专用 prompt 变体。

---

## 4. 前端 / UI

### 4.1 CLI 侧警告条

`src/components/TokenWarning.tsx`（编译产物含内嵌源码）：

- 低于警告线不渲染；进入警告区后：
  - autocompact 开启 → 暗色小字 `` `${percentLeft}% until auto-compact` ``；
  - autocompact 关闭 → `Context low (x% remaining) · Run /compact to compact & continue`，超过 error 线由 `warning` 色变 `error` 色。
- `percentLeft` 是**相对阈值**（不是相对总窗口）的剩余百分比——倒数到"事件发生点"，语义诚实。

### 4.2 CLI `/context` 明细（分布计算）

`src/utils/analyzeContext.ts:951` `analyzeContextUsage()` 产出 `ContextData`（行 202）：

- **分类**（行 1051-1199，含展示色）：`System prompt` / `System tools`（内建工具 schema）/ `MCP tools`（+ deferred 单列不计入用量）/ `Custom agents` / `Memory files`（CLAUDE.md 系）/ `Skills`（frontmatter）/ `Messages` / `Autocompact buffer`（= window − threshold，预留区）或 `Compact buffer`（3k，关自动压缩时）/ `Free space`。
- 计数方式：system prompt 按 section 分别 count（取首个 markdown 标题作 section 名，行 273）；工具 schema 整批 count 后按本地估算比例摊回单个工具，并扣除 `TOOL_TOKEN_COUNT_OVERHEAD = 500`（API 对含工具请求加一次约 500 token 的前导，逐个 count 会被重复计入 N 次，行 76-82）；消息按 block 分桶（toolCall/toolResult/attachment/assistant/user + 按工具名 top 列表）。
- 总量锚定：`finalTotalTokens = calculateCurrentContextTokenTotal(本地合计, apiUsage, contextWindow)`（行 1213）——仍旧是 max+clamp 套路。
- 可视化：10×10 方格网（1M 模型 20×10），每格代表 window/100，分类色块顺序填充，Autocompact buffer 固定排在网格尾部（行 1228-1338）。
- `estimateOnly` 参数贯穿所有 count 函数——**桌面端轮询走纯估算，用户显式打开 /context 才走精确 API 计数**。

### 4.3 桌面端：发送框右下角 Context 指示器

`desktop/src/components/chat/ContextUsageIndicator.tsx`（agent-builder 需求的直接范本）：

- **外观**：胶囊按钮 = 18px 圆环（`conic-gradient` 画进度）+ 等宽字体百分比。颜色阶梯（行 234-238）：`≥90% → --color-error`，`≥75% → --color-warning`，否则次要色。加载中转圈。
- **hover 面板**（桌面）/ 底部抽屉（移动，`compact` prop）：模型名、大百分比、`已用 / 剩余 / 窗口` 三个数字（`Intl.NumberFormat` 千分位）、**top-4 分类横条**（`pickUsedContextCategory`，行 48：过滤 `free space`、`autocompact buffer` 与 deferred，按 tokens 降序取 4，条宽 = tokens/window），更新时间（"刚刚/x 分钟前"）、数据来源为估算时显示 `ESTIMATE` 徽标。
- **数据来源**：`sessionsApi.getInspection(sessionId, { includeContext: true, contextOnly: true, timeout: 20s })` → server `src/server/api/sessions.ts:607-616` → 向 CLI 会话发 control request `{ subtype: 'get_context_usage', estimateOnly: true }`（CLI 端处理在 `src/cli/print.ts:2926`，内部调 analyzeContextUsage）。
- **降级路径**：CLI 未运行时，server 从 transcript JSONL 直接估算 —— `src/server/services/sessionService.ts:1574` `getTranscriptContextEstimate()`：扫最后一条非零 usage + `roughTokenCountEstimationForMessages` + `calculateContextBudget`，分类退化为 `Input/Cache read/Cache write/Output tokens` 四桶。前端用 `contextSource: 'live' | 'estimate'` 区分。
- **刷新策略**（行 23-29 常量 + 各 useEffect）：
  - 会话活跃时 30s 轮询；auto 模式最小间隔 10s 节流；页面不可见不刷；messageCount 变化触发刷新；
  - **`refreshNonce` 机制（#743）**：`ChatInput.tsx:1295` 传 `refreshNonce={sessionState?.compactCount ?? 0}`，chatStore 每收到一个 compact 边界就 `compactCount+1`，指示器收到 nonce 变化立即发 **force 刷新（绕过节流且不复用 in-flight 请求**——压缩前发出的请求会带回压缩前数据），失败 5s 后重试一次。**这是"压缩完成后表盘立刻回落"问题的完整解法，强烈建议照抄。**
  - 请求带 `requestSeq` + `contextIdentity（sessionId:model）` 双重防错序/防串会话。

### 4.4 桌面端：压缩中 / 压缩完成 在对话流中的展示

- **状态源**：CLI 压缩时 `setSDKStatus('compacting')`，SDK stream 事件到 chatStore；压缩产物是 system 事件 `compact_boundary`（带 `compactMetadata`）与 `compact_summary`。
- chatStore（`desktop/src/stores/chatStore.ts`）：
  - 收到 `state === 'compacting'` → 在消息尾部插入/更新 `{ type: 'compact_summary', title: 'Context compacted', phase: 'compacting' }` UI 消息（行 1633-1638）；
  - 收到 `compact_boundary` → `compactCount+1`、chatState 从 compacting 恢复 thinking、把尾部 compacting 占位翻转成完成态（行 2232-2243）；
  - 收到 `compact_summary` → 填入 title / trigger / preTokens / messagesSummarized / summary 正文（行 2251+）。
- **渲染**：`desktop/src/components/chat/MessageList.tsx:175` `CompactStatusDivider` —— 一条横贯全宽的**分隔线 + 居中胶囊**：
  - 压缩中：`LoaderCircle` 旋转图标 + "正在压缩…"文案；
  - 完成：`FileStack` 图标 + "Context compacted"（区分 auto/manual 标题），**可点击展开**：meta 行（触发方式 / 压缩前 token 数 / 摘要覆盖消息数）+ 最高 220px 可滚动的摘要全文（行 218-230）。
  - 若 chatState 为 compacting 但尾部还没有占位消息，MessageList 直接兜底渲染一个 compacting divider（行 2044-2045）。

### 4.5 桌面端：设置 UI

`desktop/src/pages/Settings.tsx`（provider 编辑表单，约 1750-1800 行）：

- **每模型 context window**：main/haiku/sonnet/opus 四个槽各一个输入框（`handleModelContextWindowChange`），说明文案 `modelContextWindowsDesc`；
- **兜底窗口**：`autoCompactWindow` 输入框，中文文案（`desktop/src/i18n/locales/zh.ts:438-442`）：
  - label "未知模型兜底窗口"、placeholder "可选，例如 200000"、desc "仅用于无法识别的模型；已配置的模型窗口会优先使用。"
  - 校验：整数 + 16000~10000000 区间，报错内联展示，非法时禁用保存按钮（Settings.tsx:1241）；
- 表单值双向同步到底部的 **settings JSON 编辑器**（env 键 `CLAUDE_CODE_AUTO_COMPACT_WINDOW` / `CLAUDE_CODE_MODEL_CONTEXT_WINDOWS`，白名单见 `desktop/src/lib/providerSettingsJson.ts:26-27`）；
- 顶部有摘要行：`模型: 窗口大小 · 兜底: xxx`，未配置显示"自动"（`contextSummaryAuto`），高级字段默认折叠、有错误时强制展开（`shouldShowContextFields`，行 1315）。
- **auto-compact 开关**：引擎读全局配置 `autoCompactEnabled`（autoCompact.ts:157），CLI `/config` 面板可切；无阈值百分比设置项（阈值是代码内的绝对 buffer，用户只能整体开关或改窗口）。

---

## 5. 精华与糟粕（Go + React 桌面应用视角）

### 值得借鉴的精华

1. **"API usage 锚点 + 本地粗估增量，取 max 后 clamp"的计量模型**：零 tokenizer 依赖、跨 provider 通用，误差被 max 单向兜住（宁可高估提早压缩）。Go 侧只需实现 chars/4（JSON /2、媒体固定 2000）就够。
2. **阈值用绝对 token buffer 而非百分比**：`window − maxOutput预留(≤20k) − 13k` 触发、`−20k` 警告、`−3k` 硬阻断，自动适配 200k/1M 窗口。
3. **失败工程化**：3 次连续失败熔断（有真实事故数据支撑）、压缩请求自身 413 时"丢最老 API round 重试 ×3"、流式中断重试 ×2、自动压缩失败静默/手动失败才提示。
4. **压缩后重建包**：边界系统消息（带 trigger/preTokens/摘要覆盖数元数据）+ 摘要 user 消息 + 定额重注入（5 文件×5k/总 50k、skills 头部截断 5k/总 25k、plan、后台任务状态），并且**保留消息的旧 usage 必须清零**（#743）。
5. **前端 refreshNonce 模式**：后端每次压缩递增 compactCount，指示器据此强制刷新（绕节流、弃用 in-flight 旧请求、失败定时重试）——解决"压缩后表盘不回落"。
6. **estimateOnly 双通道**：常驻指示器轮询走纯本地估算（快、免费），用户主动查看明细才可走精确计数。
7. **配置分层**：内置模型表（含正则）→ provider preset 默认值 → 用户 per-model 覆盖 → 未知模型兜底窗口，UI 校验 16k~10M；agent-builder 可以把"env 注入"换成 Go 侧配置结构体，思想不变。
8. **压缩 prompt 三件套**：强硬的 NO-TOOLS 前导/尾注、`<analysis>` 草稿区（输出前剥离）、9 段式模板（尤其"All user messages"和"Next Step 必须带原文引用"两条是防漂移关键），可整段移植。
9. **压缩期间的 keep-alive**（compact.ts:1199）：长压缩调用每 30s 发心跳 + 重发 compacting 状态，防 WS 空闲断连——Wails 事件桥同样适用。

### 应避免的糟粕

1. **feature flag / 内部构建分支渗透一切**：`feature('CONTEXT_COLLAPSE')` + `require()` 破循环 + stub proxy 文件遍布 compact 链路，可读性极差。agent-builder 不需要这种双构建体系。
2. **多套压缩机制叠床架屋**：microcompact / cached MC / snip / session-memory / reactive / collapse / 全量 compact 七层并存且互相 guard（shouldAutoCompact 里一半代码在处理互斥）。桌面应用做"全量摘要压缩 + 可选的旧 tool_result 清空"两层足够。
3. **token 分布明细依赖真实 API 计数**（含 Haiku fallback 发计费请求）：贵且慢（4.5k 消息会话 analyzeContext walk 要 ~11ms 还只是本地部分）；直接用 estimateOnly 路径即可。
4. **环境变量当配置总线**：CLI 架构使然（server → 子进程只能传 env），Go 单进程应用请用结构化配置 + 显式参数，不要复刻 `CLAUDE_CODE_*` env 矩阵。
5. **usage 全 0 作为"作废"哨兵值**：隐式约定（getCurrentUsage 跳过全 0），散落多处、靠注释维系；Go 里应显式建模（如 `UsageStale bool` 字段）。
6. **编译产物混入源码树**（TokenWarning.tsx 是 react-compiler 输出 + base64 sourcemap），读代码时注意甄别。

---

## 6. 对 agent-builder 的借鉴要点（按四个需求映射）

### 6.1 长对话自动触发压缩（Go 后端）

- 在 agent loop 每轮**发请求前**做检查：`used = 最近一次响应 usage(input+cache_r+cache_w+output) + 其后新消息的 chars/4 估算`；`threshold = window − min(maxOutput, 20k) − 13k`；`used ≥ threshold` 即压缩。
- 压缩实现：同模型、同上下文再发一次"摘要请求"（照抄 3.5 的 prompt，禁工具、限 20k 输出），失败重试 2 次、413 丢最老轮次重试 3 次、连续 3 次失败熔断本会话。
- 压缩产物：`CompactBoundary{trigger, preTokens, summarizedCount}` 事件消息 + 摘要 user 消息 + 重注入（最近读过的文件≤5 个/50k、活跃 todo/plan、已加载 skills 头部截断）；保留消息的 usage 清零。runtime 事件流里 emit `compacting_start / compact_boundary(带元数据) / compacting_end`，正好对上 agent-builder 现有的 runtime_events.go 模式。

### 6.2 发送框右下角 context 图标（React）

- 直接参考 `ContextUsageIndicator.tsx`：conic-gradient 圆环 + 百分比，75%/90% 变警告/错误色；hover 卡片展示 模型 / used / free / window / top-4 分类条 / 更新时间 / ESTIMATE 徽标。
- 分布数据由 Go 侧提供 `ContextSnapshot{model, window, totalTokens, percentage, categories[]{name, tokens, color, isDeferred}}`：system prompt / tools（schema 序列化后估算）/ skills / memory(CLAUDE.md) / messages / autocompact buffer / free space。Wails 直接 bind 一个 `GetContextUsage(sessionId, estimateOnly)` 方法，比 cc-haha 的 HTTP+control-request 链路简单得多。
- 刷新：活跃 30s 轮询 + 消息数变化触发 + **压缩事件驱动的强制刷新（compactCount/refreshNonce）**。

### 6.3 对话流压缩提示

- 参考 `CompactStatusDivider`：分隔线 + 居中胶囊，压缩中转圈"正在压缩上下文…"，完成后图标 + "上下文已压缩"，点击展开 meta（自动/手动、压缩前 token、覆盖消息数）+ 摘要全文（限高滚动）。
- 状态机：`compacting` 占位消息 → 收到 boundary 事件翻转为完成态并回填摘要（chatStore.ts:1633/2232 的两段式处理）。

### 6.4 设置：按 provider/model 配窗口与阈值

- 数据模型照抄 `SavedProvider`：`modelContextWindows map[string]int`（per-model，16k~10M 校验）+ `autoCompactWindow *int`（未知模型兜底）；preset 内置常见 provider 默认表（第 1.2 节的数值可直接搬）。
- 解析优先级：用户 per-model 配置 → 内置表（精确 + 正则）→（对一方 Anthropic 可选）models.list 自动获取并缓存 → 兜底 200k。
- UI：主/快模型各一个窗口输入 + 一个兜底输入，默认折叠"高级"，摘要行显示当前生效值，非法禁存。**auto-compact 阈值本身不必开放为设置**（保持 13k buffer 常量），只开放"自动压缩开关"即可，与 cc-haha 一致。
