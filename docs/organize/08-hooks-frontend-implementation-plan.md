# Hook 前端展示与可观测性完整实施方案

本文承接 `07-hooks-comparison.md` 的梳理结论：Agent Builder 当前 Hook 后端/runtime 基础已经比较完整，真正影响产品体验的主要缺口在前端可见性、诊断入口和运行态可观测性。

本方案目标不是重写 Hook runtime，也不是一次性照搬 Claude 系列全部 Hook 事件面，而是在现有 Agent Builder 架构边界内，把已经存在的 runtime 事实源展示出来：

- Go runtime 继续作为 Hook 配置、执行记录、审计和恢复的事实源。
- React 只消费 `WorkbenchViewModel` / adapter view model，不直接持有 Hook 业务状态。
- Wails、HTTP/dev transport、runtime API contract、frontend adapter 必须同步。
- Runtime events 只作为刷新触发，不作为 React 事实状态。
- UI 使用 Ant Design / Ant Design icons / theme tokens / scoped CSS Modules。
- 第一阶段先做只读展示和诊断闭环，再评估 Hook 配置编辑能力。

## 当前基线判断

### 后端已有能力

关键文件：

- `internal/hooks/hooks.go`
- `internal/hooks/runner.go`
- `internal/hooks/input.go`
- `internal/agent/hooked_tool.go`
- `internal/runtime/runtime_hooks.go`
- `internal/runtime/runtime_input.go`
- `internal/runtime/runtime_react_callchain.go`
- `internal/runtime/runtime_turn_diagnostics.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_http.go`
- `internal/runtimeapi/contract.go`
- `desktop/runtime_bridge.go`
- `internal/db/migrations/20260524100000_add_runtime_hook_executions.sql`

当前已确认的主执行链路：

- `PreToolUse`：工具执行前运行，可 deny、halt、rewrite tool input、注入 context。
- `PostToolUse`：工具成功后运行，可注入 context、deny/halt 后续流程。
- `PostToolUseFailure`：工具返回错误或 `resp.IsError` 时运行。
- `UserPromptSubmit`：turn 创建前处理 normalized input，可 block、rewrite prompt 或 prevent continuation。

当前已经进入配置/契约/枚举但仍需复核完整触发链的事件：

- `PreCompact`
- `PostCompact`
- `PostSampling`
- `Stop`

当前 runtime API：

- `Hooks(ctx)`
- `HookExecutions(ctx, req)`
- `HookExecution(ctx, executionID)`
- HTTP `GET /v1/hooks`
- HTTP `GET /v1/hook-executions`
- HTTP `GET /v1/hook-executions/{id}`
- Wails bridge 已有同名转发入口。

当前持久化表：

- `runtime_hook_executions`

当前 runtime event：

- `hook.discovered`
- `hook.configured`
- `hook.execution.started`
- `hook.execution.completed`
- `hook.execution.skipped`
- `hook.execution.blocked`
- `hook.execution.failed`
- `hook.context.injected`
- `hook.input.rewritten`

### 前端已有但不完整的能力

关键文件：

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/features/diagnostics/ReactCallchainInspector.tsx`
- `client/src/features/diagnostics/TurnDiagnosticsPanel.tsx`
- `client/src/features/settings/SettingsPanel.tsx`
- `client/src/features/timeline/Timeline.tsx`

当前可见性：

- `RuntimeNormalizedInput.hookOutcome` 会被 adapter 映射为一条 assistant error message，例如 prompt 被 `UserPromptSubmit` hook blocked。
- `ReactCallchainViewModel` 已有 `hookCount`、`usesHooks`、`hookExecutionId`。
- `ReactCallchainInspector` 能显示 hook 数量，并把 hook node 当通用 callchain node 展示。
- `TurnDiagnosticsPanel` 可以通过 stop reason 间接看到 hook halt。
- timeline 可以通过普通 tool result 或 diagnostic 间接看见 hook block 后果。

当前缺口：

- 没有 `HookViewModel` / `HookExecutionViewModel`。
- adapter 没有 hydrate hooks / hook executions 到主 view model。
- Settings 没有 Hooks 配置浏览或状态页。
- Diagnostics 没有 Hook executions 面板和详情抽屉。
- Timeline 没有一等 Hook row/card，无法清晰展示 `PreToolUse`、`PostToolUse`、input rewrite、context injected、blocked、failed。
- 没有 Hook 运行进度展示；用户只能看到执行后的聚合结果。
- `HookExecution(ctx, id)` 有后端 API，但前端没有详情入口。
- `docs/hooks/README.md` 对当前支持事件的描述需要与代码事实重新对齐。

## 总体执行顺序

推荐分六个阶段推进：

```text
阶段 0：契约和基线复核
  -> 阶段 1：前端 runtime contract 与 adapter hydration
  -> 阶段 2：Settings Hooks 只读配置页
  -> 阶段 3：Hook executions 诊断面板与详情抽屉
  -> 阶段 4：Timeline / tool / callchain 集成
  -> 阶段 5：测试、契约加固和回归场景
  -> 阶段 6：文档修正和后续扩展规划
```

这样排序的原因：

- 后端 API 已存在，先接通 adapter 和 view model 可以尽快暴露事实源。
- Settings 只读页风险最低，能先解决“用户不知道配置了什么 Hook”的问题。
- Hook execution 诊断面板依赖 adapter 数据闭环，适合放在配置页之后。
- Timeline/callchain 集成会影响主工作流信息密度，应在数据模型稳定后做。
- 配置编辑、非工具类事件扩展、运行中 progress message 都应放到只读和诊断能力稳定之后。

## 阶段 0：契约和基线复核

### 目标

在改前端前确认 runtime contract 的真实字段、命名和边界，避免 UI 直接根据推测字段开发。

### 需要复核的事实

后端 Hook 配置响应：

- `RuntimeHook` 的 JSON 字段是否统一使用 camelCase。
- hook `id` 是否稳定。
- hook `source` 是否能区分 global / workspace / project / unknown。
- hook `event` 是否总是标准事件名。
- matcher 为空时前端应显示为“全部”还是留空。
- command 是否需要在前端脱敏或截断。
- timeout 是否以秒、毫秒或 duration string 表达。

后端 Hook execution 响应：

- `RuntimeHookExecution` 是否包含以下信息：
  - identity：`id`、`hookId`、`hookName`、`hookSource`
  - scope：`sessionId`、`turnId`、`toolCallId`、`taskId`
  - capability：`capabilityId`、`mcpServer`、`skill`
  - policy/sandbox：`policyMode`、`policyProfile`、`policyRule`、`policyDecision`、`sandboxDecisionId`、`sandboxStatus`
  - result：`status`、`reason`、`error`、`inputSummary`、`outputSummary`、`contextSummary`
  - flags：`inputRewritten`、`contextInjected`、`redacted`
  - timing：`startedAt`、`completedAt`、`durationMs`
- 查询接口是否支持按 `sessionId`、`turnId`、`toolCallId`、`taskId`、`event`、`status` 过滤。
- 列表接口是否已有 limit / cursor / offset；如果没有，前端第一版只取当前 session 并限制渲染数量。

前端刷新系统：

- 当前 `wailsWorkbenchAdapter.ts` 的事件订阅是否已经统一把 runtime event 映射为 refresh trigger。
- `hook.*` 事件是否已经进入刷新白名单。
- Vite/browser development 是否通过 HTTP/dev transport fallback 可访问 Hook API。

### 阶段 0 输出

- 不改业务逻辑。
- 明确本轮需要改动的文件列表。
- 如果发现 contract 字段漂移，先补 adapter 兼容映射，不让组件直接处理多套 DTO。
- 如果发现 Wails bridge 或 HTTP route 缺失，再补最小转发和测试。

### 验收

- 能明确列出 `RuntimeHook` 和 `RuntimeHookExecution` 的真实 JSON shape。
- 能明确 Hook 刷新事件从 runtime 到 frontend adapter 的路径。
- 能明确第一版前端展示只依赖已存在 API，不依赖工具文本解析。

## 阶段 1：前端 runtime contract 与 adapter hydration

### 目标

把 Hook 配置和 Hook execution 摘要接入 `WorkbenchViewModel`，形成 runtime -> adapter -> React 的只读数据闭环。

### 前端类型改动

文件：

- `client/src/runtime/workbenchTypes.ts`

新增 view model：

```ts
export type HookExecutionStatusViewModel =
  | 'started'
  | 'completed'
  | 'skipped'
  | 'blocked'
  | 'failed'
  | string;

export interface HookViewModel {
  id: string;
  name: string;
  source: string;
  event: string;
  matcher?: string;
  commandPreview: string;
  enabled: boolean;
  status: 'active' | 'invalid' | 'unknown' | string;
  diagnostics?: string;
  reason?: string;
  timeoutMs?: number;
}

export interface HookExecutionViewModel {
  id: string;
  hookId: string;
  hookName?: string;
  hookSource?: string;
  event: string;
  status: HookExecutionStatusViewModel;
  sessionId?: string;
  turnId?: string;
  toolCallId?: string;
  taskId?: string;
  capabilityId?: string;
  policyDecision?: string;
  sandboxStatus?: string;
  reason?: string;
  error?: string;
  inputSummary?: string;
  outputSummary?: string;
  contextSummary?: string;
  inputRewritten: boolean;
  contextInjected: boolean;
  redacted: boolean;
  startedAt?: number;
  completedAt?: number;
  durationMs?: number;
}

export interface HookExecutionSummaryViewModel {
  sessionId?: string;
  items: HookExecutionViewModel[];
  total: number;
  started: number;
  completed: number;
  blocked: number;
  failed: number;
  skipped: number;
  rewritten: number;
  contextInjected: number;
  lastUpdatedAt?: number;
}
```

在 `WorkbenchViewModel` 增加：

```ts
hooks?: HookViewModel[];
hookExecutions?: HookExecutionSummaryViewModel;
```

如 callchain node 需要更明确的 Hook 摘要，可增加可选字段：

```ts
hook?: {
  executionId?: string;
  event?: string;
  status?: HookExecutionStatusViewModel;
  reason?: string;
  durationMs?: number;
  inputRewritten?: boolean;
  contextInjected?: boolean;
};
```

兼容策略：

- view model 字段使用稳定的 camelCase。
- DTO 字段兼容 camelCase 和 snake_case，但组件只消费 view model。
- `commandPreview` 只展示截断后的命令摘要，不在列表页展示完整命令。
- `redacted` 为 true 时，组件必须用明确的“已脱敏”状态，不能把空值误判为无数据。

### Adapter 改动

文件：

- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/runtime/staticWorkbenchAdapter.tsx`

新增 DTO：

```ts
interface RuntimeHookDTO {
  id?: string;
  name?: string;
  source?: string;
  event?: string;
  matcher?: string;
  command?: string;
  timeout?: number;
  timeoutMs?: number;
  enabled?: boolean;
  status?: string;
  diagnostics?: string;
  reason?: string;
}

interface RuntimeHookExecutionDTO {
  id?: string;
  hookId?: string;
  hook_id?: string;
  hookName?: string;
  hook_name?: string;
  hookSource?: string;
  hook_source?: string;
  event?: string;
  status?: string;
  sessionId?: string;
  session_id?: string;
  turnId?: string;
  turn_id?: string;
  toolCallId?: string;
  tool_call_id?: string;
  taskId?: string;
  task_id?: string;
  capabilityId?: string;
  capability_id?: string;
  policyDecision?: string;
  policy_decision?: string;
  sandboxStatus?: string;
  sandbox_status?: string;
  reason?: string;
  error?: string;
  inputSummary?: string;
  input_summary?: string;
  outputSummary?: string;
  output_summary?: string;
  contextSummary?: string;
  context_summary?: string;
  inputRewritten?: boolean;
  input_rewritten?: boolean;
  contextInjected?: boolean;
  context_injected?: boolean;
  redacted?: boolean;
  startedAt?: number;
  started_at?: number;
  completedAt?: number;
  completed_at?: number;
  durationMs?: number;
  duration_ms?: number;
}

interface RuntimeHookExecutionsRequestDTO {
  sessionId?: string;
  turnId?: string;
  toolCallId?: string;
  taskId?: string;
  event?: string;
  status?: string;
  limit?: number;
}
```

新增 mapper：

- `mapHook(dto: RuntimeHookDTO): HookViewModel`
- `mapHookExecution(dto: RuntimeHookExecutionDTO): HookExecutionViewModel`
- `summarizeHookExecutions(items: HookExecutionViewModel[], sessionId?: string): HookExecutionSummaryViewModel`

新增 hydration：

- `hydrateHooks()`
- `hydrateHookExecutions(sessionId?: string)`

接入主 hydration：

- 当前存在 active session 时，查询当前 session 的 Hook executions。
- 第一版不为每条列表记录调用详情 API，只使用列表响应。
- 详情抽屉打开时再调用 `HookExecution(id)` 获取完整记录。
- adapter 异常时保持 `hooks: []` / `hookExecutions.items: []`，并通过 diagnostics 或 adapter log 暴露错误，不阻断主 workbench 渲染。

刷新事件：

- `hook.discovered`
- `hook.configured`
- `hook.execution.started`
- `hook.execution.completed`
- `hook.execution.skipped`
- `hook.execution.blocked`
- `hook.execution.failed`
- `hook.context.injected`
- `hook.input.rewritten`

这些事件只触发重新 hydrate，不把 event payload 直接当 React 状态源。

### Static adapter 改动

文件：

- `client/src/runtime/staticWorkbenchAdapter.tsx`

要求：

- 默认返回 `hooks: []`。
- 默认返回空 `HookExecutionSummaryViewModel`。
- 可增加 1-2 条静态样例，供 Storybook 或静态预览使用；如果当前项目没有相关预览入口，则先保持空值。

### 阶段 1 验收

- `WorkbenchViewModel` 中能看到 Hook 配置列表和当前 session Hook execution 摘要。
- Vite/browser dev 和 Wails runtime 都通过 adapter 访问数据。
- React 组件中没有直接调用 Wails generated binding、`fetch`、`XMLHttpRequest` 或 axios。
- Hook runtime event 触发后，前端会刷新 hooks / hookExecutions。

## 阶段 2：Settings Hooks 只读配置页

### 目标

在 Settings 中增加 Hooks 页面，让用户能查看当前生效的 Hook 配置、来源、事件、matcher、命令摘要、timeout 和诊断状态。

### 文件

- `client/src/features/settings/SettingsPanel.tsx`
- `client/src/features/hooks/HookSettingsPanel.tsx`
- `client/src/features/hooks/HookSettingsPanel.module.css`
- 如当前 settings navigation 有独立定义文件，也需要同步增加 `hooks` key。

### UI 信息架构

Settings 左侧增加 `Hooks` 导航项。

页面主体建议分三块：

1. 顶部概览：
   - configured hooks 总数。
   - active / invalid / unknown 数量。
   - 当前支持事件集合。
   - 当前页面是只读配置浏览，不提供编辑。

2. Hook 列表：
   - 事件：`PreToolUse`、`PostToolUse`、`UserPromptSubmit` 等。
   - 名称：优先显示 `name`，缺失时显示 `event + index`。
   - 来源：global / workspace / project / unknown。
   - matcher：为空时显示“全部工具”或“全部输入”，按 event 语义决定。
   - command preview：单行截断，支持 tooltip 查看更长摘要，但不展示敏感完整环境。
   - timeout：显示统一单位。
   - status：active / invalid / unknown。
   - diagnostics：有错误时显示 warning/error 文本。

3. 选中 Hook 详情：
   - event。
   - matcher。
   - source。
   - timeout。
   - command preview。
   - 最近执行摘要：如果 `hookExecutions` 中存在同 hookId 记录，显示最近 status、duration、reason。

### 交互要求

- 第一版只读，不提供新增、编辑、删除 Hook。
- 不直接打开本地配置文件路径；如果后端以后提供 config path，可作为复制路径动作，但不能在前端猜测文件位置。
- 空状态需要明确区分：
  - 没有配置 Hook。
  - Hook API 暂不可用。
  - 当前 session 没有 Hook execution。
- Settings navigation 不应因为未知 key fallback 到 General；新增 `hooks` key 后要确保深链或内部状态能稳定选中。

### 设计约束

- 使用 Ant Design `Table` / `List` / `Tag` / `Descriptions` / `Alert` / `Tooltip`。
- 使用 Ant Design icons，例如 `BranchesOutlined`、`CheckCircleOutlined`、`WarningOutlined`、`StopOutlined`。
- 样式写入 scoped CSS Module，使用 theme token，不扩大全局 CSS。
- 列表和详情不使用嵌套卡片；页面可用 full-width layout 或左右分栏。

### 阶段 2 验收

- 用户能在 Settings 中看到当前所有 Hook 配置。
- 用户能区分 Hook 来源、事件、matcher、状态和 timeout。
- 无 Hook 时有清晰空状态。
- Settings Hooks 页面只依赖 `WorkbenchViewModel.hooks` 和 `WorkbenchViewModel.hookExecutions`。

## 阶段 3：Hook executions 诊断面板与详情抽屉

### 目标

让用户能查看当前 session 的 Hook 执行历史，定位哪个 Hook 阻断、失败、改写输入或注入上下文。

### 文件

- `client/src/features/hooks/HookExecutionsPanel.tsx`
- `client/src/features/hooks/HookExecutionDetailDrawer.tsx`
- `client/src/features/hooks/HookExecutionsPanel.module.css`
- `client/src/features/diagnostics/TurnDiagnosticsPanel.tsx`
- `client/src/runtime/wailsWorkbenchAdapter.ts`

如果项目已有统一 Drawer / inspector 组件，应复用现有模式。

### 面板入口

推荐第一版放在 diagnostics 区域：

- `TurnDiagnosticsPanel` 增加 Hook executions section。
- 或在现有右侧 inspector 增加 `Hooks` tab。

后续可再把高价值异常记录投放到 timeline。

### 列表字段

Hook execution 列表显示：

- startedAt / duration。
- event。
- hook name。
- status。
- tool / turn / task 关联。
- reason。
- flags：
  - input rewritten。
  - context injected。
  - redacted。
- error 摘要。

过滤器：

- event。
- status。
- toolCallId。
- only blocked/failed。
- only rewritten/context injected。

排序：

- 默认按 startedAt 倒序。
- 如果 startedAt 缺失，按列表原始顺序兜底。

### 详情抽屉

打开某条 execution 时调用 adapter detail 方法：

- `hookExecution(id)` 或与现有 adapter 命名一致的方法。
- 方法内部调用 Wails/HTTP runtime API。

详情展示：

- 基本信息：id、hookId、hookName、source、event、status。
- Scope：sessionId、turnId、toolCallId、taskId。
- Timing：startedAt、completedAt、durationMs。
- Result：reason、error。
- Summaries：inputSummary、outputSummary、contextSummary。
- Policy / sandbox：policyDecision、sandboxStatus 等。
- Flags：inputRewritten、contextInjected、redacted。

跳转/关联：

- 如果有 `toolCallId`，提供“查看工具调用”动作或高亮对应 tool card。
- 如果有 `turnId`，提供“查看 turn diagnostics”动作。
- 如果 callchain node 有 `hookExecutionId`，详情抽屉可以从 callchain inspector 打开。

### 脱敏和安全

- `redacted` 为 true 时，摘要区域显示“内容已脱敏”，不猜测原始内容。
- error 和 reason 也需要按后端返回原样展示，不拼接用户输入。
- command full text 不进入 execution 详情，除非后端明确提供已脱敏字段。

### 阶段 3 验收

- 用户能查看当前 session 的 Hook execution 历史。
- 用户能快速过滤 blocked / failed / rewritten / context injected。
- 点击 execution 能打开详情。
- 详情抽屉按需调用单条 API，不在列表加载时 N+1 请求。

## 阶段 4：Timeline / tool / callchain 集成

### 目标

把 Hook 影响主流程的事实放到用户实际工作路径里，而不是只藏在 Settings 或 Diagnostics。

### React callchain 集成

文件：

- `client/src/features/diagnostics/ReactCallchainInspector.tsx`
- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`

改动：

- hook node 显示 event tag、status tag、duration。
- 如果 `inputRewritten` 为 true，显示 rewritten tag。
- 如果 `contextInjected` 为 true，显示 context tag。
- 如果 `reason` 存在，显示 reason 摘要。
- 如果 `hookExecutionId` 存在，提供打开详情抽屉动作。

原则：

- callchain node 的 Hook 展示优先使用 runtime callchain 已有字段。
- 如果需要补充详情，使用 `hookExecutionId` 查 execution。
- 不从 tool result 文本中解析 Hook 语义。

### Timeline 集成

文件：

- `client/src/features/timeline/Timeline.tsx`
- 可能新增 `client/src/features/hooks/HookTimelineRow.tsx`
- 可能新增 `client/src/features/hooks/HookTimelineRow.module.css`

推荐第一版只把高信号 Hook 事件放入 timeline：

- blocked。
- failed。
- input rewritten。
- context injected。
- halt / stop reason 与 Hook 相关。

展示形态：

- 能归属到 tool call 的 Hook，作为 tool card 内部 badge/inline row。
- 不能归属到 tool call 的 Hook，例如 `UserPromptSubmit`，作为 turn 开头或 prompt 附近的 Hook row。
- 多条 Hook execution 可以折叠成一行摘要：“3 个 hooks 运行，1 个改写输入，1 个注入上下文”。

不建议第一版把每一条 completed Hook 都塞进 timeline，因为会降低主工作流可读性。

### Tool card 集成

如果现有 timeline/tool card 有工具调用详情：

- 增加 Hook badges：
  - `blocked`
  - `rewritten`
  - `context`
  - `failed`
- 点击 badge 打开 Hook executions filtered view 或 detail drawer。
- tool result 的 Hook metadata 只作为关联线索，最终展示仍以 `RuntimeHookExecution` 为准。

### Prompt blocked 集成

当前 `RuntimeNormalizedInput.hookOutcome` 已能把 blocked prompt 映射为 assistant error message。

补齐方向：

- 如果 hook outcome metadata 中存在 execution id，错误消息增加“查看 Hook”动作。
- 如果当前 metadata 没有 execution id，需要在后端或 adapter 层补 `hookExecutionId`，但不要通过 reason 文本反查 execution。

### 阶段 4 验收

- Hook block / failure / rewrite / context injection 能在主 timeline 或 callchain 中看到。
- 用户可以从 timeline/callchain 进入 Hook execution 详情。
- 普通成功且无影响的 Hook 不会刷屏。
- Timeline 不依赖字符串解析推断 Hook 状态。

## 阶段 5：测试、契约加固和回归场景

### Go 测试

建议先跑聚焦测试：

```powershell
go test ./internal/runtime -run "Hook|ReactCallchain|Input"
go test ./desktop -run Hook
```

再跑全量：

```powershell
go test ./...
```

如果阶段 0 发现 bridge 或 contract 需要补测试：

- `desktop/runtime_bridge_test.go`：验证 `Hooks`、`HookExecutions`、`HookExecution` 转发。
- `internal/runtime/runtime_hooks_test.go`：验证 filter、status、detail 查询。
- `internal/runtime/runtime_input_test.go`：验证 `UserPromptSubmit` block/rewrite 的 hook outcome 仍稳定。
- `internal/runtime/runtime_react_callchain_test.go`：验证 callchain hook node 带 execution id 或 hook count。

### 前端测试和构建

必须跑：

```powershell
cd client
npm run build
```

如果项目已有 test/lint 命令，则补跑：

```powershell
cd client
npm run lint
npm test
```

如无 test/lint 命令，不新增大规模测试框架；先保证 build 和类型检查通过。

### Adapter 单元测试建议

如果当前 runtime adapter 已有测试文件，补充：

- `mapHook` 兼容 camelCase / snake_case。
- `mapHookExecution` 正确处理 missing optional fields。
- `summarizeHookExecutions` 统计 blocked、failed、rewritten、context injected。
- `redacted` 为 true 时 view model 保留 flag。

### 场景验收

至少覆盖以下手工场景：

1. 未配置 Hook：
   - Settings Hooks 显示空状态。
   - Diagnostics 不报错。

2. 配置 `PreToolUse` allow Hook：
   - Settings 能看到 Hook。
   - 执行工具后 Hook execution 出现在 diagnostics。
   - timeline 不刷屏，除非有 rewrite/context。

3. 配置 `PreToolUse` deny Hook：
   - 工具调用被阻断。
   - timeline 或 tool card 显示 blocked。
   - detail drawer 显示 reason。

4. 配置 rewrite input Hook：
   - 工具输入被改写。
   - Hook execution 显示 `inputRewritten`。
   - callchain/timeline 有 rewritten tag。

5. 配置 context injected Hook：
   - Hook execution 显示 `contextInjected`。
   - detail drawer 显示 context summary 或脱敏提示。

6. 配置 `UserPromptSubmit` block：
   - prompt 被阻断。
   - assistant error message 出现。
   - 如果有 execution id，可从消息进入 Hook 详情。

7. Hook 命令失败或超时：
   - execution status 显示 failed 或对应状态。
   - 主流程是否继续遵循当前 runtime 语义。
   - UI 能显示 error 摘要，不吞错。

8. 权限与 sandbox 边界：
   - Hook allow 不应绕过 deterministic deny、headless fail-closed、sandbox、scope 等后续边界。
   - UI 文案不能暗示 Hook allow 等于最终权限允许。

### 阶段 5 验收

- `go test ./...` 通过，或明确记录与本次无关的既有失败。
- `cd client && npm run build` 通过。
- adapter mapper 对缺失字段、脱敏、snake_case/camelCase 有兜底。
- Hook block/rewrite/context/failure 的主要用户路径完成手工验证。

## 阶段 6：文档修正和后续扩展规划

### 文档修正

文件：

- `docs/hooks/README.md`
- `docs/frontend-runtime-integration-notes.md`，如 Hook adapter 接入方式需要补充。
- `docs/organize/07-hooks-comparison.md`，如实现过程中发现事实变化。

需要修正的内容：

- `docs/hooks/README.md` 不应继续写“当前只支持 PreToolUse”这类与代码事实不一致的描述。
- 文档应区分：
  - 已有完整执行链的事件。
  - 已定义但需复核触发链的事件。
  - 未来扩展事件。
- 文档应说明 Hook execution 是前端展示的事实源。
- 文档应说明 Hook allow 与 permission/sandbox 的边界关系。

### 后续扩展

第一版完成后再评估：

- Hook 配置编辑 UI。
- Hook command 测试运行。
- `Stop` / `PreCompact` / `PostCompact` / `PostSampling` 的完整触发链补齐。
- `PermissionRequest` / `PermissionDenied` 事件。
- session start/end、subagent start/stop、file changed、worktree 等事件。
- Hook progress message。
- Hook attachment/message 展示。
- 非 command Hook，例如 callback/http/plugin Hook。
- 多 Hook 聚合执行中的单命令级 duration 和输出详情。

## 推荐 PR 拆分

### PR 1：Adapter 类型与 hydration

范围：

- `workbenchTypes.ts`
- `wailsWorkbenchAdapter.ts`
- `staticWorkbenchAdapter.tsx`
- 必要 bridge/contract 测试。

验收：

- `WorkbenchViewModel` 出现 `hooks` 和 `hookExecutions`。
- 前端 build 通过。

### PR 2：Settings Hooks 只读页

范围：

- Settings navigation。
- `HookSettingsPanel`。
- CSS Module。

验收：

- 用户能查看 Hook 配置。
- 无 Hook 和 API 不可用状态清晰。

### PR 3：Hook executions 诊断面板

范围：

- `HookExecutionsPanel`。
- `HookExecutionDetailDrawer`。
- adapter detail 方法。
- diagnostics 集成。

验收：

- 用户能查看、过滤、打开 Hook execution 详情。

### PR 4：Timeline / callchain 集成

范围：

- `ReactCallchainInspector`。
- `Timeline` / tool card。
- prompt blocked 查看 Hook 入口。

验收：

- Hook block/rewrite/context/failure 出现在主工作流。

### PR 5：测试和文档加固

范围：

- Go focused tests。
- Adapter mapper tests。
- `docs/hooks/README.md` 修正。
- organize 文档同步。

验收：

- 全量或记录过的测试命令完成。
- 文档与代码事实一致。

### PR 6：事件面扩展

范围：

- `Stop` / compact / sampling 等事件的触发链。
- 未来事件和配置编辑能力。

验收：

- 每个新增事件都有 runtime 触发、execution 记录、frontend 展示和测试。

## 总体验收标准

完成本方案的第一轮后，应满足：

- 用户能在 Settings 中看到当前配置了哪些 Hook。
- 用户能看到当前 session 的 Hook execution 历史。
- Hook block、failure、input rewrite、context injected、halt 能在 diagnostics 和主流程中被定位。
- 用户能从 timeline/callchain/tool/prompt blocked 入口进入 Hook execution 详情。
- Hook 前端展示以 `RuntimeHookExecution` 为事实源，不解析工具文本。
- Hook runtime event 只触发刷新，不成为 React 临时事实源。
- 前端组件不直接调用 Wails binding、`fetch`、`XMLHttpRequest` 或 axios。
- UI 使用 Ant Design、theme tokens 和 scoped CSS Modules。
- Go 和 frontend build/test 在可控范围内通过。

## 风险和注意点

- 当前 Hook runner 会并行执行多个 Hook，但按配置顺序聚合结果；UI 文案不能暗示外部副作用也按配置顺序发生。
- 当前 execution 可能是聚合记录，也可能未来拆成单命令级记录；第一版 UI 应避免过度承诺“每个命令一条详情”。
- `PreCompact`、`PostCompact`、`PostSampling`、`Stop` 已在事件面出现，但触发链需要以代码复核为准。
- Hook progress message 目前不是稳定 runtime fact，第一版不要伪造运行中进度。
- 脱敏字段必须尊重后端 `redacted` 标记，前端不能从其他字段拼回敏感内容。
- Hook allow 与 permission/sandbox 不是同一层决策，UI 需要明确“Hook 允许”不等于“最终权限允许”。
- 列表接口如果没有分页，前端需要限制当前 session 查询和渲染数量，避免长 session 卡顿。
- command preview 需要截断，完整命令展示应等后端提供明确脱敏策略后再做。

## 第一轮建议落地范围

如果希望控制风险，第一轮只做以下内容：

1. 接入 `hooks` 和 `hookExecutions` view model。
2. Settings 增加 Hooks 只读页。
3. Diagnostics 增加 Hook executions 列表和详情抽屉。
4. Callchain hook node 增加 status/event/duration/reason 展示。
5. Timeline 只展示 blocked、failed、rewritten、context injected 四类高信号 Hook。
6. 修正 `docs/hooks/README.md` 中与当前代码事实不一致的描述。

这能覆盖用户最关心的问题：Hook 机制本身已有较完整后端基础，但前端缺少“配置看得见、执行查得到、影响能定位”的产品化展示。
