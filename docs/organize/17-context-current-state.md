# Agent Builder 上下文/压缩/用量 现状梳理

本文是实施「上下文自动压缩 + context 展示」重构前的代码现状快照(2026-07-03,基于 main 分支,conv-ui 流式改造已合入)。所有结论以代码为准,附文件坐标。参考项目对照见 14(cc-haha)、15(DeepSeek-GUI)、16(myclaw/claude-code)号文档,实施方案见 18 号文档。

## 0. 结论速览

1. **多层压缩框架已存在但基本休眠**:`internal/contextmgr/` 有完整的 tool result budget → microcompact → snip → auto compact → full compact 管道 + reactive compact + 全套 SQLite 表,并且**已接入主聊天链路**(投影消息真的会替换发给模型的消息)。但 runtime 调用点传入零值配置:除 tool result budget(有内置默认值)外,其余各层全部 `Enabled=false`,`CompactSummarizer` 从未注入 → **自动压缩事实上不工作**。
2. **token 计量全靠 chars/4 估算**:provider 返回的真实 usage 只累计到 session 表(且是"最新一次覆盖"而非锚点语义),cache read/creation 字段在 runtime DTO 层被丢弃;`RuntimeBudgetReport` 是对 DB 消息的字符估算,与真实 usage 无关。
3. **模型 context window 是三处互相矛盾的硬编码**:用户配置的模型一律 64000(两处),runtime budget 默认 200000(一处),catwalk 内置 catalog 的真实 ContextWindow 只对内置 provider 生效。**没有任何 UI 可以配置 context window 或压缩阈值**。
4. **事件与前端大多是"声明了没接线"**:`compact.*` 事件常量已注册但无 emitter;前端 timeline 已有 `compact_boundary/microcompact_marker` 等 kind 和渲染组件,但数据来自诊断面板旁路而非事件流;设置页 `ContextGovernanceSettingsViewModel` 是静态假数据;composer 无任何 usage 展示。
5. **持久化架构天然支持"投影式压缩"**:messages 表保存原始消息不可变,contextmgr 的 projection/boundary/replacement 表记录"发给模型的视图",与 Claude Code 的 transcript+boundary 模式同构。这个架构方向是对的,应保留并接满。

## 1. Token 用量链路

### 1.1 Agent loop 侧(真实 usage 的入口)

- `internal/agent/agent.go`
  - `OnStepFinish`(约 471-509 行):从 fantasy 拿 `stepResult.Usage`,经 `fallbackStepUsage` 兜底后调 `updateSessionUsage`(1456-1483 行)。
  - `updateSessionUsage`:按 `CostPer1MIn/Out/…Cached` 算成本,发结构化日志事件 `eventTokensUsed`(`internal/agent/event.go:26-38`,**不写 DB**),再调 `updateSessionTokenCounters`(1485-1492 行)。
  - **session 只保留最新一次调用的快照**:`PromptTokens = InputTokens + CacheReadTokens`、`CompletionTokens = OutputTokens`,覆盖式更新,无 per-turn/per-message usage 记录。
- fantasy usage 字段:`InputTokens / OutputTokens / CacheReadTokens / CacheCreationTokens / ReasoningTokens / TotalTokens`(`internal/agent/usage_fallback.go:10-16`)。
- 无 usage 时兜底:`fallbackStepUsage / estimateMessageTokens`,文本按 `(len+3)/4` 估算。

### 1.2 存储

- `sessions` 表(`internal/db/schema.sql:676-707`):`prompt_tokens / completion_tokens / cost` 各一列;`EstimatedUsage` 标志只在内存 map(`internal/session/session.go:104-106`)。
- `runtime_turns` 表(schema.sql:610-626):`usage_before_json / usage_after_json / usage_delta_json`,由 `internal/runtime/runtime_turn_store.go:57-101` 写入;值来自 **session 表快照差值**(`runtime_sessions.go:854-865` `sessionUsage()`,`runtime_turns.go:892-899` `RuntimeUsage.Sub`),不是 provider 原值。
- `messages` 表(schema.sql:33-43)无 usage 字段。

### 1.3 Runtime DTO 与事件

- `RuntimeUsage`(`runtime_contract_types.go:2081-2086`)= `{promptTokens, completionTokens, totalTokens, cost}`——**cache read/creation 在这一层丢失**,前端无法区分缓存命中。
- `RuntimeTurn.UsageBefore/After/Delta`(565-568)。
- 事件:`usage.updated`(`runtimeapi/contract.go:259`,`runtime_events.go:520-529`)、`budget.updated`(contract.go:222,`runtime_budget.go:84-105`,在 turn 开始 `runtime_turns.go:243` 和结束 `runtime_turns.go:777` 发布)。

### 1.4 RuntimeBudgetReport(估算口径)

- `RuntimeBudgetReport`(`runtime_contract_types.go:2284-2300`):`contextWindow, inputBudget, messages, contextSources, toolSchemas, skills, mcp, toolOutputs, selectedToolSchemas, omittedToolSchemas, totalEstimatedTokens`,bucket = `{count, estimatedTokens}`。
- 计算:`computeRuntimeBudget`(`runtime_budget.go:15-82`)对 DB 中 messages/tool calls/capabilities 做 `(rune+3)/4` 字符估算(107-118 行)。**与真实 API usage 完全无关**,长会话下会系统性偏离。

## 2. contextmgr:已建成但休眠的多层压缩

### 2.1 包结构与管道

`internal/contextmgr/`:`types.go`(接口与配置)、`manager.go`(`DefaultManager.BuildModelInput` 管道)、`budget.go`、`microcompact.go`、`snip.go`、`auto_compact.go`、`compact.go`、`reactive.go`、`store.go`(SQLStore)。

`BuildModelInput`(manager.go:48-190)按序执行:

| 层 | 文件 | 触发/配置 | 行为 | 现状 |
|---|---|---|---|---|
| tool result budget | budget.go:28-101 | 零值时用默认:单条 16000 chars、消息级 200000 chars | 超限 tool result 替换为 `<persisted-output>` 摘要,记 ContentReplacement | **实际生效**(唯一生效层) |
| microcompact | microcompact.go:15-63 | `Enabled` + idle 时间/tool 数/估算 token 阈值,KeepRecent 默认 3 | 旧 tool result 替换为 `[Old tool result content compacted…]`,记 micro boundary | 休眠(Enabled=false) |
| snip | snip.go:11-60 | `Enabled` + 保留 head/tail 条数 | 中段消息移除,塞 `<snip-boundary>` 摘要 | 休眠 |
| auto compact | auto_compact.go:10-55 | `Enabled` + `EffectiveContextWindowTokens/OutputReserve/Warning/AutoCompact/BlockingBufferTokens` | 估算 used,`remaining <= AutoCompactBuffer` 时置 `FullCompact.Enabled=true, Trigger="auto"`;越过 warning/blocking 阈值记 Warning;连续失败熔断(MaxConsecutiveFailures);HelperCall/RecursionGuard 跳过 | 休眠(Enabled=false 且窗口=0 双重关闭) |
| full compact | compact.go:14-105 | `Enabled`(手动或被 auto 置位) | `CompactSummarizer` 生成 `<compact-summary>` 系统消息 + 保留尾部 + reinject;`MaxSummaryRetries` + `dropOldestSummaryRound` | 休眠且 **summarizer 未注入**(`ManagerOptions.Summarizer` 为 nil,见 runtime_lifecycle.go:298-299) |
| reactive | reactive.go | `IsContextLengthError`(10-32,正则匹配 provider 超长错误)→ `ReactiveCompact`(34-75) | attempt 1 记 projection_reduction,后续 attempt 走 full_compact | **无调用方**(agent loop 错误路径未接) |

### 2.2 接线状态(重要修正)

contextmgr **已经在主聊天链路上**,而不是纯记录旁路:

```text
agent.go:351 PrepareStep
  → a.buildModelInput(...)                       // agent.go:721-739
  → runtimeSchedulerRecorder.BuildModelInput      // runtime_prompt_assembly.go:26
  → runtimeService.buildModelInputProjection      // runtime_prompt_assembly.go:339-373
  → injectProjectMemory + contextManager.BuildModelInput
  → agent.go:364 prepared.Messages = projectedMessages   // 投影真实替换发给模型的消息
```

另两个调用点:agent.go:1047、1341(helper/summarize 路径,TurnID 置 `helper_<sessionID>`)。

**问题在配置**:`buildModelInputProjection` 构造 `BuildInputRequest` 时(runtime_prompt_assembly.go:351-359)只传 session/turn/step/provider/model/messages,`ToolResultBudget/Microcompact/Snip/FullCompact/AutoCompact` 全为零值 → 除 budget 层默认值外全部关闭。**没有任何配置源(config 文件/DB/设置 UI)能把这些开关和阈值送进来。**

### 2.3 contextmgr 的 SQLite 表(schema.sql:226-354)

- `runtime_context_projections`(279-295):每次 model input 一条,`budget_before_json/budget_after_json`。
- `runtime_context_projection_messages`(263-277):消息在投影中的状态(selected/replaced,token_estimate,replacement_id)。
- `runtime_context_boundaries`(226-244):kind(full/micro/manual)、trigger、message_refs、reinjected_refs、budget before/after、summary_ref。
- `runtime_context_content_replacements`(246-261)、`runtime_context_snip_boundaries`(332-343)、`runtime_context_warnings`(345-354)、`runtime_context_reactive_attempts`(297-308)。
- `runtime_context_read_state_snapshots`(310-317)、`runtime_context_reinjections`(319-330):**建表未使用**。

### 2.4 与 runtime 侧第二套 compact 表的重复

- `runtime_compact_boundaries`(schema.sql:208-224)+ `RuntimeCompactBoundary` DTO(runtime_contract_types.go:2238-2254)+ `runtime_compact_store.go`:结构与 contextmgr boundary 高度重复,**只有读 API(`TurnCompactBoundaries/SessionCompactBoundaries`),没有任何写入路径**,是死 stub。重构时应二选一(方案:删 runtime 侧 stub,统一走 contextmgr 表)。

### 2.5 手动入口与事件

- `ManualCompact/ManualSnip` HTTP/Wails 入口(runtime_prompt_assembly.go:76-162):只写一条 boundary 记录 + 发 `"context.compact.manual"/"context.snip.manual"` 事件,**不做真实压缩**(不调 summarizer、不改消息)。且这两个事件字符串未注册进 `runtimeapi.EventTypes`。
- `compact.boundary.recorded / compact.micro.completed / compact.full.completed / compact.failed / compact.output.preserved`(contract.go:223-227):**已声明,无 emitter**。
- `"prompt.assembly.recorded"`(runtime_prompt_assembly.go:288-308)也是未注册的裸字符串事件。

## 3. Prompt assembly / context sources / read-files

### 3.1 Prompt assembly(context 分布展示的数据源)

- 类型:`internal/agent/prompt_assembly.go` — `PromptAssemblySnapshot{Sections, System, Messages, Tools, Skills, MCP}`;`PromptSectionSummary` 含 `Kind/Role/Order/CachePolicy/Source/Hash/Length/TokenEstimate/Redacted/RawStored`。
- 生产:`agent.go:668-719 recordPromptAssembly`(每 step 一次,在投影之后),section 包括 coder prompt 各段、provider prefix、MCP instructions(`agent.go:741-773`);memory 注入见 `runtime_prompt_assembly.go:340-347 injectProjectMemory`。
- 存储:`runtime_prompt_assemblies` 表(schema.sql:455-471,`sections_json/context_sources_json/compact_json/budget_json/projection_id`)。
- DTO:`RuntimePromptAssembly`(runtime_contract_types.go:2307-2329),`enrichPromptAssembliesWithContext`(runtime_prompt_assembly.go:375-419)会把 contextmgr 的 boundaries/snip/replacements/reactive attempts 合并进来。
- **原始内容不落库**(RawStored=false),只有 hash + token 估算——安全约束,方案需保持。

### 3.2 Context sources

- `RuntimeContextSource` 生产:`runtime_context.go:18-28` → `prompt.LoadContextSources` + skills/MCP 派生源;聚合 `runtimeTurnContextSummary`(141-159)。
- 生命周期事件 `context.*` 已注册(contract.go:210-218)。
- 无独立表,快照存于 prompt assembly。

### 3.3 Read-files 追踪(压缩后回填文件的基础)

- `read_files` 表(schema.sql:112-118):`turn_id/tool_call_id/size_bytes/content_hash/mtime_unix/offset/read_limit/partial/token_estimate/state`。
- writer `internal/filetracker/service.go`;runtime API `ReadFiles`(runtime_context.go:215-238)读取时做 stat/hash 比对判 `recorded/stale/missing/hash_changed/mtime_changed`(240-280)。
- **这正是 Claude Code "压缩后回填最近 5 个文件" 所需的数据,已经齐了。**

## 4. Provider / Model 配置与 context window

### 4.1 类型与存储

- `internal/config/config.go`:`SelectedModel`(66-90,含 MaxTokens/Temperature 等,**无 ContextWindow**)、`ProviderConfig`(92-140,`Models []catwalk.Model`)。
- `catwalk.Model` 元数据(config.go:143-170 转换):**有 `ContextWindow` 和 `DefaultMaxTokens` 字段**,内置 catalog(`runtime_provider_catalog.go:31-69`,来自 catwalk embedded + hyper)带真实值。
- SQLite:`provider_catalog`(schema.sql:96-110,无 model 级元数据)、`configured_providers`(3-19,**model 只存 ID 数组 `model_ids_json`,无 per-model 属性**)、`selected_models`(663-674,scope=global/project/session)。

### 4.2 Context window 硬编码问题(三处不一致)

| 位置 | 值 | 影响 |
|---|---|---|
| `runtime_model_config.go:269-270` `applyModelConfig` | ContextWindow 64000 / DefaultMaxTokens 4096 | 所有用户配置的模型 |
| `runtime_selected_model.go:249-250` `applyConfiguredProviderModel` | 同上 | configured provider 场景 |
| `runtime_budget.go:13` `defaultRuntimeContextWindow` | 200000 | budget 报告兜底 |

### 4.3 模型发现

- `discoverModelIDs`(runtime_model_config.go:147-202):GET `<baseURL>/models`,openai/anthropic 分 header(204-221),**只解析 id/display_name,不取 context window 等元数据**。前端入口 `SettingsPanel.tsx:564-594 refreshModels`。
- 注:OpenAI 兼容 `/v1/models` 响应普遍无窗口字段;OpenRouter 等有 `context_length`;Anthropic `/v1/models` 无窗口字段。"自动获取"只能对部分 provider 生效,必须保留手动覆盖 + 内置表兜底。

## 5. 前端现状

### 5.1 Composer(context 图标的落点)

- `client/src/features/composer/Composer.tsx` + `Composer.module.css`。
- footer 右侧 `.rightControls`:模型下拉(96-118 `modelMenu`,152-158 `<Dropdown>`)+ 发送/停止按钮。**context 图标的目标位置 = 模型下拉左侧**。
- 左侧 `.leftControls`:`+` 添加上下文占位按钮(148 行,无逻辑)+ `PermissionModeControl`。
- `ComposerViewModel`(workbenchTypes.ts:118-129):`modelLabel/selectedModel/modelOptions/busy/activeTurnId`,**无 usage/budget 字段**。

### 5.2 设置-服务商

- `client/src/features/settings/SettingsPanel.tsx`:`ProvidersSettings`(312-491)、`ProviderEditorModal`(493-726)。
- 现有字段:providerId/name/remark/apiEndpoint/protocol(openai-compat|anthropic)/token/defaultModel/models(tags 多选)/proxy;按钮:刷新模型/测试/测速。
- **无任何 per-model 配置能力**(model 只是 ID 字符串)。
- 设置 nav 有 `token-usage` 项(`staticWorkbenchAdapter.tsx:45`)但 `renderContent`(147-199)无对应 case,落 General。
- `ContextGovernanceSettingsViewModel`(workbenchTypes.ts:1200-1208)是静态假数据(`staticWorkbenchAdapter.tsx:52-60`:autoCompactEnabled:true 等),无后端读写路径。

### 5.3 对话流渲染

- `client/src/features/timeline/Timeline.tsx`;kind 枚举 `ConversationTimelineKind`(workbenchTypes.ts:180-205)**已含** `compact_boundary/snip_boundary/microcompact_marker/reactive_compact_retry/tool_result_replacement/context_source/diagnostic_warning/exploration_summary`。
- 渲染组件已有:`WorkflowNoticeRow`(301-315,hook/todo/recovery/turn_terminal)、`ContextGovernanceRow`(332-346,compact 系)、`TurnDiagnosticWarning`(470-484)。
- **但数据来源是旁路**:`wailsWorkbenchAdapter.attachContextGovernanceToTimeline`(wailsWorkbenchAdapter.ts:3150-3211)从 `ContextDiagnosticsViewModel` 后期拼进 timeline,不经 runtime projection/事件流 → 与 doc-12 确立的 "runtime 是唯一投影事实源" 相违背,重构时应改由后端投影产出 item。

### 5.4 现有 token 展示

- 仅 `ContextDiagnosticsPanel`(`client/src/features/diagnostics/ContextDiagnosticsPanel.tsx`):Budget 大数字(82)、BudgetRow 分类(232-239)、context sources tokenEstimate(147)、手动 Compact/Snip 按钮(40-61)。诊断面板,不在主界面。
- Composer、消息卡片(`MessageFooter` 846-892)均无 token 展示。

### 5.5 事件/数据流(压缩事件的前端接入点)

- push 通道:`client/src/runtime/outputStream.ts` `subscribeSessionOutput`(Wails Events → SSE → 1.2s 轮询三级 fallback,50ms 批处理)。
- `RuntimeOutputEvent`(outputTypes.ts:280-301)**已有 `compact` 字段占位**(299 行,类型 unknown);`RuntimeOutputSnapshot.Compact` 后端也带 boundary 数组(runtime_output.go:120-122)。
- 事件刷新分类:`runtimeEventRefresh.ts`(immediate/coalesced/covered-by-stream 三档)。
- 后端投影把 boundary 转 timeline item:`runtime_output.go:664-700`。

## 6. 新增一类事件的标准接线路径(供方案引用)

1. `runtimeapi/contract.go`:加事件常量 + 入 `EventTypes`(如 ephemeral 加 `EphemeralEventTypes`);
2. 后端 emitter 用 `storeRuntimeEvent/publishRuntimeEvent`(runtime_events.go:466-488);
3. `runtime_output.go` `eventsFromRuntimeEvents`/`itemsForRuntimeEvent` 分派为 output event / conversation item;需要即时推送的走 `runtime_output_stream.go` `publishSessionOutputEventFromRuntime` 特化分支(参照 delta,217-236);
4. 前端 `outputTypes.ts` 补类型、`outputReducer.ts` 处理、`runtimeEventRefresh.ts` 归类;
5. Wails/HTTP/契约测试同步:`desktop/runtime_bridge.go`、`runtime_http.go`、`contract_test.go`。

## 7. 主要问题清单(方案要解决的)

1. contextmgr 各层无配置源,auto compact 永不触发;summarizer 未实现/未注入;manual compact 是假动作(只记 boundary 不改消息)。
2. reactive compact 无调用方,provider 超长错误直接失败。
3. token 计量:budget 全估算、usage 契约丢 cache 字段、session usage 覆盖式无锚点语义、无 per-turn 真实 usage。
4. context window 三处硬编码互相矛盾;模型发现不取元数据;无 per-provider/per-model 配置与 UI。
5. 事件契约漂移:compact.* 无 emitter;三个裸字符串事件未注册。
6. 两套 compact boundary 表重复,runtime 侧是无 writer 的死代码。
7. 前端 compact 展示走 diagnostics 旁路而非 runtime 投影;composer 无 context 指示;设置页 context governance 是假数据;token-usage nav 无页面。
8. `runtime_context_read_state_snapshots/reinjections` 建表未用(可作压缩回填的落点或删除)。

## 附:关键文件坐标一览

| 层 | 文件 |
|---|---|
| fantasy usage 消费 | `internal/agent/agent.go`(471-509, 1456-1499)、`internal/agent/event.go`、`internal/agent/usage_fallback.go` |
| session usage 存储 | `internal/session/session.go`、`internal/db/schema.sql:676-707` |
| contextmgr 实现 | `internal/contextmgr/{types,manager,budget,microcompact,snip,auto_compact,compact,reactive,store}.go` |
| contextmgr schema | `internal/db/schema.sql:226-354` |
| 投影接线 | `internal/agent/agent.go:351-364,721-739`、`internal/runtime/runtime_prompt_assembly.go:339-373` |
| contextmgr 初始化(无 summarizer) | `internal/runtime/runtime_lifecycle.go:298-299`、`runtime_prompt_assembly.go:421-432` |
| prompt assembly | `internal/agent/prompt_assembly.go`、`internal/agent/prompt/sections.go`、`internal/agent/agent.go:668-773`、`internal/runtime/runtime_prompt_assembly.go`、schema.sql:455-471 |
| runtime budget | `internal/runtime/runtime_budget.go` |
| 事件契约 | `internal/runtimeapi/contract.go`(123-296) |
| turn usage | `internal/runtime/runtime_turn_store.go`、schema.sql:610-626、`runtime_turns.go:706-822,892-899` |
| compact boundary(死 stub) | `internal/runtime/runtime_compact.go`、`runtime_compact_store.go`、schema.sql:208-224 |
| manual compact/snip | `internal/runtime/runtime_prompt_assembly.go:76-162` |
| context sources / read-files | `internal/runtime/runtime_context.go`、`internal/agent/prompt/prompt.go`、`internal/filetracker/service.go`、schema.sql:112-118 |
| provider/model 配置 | `internal/config/config.go`、`internal/runtime/runtime_provider_catalog.go`、`runtime_provider_settings.go`、`runtime_selected_model.go`、`runtime_model_config.go`(硬编码 269-270)、`runtime_selected_model.go:249-250`、`runtime_budget.go:13` |
| runtime→前端 DTO | `internal/runtime/runtime_contract_types.go`(RuntimeUsage 2081、RuntimeBudget* 2284、RuntimeCompact* 2238、RuntimePromptAssembly 2307) |
| output 流 | `internal/runtime/runtime_output_stream.go`、`runtime_output.go`(120-122, 279+, 664-700)、`runtime_events.go`(466-553) |
| 前端类型 | `client/src/runtime/workbenchTypes.ts`(Kind 180、Composer 118、ContextGovernanceSettings 1200、ContextDiagnostics 564、PromptBudget 705) |
| 前端 adapter/数据层 | `client/src/runtime/wailsWorkbenchAdapter.ts`(attachContextGovernanceToTimeline 3150)、`outputTypes.ts`(compact 占位 299)、`outputStream.ts`、`runtimeEventRefresh.ts`、`staticWorkbenchAdapter.tsx`(45, 52-60) |
| 前端 UI | `client/src/features/composer/Composer.tsx`(96-158)、`client/src/features/settings/SettingsPanel.tsx`(312-726)、`client/src/features/timeline/Timeline.tsx`(301-346, 470-484)、`client/src/features/diagnostics/ContextDiagnosticsPanel.tsx` |
