# 上下文自动压缩与 Context 展示 实施方案

本方案覆盖四个产品需求,并按用户约束执行:**只做方案,不改代码;发现不好的架构直接重构,不做迁移兼容**。

1. 长对话自动触发上下文压缩(多层级联,不是单一 auto-compact);
2. 发送框右下角(模型选择器左侧)context 用量图标:总量/已用/分布(system prompt、skills、tools、messages 等占比);
3. 对话流中展示"正在压缩/压缩完成";
4. 设置中按 provider/model 配置 context window 与 auto-compact 阈值(可自动获取,有默认值)。

依据文档:现状 [17-context-current-state.md](17-context-current-state.md);参考 [14-cc-haha](14-context-ref-cc-haha.md)、[15-DeepSeek-GUI](15-context-ref-deepseek-gui.md)、[16-Claude Code 源码](16-context-ref-myclaw-claude-code.md)(§8 多层级联、§9 持久化交互)。架构须与 [12-两阶段+流式方案](12-conversation-two-phase-and-streaming-refactor-plan.md) 的 runtime projection / output stream 对齐。

## 0. 现状要点(决定方案形态的事实)

- `internal/contextmgr` 已有完整五层管道(tool result budget → microcompact → snip → auto → full)+ reactive + 全套 SQLite 表,**且已接入主聊天链路**(agent.go:351→364 投影消息真实替换发给模型的消息)。但调用点传零值配置:除 tool result budget 默认生效外全部休眠,`CompactSummarizer` 未注入,manual compact 是只记 boundary 的假动作。
- 已有"追加式持久化 + 读时截断"雏形:`Summarize` 写 `session.SummaryMessageID`,`getSessionMessages`(agent.go:1286-1306)从摘要处切片并翻转 role——与 Claude Code 的 append-only transcript + boundary 走链模式同构。
- token 计量全是 chars/4 估算(`runtime_budget.go`),真实 usage 只覆盖式存 session 表,cache 字段在 `RuntimeUsage` DTO 丢失。
- context window 三处硬编码矛盾(64000×2 + 200000),configured provider 的模型没有任何元数据。
- 事件契约漂移:`compact.*` 五个常量无 emitter;前端 compact 展示走 `attachContextGovernanceToTimeline` 旁路;设置页 context governance 是静态假数据。

## 1. 核心设计决策

### D1 持久化:DB 永远 append-only,压缩只作用于"发给模型的投影"

采纳 Claude Code 的实际方案(16 号文档 §9),也与本项目 audit/replay 架构一致:

- `messages` 表原始消息**永不改写、永不删除**;
- 压缩产物是**追加**:一条 `IsCompactSummary` 标记的摘要消息 + 一条 `runtime_context_boundaries` 记录(kind=full,含 summaryMessageID、preTokens、messageRefs、budget before/after);
- microcompact / tool result budget 产生的替换只落 `runtime_context_content_replacements` 表,**每轮投影时从原文重算**,持久化零影响、随时可整体关闭恢复;
- 会话恢复(resume/重启)= 读最新 completed full boundary → 从摘要消息处截取 + 应用 replacement 决策,不需要"重放压缩过程";
- **保留段消息的 usage 锚点必须失效**:压缩后,boundary 之前的 assistant usage 不得再作为计量锚点,否则表盘不回落、甚至触发压缩死循环(CC 用"usage 清零"实现;我们因 usage 拟独立成列,改为"锚点查询限定在最新 boundary 之后",语义显式,见 D2)。

### D2 计量:信 API usage,只估增量

废弃"纯 chars/4 全量估算"的 `computeRuntimeBudget` 作为水位判断依据(保留其分布估算用途),改为三方共识的锚点模型:

```
used = anchorUsage(最新 boundary 之后、最近一条带真实 usage 的 assistant 消息)
         .input + .cacheCreation + .cacheRead + .output
     + estimate(锚点之后的新消息)          // estimate = (utf8 rune 数+3)/4,图片/文档固定 2000
used = min(used, contextWindow)            // clamp
```

- 与本地全量估算取 **max** 作最终判定值(DeepSeek-GUI 的自纠错思路,宁可高估提早压缩);
- 需要 per-message usage 持久化:`messages` 表新增 `usage_json`(input/output/cacheRead/cacheCreation/reasoning),`OnStepFinish` 时写入。这同时修复"重启后水位丢失"和"turn usage 只能从 session 快照差值反推"两个问题;
- `RuntimeUsage` DTO 重构为含 cache 字段(直接改,不留旧形状):`{input, output, cacheRead, cacheCreation, total, cost}`;session 级累计成本视角保留在 `usage.updated` 事件,**上下文水位视角**走新的 `context.usage.updated`(见 §4)。

### D3 阈值:绝对 buffer 推导,不暴露复杂参数

照抄 Claude Code 公式,加小窗口保护(它假设窗口 ≥200k,我们有 64k 级模型):

```
outputReserve   = min(modelMaxOutputTokens, 20_000)
effectiveWindow = contextWindow − outputReserve
autoCompactAt   = effectiveWindow − min(13_000, contextWindow×10%)
warningAt       = autoCompactAt  − min(20_000, contextWindow×10%)
blockingAt      = effectiveWindow − min(3_000,  contextWindow×2%)
percentLeft     = max(0, round((autoCompactAt − used) / autoCompactAt × 100))
```

- 200k 模型:约 167k 触发(83.5%)、147k 警告、177k 阻断——与 CC 一致;
- 用户可配的只有三项:`autoCompactEnabled`(默认 true)、可选的 per-provider/per-model `contextWindow` 覆盖、可选的 `autoCompactPercent` 覆盖(设置了则 `autoCompactAt = window×pct`,与推导值取小;主要用于调试和用户强偏好);warning/blocking buffer 不开放。

### D4 多层级联:六层防线,各司其职

对照 Claude Code 的层级(16 号 §8),结合我们已有实现取舍:

| 层 | 我们的实现 | 触发 | 处理 | 取舍说明 |
|---|---|---|---|---|
| L0 写入端 guard | `internal/agent/tool_result_guard.go`(已有)+ contextmgr `applyToolResultBudget` | 工具结果产生时 / 每次投影 | 超限结果全文落盘(output ref 已有),投影中替换为 `<persisted-output>` 摘要 + 预览;单条 16k chars、单轮聚合 200k chars | 已生效,补齐"替换文本附 ref 路径提示模型可用工具取回" |
| L1 microcompact | contextmgr `applyMicrocompact`(激活) | 投影时:可裁剪工具结果 token 估算 > 阈值(默认 `effectiveWindow×25%`)或数量 > 上限 | 旧的 file_read/shell/search 类结果替换为 `[旧工具结果已清理,可重新读取]`,保留最近 K=5 个;记 replacement + micro boundary;**UI 不打扰**(仅诊断可见) | 保留;CC 的 time-based(60min 缓存过期)路径不做——桌面常驻场景收益低 |
| L2 snip | contextmgr `applySnip`(仅保留两个用途) | ①手动"修剪中段";②full compact 请求自身超长时的应急降载 | 挖除历史中段,`<snip-boundary>` 摘要占位 | 不做 CC 的模型自主 snip 工具(ant-only 实验,复杂度不值) |
| L3 auto → full compact | contextmgr `applyAutoCompact` + `applyFullCompact` + 新 summarizer | 每次投影(即每个 PrepareStep)计算 `used ≥ autoCompactAt` | 见 §3 压缩执行 | 核心路径 |
| L4 blocking | contextmgr warning 已有 code,补拒发逻辑 | `used ≥ blockingAt` 且压缩未成功 | 不发请求,turn 失败并给出明确错误("上下文超限,请手动压缩或新开会话") | 防 413 浪费 |
| L5 reactive | contextmgr `ReactiveCompact`(接线) | provider 返回 context-length 错误(`IsContextLengthError` 已有) | attempt1:强化投影(microcompact 全量 + snip 中段);attempt2+:强制 full compact;≤3 次 | 已有实现,只差调用方 |

**不引入** CC 的 context collapse(后台渐进摘要 agent)和 session memory compaction——均为其内部实验,复杂度/收益比差,且与 autocompact 有互斥竞态(CC 源码注释自证)。**熔断器**:连续 3 次 full compact 失败后本会话停止自动压缩(contextmgr `MaxConsecutiveFailures` 已有字段),状态挂 per-session,不用模块级全局量(规避 CC 糟粕)。

### D5 单一事实源与死代码清理

- **删除** `runtime_compact_boundaries` 表、`runtime_compact_store.go`、`RuntimeCompactBoundary` 系 DTO(无 writer 的死 stub);boundary 唯一事实源 = contextmgr 的 `runtime_context_boundaries`,runtime 读 API(`TurnCompactBoundaries` 等)改读它;
- **删除** context window 三处硬编码,唯一入口 = 新的 `resolveModelContextWindow`(§2);
- **删除** 前端 `attachContextGovernanceToTimeline` 旁路(wailsWorkbenchAdapter.ts:3150-3211),compact 展示一律走 runtime projection 产出的 conversation item;
- **删除** `staticWorkbenchAdapter` 的假 `ContextGovernanceSettingsViewModel` 数据,接真后端;
- **删除** `runtime_context_read_state_snapshots` / `runtime_context_reinjections` 两张未用表,压缩回填改记在 boundary 的 `ReinjectedRefs`(已有字段);
- 修事件契约漂移:`prompt.assembly.recorded`、manual compact/snip 事件正式注册或删除(见 §4)。

### D6 压缩配置的解析链

```
per-model 用户覆盖(设置 UI)
  > per-provider 兜底(未知模型默认窗口)
  > 模型发现元数据(models API 带 context_length / max_input_tokens 时,缓存)
  > 内置表(catwalk embedded 真实值 + 补充常见模型表,最长 id 优先子串匹配)
  > 全局默认 128_000
```

## 2. 后端:模型元数据与 context window(PR1)

### 2.1 新包 `internal/modelmeta`

```go
type ModelLimits struct {
    ContextWindow   int    // tokens
    MaxOutputTokens int
    Source          string // user_override | provider_default | discovered | builtin | fallback
}
func Resolve(req ResolveRequest) ModelLimits
```

- 输入:providerID、modelID、configured overrides、discovered metadata、catwalk catalog;
- 内置补充表(catwalk 覆盖不到的常见第三方模型:deepseek-chat 128k、qwen 系 128k/256k/1M、glm 128k、kimi 256k 等),匹配规则 = 精确 id → 最长前缀/子串(id 降序排序,借 CC modelCapabilities 的 sortForMatching 思路);
- 校验区间 16k~10M(借 cc-haha)。

### 2.2 存储与发现

- `configured_providers` 表:`model_ids_json` **重构为** `models_json`(`[{id, contextWindow?, maxOutputTokens?, displayName?}]`,直接改列,不做双写兼容);新增列 `default_context_window`(provider 级兜底,可空)。
- 新表 `model_metadata_cache`:`provider_id, model_id, context_window, max_output_tokens, source, fetched_at`。`discoverModelIDs`(runtime_model_config.go:147-202)升级为 `discoverModels`:除 id/display_name 外解析 OpenRouter `context_length`、Ollama `model_info`、Anthropic `models.list` 等可得字段,写缓存;拿不到就只存 id。
- 删除 `applyModelConfig` / `applyConfiguredProviderModel` 的 64000/4096 硬编码与 `defaultRuntimeContextWindow`,统一调 `modelmeta.Resolve`,结果填进 `catwalk.Model.ContextWindow/DefaultMaxTokens`,下游(agent、budget、阈值)自然拿到真值。

### 2.3 上下文治理配置

全局配置(config store 的 global JSON)新增段:

```jsonc
"contextGovernance": {
  "autoCompactEnabled": true,
  "autoCompactPercent": null,        // 可选覆盖,null=公式推导
  "microcompactEnabled": true,
  "microcompactKeepRecent": 5,
  "summaryModel": "session",         // session=跟随会话大模型 | small=用小模型
  "providerOverrides": { "<providerID>": { "autoCompactPercent": 0.8, "models": { "<modelID>": {...} } } }
}
```

runtime 提供 `ContextGovernanceSettings` / `SaveContextGovernanceSettings` API(替换前端假数据),并在构造 `BuildInputRequest` 时把解析结果注入各层 config。

## 3. 后端:压缩执行(PR3/PR4)

### 3.1 Summarizer 实现(新 `internal/agent/compact_summarizer.go`)

实现 `contextmgr.CompactSummarizer`,复用 `Summarize`(agent.go:993)的基建但独立成专用路径:

- prompt 采用 Claude Code 9 节结构(16 号 §3.4 全套):NO-TOOLS 前导+尾注、`<analysis>` 草稿区(输出后正则剥离)、9 节固定结构(重点:**All user messages** 防意图漂移、**Next Step 必须 verbatim 引用**)、支持 additional instructions(手动 compact 参数 / PreCompact hook 注入);
- 调用参数:按 `summaryModel` 配置选模型;禁工具、thinking off、maxOutput = min(20k, 模型上限)、图片/文档替换为 `[image]`/`[document]` 占位;
- **摘要请求自身超长**:按 API round 分组从最旧丢弃(fantasy messages 按 assistant 起始切组),重试 ≤3 次;
- 超时(默认 60s)/失败 → 返回错误由 full compact 层计入熔断;**手动 compact 额外提供启发式降级**(DeepSeek-GUI 思路:本地拼装 用户消息列表 + 文件清单 + 未完成 todo,保证手动压缩永不彻底失败);
- 压缩期间每 30s 通过事件通道发 keep-alive 进度(防前端把长压缩当卡死)。

### 3.2 Full compact 的完整重建(改造 contextmgr `applyFullCompact` + runtime 落地层)

压缩成功后(所有写入在一个事务内):

1. 持久化摘要消息:`messages` 追加 user role、`IsCompactSummary=true`(新增标志,与现有 `IsSummaryMessage` 区分后**合并二者**——旧 Summarize 链路整体并入 compact,删除独立 summary 概念),内容 = 包裹文案("本会话由早前对话压缩延续…" + 摘要 + "细节可查阅完整会话记录")+ autocompact 时附"直接继续、勿复述摘要";
2. upsert boundary:kind=full、trigger=auto|manual|reactive、status=completed、summaryMessageID、preTokens/postTokens、messageRefs(被折叠范围)、reinjectedRefs;
3. 更新 `session.SummaryMessageID` 指向摘要消息(读取路径 `getSessionMessages` 保持现状语义:从摘要起切片);
4. **重注入**(附加在摘要之后、作为摘要消息的附属 part 或紧随的 synthetic user part):
   - 最近读过的文件:查 `read_files` 表(state=recorded,按 mtime/时间最近优先,排除 CLAUDE.md/plan 类),≤5 个、单文件 ≤5k token、总预算 50k,内容重新从磁盘读取(hash 变了标注 stale);
   - 未完成 todos(session.Todos 已有);
   - 已加载 skills 头部(单个截断 5k、总 25k,截断处提示可重新 Read);
   - 运行中的 agent task / 后台 job 状态一览;
5. 压缩边界完整性:切分点校验 tool_use/tool_result 配对与同 message 的 thinking 块不被拆散(CC `adjustIndexToPreserveAPIInvariants`;OpenAI 系 tool_call/tool 配对同理);末尾悬空 tool_call 先剔除(DeepSeek `trimTrailingToolCalls`);
6. 幂等:boundary 附被折叠内容的短哈希 digest,投影层遇到已折叠段直接跳过(DeepSeek digestMarker);
7. 清理:filetracker 已读状态标记过期(压缩后强制重新 read)、`markDeliveredToolResults` 状态重置;
8. 触发 `PreCompact`(压缩前,可注入摘要指令)/ `PostCompact` hooks——hooks 枚举已存在,本方案补完触发链;
9. 计量锚点自然失效:锚点查询限定 boundary 之后(D1),无需清零 usage。

### 3.3 触发编排(激活 `buildModelInputProjection`)

`runtime_prompt_assembly.go:339` 构造 `BuildInputRequest` 时注入完整配置;`applyAutoCompact` 的判定改用 D2 计量(新增 `UsedTokensHint` 字段由 runtime 传入锚点计量值,与内部估算取 max);`HelperCall/RecursionGuard`:summarizer 自己的调用、title/summary helper 路径(TurnID 前缀 `helper_`/`summary_`/`compact_`)一律跳过压缩检查(字段已有,补赋值)。

阻塞语义:full compact 在 PrepareStep 内同步执行(压缩完成前不发主请求),事件流保证 UI 可见进度。

### 3.4 Reactive 接线

agent loop 的 provider 错误路径(`OnStepFinish`/stream error 处理处)捕获错误文本 → `contextmgr.IsContextLengthError` → 调 `ReactiveCompact` 记录 attempt → 按 attempt 策略重建投影后重试本 step(≤3 次);超次后 turn 失败,错误信息引导手动处理。

### 3.5 Manual compact

- `ManualCompact`(runtime_prompt_assembly.go:76)重构为真实执行:走 3.1/3.2 全流程(阈值检查跳过、buffer 用 manual 3k 线),支持 `instructions` 参数;
- 前端入口:composer 斜杠命令 `/compact [说明]`(meta input 已有 slash 归一化链路)+ context 面板按钮;
- 失败提示:手动失败弹 toast;自动失败静默重试下一轮(仅熔断后在 context 面板露出警示)。

## 4. 事件与投影契约(PR2/PR3)

### 4.1 事件(runtimeapi/contract.go 重构)

删除未用的 `EventCompactBoundaryRecorded/Micro/Full/Failed/OutputPreserved` 五常量,替换为:

```go
EventContextUsageUpdated = "context.usage.updated"   // 取代 budget.updated(删除旧常量与 emitter)
EventCompactStarted      = "compact.started"          // kind=full|micro trigger=auto|manual|reactive
EventCompactCompleted    = "compact.completed"        // + boundaryID, preTokens, postTokens, summarizedCount
EventCompactFailed       = "compact.failed"           // + attempt, willRetry, circuitOpen
```

- `context.usage.updated` payload = `RuntimeContextUsage`(见 4.3),发布时机:每次 assistant step 完成(有新 usage 锚点)、turn 开始/结束、compact 完成后;coalesced(350ms 档);
- DeepSeek-GUI 教训:**started 必须真的在压缩前发出**(它契约里有、后端从没发,running 态形同虚设),契约测试断言 started→completed/failed 成对;
- 注册 `prompt.assembly.recorded` 进 EventTypes;manual compact/snip 的裸字符串事件删除(统一走 compact.* 事件)。

### 4.2 Conversation item(对话流展示的数据源)

复用 doc-12 的 projection 模式,`compact_boundary` item 升级为**两段式状态 item**(类比 `exploration_summary`):

- 压缩开始:投影产出 item `compact-<boundaryID>`,`status=compacting`(boundary row status=started 即产出);
- 完成:同 ID 原地更新为 `status=completed`,Display 携带 `{trigger, preTokens, postTokens, summarizedCount, summaryMessageID}`;摘要正文不进 item(前端按需经 summaryMessageID 取消息);
- 失败:`status=failed` + reason(仅手动/熔断时产出 failed item,自动静默重试不打扰);
- microcompact **不产出对话流 item**(CC 的选择:对用户透明),只进 diagnostics;
- 该 item 走既有 output stream 通道(`runtime_output_stream.go`)即时推送;`itemsForRuntimeEvent` 增加 compact.* 事件到 item 的映射;
- 删除前端旁路后,`ConversationTimelineKind` 收敛:保留 `compact_boundary`(升级语义),删除 `snip_boundary/microcompact_marker/reactive_compact_retry/tool_result_replacement` 四个 timeline kind(这些信息进 ContextDiagnosticsPanel,不进主对话流)。

### 4.3 RuntimeContextUsage DTO(新)

```go
type RuntimeContextUsage struct {
    SessionID        string
    Model            string
    ContextWindow    int
    UsedTokens       int     // D2 锚点计量
    PercentUsed      int
    AutoCompactAt    int     // 阈值,0=禁用
    PercentLeft      int     // 相对 autoCompactAt
    Level            string  // ok | warning | error(后端算好,前端纯渲染)
    Estimated        bool    // 无真实 usage 锚点时 true
    OutputReserve    int
    AutoCompactBuffer int
    Breakdown        []RuntimeContextCategory // {key,label,tokens,estimated}
    CompactCount     int     // 本会话累计压缩次数(前端 refreshNonce)
}
```

Breakdown 分类与来源(全部本地估算,不做 count-tokens API):`system_prompt`(prompt assembly sections 汇总)、`tools`(工具 schema 估算,budget 已有)、`skills`、`mcp`、`memory`(context sources + project memory)、`messages`(= used − 其余固定项,负值 clamp 0)、`reserved`(outputReserve+autoCompactBuffer)、`free`。计算入口重构 `computeRuntimeBudget` → `computeContextUsage`,`RuntimeBudgetReport` 保留供 diagnostics,但 UI 主路径只用新 DTO。

### 4.4 契约同步面

每个 PR 同步:`runtimeapi.Endpoints/EventTypes` + `runtime_http.go` 路由 + `desktop/runtime_bridge.go` 方法 + `client/src/runtime` DTO/adapter + `contract_test.go`/`runtime_bridge_test.go`。新增 HTTP:`GET /v1/sessions/{id}/context-usage`、`POST /v1/sessions/{id}/compact`(带 instructions)、`GET/PUT /v1/settings/context-governance`、provider models 元数据读写并入现有 provider settings 端点。

## 5. 前端方案

### 5.1 Composer context 指示器(PR2)

范本:cc-haha `ContextUsageIndicator`(14 号 §4.3)。

- **位置**:`Composer.tsx` footer `.rightControls`,模型 Dropdown 左侧;
- **形态**:胶囊按钮 = 18px `conic-gradient` 圆环 + 等宽百分比数字;色阶用 antd token:`level=ok` → colorTextSecondary、`warning`(≥75%) → colorWarning、`error`(≥90%) → colorError;`estimated` 时环用虚线/降饱和;
- **点击 Popover**(antd Popover,不用 hover——桌面/触屏一致):模型名 + 大号 `已用 / 总量 (xx%)`、`距自动压缩还剩 x%`、分类横条列表(top 5 + 其余合并,条宽=tokens/window,附 token 数)、`reserved` 单独一行解释"为输出与压缩预留"、更新时间、`估算` 徽标、底部"手动压缩"按钮(busy 时禁用);
- **数据**:`ComposerViewModel` 增加 `contextUsage?: ContextUsageViewModel`;更新通道:`context.usage.updated` 事件(coalesced 350ms)+ **compactCount 变化触发强制刷新**(丢弃 in-flight 旧数据,cc-haha refreshNonce 模式);
- **警告条**:`level != ok` 时 composer 上方一行小字(复用现有 `composer-limit-bar` 位):autocompact 开启 → 灰字 `距自动压缩还剩 x%`;关闭 → warning/error 色 `上下文即将用尽(剩 x%),请手动压缩`;**压缩完成到下一次 usage 锚点更新前抑制**(后端 `Estimated`+compact 时序已保证,前端不用另做状态)。

### 5.2 对话流压缩提示(PR3)

范本:cc-haha `CompactStatusDivider`(14 号 §4.4)+ 我们的 projection item(4.2):

- `Timeline.tsx` 对 `compact_boundary` item 渲染为**横贯分隔线 + 居中胶囊**(独立组件 `features/timeline/CompactDivider.tsx`,CSS Modules + antd token):
  - `compacting`:Loading 图标 + "正在压缩上下文…"(它不属于任何 exploration trace,独立于 turn 分块);
  - `completed`:压缩图标 + "上下文已压缩"(区分 自动/手动/恢复触发),点击展开:meta(触发方式、压缩前后 token、覆盖消息数)+ 摘要全文(经 summaryMessageID 从消息取,限高 240px 滚动);
  - `failed`(仅手动/熔断):colorError 边框 + 原因;
- 历史会话回放:boundary 持久化,item 由快照投影自然产出,重开会话可见完整压缩痕迹;
- 摘要消息本体(`IsCompactSummary` user 消息)在 timeline **默认不再单独渲染**(信息已在 divider 展开区),投影层跳过该消息的常规 user_message item。

### 5.3 设置(PR1 + PR5)

**服务商编辑弹窗**(`SettingsPanel.tsx` ProviderEditorModal 重构):

- models 字段从 tags 多选升级为**模型表格**:列 = 模型 ID | 上下文窗口(placeholder 显示解析链生效值 + 来源 Tag:内置/自动获取/默认,输入即覆盖)| 最大输出(同样式);行内校验 16k~10M,非法禁存;
- provider 级"未知模型默认窗口"输入(可空);
- "刷新模型"按钮升级为同时拉取元数据,拉到的值实时回填 placeholder;
- 顶部摘要行:`N 个模型 · 窗口 128k~1M · 兜底 200k`。

**新"上下文"设置分区**(替换 token-usage 死导航项;`ContextGovernanceSettings` 接真 API):

- 自动压缩开关(默认开)+ 说明文案(解释触发点公式与当前会话模型的实际触发值);
- 高级(折叠):触发百分比覆盖(空=自动)、microcompact 开关与保留条数、摘要模型(跟随会话/小模型);
- 只读展示当前选中模型的生效 profile 卡片(窗口/触发点/警告点/来源),DeepSeek-GUI 4.3 形态。

### 5.4 Diagnostics 面板(PR5)

`ContextDiagnosticsPanel` 保留为深度视图:改消费 `RuntimeContextUsage` + boundary/replacement/reactive 明细(microcompact、snip、替换记录都在这里看),Budget 大数字改为与指示器同源,消灭两处数字对不上。

## 6. PR 序列

依赖顺序,每个可独立合入、旧功能不断:

| PR | 内容 | 关键验证 |
|---|---|---|
| **PR1 模型元数据与窗口解析** | `internal/modelmeta`、configured_providers 列重构、`model_metadata_cache`、discovery 升级、删三处硬编码、Provider 编辑弹窗模型表格 | 首轮完成(2026-07-03); `modelmeta.Resolve` 全链路单测(五级优先级);配置模型后 budget/agent 拿到真值;`go test ./... && cd client && npm run build`。未在修复轮(19 号方案)复审出问题,状态不变。 |
| **PR2 计量与指示器** | messages.usage_json、锚点计量 `computeContextUsage`、`RuntimeContextUsage` + `context.usage.updated`(删 budget.updated)、RuntimeUsage 加 cache 字段、Composer 圆环 + Popover + 警告条 | 首轮完成基础实现(2026-07-04),但 review 发现锚点计量秒/毫秒混淆、估算 usage 冒充真实锚点、字节/rune 估算单位不一致等问题(19 号方案 WP1 问题清单 B 系列);由修复轮 WP1 修复(边界锚定改 ID 序位、真实 usage 门槛、估算口径统一),fix-round commit 待主控合并后回填。 |
| **PR3 手动压缩全链路** | summarizer(9 节 prompt + PTL 重试 + 启发式降级)、full compact 重建(摘要消息/boundary/重注入/hooks/边界配对校验)、compact.* 事件、compact_boundary 两段式 item、CompactDivider、`/compact` 命令、删 attachContextGovernanceToTimeline | 首轮(2026-07-04)按"实施偏离记录"所述,仅落地本地启发式摘要与 append-only 摘要消息/boundary/事件/前端 divider 基础链路,未接模型 summarizer,尾部切分未做 tool_use/tool_result 配对校验。修复轮补齐:WP1 修复切分配对与投影持久化(B1/B2/B3);WP2 新增 `internal/agent/compact_summarizer.go` 模型摘要、read_files/todos/skills 重注入、PreCompact/PostCompact hooks 接线,`EventContextReinjected` 获得首个真实 emitter。fix-round commit 待主控合并后回填。 |
| **PR4 自动触发与防线** | contextGovernance 配置注入、autoCompact 阈值判定(D2/D3)、熔断、blocking、reactive 接线、警告抑制时序、microcompact 激活 | 首轮(2026-07-04)按"实施偏离记录"所述,仅接入默认开启的自动压缩阈值判定与 blocking 拒发,配置存储/API/UI 与 reactive 完整重试/熔断均未完成。修复轮补齐:WP3 新增 `contextGovernance` 配置存储 + GET/PUT `/v1/settings/context-governance` API + 设置 UI「上下文」分区;WP4 接线 reactive 413 重试(attempt1 投影强化/attempt2+ 强制 full compact)与连续 3 次熔断,`compact.failed` 的 attempt/will_retry/circuit_open 改真值。fix-round commit 待主控合并后回填。 |
| **PR5 清理与打磨** | 删 runtime_compact_boundaries 表/store/DTO、删四个 timeline kind 与旧渲染、删假 governance 数据、"上下文"设置分区、diagnostics 对齐、README 索引 | 首轮(2026-07-04)按"实施偏离记录"所述,仅删除了前端 timeline 的四个旧 kind 与旧 token-usage 导航,`runtime_compact_boundaries` 表/store/DTO 的物理删除被推迟。修复轮 WP6 补齐:删除 `runtime_compact.go`/`runtime_compact_payload.go`/`runtime_compact_store.go`(内容迁移到新 `runtime_compact_boundaries.go`,改读 contextmgr 的 `runtime_context_boundaries`);新增 drop 表 migration(`20260705000000_drop_runtime_compact_boundaries.sql`)+ schema.sql 同步;recovery/recordPromptAssembly/runtime_purge 的旧 store 引用清理;`ContextDiagnosticsPanel` 头部数字改消费 `contextUsage`(与 Composer 指示器同源);`contract_test.go` 新增 `TestNoOrphanEventTypes` 并移除 6 个真孤儿事件常量(`EventRuntimeStarted`/`EventRuntimeFailed`/`EventContextLoaded`/`EventContextSourceDiscovered`/`EventRecoveryHistoryHygieneApplied`/`EventTurnProgress`,全仓无任何非声明引用);修复 `internal/fsext` 的 Windows 根路径分隔符比较 bug 与 skills 测试的正反斜杠前缀比较问题。注:`RuntimeCompactBoundary`/`RuntimeReinjectedRef`/`RuntimeCompactToolCallRef` 三个 DTO 未删除——`runtime_output.go`(WP5 所有)与 `runtime_context_helpers.go`(WP2 重注入)已将其复用为活跃的运行时投影/事件类型,不再是"无 writer 的死 stub",删除会破坏这两处不在 WP6 所有权范围内的文件;仅其 SQL 存储层(`runtime_compact_boundaries` 表/store)被删除,详见 19 号方案 WP6 完成状态记录。fix-round commit 待主控合并后回填。 |

## 7. 验证方案

- 单测重点:modelmeta 解析优先级、锚点计量(含"boundary 后锚点失效"回归——防压缩死循环)、阈值推导三档窗口、压缩切分点 API 不变量、digest 幂等、事件成对(started↔completed/failed);
- 集成:`runtime_prompt_assembly` 投影测试扩展——注入配置后各层按序生效、投影结果与 DB 原文分离;
- 人工链路(两通道:桌面 Wails + Vite dev):
  1. 把某模型 `autoCompactPercent` 设 5% → 三五轮对话即触发,观察:警告条出现 → divider 转圈 → 完成 → 指示器百分比回落 → 继续对话模型记得早前意图;
  2. 重启应用打开该会话:divider 仍在,新请求上下文 = 摘要之后;
  3. 手动 `/compact 保留所有文件路径` → 摘要含指令效果;
  4. 关自动压缩 → 警告条变色提示手动;
  5. 塞超大工具输出 → L0 替换生效且模型能通过 ref 取回。

## 8. 风险与开放问题

- **PrepareStep 内同步压缩会拉长该 step 时延**(20-60s):事件 + keep-alive 缓解;若体验不佳,后续可改"turn 之间异步预压缩",本期不做;
- **chars/4 对中文偏乐观**(实际 ~1 char/token 级别):锚点模型下增量占比小,且 max() 兜底;若实测偏差大,估算系数做成 modelmeta 字段;
- **摘要质量依赖会话模型**:第三方弱模型摘要可能差,`summaryModel=small` 与启发式降级是兜底;可在 diagnostics 展示 preTokens→postTokens 供用户判断;
- **与 doc-12 PR4/PR5(前端渲染重写)并行**:CompactDivider 属新增组件冲突面小,但 Timeline 分块逻辑改动需要 rebase 协调,建议本方案 PR3 排在 doc-12 PR4 合入之后;
- 旧 `Summarize`/`IsSummaryMessage` 并入 compact 后,依赖它的 runtime API(若有暴露)一并删除——按"不留兼容"原则执行。

## 实施偏离记录

- PR3(2026-07-04):手动 compact 的摘要先采用本地确定性 9 节结构生成,未在本 PR 新增 provider summarizer 调用路径;原因是 PR3 需要先打通 append-only 摘要消息、contextmgr boundary、compact.* 事件、runtime projection 和前端 divider 的端到端链路,自动触发/模型摘要配置仍归 PR4 接线。
- PR3(2026-07-04):摘要消息继续落在现有 `is_summary_message` 存储字段上,由 runtime projection/消息列表过滤保证不作为普通 user item 展示;物理字段命名收敛到 `IsCompactSummary` 留在 PR5 清理阶段与旧 `Summarize` 概念一并删除。
- PR4(2026-07-04):自动 compact 先采用默认开启策略并在 runtime projection 中按后端 D2/D3 计量触发;设置 API/UI 与 `autoCompactPercent` 持久化覆盖仍留到 PR5 设置收敛,避免在同一 PR 同时引入配置存储迁移与触发链路风险。
- PR4(2026-07-04):reactive 413 重试与三次熔断仅接入 blocking 阈值失败返回和 `compact.failed` 事件语义,未完整重试 provider step;原因是现有 agent loop 错误路径需要更大范围重构,先保证 PrepareStep 前自动压缩与 blocking 拒发可验证。
- PR5(2026-07-04):`runtime_compact_boundaries` 旧表/store/DTO 未物理删除;尝试将旧读 API/恢复路径全部切到 contextmgr 表时暴露出恢复与 replay 测试仍依赖旧 store fixture,为避免破坏审计/恢复链路,本轮只删除前端主 timeline 的四个旧 kind 与旧 token-usage 导航,保留后端物理删除给单独 cleanup PR。
