# Conversation Output 展示重构实施方案

## 背景

当前 Agent Builder 的对话展示问题不是单纯的 React 样式问题，而是输出事实、展示投影和 UI 渲染之间的边界不清。

现状里已经存在比较接近正确方向的链路：

- `internal/message/content.go` 已经有结构化 message parts：text、reasoning、tool call、tool result、finish。
- `internal/runtime/runtime_output.go` 已经能从 `SessionActivity` 构造 `RuntimeOutputSnapshot`。
- `client/src/runtime/outputReducer.ts` / `outputSelectors.ts` 已经有 `OutputStore -> timeline` 的雏形。
- `client/src/features/timeline/Timeline.tsx` 和 `features/tools/ToolCallCard.tsx` 已经能展示 message、thinking、tool、permission、agent task、hook row、context governance。

但当前仍有几个结构性问题：

- `RuntimeOutputSnapshot` 与 `SessionActivity -> mapActivityTimeline` 两条展示投影并存。
- `wailsWorkbenchAdapter.ts` 仍承担大量排序、阶段判断、fallback、tool grouping 和旧 activity 合并逻辑。
- tool result、permission、hook、AgentTask、Todo、compact/recovery 等事实没有统一进入同一个 conversation projection。
- assistant message 的中间 tool-only step、final answer、thinking、tool batch 之间的关系不够明确。
- UI 组件仍需要根据名字、summary、状态字符串和旧字段推断展示语义。

本方案按“第一版彻底重构”处理：不以兼容旧展示推断为目标，而是建立新的 runtime-owned conversation output contract，前端只消费稳定 view model，并删除旧推断主路径。

## 参考项目结论

### crush

`crush` 的关键是后端 message part 结构清晰，TUI 只做投影和渲染：

- assistant message 被拆成 assistant text item 和 tool item。
- tool result 通过 `tool_call_id` 挂回 tool item，不作为普通聊天行。
- 没有文本/thinking/error 的纯 tool assistant 不显示成空消息。
- tool item 有稳定状态：awaiting permission、running、success、error、canceled。
- 每类工具有专门 renderer，header 总是给动作摘要，body 只在有价值时展开。
- nested agent tool 通过子 session 消息挂到父 tool item 下。

### cc-haha

`cc-haha` 的关键是 stream event 到 render model 的状态机：

- text delta 被缓冲成一条 streaming assistant bubble。
- tool_use 创建 pending tool block。
- tool input 的部分 JSON 用于提前预览 command/file/path。
- tool_result 被隐藏并挂回 tool_use。
- render model 做 child tool grouping、Agent tool 特殊展示、连续工具分组。

但它很多事实来自前端对 Claude Code JSONL 的推断。Agent Builder 不应复制这个做法，应由 Go runtime 输出事实。

### claude-code

`claude-code` 的关键是 transcript normalization 和 lookup：

- `normalizeMessages` 把多 content block 拆成单 block render message。
- `buildMessageLookups` 预计算 tool use、tool result、progress、error、hook counts 的关系。
- `reorderMessagesInUI` 把 tool result / hook attachment 放到对应 tool use 附近。
- tool 自己提供 user-facing name、tool use summary、progress renderer、grouped renderer。
- Todo/TaskList 是持续状态视图，不依赖普通聊天文本承载任务状态。

## 目标

第一版重构后，主对话输出必须满足：

1. Go runtime 是输出事实和展示投影的唯一来源。
2. Runtime events 只负责增量刷新和审计，不作为前端业务事实。
3. React 不从 raw message、tool name、文本内容临时推断业务状态。
4. tool result 不再作为普通对话行展示，只作为 tool call lifecycle 的一部分。
5. assistant 的中间步骤和最终回答区分明确。
6. Todo、AgentTask、Hook、Permission、Compact、Recovery 都是主 workflow 可见对象，不藏在 diagnostics。
7. UI 层只做局部交互：折叠、展开、复制、打开详情、按 runtime 提供的 display policy 分组。
8. Vite/browser dev 和 Wails desktop 走同一 runtime output contract。

## 非目标

- 不复刻任何参考项目的视觉样式、品牌、文案或布局。
- 不把 Claude Code 的 TS AppState/transcript 结构搬进 Agent Builder。
- 不继续维护旧 `SessionActivity -> mapActivityTimeline` 作为主展示路径。
- 不通过前端解析 tool input 文本来生成 Todo、AgentTask、Hook 或 Recovery 状态。
- 不把所有原始 stdout/stderr/tool output 塞进主 timeline；大输出继续通过 runtime refs 和详情读取。

## 目标架构

新的输出链路：

```text
provider / agent loop / tool scheduler
  -> internal/message structured parts
  -> runtime stores: messages, turns, tool calls, tool results, permissions, hooks, tasks, todos, context governance
  -> RuntimeConversationProjection
  -> RuntimeOutputSnapshot + RuntimeOutputEvents
  -> client OutputStore
  -> outputSelectors: view model only
  -> Timeline / ToolCallCard / TodoTaskBar / AgentTask views
```

关键边界：

- `internal/message`：保留模型历史事实，不负责 UI 分组。
- `internal/tools/scheduler`：记录工具生命周期和结构化结果。
- `internal/runtime`：聚合所有事实并输出 conversation projection。
- `client/src/runtime`：只做 DTO 映射、增量 reducer、轻量 selector。
- `client/src/features`：只渲染 view model，不补业务事实。

## Runtime 输出合同

新增或重构 `RuntimeOutputSnapshot` 为主合同。建议版本化为 `RuntimeConversationSnapshot`，但可以先沿用现名并收敛字段。

### 顶层结构

```go
type RuntimeConversationSnapshot struct {
	SessionID string `json:"sessionId"`
	Cursor    string `json:"cursor,omitempty"`
	Version   int    `json:"version"`

	Turns       []RuntimeConversationTurn       `json:"turns"`
	Items       []RuntimeConversationItem       `json:"items"`
	ToolCalls   []RuntimeConversationToolCall   `json:"toolCalls,omitempty"`
	Permissions []RuntimeConversationPermission `json:"permissions,omitempty"`
	Hooks       []RuntimeConversationHookRun    `json:"hooks,omitempty"`
	AgentTasks  []RuntimeAgentTask              `json:"agentTasks,omitempty"`
	Todos       *RuntimeTodoSummary             `json:"todos,omitempty"`
	Context     []RuntimeContextGovernanceItem  `json:"context,omitempty"`
}
```

其中 `Items` 是 UI timeline 的主输入，不再让前端从 messages、turns、toolCalls 重新拼装主流程。细节对象仍单独给出，供详情页和 card 使用。

### Conversation item

```go
type RuntimeConversationItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId,omitempty"`
	ParentID  string `json:"parentId,omitempty"`
	Sequence  int64  `json:"sequence"`

	Role      string `json:"role,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Status    string `json:"status,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`

	MessageID    string `json:"messageId,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	PermissionID string `json:"permissionId,omitempty"`
	HookRunID    string `json:"hookRunId,omitempty"`
	AgentTaskID  string `json:"agentTaskId,omitempty"`
	ContextID    string `json:"contextId,omitempty"`

	Display RuntimeConversationDisplay `json:"display,omitempty"`
	CreatedAt int64 `json:"createdAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}
```

`Kind` 的第一版集合：

- `user_message`
- `assistant_message`
- `assistant_thinking`
- `tool_call`
- `tool_group`
- `permission_request`
- `hook_run`
- `agent_task`
- `todo_summary`
- `compact_boundary`
- `snip_boundary`
- `microcompact_marker`
- `tool_result_replacement`
- `recovery_notice`
- `turn_progress`
- `turn_terminal`
- `diagnostic_warning`

前端不再把 `kind: message + role` 当主分类。可以在 view model mapper 中兼容转换到现有 React props，但 runtime 合同应表达真实语义。

### Assistant step

Assistant 输出要明确分为三类：

- `assistant_thinking`：reasoning 内容或摘要。
- `assistant_message phase=intermediate`：工具前或工具间的可读说明，默认可折叠。
- `assistant_message phase=final`：本 turn 最终回答。

判定规则放在 runtime：

- assistant message 只有 tool calls、无 text/reasoning/error：不产出 assistant message item。
- assistant message finish reason 为 `tool_use`：其中 text 若存在，产出 intermediate assistant message。
- assistant message finish reason 为 `end_turn` 且有 text：产出 final assistant message。
- turn failed/cancelled 且没有 final assistant：产出 `turn_terminal` 或 `recovery_notice`，不伪造 assistant 回答。

### Tool call

工具展示对象应明确包括生命周期、展示策略和关系：

```go
type RuntimeConversationToolCall struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	TurnID          string `json:"turnId"`
	AssistantItemID string `json:"assistantItemId,omitempty"`
	MessageID       string `json:"messageId,omitempty"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`

	Name         string `json:"name"`
	Source       string `json:"source"`
	CapabilityID string `json:"capabilityId,omitempty"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`

	InputSummary  string `json:"inputSummary,omitempty"`
	OutputSummary string `json:"outputSummary,omitempty"`
	Error         string `json:"error,omitempty"`

	Display RuntimeToolCallDisplay `json:"display,omitempty"`
	Policy  RuntimeToolPolicyView  `json:"policy,omitempty"`
	Result  RuntimeToolResultView   `json:"result,omitempty"`
	Refs    RuntimeToolRefsView     `json:"refs,omitempty"`

	GroupKey        string `json:"groupKey,omitempty"`
	Groupable       bool   `json:"groupable"`
	Quiet           bool   `json:"quiet"`
	DefaultExpanded bool   `json:"defaultExpanded"`

	StartedAt  int64 `json:"startedAt,omitempty"`
	FinishedAt int64 `json:"finishedAt,omitempty"`
}
```

状态标准化：

- `queued`
- `running`
- `waiting_permission`
- `completed`
- `failed`
- `denied`
- `cancelled`
- `interrupted`

展示策略由 runtime 给出：

- `quiet=true`：成功的 read/search/list/glob/grep 等默认收进工具摘要。
- `groupable=true`：相邻同 kind、同 turn、同 assistant step 的工具可以组成 `tool_group`。
- `defaultExpanded=true`：running、waiting_permission、failed、denied、cancelled、interrupted 默认展开。

### Tool result

tool result 不再作为主 timeline item。它只挂在 tool call 上：

```go
type RuntimeToolResultView struct {
	ID               string   `json:"id,omitempty"`
	MessageID        string   `json:"messageId,omitempty"`
	Status           string   `json:"status,omitempty"`
	ContentPreview   string   `json:"contentPreview,omitempty"`
	DataPreview      string   `json:"dataPreview,omitempty"`
	DeliveredToModel bool     `json:"deliveredToModel,omitempty"`
	Synthetic         bool     `json:"synthetic,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	ArtifactRefs     []string `json:"artifactRefs,omitempty"`
	DiffRefs         []string `json:"diffRefs,omitempty"`
	CreatedAt        int64    `json:"createdAt,omitempty"`
}
```

规则：

- 有 matching tool result：更新 tool status/result。
- result `is_error=true`：tool status 至少为 failed，错误摘要进入 `Error` 或 `Result.ContentPreview`。
- missing result 且 turn 已结束：runtime 标为 interrupted/failed，不让 UI 无限 spinning。
- synthetic result 必须可见于 diagnostics/recovery，但主 UI只显示“工具结果由恢复逻辑补齐/缺失”摘要。

### Permission

Permission 作为 tool lifecycle 的一部分：

- pending permission 让 tool call status 变为 `waiting_permission`。
- timeline 产出 `permission_request` item，紧跟 tool item。
- permission 决策后更新 permission item 和 tool status。
- permission 不再通过普通 system/message 行展示。

### Hook

Hook 不应只在 diagnostics 可见。第一版展示规则：

- `running` hook：作为相关 tool/progress 的轻量状态，仅在进行中展示。
- `blocked`、`failed`、`inputRewritten`、`contextInjected`：产出 `hook_run` timeline item。
- `completed` 且无 rewrite/context/error：默认不进入主 timeline，可在详情/diagnostics 查。
- Hook detail 从 `RuntimeHookExecution` hydrate，不把完整 stdout/stderr 放进 timeline。

### AgentTask / nested agent

AgentTask 必须与父 tool call 关联：

- 父 tool call display kind 可为 `agent_task`。
- tool card 内展示任务标题、role/model/status/progress/result summary。
- timeline 产出 `agent_task` item，点击打开任务详情。
- child session 的消息和工具默认不铺到主 timeline；在 AgentTask 详情中查看。
- 子任务失败、等待权限、interrupted 时，父 tool call 和 AgentTask item 都要反映状态。

### Todo

Todo 是持续状态，不是聊天消息：

- runtime `RuntimeTodoSummary` 是唯一来源。
- 主界面可显示当前 Todo task bar。
- timeline 只在 Todo 发生结构性变化时产出轻量 `todo_summary`，例如首次创建、全部完成、任务数变化。
- 不从 TodoWrite tool input 在前端解析任务状态。

### Context governance / Recovery

Compact、snip、microcompact、tool result replacement、reactive retry、recovery 都应作为明确 item：

- `compact_boundary`
- `snip_boundary`
- `microcompact_marker`
- `tool_result_replacement`
- `recovery_notice`

这些 item 默认轻量显示，失败或影响当前 turn 时展开。

## 后端实施

### 阶段 1：建立 projection builder

新增文件建议：

```text
internal/runtime/runtime_conversation_projection.go
internal/runtime/runtime_conversation_projection_test.go
internal/runtime/runtime_conversation_display.go
internal/runtime/runtime_conversation_events.go
```

职责：

- 从 messages、turns、tool calls、tool results、permissions、hook executions、agent tasks、todos、context diagnostics 构造 `RuntimeConversationSnapshot`。
- 所有排序、分组、phase 判定、quiet/defaultExpanded 判定放在这里。
- 保留 `runtime_output.go` 作为过渡入口，但内部调用新 projection。

排序规则：

1. turn started time。
2. turn 内 sequence。
3. user message。
4. prompt hook 高信号 item。
5. assistant thinking。
6. assistant intermediate message。
7. tool call / tool group。
8. permission / tool hook。
9. AgentTask / Todo / context governance。
10. turn terminal/recovery。
11. assistant final message。

需要保证 sequence 稳定，不依赖 map 遍历顺序。

### 阶段 2：重构 RuntimeOutputSnapshot

当前 `RuntimeOutputSnapshot` 包含 messages、turns、assistantSteps、toolCalls、toolResults、permissions。第一版可以选择两步：

第一步兼容名称，替换语义：

- 增加 `items []RuntimeConversationItem`。
- 增加 `version`。
- 保留旧数组只用于详情和调试。
- 前端 selector 改为优先消费 `items`。

第二步删除旧主路径：

- `selectConversationTimeline` 不再从 assistantSteps/toolCalls 拼主流程。
- `mapActivityTimeline` 不再作为主路径，只保留 diagnostics/dev fallback 或直接删除。
- Wails/HTTP 都以 `SessionOutput` 为主。

### 阶段 3：统一 tool display

重构 `runtime_tool_calls.go` 中的 display 逻辑：

- 保留 `RuntimeToolCallDisplay`，但增加 display policy 字段到 tool call，而不是让前端推断。
- `runtimeToolDisplayKind` 从工具注册表/metadata 优先获取，字符串匹配只作为 fallback。
- 对内置工具建立稳定映射：
  - `bash/job_output/job_kill` -> `shell`
  - `read/view` -> `file_read`
  - `write` -> `file_write`
  - `edit/multiedit/apply_patch` -> `file_edit`
  - `glob/grep/ls/search` -> `file_search`
  - `todos` -> `todo`
  - `agent/agentic_fetch/task_create` -> `agent_task`
  - MCP/plugin/custom -> `generic`，但允许 metadata 覆盖。

第一版不追求每个工具都有精致卡片，但必须保证每个工具都有：

- 稳定 title。
- 主要 target/command。
- 状态。
- 错误摘要。
- refs 计数。
- 是否 quiet/groupable/defaultExpanded。

### 阶段 4：tool result 与 assistant step 关联

当前 `runtime_output.go` 通过 message parts 构造 tool result，并用 `stepByCallID` 回填 assistant step。需要加强：

- tool call 必须保存 `assistantItemID` 或 `assistantStepID`。
- tool result 必须保存 `toolCallID`、`messageID`、`deliveredToModel`、`synthetic`、`reason`。
- assistant step 的 `ToolCallIDs` 顺序必须来自 message part 顺序。
- 如果 tool result 到达但 tool call 缺失，runtime 创建 orphan marker，并在 recovery/diagnostic 可见。
- turn 结束时未完成工具统一标记 interrupted/cancelled，避免 UI 持续 loading。

### 阶段 5：hooks/tasks/todos/context 进入 projection

将以下 hydrate 从 workbench adapter 推回 runtime projection：

- high-signal hook item 选择。
- AgentTask 关联到 parent tool call。
- Todo summary item 生成。
- context governance item 生成。
- recovery notice item 生成。

前端可以继续单独 hydrate detail 面板，但主 timeline 顺序和是否展示由 runtime 决定。

## 前端实施

### 阶段 1：更新 TypeScript DTO

修改：

- `client/src/runtime/outputTypes.ts`
- `client/src/runtime/workbenchTypes.ts`

新增：

- `RuntimeConversationItem`
- `RuntimeConversationDisplay`
- `RuntimeConversationToolCall`
- `RuntimeToolResultView`
- `RuntimeDisplayPolicy`

`ConversationTimelineItemViewModel` 应与 runtime item 更接近，避免 `kind: message + role` 这种二次分类作为主模型。

### 阶段 2：简化 OutputStore

`OutputStore` 应变成：

```ts
interface OutputStore {
  sessionId: string;
  cursor?: string;
  version?: number;
  itemsById: Record<string, RuntimeConversationItem>;
  turnsById: Record<string, RuntimeConversationTurn>;
  toolCallsById: Record<string, RuntimeConversationToolCall>;
  permissionsById: Record<string, RuntimeConversationPermission>;
  hooksById: Record<string, RuntimeConversationHookRun>;
  agentTasksById: Record<string, AgentTaskViewModel>;
  optimisticByClientRequestId: Record<string, OptimisticUserSubmit>;
  appliedEventIds: Record<string, true>;
}
```

selector 只做：

- 按 sequence 排序。
- optimistic 用户消息替换。
- 将 runtime item 映射为 React 组件 props。
- 不再从 tool calls 自行生成主 timeline。

### 阶段 3：删除旧 activity 主路径

修改 `client/src/runtime/wailsWorkbenchAdapter.ts`：

- `hydrateWorkbench` 中 active session 的 conversation/timeline 只来自 `SessionOutput`。
- `SessionActivity` 只用于仍未迁移的 diagnostics/run projection，或短期 fallback。
- 删除或降级：
  - `mapActivityTimeline`
  - `mergeActivityTimeline`
  - `compareActivityTimelineItems`
  - `runtimeGroupedThinkingItems`
  - `runtimeMessageHasToolCalls`
  - `timelineKindRank` 等旧排序推断。

如果某些 diagnostics 仍依赖 `SessionActivity`，必须明确注释为 diagnostics-only，不允许影响主 conversation rendering。

### 阶段 4：重写 Timeline 渲染入口

`Timeline.tsx` 应从 runtime item kind 分派：

- `user_message` -> `UserMessageBubble`
- `assistant_message phase=final` -> `AssistantFinalMessage`
- `assistant_message phase=intermediate` -> `AssistantProcessNote`
- `assistant_thinking` -> `ThinkingItem`
- `tool_call` -> `ToolCallCard`
- `tool_group` -> `ToolCallGroupCard`
- `permission_request` -> `PermissionTraceRow`
- `hook_run` -> `HookTimelineRow`
- `agent_task` -> `AgentTaskTimelineRow`
- `todo_summary` -> `TodoTimelineRow`
- context/recovery kinds -> `ContextGovernanceRow` / `RecoveryNoticeRow`
- `turn_progress` -> `TurnProgressRow`
- `turn_terminal` -> `TurnTerminalRow`

`buildTurnBlocks` 可以保留，但只负责视觉分块，不再决定 item 的业务阶段。

### 阶段 5：ToolCallCard 只消费 display policy

`ToolCallCard.tsx` 当前还通过 `toolKind()`、`isShellTool()`、`isSearchToolName()` 推断很多语义。重构后：

- `kind = toolCall.kind || toolCall.display.kind`。
- `status = toolCall.status`，非零 exit code 的 failed 判定由 runtime 做。
- title、detail、target、refs、duration、defaultExpanded 由 runtime 给。
- 前端只保留 icon 映射、折叠/复制/打开详情。

需要保留极少 fallback，只用于防御未知 runtime 数据，不能作为主逻辑。

### 阶段 6：UI 降噪规则

这些规则由 runtime 给出，前端只执行：

- `quiet && completed`：默认进 group summary。
- `defaultExpanded`：默认展开。
- `groupKey` 相同且相邻：显示 `tool_group`。
- failed/denied/cancelled/interrupted：展开并显示 failure reason。
- running/waiting_permission：展开并显示 spinner/permission。
- final assistant 始终在 process trace 后。

## 旧结构删除清单

第一版目标是删除或降级这些不健康点：

- 前端从 `RuntimeMessageDTO.parts` 判断 assistant 是否 intermediate。
- 前端从 tool name/summary 推断 tool kind 作为主逻辑。
- 前端从 `SessionActivity` 重新排序 message/tool/permission。
- tool result 独立作为 message row。
- hook 状态只能从 diagnostics 间接看到。
- AgentTask 只作为 diagnostics 或右侧隐蔽入口。
- Todo 通过 tool input 前端解析。
- failed turn 伪装成普通 assistant message。

保留但降级：

- `SessionActivity`：runtime parity/debug/diagnostics 数据源。
- `ReactCallchain`：诊断和审计，不作为主 timeline 事实源。
- `TurnDiagnostics`：高信号 warning 可以投影到 conversation item，但 UI 不直接从 diagnostics 拼流程。

## 测试计划

### Go 单元测试

新增 `runtime_conversation_projection_test.go`，覆盖：

- assistant text only -> user + final assistant。
- assistant thinking + text -> thinking + final assistant。
- assistant tool-only -> 不生成空 assistant message。
- assistant text + tool call + result + final text -> intermediate + tool + final。
- tool result 挂回 tool call，不生成普通 message item。
- permission pending -> tool waiting_permission + permission_request。
- permission denied -> tool denied + permission decided。
- shell exit code 非 0 -> failed。
- read/search 成功 -> quiet/groupable。
- failed tool -> defaultExpanded。
- missing tool result + final turn -> interrupted/failed，不 spinning。
- hook blocked/rewrite/context injected -> hook_run item。
- hook completed no signal -> 不进主 timeline。
- AgentTask parentToolCallID -> tool card 和 agent_task item 关联。
- Todo summary -> todo_summary 或 top-level todos。
- compact/recovery -> context/recovery item。
- sequence 稳定，不受 map 遍历影响。

### Runtime API/bridge 测试

覆盖：

- HTTP `/session/{id}/output` 返回新 version/items。
- Wails `SessionOutput` 返回相同合同。
- `SessionOutputEvents` 对 message/tool/permission/hook/task/todo/context 事件返回对应 item 更新。
- snapshot + events replay 后与 fresh snapshot 等价。
- Vite/dev HTTP fallback 与 Wails 输出一致。

### 前端单元测试

覆盖：

- `hydrateOutputStore` snapshot 初始化。
- event append/update/delete 幂等。
- optimistic user submit 被 runtime user item 替换。
- selector 不生成额外 tool result row。
- sequence 排序稳定。
- tool group item 直接渲染，不再前端重组业务状态。

### 组件测试

覆盖：

- final assistant 在 process trace 后。
- tool running 默认展开。
- permission waiting 显示权限行。
- failed tool 展示 error/stderr/failure reason。
- quiet completed tool group 默认折叠。
- AgentTask 行能打开详情。
- Hook 高信号行能打开详情。
- Todo task bar 使用同一 runtime todos。

### 场景测试

建议用 runtime scenario harness 增加：

1. 简单问答。
2. 搜索多个文件后总结。
3. 运行命令成功。
4. 运行命令失败。
5. 编辑文件并展示 diff refs。
6. 工具需要权限并被允许。
7. 工具需要权限并被拒绝。
8. hook block tool。
9. hook rewrite input。
10. TodoWrite 创建/更新/完成。
11. Agent task 启动、产出结果、失败。
12. compact 后继续对话。
13. provider error/recovery notice。
14. interrupted turn resume。

## 迁移顺序

### PR 1：Runtime conversation projection skeleton

- 新增 projection 类型和 builder。
- `SessionOutput` 返回 `version/items`。
- 保留旧字段。
- 加 Go projection 单测。

验收：

- 新 snapshot 能表达 user、assistant final、thinking、tool、permission。
- 旧 UI 不破。

### PR 2：Tool lifecycle/display policy 下沉 runtime

- 增强 `RuntimeToolCall` display policy。
- runtime 负责 kind/status/defaultExpanded/quiet/groupable。
- 补 tool display 单测。

验收：

- 前端不需要从 tool name 判断主要 kind。
- failed/nonzero shell 在 runtime 标准化。

### PR 3：Frontend OutputStore 切主路径

- `outputTypes.ts`、`outputReducer.ts`、`outputSelectors.ts` 改为消费 `items`。
- `hydrateWorkbench` 主 conversation/timeline 来自 `SessionOutput`。
- `SessionActivity` 主 timeline fallback 加 feature flag 或删除。

验收：

- 主对话不经过 `mapActivityTimeline`。
- tool result 不再独立出现。

### PR 4：Timeline/ToolCallCard 重写为 display-only

- `Timeline.tsx` 按 runtime item kind 分派。
- `ToolCallCard.tsx` 只消费 runtime display 和 policy。
- 删除前端主要推断函数。

验收：

- 简单问答、工具调用、权限、失败、final answer 顺序正确。

### PR 5：Hook/AgentTask/Todo/Context 接入 projection

- runtime projection 产出 hook high-signal items、AgentTask items、Todo summary、context governance。
- 前端组件接入新 item kinds。

验收：

- 这些能力在主 workflow 可见，且 detail 仍从 runtime hydrate。

### PR 6：删除旧 activity rendering

- 删除 `mapActivityTimeline`、`mergeActivityTimeline` 等旧主展示逻辑。
- `SessionActivity` 只保留 diagnostics/runtime parity 用途。
- 更新测试。

验收：

- 搜索代码时不存在前端旧 activity timeline 主路径。
- 所有主对话场景通过 `SessionOutput` 完成。

## 风险与控制

### 风险：一次重构范围大

控制：

- 分 PR，但每个 PR 都朝同一新合同收敛。
- 不做双主路径长期并存。
- 每个阶段都有 snapshot/event replay 测试。

### 风险：runtime projection 太重

控制：

- projection 是纯函数，输入为 runtime stores/activity window。
- 支持 cursor/limit window。
- refs 只放摘要，大内容按需读取。

### 风险：事件增量和 snapshot 不一致

控制：

- 定义 replay invariant：`snapshot(cursor0) + events(cursor0..N) == snapshot(cursorN)`。
- 对 message/tool/permission/hook/task/todo/context 都测。

### 风险：丢失 diagnostics 可观测性

控制：

- diagnostics 不作为主流程拼装来源，但可以作为 item evidence。
- ReactCallchain/TurnDiagnostics 保留在诊断面。

### 风险：工具 kind/display 误判

控制：

- 内置工具优先由注册 metadata 提供 kind。
- 字符串匹配只作为 fallback。
- unknown tool 渲染 generic card，但状态/结果/错误仍准确。

## 完成标准

重构完成后，应满足：

- 主 timeline 只从 `SessionOutput` / conversation projection 生成。
- `SessionActivity -> mapActivityTimeline` 不再参与主对话展示。
- tool result 不会作为独立聊天消息出现。
- tool lifecycle、permission、hook、AgentTask、Todo、compact/recovery 在主 workflow 中有明确位置。
- React 组件没有业务级 tool/status 推断，只执行 runtime display policy。
- Wails 和 HTTP/dev transport 展示一致。
- snapshot 和 event replay 一致。
- 所有核心场景有 Go + frontend 测试覆盖。

## 第一版推荐切入点

先做 `RuntimeConversationItem` 和 `SessionOutput` 主路径，不先改视觉组件。

理由：

- 现在 UI 问题根源在“输出怎样被穿起来”，不是按钮、卡片或 CSS。
- 一旦 runtime projection 稳定，Timeline 和 ToolCallCard 可以很直接地重写。
- 如果先改 React，会继续扩大 `wailsWorkbenchAdapter.ts` 的推断逻辑，后面更难删。

推荐第一周目标：

1. 新建 projection builder。
2. `SessionOutput` 返回 `items`。
3. 覆盖 8 个最核心 projection 测试：simple final、thinking、tool-only hidden、tool result attached、permission pending、failed tool、quiet group、final ordering。
4. 前端 selector 优先消费 `items`，保留旧 selector fallback 仅用于未返回 version 的静态 adapter。

