# Todo 与 Subagent 完整实施方案

本文是 `05-todo-subagent-comparison.md` 之后的实施蓝图，覆盖三部分工作：

1. Todo 补齐前端展示。
2. Subagent 先将已有 AgentTask 能力展示和入口化。
3. 对齐 Claude 系列，扩展 subagent 的 AgentTool。

方案目标不是一次性重写 runtime，而是在现有 Agent Builder 架构边界内有序补齐：

- Go runtime 仍是事实来源。
- React 只消费 `WorkbenchViewModel` / adapter view model。
- Wails、HTTP/dev transport、runtime API contract、frontend adapter 必须同步。
- Runtime events 只作为刷新触发，不作为 React 事实状态。
- UI 使用 Ant Design / Ant Design icons / theme tokens / scoped CSS Modules。

## 总体执行顺序

推荐顺序：

```text
阶段 0：契约和基线核对
  -> 阶段 1：Todo 只读展示闭环
  -> 阶段 2：AgentTask 展示和入口化
  -> 阶段 3：AgentTool schema 扩展
  -> 后续：Todo 可编辑、plan sync、background/fork/team 等增强
```

原因：

- Todo 后端已有 `SessionTodos` / `TurnTodos`，前端缺口最大，先做收益最高。
- AgentTask 后端和 adapter 已接好，先做 UI 产品化，风险低于扩 runtime 行为。
- AgentTool 扩展会影响模型可见工具 schema、agent role 解析、scope、权限和 UI 展示，应放在已有展示能力稳定之后。

## 阶段 0：契约和基线核对

### 目标

在开始实现前，把已有 runtime 能力、缺失 bridge、frontend 接入点确认清楚，避免 UI 直接绕过 adapter。

### 需要确认的事实

Todo 当前事实：

- 后端已有：
  - `internal/agent/tools/todos.go`
  - `internal/runtime/runtime_todos.go`
  - `internal/runtime/runtime_contract_types.go`
  - `internal/runtime/runtime_http.go`
  - `internal/runtimeapi/contract.go`
- HTTP 已有：
  - `GET /v1/sessions/{session_id}/todos`
  - `GET /v1/turns/{turn_id}/todos`
- runtime event 已有：
  - `todo.updated`
- Wails bridge 目前有 `RuntimeTodosResponse` 类型别名，但需要补实际方法：
  - `SessionTodos(ctx, sessionID)`
  - `TurnTodos(ctx, turnID)`
- 前端目前没有：
  - `TodoViewModel`
  - `TodoSummaryViewModel`
  - `RuntimeTodoDTO`
  - `hydrateTodos`
  - Todo UI component

AgentTask 当前事实：

- 后端已有：
  - `internal/agent/agent_tool.go`
  - `internal/agent/task_tools.go`
  - `internal/runtime/runtime_agent_tasks.go`
  - `internal/runtime/runtime_agent_task_store.go`
  - `internal/runtime/runtime_agent_task_comm_store.go`
  - `internal/runtime/runtime_agent_task_tools.go`
- Wails bridge 已有：
  - `AgentTask`
  - `SessionAgentTasks`
  - `TurnAgentTasks`
  - `CancelAgentTask`
  - `AgentTaskFollowUp`
  - `AgentTaskOutput`
  - `AgentRoles`
- 前端已有：
  - `AgentTaskViewModel`
  - `hydrateAgentTasks`
  - `AgentTaskPanel`
  - timeline `agent_task`
- 前端不足：
  - `AgentTaskPanel` 放在 review/diagnostics 语义里。
  - timeline row 不能作为清晰的打开详情入口。
  - agent tool card 没有充分展示 child task 状态。
  - 没有显性 AgentTask 工作区入口。

### 阶段 0 输出

- 不改业务逻辑。
- 确认后续需要动的文件列表。
- 如果发现 contract 漂移，先补测试或 TODO 记录。

### 验收

- 明确 Todo bridge 缺口。
- 明确 AgentTask UI 入口方案。
- 明确 AgentTool schema 扩展分批策略。

## 阶段 1：Todo 补齐前端展示

阶段 1 只要求展示闭环，不先做用户手动改 Todo。原因是当前 Todo item 没有稳定 `id`，直接做可编辑列表会产生 key、diff、并发写回和审计问题。

### 阶段 1A：补 Wails bridge 和 frontend DTO

#### 目标

让 `SessionTodos` 进入 `WorkbenchViewModel`，完成 runtime -> adapter -> React 的只读数据链路。

#### 后端改动

文件：

- `desktop/runtime_bridge.go`
- `desktop/runtime_bridge_test.go`

新增 bridge 方法：

```go
func (r *RuntimeBridge) SessionTodos(ctx context.Context, sessionID string) (RuntimeTodosResponse, error) {
	return r.service.SessionTodos(ctx, sessionID)
}

func (r *RuntimeBridge) TurnTodos(ctx context.Context, turnID string) (RuntimeTodosResponse, error) {
	return r.service.TurnTodos(ctx, turnID)
}
```

测试要求：

- bridge 调用 `SessionTodos("session-1")` 能转发到 service。
- bridge 调用 `TurnTodos("turn-1")` 能转发到 service。

#### 前端 runtime 类型改动

文件：

- `client/src/runtime/workbenchTypes.ts`

新增 view model：

```ts
export type TodoStatusViewModel = 'pending' | 'in_progress' | 'completed' | string;

export interface TodoItemViewModel {
  id: string;
  content: string;
  status: TodoStatusViewModel;
  activeForm?: string;
  createdAt?: number;
  updatedAt?: number;
  source?: {
    kind: string;
    label?: string;
    ref?: string;
  };
}

export interface TodoSummaryViewModel {
  sessionId: string;
  turnId?: string;
  items: TodoItemViewModel[];
  pending: number;
  inProgress: number;
  completed: number;
  total: number;
  updatedAt?: number;
}
```

在 `WorkbenchViewModel` 增加：

```ts
todos?: TodoSummaryViewModel;
```

兼容策略：

- 当前 runtime DTO 没有 `id`，mapper 先生成稳定 display id：
  - 优先使用后续 DTO 的 `id`。
  - 否则使用 `${index + 1}` 作为展示 id。
  - 不把这个临时 id 写回 runtime。
- 当前 runtime DTO 是 `todos`，前端 view model 使用 `items`，避免和 summary 名称冲突。

#### 前端 adapter 改动

文件：

- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/runtime/staticWorkbenchAdapter.tsx`

新增 DTO：

```ts
interface RuntimeTodoDTO {
  id?: string;
  content?: string;
  status?: string;
  activeForm?: string;
  active_form?: string;
  createdAt?: number;
  updatedAt?: number;
  source?: {
    kind?: string;
    label?: string;
    ref?: string;
  };
}

interface RuntimeTodoSummaryDTO {
  sessionId?: string;
  turnId?: string;
  todos?: RuntimeTodoDTO[];
  pending?: number;
  inProgress?: number;
  completed?: number;
  total?: number;
  updatedAt?: number;
}

interface RuntimeTodosResponseDTO {
  summary?: RuntimeTodoSummaryDTO;
}
```

在 `RuntimeBridgeModule` 增加：

```ts
SessionTodos?: (sessionID: string) => Promise<RuntimeTodosResponseDTO>;
TurnTodos?: (turnID: string) => Promise<RuntimeTodosResponseDTO>;
```

新增函数：

- `hydrateTodos(bridge, sessionID)`
- `mapTodoSummary(summary)`
- `mapTodoItem(todo, index)`

在 `hydrateWorkbench` 中：

- 有 active session 且 `refreshActivity` 为 true 时 hydrate todos。
- full hydration 时 hydrate todos。
- optional failure 不得清空其他已成功 hydrate 的数据。

`staticWorkbenchAdapter` 初始 view model 增加：

```ts
todos: undefined
```

#### 事件刷新策略

文件：

- `client/src/app/shell/WorkbenchShell.tsx`
- `client/src/runtime/wailsWorkbenchAdapter.ts`

现状是 runtime event 触发 adapter refresh。Todo 第一阶段可以先复用完整 refresh，不引入细粒度 query。

要求：

- 收到 `todo.updated` 后会触发 refresh。
- refresh 后 `viewModel.todos` 来自 `SessionTodos`。
- 不从 event payload 直接构造 Todo UI 状态。

后续可优化：

- 增加 refresh target `todos`。
- `hydrateWorkbench` 在 `refreshTargets` 包含 `todos` 时只窄刷新 todos。

### 阶段 1B：实现 Todo 可见 UI

#### 目标

在主工作流中展示 Todo，而不是只埋在 tool call 中。

#### 推荐 UI 组成

新增文件：

- `client/src/features/todos/TodoTaskBar.tsx`
- `client/src/features/todos/TodoTaskBar.module.css`
- `client/src/features/todos/TodoPanel.tsx`
- `client/src/features/todos/TodoPanel.module.css`

`TodoTaskBar` 定位：

- 放在 conversation timeline 与 composer 之间，或 timeline 顶部靠近当前会话 header 的稳定区域。
- 只在 `viewModel.todos?.items.length > 0` 时展示。
- 紧凑显示：
  - 当前 in_progress 的 `activeForm || content`
  - completed/total
  - Progress
  - 展开/收起按钮
- 展开后显示任务列表：
  - pending：空心图标/灰色
  - in_progress：loading/processing 图标，突出 activeForm
  - completed：check 图标，弱化/删除线
- 已全部完成时：
  - 显示 100%。
  - 可折叠，但第一阶段不做 dismiss 持久化。

`TodoPanel` 定位：

- 放在右侧 inspector 的一个独立 tab，建议新增 `tasks` tab，避免继续塞进 `review`。
- 展示：
  - pending / in_progress / completed 三个统计。
  - 列表。
  - updatedAt。
  - 空态。
- 第一阶段只读，不提供状态切换按钮。

#### Workspace 接入

文件：

- `client/src/features/workspace/Workspace.tsx`
- `client/src/features/workspace/Workspace.module.css`

改动：

- 新增右侧面板类型，例如：

```ts
type RightPanelKind = 'terminal' | 'files' | 'review' | 'tasks';
```

- `RightPanelAddMenu` 增加 “任务” 入口。
- 如果存在 todos 或 agentTasks，右侧 launcher 显示任务入口。
- 在主 conversation 区加入 `TodoTaskBar`。
- 在 `tasks` panel 中展示：
  - `TodoPanel`
  - 后续阶段的 `AgentTaskPanel`

注意：

- 不要把 TodoPanel 做成嵌套卡片。
- 文本要短，避免解释性说明。
- 使用 Ant Design `Progress`、`Tag`、`Button`、`Tooltip`、`Empty`。
- 图标使用 `@ant-design/icons`，如 `CheckCircleOutlined`、`ClockCircleOutlined`、`LoadingOutlined`、`UnorderedListOutlined`。

#### Timeline 策略

文件：

- `client/src/features/timeline/Timeline.tsx`

第一阶段不强制新增 timeline item。保留 tool call card。Todo 的主展示在 `TodoTaskBar` 和 `TodoPanel`。

如果后续需要 timeline 摘要，可以新增 `todo_summary` item，但必须由 runtime summary 生成，不从 assistant 文本推断。

### 阶段 1C：Todo 数据模型增强

这一阶段建议在只读展示稳定后做。如果后续需要手动切状态、clear、plan sync，就必须做。

#### 目标

给 Todo item 增加稳定身份和来源，支持 UI key、手动更新、plan sync 和恢复。

#### 后端模型改动

文件：

- `internal/session/session.go`
- `internal/agent/tools/todos.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_todos.go`
- `internal/db/migrations/...`

扩展：

```go
type Todo struct {
	ID         string
	Content    string
	Status     TodoStatus
	ActiveForm string
	Source     *TodoSource
	CreatedAt  int64
	UpdatedAt  int64
}
```

建议 `Source` 初始支持：

```go
type TodoSource struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
	Label string `json:"label,omitempty"`
}
```

兼容旧数据：

- 旧 JSON 缺字段时正常解析。
- 保存时补齐：
  - `ID`：优先保留旧值，否则用稳定 hash 或新 UUID。
  - `CreatedAt`：旧值缺失时使用 session updatedAt 或当前时间。
  - `UpdatedAt`：状态/content/activeForm 变化时更新。
- 不改变旧 session 的语义。

工具层约束：

- 最多一个 `in_progress`。
- 如果模型传多个，则：
  - 推荐返回错误，让模型修正。
  - 或只保留第一个，其他降为 `pending`。选择前需要确认产品预期。
- `content` 不能为空。
- `active_form` 对 `in_progress` 推荐必填；第一版可 warning，后续可强校验。

#### Runtime API 增强

新增写 API 需要谨慎。建议增加 session 级 action，而不是让前端直接调用工具。

新增接口：

- `UpdateSessionTodos(ctx, sessionID, req)`
- HTTP: `PUT /v1/sessions/{session_id}/todos`
- Wails: `UpdateSessionTodos`

请求结构：

```go
type RuntimeTodoUpdateRequest struct {
	Todos []RuntimeTodo `json:"todos"`
	Reason string `json:"reason,omitempty"`
}
```

返回：

```go
type RuntimeTodosResponse struct {
	Summary RuntimeTodoSummary `json:"summary"`
	Action *RuntimeWriteActionMetadata `json:"action,omitempty"`
}
```

Action metadata：

- `Kind`: `todo`
- `Action`: `update_todos`
- `IdempotentBy`: `session_id`
- `RefreshTargets`: `["todos", "session_activity"]`
- evidence 包含 `sessions.todos`、`runtime_events`、`audit`

事件：

- 继续使用 `todo.updated`。
- payload 增加 summary 和 changed ids。

#### 前端可编辑能力

在 `TodoPanel` 增加：

- mark completed/pending
- set in_progress
- clear completed 或 clear all

约束：

- 所有操作通过 adapter mutation。
- 乐观更新可后置；第一版点击后 loading，等待 runtime 返回。
- 操作失败显示 message/error，不改本地事实状态。

### 阶段 1D：Todo prompt 增强

文件：

- `internal/agent/tools/todos.md`
- `internal/agent/tools/todos_test.go`

目标：

- 对齐 Claude 系 TodoWrite 的行为约束，但不要复制原文案。
- 强调：
  - 复杂任务、多步骤任务、用户明确要求时使用。
  - 单步简单任务不使用。
  - 开始前标记 in_progress。
  - 完成后立即标记 completed。
  - 最多一个 in_progress。
  - 不把失败、未验证、部分完成标为 completed。
  - `content` 用待办命令式，`active_form` 用正在进行式。

验收：

- prompt 中包含关键规则。
- 工具 schema 与 prompt 字段命名一致。
- 测试覆盖 status 校验和多 in_progress 策略。

## 阶段 2：Subagent / AgentTask 展示和入口化

阶段 2 不先扩 `agent` 工具能力，只把已有 runtime-owned AgentTask 变成用户可见、可操作、可恢复的主工作流能力。

### 阶段 2A：从 diagnostics 中抽出 AgentTask UI

#### 目标

`AgentTaskPanel` 不再只是 review/diagnostics 子面板，而是任务工作区的一等组件。

#### 文件调整

建议新增或移动：

- `client/src/features/agentTasks/AgentTaskPanel.tsx`
- `client/src/features/agentTasks/AgentTaskPanel.module.css`
- `client/src/features/agentTasks/AgentTaskList.tsx`
- `client/src/features/agentTasks/AgentTaskDetail.tsx`
- `client/src/features/agentTasks/agentTaskUtils.ts`

保留兼容：

- `features/diagnostics/AgentTaskPanel.tsx` 可以临时 re-export，避免一次性改完所有 import。

组件拆分：

- `AgentTaskPanel`
  - 布局容器。
  - 接收 tasks、selectedTaskId、callbacks。
- `AgentTaskList`
  - active tasks 在前。
  - final tasks 在后。
  - 显示 title/status/role/progress。
- `AgentTaskDetail`
  - title、promptSummary、resultSummary。
  - status/progress。
  - scope 信息：role/model/provider/cwd/worktree/allowedTools/capabilityScope。
  - refs：output/artifact/compact。
  - messages。
  - follow-up / cancel 操作。

### 阶段 2B：右侧任务面板入口

#### 目标

用户能主动打开 “任务” 面板查看 Todo 和 AgentTask。

#### Workspace 改动

文件：

- `client/src/features/workspace/Workspace.tsx`
- `client/src/features/workspace/Workspace.module.css`

新增右侧 tab：

```ts
type RightPanelKind = 'terminal' | 'files' | 'review' | 'tasks';
```

`tasks` tab 内容：

- 上半部分：TodoPanel。
- 下半部分：AgentTaskPanel。
- 如果只有一种数据，只显示对应组件。
- 如果都为空，显示简洁空态。

右侧 launcher：

- 有 todos 或 agentTasks 时优先显示“任务”。
- `RightPanelAddMenu` 增加任务入口。

状态：

- `selectedAgentTaskID` 由 Workspace 持有。
- 点击 timeline row 时设置 selected 并打开 tasks panel。
- 选中项在 list 中高亮。

### 阶段 2C：timeline row 入口化

#### 目标

timeline 中的 AgentTask 行不只是状态文本，而是可点击的任务入口。

#### 文件

- `client/src/features/timeline/Timeline.tsx`
- `client/src/features/timeline/Timeline.module.css`
- `client/src/runtime/workbenchTypes.ts`

改动：

- `Timeline` 增加 prop：

```ts
onAgentTaskOpen?: (taskID: string) => void;
```

- `AgentTaskTimelineRow`：
  - 使用 Button 或 clickable row。
  - 展示：
    - title
    - status Tag
    - role/kind
    - progress
    - resultSummary 截断
  - 点击触发 `onAgentTaskOpen(task.id)`。

状态颜色：

- queued/running: processing
- completed: success
- failed/interrupted: error
- cancelled: default

验收：

- 任务运行时 timeline 有可识别行。
- 点击行打开右侧 tasks 面板并选中对应任务。

### 阶段 2D：agent tool card 增强

#### 目标

当模型调用 `agent` 工具时，用户能在普通 tool call card 中看懂“委派了什么、对应哪个 AgentTask、当前状态如何”。

#### 后端可选增强

文件：

- `internal/runtime/runtime_tool_calls.go`
- `internal/runtime/runtime_agent_tasks.go`

如果 tool call display metadata 已能根据 tool name 分类，补充：

- `agent` 工具 display kind 为 `agent_task` 或 `subagent`。
- display title 优先来自：
  - AgentTask title。
  - tool input `description`，如果阶段 3 已有。
  - prompt summary。

#### 前端改动

文件：

- `client/src/features/timeline/Timeline.tsx`
- `client/src/runtime/wailsWorkbenchAdapter.ts`

策略：

- 在 `attachAgentTasksToTimeline` 时，将 `parentToolCallId` 与 tool call 关联。
- tool call card 显示 “子代理任务” 标识和 task status。
- 如果 task 已有 resultSummary，显示 compact result。
- 点击打开 task detail。

注意：

- 不从 tool input 文本正则推断事实状态。
- 只用 `AgentTaskViewModel.parentToolCallId` 关联。

### 阶段 2E：操作能力产品化

已有 callbacks：

- `sendAgentTaskFollowUp`
- `cancelAgentTask`

需要补的 UX：

- follow-up 成功后清空输入并 refresh。
- cancel 成功后状态立即显示 cancelled。
- final task 禁用 follow-up/cancel。
- output refs/artifact refs 可复制。
- childSessionID 可跳转到对应 session，若 runtime 支持 select session。

可新增 adapter action：

- `openAgentTaskChildSession(taskID)` 不一定需要新 API，前端可用 `selectSession(childSessionID)`。
- 如果 child session 不在 session list，refresh 后再 select。

验收：

- 用户可以从任务面板取消 active task。
- 用户可以向 active task 发送 follow-up。
- 用户可以复制 refs。
- final task 操作不可用且提示清楚。

### 阶段 2F：Agent roles 展示

已有后端：

- `AgentRoles`
- `AgentRole`

前端建议：

- 在 `WorkbenchViewModel` 增加 `agentRoles?: AgentRoleViewModel[]`。
- adapter hydrate roles。
- 在 AgentTask detail 中显示 role title/description，而不是只显示 role id。

这一步为阶段 3 的 `subagent_type` / `role` 选择打基础。

## 阶段 3：对齐 Claude 系列，扩展 subagent AgentTool

阶段 3 的原则是小步扩展 `agent` 工具，不一次性引入 team/fork/remote/swarm。

### 阶段 3A：扩展 AgentTool schema 第一批

#### 目标

让模型可以表达子代理意图、角色、模型和 cwd，但仍使用 Agent Builder 现有 AgentTask runtime。

#### 输入字段

文件：

- `internal/agent/agent_tool.go`
- `internal/agent/templates/agent_tool.md`

扩展 `AgentParams`：

```go
type AgentParams struct {
	Description  string `json:"description,omitempty" description:"Short title for the delegated task"`
	Prompt       string `json:"prompt" description:"The task for the agent to perform"`
	SubagentType string `json:"subagent_type,omitempty" description:"Specialized agent role to use"`
	Role         string `json:"role,omitempty" description:"Agent Builder role id; alias of subagent_type"`
	Model        string `json:"model,omitempty" description:"Optional model override"`
	CWD          string `json:"cwd,omitempty" description:"Optional working directory for the child agent"`
}
```

字段策略：

- `prompt` 必填。
- `description` 用于 task title 和 UI summary。
- `subagent_type` 与 `role` 二选一；内部规范化为 role id。
- 第一版不加入 `run_in_background`，避免行为语义不清。
- 第一版不加入 `isolation`、`team_name`、`name`、`mode`。

#### Role 解析

当前 `agentTool` 默认使用 `config.AgentTask`。

扩展策略：

1. `role := firstNonEmpty(params.Role, params.SubagentType, config.AgentTask)`
2. 从 `c.cfg.Config().Agents[role]` 查找 agent config。
3. 如果不存在：
   - 返回 tool error：unknown subagent role。
   - 错误中列出可用 role ids。
4. 使用该 role 的 allowed tools / model / provider / prompt。

#### Model override

第一版建议只接受项目已有配置中合法的模型别名或 provider model。

实现选择：

- 简化策略：只把 `Model` 写入 `AgentTaskRecord`，不真正 override provider execution。优点是风险低，缺点是模型以为生效但未生效，不推荐。
- 推荐策略：在 `buildAgent` 前复制 agent config，并覆盖 model 字段。若 config 结构支持 provider/model，按现有模型配置语义覆盖。

如果短期不确定 config 结构，先不开放 `model` 给 schema，只在方案中留到 3B。

#### CWD override

要求：

- 必须是绝对路径或 workspace 内相对路径规范化结果。
- 不允许越过 workspace/project scope，除非 policy 明确允许。
- 必须写入 AgentTask scope，用于 runtime scope enforcement。

第一版可只支持 workspace 内 cwd。

#### Task record 映射

`runSubAgent` 的 `subAgentParams` 应增加或使用已有字段：

- `SessionTitle`: `description || "New Agent Session"`
- `Kind`: `"subagent"`
- `Role`: resolved role
- `Name`: `description || AgentToolName`
- `Prompt`: prompt
- `AllowedTools`: role allowed tools
- `CWD`: normalized cwd
- `Model`: selected model

### 阶段 3B：AgentTool prompt 对齐

#### 目标

让模型知道什么时候使用子代理、有哪些 role、每个 role 有什么工具和边界。

#### 文件

- `internal/agent/templates/agent_tool.md`
- `internal/runtime/runtime_agent_roles.go`
- 可能涉及 prompt assembly 相关测试。

Prompt 内容建议：

- 何时使用 subagent：
  - 大范围搜索。
  - 可并行调查。
  - 独立验证。
  - 需要隔离上下文的分析。
- 何时不要使用：
  - 单个小修改。
  - 需要用户即时确认的步骤。
  - 已经有足够上下文能直接完成。
- 输入字段说明：
  - `description`
  - `prompt`
  - `subagent_type`
  - `model`
  - `cwd`
- role 列表：
  - 从 runtime/config agent roles 生成，或在 description 中静态说明 default role。

注意：

- 不复制 Claude 文案。
- 用 Agent Builder 自己的 role 名称和能力边界。

### 阶段 3C：前端展示新 AgentTool 字段

#### 目标

模型使用扩展字段后，UI 能展示这些字段。

#### 文件

- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/features/timeline/Timeline.tsx`
- `client/src/features/agentTasks/AgentTaskDetail.tsx`

展示：

- `description` -> task title / tool card title。
- `subagent_type` / `role` -> role tag。
- `model` -> provider/model row。
- `cwd` -> scope row。

如果 `AgentTaskViewModel` 已有字段，优先显示 runtime AgentTask，而不是 raw tool input。

### 阶段 3D：run_in_background 第二批

`run_in_background` 不建议和 3A 同时做。它不是简单字段，而是生命周期语义。

#### 目标

允许模型启动后台子代理并立即返回 task id，同时 UI 持续展示任务状态。

#### 设计问题

必须先明确：

- 当前 `fantasy.NewParallelAgentTool` 是否仍等待子 agent 完成。
- `runSubAgent` 是否能创建 task 后 detach。
- 子 agent goroutine 的 cancellation、panic、provider error 是否都能写回 AgentTask。
- parent turn 如何收到 background task result。
- background result 是否需要插入后续 assistant message，还是只进入 task panel。

#### 后端方案

新增输入：

```go
RunInBackground bool `json:"run_in_background,omitempty"`
```

行为：

- false：保持现有同步/前台语义。
- true：
  - 创建 AgentTask。
  - 启动 worker goroutine。
  - 立即返回 tool result：

```json
{
  "status": "background_started",
  "task_id": "...",
  "description": "...",
  "output_refs": []
}
```

要求：

- worker 必须持有 runtime recorder。
- task started/progress/completed/failed/cancelled 都持久化。
- cancellation 通过 `CancelAgentTask` 生效。
- parent session activity 能看到 background task。

#### 前端方案

- Tool call card 显示 background started。
- AgentTaskPanel 显示 running。
- 任务完成后 runtime event 触发 refresh。
- 如果有 resultSummary，在 panel 和 timeline row 显示。

#### 验收

- 后台 agent 启动后 parent turn 不被阻塞到子任务完成。
- 刷新/重启后仍能看到 task。
- cancel 可以终止或至少 terminalize runtime task 状态。

### 阶段 3E：isolation / worktree 第三批

只在 3A/3D 稳定后做。

字段：

```go
Isolation string `json:"isolation,omitempty"`
```

第一版只支持：

- `worktree`

不支持：

- `remote`

要求：

- 复用 `runtime_worktrees.go` 能力。
- AgentTask 记录 `worktree`。
- effective scope 指向 worktree path。
- cleanup/preserve policy 明确。
- UI 展示 worktree 入口和清理状态。

### 阶段 3F：暂缓能力

以下能力不进入第一轮实现：

- `team_name`
- `name` addressable teammate
- fork path
- remote isolation
- swarm / coordinator team
- agent memory scope

这些能力需要独立设计：

- 多 agent 通信拓扑。
- teammate roster。
- session ownership。
- worktree/remote 生命周期。
- budget/cost。
- permission delegation。
- UI 信息架构。

## 横向测试计划

### Go 测试

建议逐阶段运行：

```powershell
go test ./internal/runtime ./internal/agent ./desktop
```

最终合并前：

```powershell
go test ./...
```

重点测试：

- `runtime_todos_test.go`
- `runtime_http_test.go`
- `runtime_service_test.go`
- `runtime_agent_task_store_test.go`
- `task_tools_test.go`
- `desktop/runtime_bridge_test.go`

新增测试：

- bridge SessionTodos / TurnTodos。
- Todo summary mapping 兼容旧字段。
- AgentTool role 解析。
- AgentTool unknown role error。
- 多 in_progress 策略。
- background task lifecycle，如果实现 3D。

### 前端测试

建议：

```powershell
cd client
npm run build
```

如果项目有测试命令，再运行对应单测。

新增测试或 smoke：

- `TodoTaskBar`：
  - 无 todos 不展示。
  - running todo 显示 activeForm。
  - completed/total 正确。
- `TodoPanel`：
  - 三类统计正确。
  - 空态正确。
- `AgentTaskPanel`：
  - active 排在 final 前。
  - final task 禁用 follow-up/cancel。
  - selected task 展示 detail。
- `Timeline`：
  - 点击 agent task row 触发回调。

### 浏览器/桌面验证

每个 UI 阶段至少验证：

- Vite/browser HTTP/dev fallback。
- Wails desktop bridge。
- runtime event 后刷新。
- session 切换后 Todo/AgentTask 不串 session。
- 新 chat 后不会保留旧 session Todo/AgentTask。

需要特别注意：

- in-app browser 不一定有 `fetch` / `XMLHttpRequest`。
- 不要在 React 组件里直接调用 Wails binding。
- 不要使用 frontend-only mock 数据作为业务默认值。

## 分阶段交付建议

### PR 1：Todo read-only runtime-to-UI

范围：

- Wails bridge `SessionTodos` / `TurnTodos`。
- frontend Todo DTO/view model/adapter hydration。
- `TodoTaskBar` 只读展示。
- 基础测试。

不包含：

- Todo 手动编辑。
- Todo 数据模型扩展。
- plan sync。

验收：

- 模型调用 `todos` 后，UI 自动展示任务进度。
- 刷新后仍能从 session 恢复 Todo。

### PR 2：TodoPanel 和右侧任务入口

范围：

- `tasks` right panel。
- `TodoPanel`。
- launcher/add menu 入口。
- 空态和统计。

验收：

- 用户能从右侧打开 Todo 面板。
- TodoTaskBar 与 TodoPanel 使用同一个 `viewModel.todos`。

### PR 3：AgentTask 主工作流入口

范围：

- 抽出 `features/agentTasks`。
- `tasks` panel 中展示 AgentTask。
- timeline row 点击打开任务详情。
- follow-up/cancel UX 完善。

验收：

- 已有 AgentTask 不再只藏在 diagnostics/review。
- 用户能从 timeline 到详情再执行 follow-up/cancel。

### PR 4：Agent tool card 与 roles 展示

范围：

- agent tool call 与 AgentTask 关联展示。
- hydrate/display `AgentRoles`。
- role title/description 展示。

验收：

- 用户能看懂每个 agent tool call 对应哪个子任务。
- 子任务详情显示 role/provider/model/scope。

### PR 5：AgentTool schema 第一批

范围：

- `description`
- `subagent_type` / `role`
- 可选 `model`
- 可选 `cwd`
- prompt 更新。
- 后端测试。

验收：

- 模型可以指定 role。
- unknown role 有清晰错误。
- UI 显示 description/role/cwd/model。
- 现有 `agent({"prompt": ...})` 兼容。

### PR 6：AgentTool background

范围：

- `run_in_background`
- detached lifecycle。
- tool result 返回 task id。
- UI 持续展示。
- cancel/recovery 测试。

验收：

- 后台任务不阻塞 parent turn。
- 刷新后 task 仍可见。
- 完成/失败/取消都能持久化。

### PR 7：Todo 可编辑和数据模型增强

范围：

- Todo item id/timestamps/source。
- `PUT /v1/sessions/{id}/todos`。
- frontend mutation。
- clear/status 操作。

验收：

- 用户能在 TodoPanel 修改状态。
- 所有修改经 runtime 持久化和事件刷新。
- 重启后状态可恢复。

## 风险与边界

### Todo 风险

- 没有稳定 id 时做可编辑 UI 会导致状态错配。
- 直接从 tool call input 抽取 Todo 会绕过 runtime 事实来源。
- `todo.updated` event payload 不应成为唯一状态来源。
- 多个 `in_progress` 需要后端兜底，否则 UI 会出现多个当前任务。

### AgentTask 风险

- 把 AgentTaskPanel 留在 diagnostics 会让能力不可发现。
- follow-up/cancel 若只改前端状态，会破坏 runtime recovery。
- child session 跳转要确认 session list hydration，否则可能跳转失败。

### AgentTool 风险

- 一次性加入 Claude 系所有字段会扩大行为面。
- `run_in_background` 是生命周期功能，不是普通 schema 字段。
- `cwd` 和 `isolation` 必须接 permission/scope/worktree。
- `model` override 必须真实生效或不暴露，不能只展示不执行。
- team/fork/remote/swarm 应独立设计，不进入本轮核心交付。

## 最终完成定义

三部分完成后，应满足：

- Todo：
  - 模型更新 Todo 后，主工作流可见。
  - 刷新/重启可恢复。
  - 用户能在右侧任务面板查看完整 Todo。
  - 后续可通过 runtime API 安全编辑。

- Subagent 展示：
  - AgentTask 在 timeline、右侧任务面板、tool card 中都有清晰入口。
  - 用户可以查看状态、scope、消息、结果、refs。
  - 用户可以 follow-up/cancel active task。

- AgentTool：
  - 保持旧 `prompt` 调用兼容。
  - 支持 description 和 role/subagent_type。
  - 逐步支持 model/cwd/background。
  - 所有新字段都进入 AgentTask runtime record 和 UI 展示。
  - 不引入未设计完成的 team/fork/remote/swarm 复杂度。
