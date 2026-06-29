# Todo Write 与 Subagent 梳理对比

本文基于 `docs/organize/` 现有项目梳理，继续聚焦两个能力面：

- Todo Write：模型侧结构化任务清单、runtime 持久化、事件、前端可见进度。
- Subagent：模型侧委派工具、runtime AgentTask、子任务生命周期、前端展示与操作。

对比参考项目：

- `C:\Users\ytq\work\ai\cc-haha`
- `C:\Users\ytq\work\ai\DeepSeek-GUI`
- `C:\Users\ytq\work\ai\myclaw\claude-code`

## 结论摘要

- Agent Builder 当前 Todo 后端链路已经存在，但前端产品化明显不足。`todos` / `todospan` 工具会保存到 session，并通过 runtime API 与 `todo.updated` 事件暴露；但 `client/src/runtime` 和 `client/src/features` 里没有对应 Todo view model、hydration、独立面板或 sticky task bar。
- Agent Builder 当前 Subagent 链路比 Todo 完整。后端有 `AgentTask` 存储、消息、结果、输出 refs、取消和 follow-up；前端已有 `AgentTaskViewModel`、timeline row 和 `AgentTaskPanel`。
- Agent Builder 的 `agent` 工具输入面偏窄，当前只有 `prompt`。参考项目的 Claude 系 `AgentTool` 支持 `description`、`subagent_type`、`model`、`run_in_background`、`name`、`team_name`、`mode`、`isolation`、`cwd` 等能力；DeepSeek-GUI/Kun 的 `delegate_task` 更克制，但有预算和并发约束。
- Todo 的优先补齐方向应是先建立 runtime-first 的 Todo UI 契约，再做展示。不要让 React 自己从 tool call 文本里临时推断成为事实来源。
- Subagent 的优先补齐方向应是把已有能力更显性地展示和入口化，而不是先扩成 Claude 系完整 AgentTool。Agent Builder 已有 runtime-owned AgentTask，应继续沿用该边界。

## Agent Builder 当前 Todo 链路

### 后端与 runtime

关键文件：

- `internal/agent/tools/todos.go`
- `internal/agent/tools/todos.md`
- `internal/session/session.go`
- `internal/runtime/runtime_todos.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtimeapi/contract.go`
- `internal/runtime/runtime_http.go`
- `desktop/runtime_bridge.go`
- `internal/db/migrations/20250812000000_add_todos_to_sessions.sql`

当前工具名不是 Claude Code 风格的 `TodoWrite`，而是：

- `todos`
- `todospan`

工具输入结构：

```go
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"active_form"`
}
```

session 持久化结构：

```go
type Todo struct {
	Content    string
	Status     TodoStatus
	ActiveForm string
}
```

runtime 暴露：

- `SessionTodos(ctx, sessionID)`
- `TurnTodos(ctx, turnID)`
- HTTP: `GET /v1/sessions/{session_id}/todos`
- HTTP: `GET /v1/turns/{turn_id}/todos`
- Runtime event: `todo.updated`

当前优点：

- Todo 是 Go runtime/session 状态，不是浏览器内存状态。
- 工具调用后会发布 `TodoUpdatedEvent`，runtime 会落 runtime event 和 audit。
- API 已有 session/turn 两种查询入口，具备恢复展示的基础。

当前限制：

- Todo item 没有稳定 `id`，前端列表 key、手动操作、diff、高亮、plan 同步都会受限。
- 没有 `createdAt` / `updatedAt`，难以做最近完成排序、动画、历史恢复和审计细节。
- 没有 `source`，无法表达来自 plan、用户输入、agent task 或外部导入。
- `todos.md` prompt 过短，只描述状态和使用场景，缺少 when/when-not-to-use、实时更新规则、完成条件、阻塞处理、active form 规范等模型行为约束。
- 只校验 status 合法性，未在工具层强制“最多一个 in_progress”。prompt 说 exactly one，但代码没有兜底。

### 前端现状

当前未发现独立 Todo UI 链路：

- `client/src/runtime/workbenchTypes.ts` 没有 Todo view model。
- `client/src/runtime/wailsWorkbenchAdapter.ts` 没有 hydrate todos / map todos / attach todo UI 的逻辑。
- `client/src/features` 没有 TodoPanel、TaskBar 或任务摘要组件。
- timeline 只能通过普通 tool call 间接看到 `todos` / `todospan` 调用。

这意味着 Todo 对模型和后端是结构化的，但对用户仍接近隐藏状态。对桌面客户端而言，这是当前 Todo 最大缺口。

## Agent Builder 当前 Subagent / AgentTask 链路

### 后端与 runtime

关键文件：

- `internal/agent/agent_tool.go`
- `internal/agent/task_tools.go`
- `internal/agent/coordinator.go`
- `internal/runtime/runtime_agent_tasks.go`
- `internal/runtime/runtime_agent_task_store.go`
- `internal/runtime/runtime_agent_task_comm_store.go`
- `internal/runtime/runtime_agent_task_runner.go`
- `internal/runtime/runtime_agent_task_tools.go`
- `internal/runtime/runtime_agent_task_scope.go`
- `internal/db/migrations/20260524010000_add_runtime_agent_tasks.sql`
- `internal/db/migrations/20260524011000_add_agent_task_roles_messages_results.sql`
- `internal/db/migrations/20260527000000_harden_agent_task_messages.sql`

模型可调用入口：

- `agent`
- 输入只有 `prompt`

运行时管理工具：

- `task_list`
- `task_get`
- `task_message`
- `task_stop`
- `task_output`

runtime API 能力：

- `AgentTask`
- `SessionAgentTasks`
- `TurnAgentTasks`
- `CancelAgentTask`
- `AgentTaskMessages`
- `CreateAgentTaskMessage`
- `SendAgentTaskFollowUp`
- `AgentTaskResult`
- `AgentTaskOutput`

AgentTask 状态：

- `queued`
- `running`
- `completed`
- `failed`
- `cancelled`
- `interrupted`

当前优点：

- AgentTask 是 runtime-owned 结构，持久化、事件、audit、replay 和 UI hydration 都有基础。
- 消息和结果分表保存，能表达 parent-to-child follow-up、child-to-parent progress/result、artifact refs、compact refs、output refs。
- `CancelAgentTask` 有明确 action metadata，标记 refresh targets 和证据来源。
- AgentTask scope 已接入工具/能力/cwd/worktree 约束，不只是简单启动子 session。

当前限制：

- `agent` 工具 schema 过窄，模型无法显式选择 role/type/model/background/isolation/cwd。
- 虽然 runtime 有 default agent roles，但前端入口没有把 role/type 选择做成主流程。
- Follow-up/cancel/output refs 目前偏诊断面板能力，不是主聊天工作流中的显性操作。
- 对比参考项目，尚未形成清晰的 background agent、fork agent、team/teammate、remote isolation 产品语义。

### 前端现状

关键文件：

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/app/shell/WorkbenchShell.tsx`
- `client/src/features/timeline/Timeline.tsx`
- `client/src/features/diagnostics/AgentTaskPanel.tsx`
- `client/src/features/diagnostics/AgentTaskPanel.module.css`

已有前端能力：

- `AgentTaskViewModel` 覆盖 task、message、result、refs、scope、status、progress。
- `wailsWorkbenchAdapter.ts` 会通过 `SessionAgentTasks` hydrate 当前 session 的任务，再拉 `AgentTask` 和 `AgentTaskOutput` 做详情映射。
- timeline 支持 `agent_task` 行。
- `AgentTaskPanel` 支持列表、详情、progress、refs、最近消息、follow-up、cancel。
- `WorkbenchShell` 已把 `sendAgentTaskFollowUp` 和 `cancelAgentTask` 接到 adapter。

当前前端限制：

- 面板位于 diagnostics 语义下，更像排查工具，不像用户主工作流。
- timeline row 与详情面板之间的操作入口需要继续产品化，例如点击 task row 打开详情、从 tool call card 展开 child session/result。
- 没有创建 subagent 的显性 UI 控件，主要还是依赖模型调用 `agent`。

## 参考项目对比

### `myclaw\claude-code`

Todo 关键文件：

- `src/tools/TodoWriteTool/TodoWriteTool.ts`
- `src/tools/TodoWriteTool/prompt.ts`
- `src/utils/todo/types.ts`
- `src/components/TaskListV2.tsx`

可借鉴点：

- `TodoWrite` prompt 很完整，明确 when-to-use、when-not-to-use、实时更新、完成条件、阻塞处理、只能一个 `in_progress`、`content` 与 `activeForm` 双形式。
- tool result 有 verification nudge：当一次性关闭 3 个以上任务且没有 verification 步骤时，提示生成验证 agent。
- `TaskListV2` 展示层考虑了排序、owner、blockedBy、teammate activity、终端高度截断等细节。

不宜照搬点：

- 它是 CLI/TUI 主导的交互模型，Agent Builder 的主产品是 React desktop，不能把 TUI 组件结构直接搬进前端。
- Todo 状态存在 AppState/session 或 agentId 维度，Agent Builder 应继续以 Go runtime/session 为事实来源。

Subagent 关键文件：

- `src/tools/AgentTool/AgentTool.tsx`
- `src/tools/AgentTool/prompt.ts`
- `src/tools/AgentTool/runAgent.ts`
- `src/tools/AgentTool/forkSubagent.ts`

可借鉴点：

- AgentTool schema 很完整：`description`、`prompt`、`subagent_type`、`model`、`run_in_background`、`name`、`team_name`、`mode`、`isolation`、`cwd`。
- 支持 fresh subagent 与 fork path 两种语义。
- 支持 background agent、worktree/remote isolation、team/teammate。
- prompt 会把 agent 类型、可用工具和使用边界提供给模型。

不宜照搬点：

- 功能面过宽，直接扩入 Agent Builder 会冲击现有 runtime-owned AgentTask、permission scope、worktree scope 和 UI contract。
- 产品设计只能作为信息架构参考，不应复制品牌、视觉、文案或完整交互。

### `DeepSeek-GUI`

Todo 后端关键文件：

- `kun/src/adapters/tool/todo-tools.ts`
- `kun/src/shared/todos.ts`
- `kun/src/services/thread-service.ts`
- `kun/src/contracts/threads.ts`
- `kun/src/contracts/events.ts`

Todo 前端关键文件：

- `src/renderer/src/components/todo/TodoPanel.tsx`
- `src/renderer/src/agent/types.ts`
- `src/renderer/src/agent/kun-runtime.ts`
- `src/renderer/src/agent/kun-mapper.ts`
- `src/renderer/src/store/chat-store-runtime.ts`
- `src/renderer/src/store/chat-store-maintenance-actions.ts`

可借鉴点：

- 工具有 `todo_list` 和 `todo_write`，既能读取也能替换完整 Todo 表。
- Todo 是 thread-level 结构，字段包含 `id`、`content`、`status`、`source`、`createdAt`、`updatedAt`。
- `ThreadService.setTodos` 会保留 id/timestamps，并兜底最多一个 `in_progress`。
- runtime events 有 `todos_updated` / `todos_cleared`。
- `TodoPanel` 是右侧独立面板，包含 pending/in_progress/completed 统计、状态切换、mark completed、clear、plan source 跳转。
- 支持从 markdown checklist 提取 plan todos，并把状态变化 patch 回 plan。

不宜照搬点：

- DeepSeek-GUI 的 Todo 是 thread-centric，Agent Builder 当前是 session/turn-centric，需要映射到现有 session DTO。
- 它允许用户直接改 Todo 状态，Agent Builder 若要支持，应通过 runtime API 写回，不应只改 React store。

Subagent 关键文件：

- `kun/src/adapters/tool/delegation-tool-provider.ts`
- `kun/src/delegation/delegation-runtime.ts`
- `src/renderer/src/components/chat/message-timeline-process.tsx`

可借鉴点：

- `delegate_task` schema 克制：`label`、`prompt`、`workspace`、`model`。
- runtime 有 `maxParallel` 和 `maxChildRuns` 预算，避免子代理失控。
- child run record 包含 parent thread/turn、label、prompt、workspace、model、status、summary、error、usage、timestamps。
- runtime event 带 child metadata，前端可以在 process/timeline meta badge 中展示 child agent。

不宜照搬点：

- 它的前端 subagent 展示比 Agent Builder 更轻，主要是 badge/meta，不足以替代 Agent Builder 已有的 AgentTaskPanel。

### `cc-haha`

Todo/任务前端关键文件：

- `desktop/src/stores/chatStore.ts`
- `desktop/src/components/chat/SessionTaskBar.tsx`
- `desktop/src/components/chat/InlineTaskSummary.tsx`
- `desktop/src/components/chat/ToolCallBlock.tsx`
- `src/components/TaskListV2.tsx`
- `src/tools/TodoWriteTool/TodoWriteTool.ts`

可借鉴点：

- `chatStore.ts` 会从 live `TodoWrite` tool input 中同步任务到 `useCLITaskStore.setTasksFromTodos(todos, sessionId)`。
- history reload 会通过 `extractLastTodoWriteFromHistory` 恢复最后一次 TodoWrite。
- `SessionTaskBar` 是聊天区 sticky task bar，展示 progress、completed/total、展开任务列表、in_progress activeForm。
- `InlineTaskSummary` 在消息完成后展示 compact summary。
- `ToolCallBlock` 对 Agent tool 做了独立图标和 summary 处理。

不宜照搬点：

- 从 tool input/history 中抽 Todo 是适配 Claude Code 数据源的做法。Agent Builder 已有 runtime Todo API，应优先 hydrate runtime summary，而不是倒推 tool call input。
- UI 使用项目自己的设计系统和 material symbols；Agent Builder 新前端应使用 Ant Design tokens、Ant Design 组件和 scoped CSS Modules。

Subagent 可借鉴点：

- background agent task 会合并进 session state，并通过通知和 message list 形成用户可感知状态。
- Agent tool card 会优先显示 description，让用户不用展开 prompt 才知道子任务意图。

## 对比矩阵

| 维度 | Agent Builder | myclaw/claude-code | DeepSeek-GUI | cc-haha |
| --- | --- | --- | --- | --- |
| Todo 工具名 | `todos` / `todospan` | `TodoWrite` | `todo_list` / `todo_write` | `TodoWrite`，另有 TaskCreate/TaskUpdate 等 |
| Todo 字段 | `content/status/active_form` | `content/status/activeForm` | `id/content/status/source/createdAt/updatedAt` | 从 TodoWrite 映射为 desktop task，含 activeForm/owner 等展示字段 |
| Todo 持久化 | session.todos JSON | AppState/session 或 agentId | thread.todos | desktop store + history extraction |
| Todo runtime event | `todo.updated` | 主要在 CLI/app state | `todos_updated` / `todos_cleared` | websocket/history 驱动 store |
| Todo 前端展示 | 暂无独立 UI | TUI TaskListV2 | 右侧 TodoPanel | sticky SessionTaskBar + InlineTaskSummary |
| 用户操作 Todo | 暂无 | 主要模型更新 | 前端可切状态、clear、打开 plan | 可展开/收起、完成后 dismiss |
| Plan 同步 | 暂无 | 无明显结构化 plan source | 支持 markdown checklist 双向同步 | 主要展示任务 |
| Subagent 工具名 | `agent` | `Agent` / AgentTool | `delegate_task` | `Agent` / AgentTool |
| Subagent 输入 | `prompt` | description/prompt/type/model/background/name/team/mode/isolation/cwd | label/prompt/workspace/model | 接近 Claude Code，桌面 UI 额外展示 |
| Subagent runtime | AgentTask store/messages/results/output refs | local/remote/background/fork/team runtime | DelegationRuntime child records | background task/session state |
| Subagent 前端 | timeline row + AgentTaskPanel | TUI/CLI 进度与 task 输出 | process meta badge | ToolCallBlock、background task notifications |
| 并发/预算 | 有 runtime run/scheduler 线索，但 agent tool 输入未显式暴露 | 支持后台和团队语义 | `maxParallel` / `maxChildRuns` | background task 管理 |

## 推荐补齐顺序

### Todo 第一阶段：补 runtime-first 前端契约

目标是让已有后端能力进入 Workbench view model：

- 在 `client/src/runtime/workbenchTypes.ts` 增加 `TodoViewModel` / `TodoSummaryViewModel`。
- 在 `client/src/runtime/wailsWorkbenchAdapter.ts` 增加 `SessionTodos` bridge DTO、mapper、hydrate。
- 在 Workbench view model 上挂 `todos?: TodoSummaryViewModel`，按 active session hydrate。
- 监听或刷新 `todo.updated` 后更新当前 session Todo summary。
- 保持 Wails/HTTP/dev fallback 统一走 adapter，不在组件里直接调用 Wails binding 或裸 fetch。

### Todo 第二阶段：补展示

建议先做轻量但常驻的任务条，再做完整面板：

- 聊天区顶部或 composer 上方增加 session task bar：进度、当前 activeForm、completed/total、展开列表。
- 右侧或详情区增加 Todo panel：状态统计、列表、最近完成、空态。
- timeline 中保留 tool call card，但不把它作为 Todo 的主要展示。

设计参考：

- `cc-haha` 的 sticky `SessionTaskBar` 适合作为轻量入口。
- `DeepSeek-GUI` 的 `TodoPanel` 适合作为完整管理面板。

### Todo 第三阶段：补数据模型

如果要支持用户手动操作、plan sync 和稳定 UI，建议扩展 Go runtime DTO：

- `id`
- `created_at`
- `updated_at`
- `source`

迁移策略应兼容旧 session.todos：

- 旧 Todo 没有 id 时，用稳定 hash 或首次读取时补齐并保存。
- `source` 先做可选字段，初期只支持 `plan` 或保留为空。
- 工具层强制最多一个 `in_progress`，避免模型 prompt 失效导致 UI 状态异常。

### Todo 第四阶段：强化模型 prompt

把 `internal/agent/tools/todos.md` 从短描述扩成行为规范：

- 什么时候使用 Todo。
- 什么时候不要使用 Todo。
- 必须实时更新，完成后立即标记。
- 最多一个 `in_progress`。
- `content` 使用命令式，`active_form` 使用正在进行式。
- 遇到阻塞时如何表达。
- 不得把未验证、测试失败或部分完成的项标成 completed。

### Subagent 第一阶段：产品化已有 AgentTask UI

不用先扩大后端能力，先让已有能力更可见：

- timeline 的 `agent_task` row 支持打开 `AgentTaskPanel` 或同屏详情。
- tool call card 中对 `agent` 调用优先展示 prompt summary / title / status / child session。
- AgentTaskPanel 从 diagnostics 语义中抽出，变成会话任务面板的一部分。
- follow-up、cancel、output refs 做成明确按钮和状态反馈。

### Subagent 第二阶段：扩展 `agent` 工具 schema

建议小步扩展，不一次性照搬 Claude 系完整 AgentTool：

- `description`：短标题，用于 UI 展示和 timeline summary。
- `role` 或 `subagent_type`：映射到 Agent Builder 的 runtime agent roles。
- `model`：可选覆盖，需遵守 provider/model 配置。
- `background`：是否允许后台运行，先绑定现有 AgentTask lifecycle。
- `cwd`：可选，但必须接入 existing scope validation。

暂缓项：

- `team_name` / teammate。
- remote isolation。
- fork path。
- 多 agent swarm。

这些能力会显著扩大 runtime、permission、worktree、UI 和恢复边界，应单独设计。

### Subagent 第三阶段：补预算和安全阈值

参考 DeepSeek-GUI 的 `maxParallel` / `maxChildRuns`，建议在 Agent Builder 侧明确：

- 单 turn 最大 agent task 数。
- 单 session 最大并发 agent task 数。
- 后台任务超时/取消策略。
- child task token/cost 汇总。
- 触发预算时的 tool result 和 UI 提示。

## 后续修改注意事项

- Todo 和 AgentTask 都应保持 Go runtime 为事实来源，React 只消费 adapter view model。
- 新 UI 使用 Ant Design、Ant Design theme tokens 和 scoped CSS Modules。
- 新 runtime 能力必须同步 Wails bridge、HTTP route、frontend adapter、runtimeapi contract 和测试。
- 不要直接复制参考项目的品牌、视觉、文案或私有交互细节；只吸收信息架构和工程边界。
- 如果引入用户可编辑 Todo，必须通过 runtime API 写回并产生可恢复事件，不能只改前端 store。
