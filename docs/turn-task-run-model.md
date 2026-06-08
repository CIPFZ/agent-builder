# Turn / Task / Run 数据模型

Status: partially implemented design baseline. Runtime turns, ToolCalls,
permissions, audit, recovery, and `RuntimeAgentTask` persistence now exist as
foundations. This document remains useful for object semantics, but several
items below are now implemented rather than future-only.

Current remaining gaps are tracked in:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

本文定义 Agent Builder 客户端化后的核心执行数据模型。目标是让客户端、runtime、audit、恢复机制对同一套对象达成一致。

## 为什么需要这层模型

当前实现里 `RuntimeChatRequest` 触发 `Chat()`，`requestID` 被当作 turn id 使用。这可以跑通桌面聊天，但不足以支撑 Codex 形态客户端：

- 一个用户请求可能产生多个工具调用。
- 一个工具调用可能需要权限审批。
- 一个 turn 可能派生后台子任务。
- 客户端重启后需要恢复 active turn。
- audit 需要按 turn 聚合完整证据。
- 后续 agentic operations 需要比 chat turn 更长生命周期的 run。

因此需要区分：

| 概念 | 含义 |
| --- | --- |
| `Session` | 一条会话或工作线程，承载上下文和历史。 |
| `Turn` | 用户在 session 中发起的一次交互。 |
| `Task` | turn 内或后台产生的可跟踪工作单元，例如 subagent、长工具任务、计划步骤。 |
| `Run` | 跨 turn 的较长操作，适合未来 agentic operations。 |

## 初始阶段的边界

`Turn` and `ToolCall` are now runtime objects. `AgentTask` persistence also
exists as a foundation for subagent/background work. `Run` remains a future
business-operation abstraction and should not be implemented before compact,
scoped policy, tool budget/search, and AgentTask communication are stable.

```text
Session
  Turn
    Message[]
    ToolCall[]
    PermissionRequest[]
    AuditEvent[]
    Artifact[]
    Task[] optional

Run optional later
  Task[]
  Turn[]
```

## Turn

推荐字段：

```text
Turn
  id
  session_id
  status: queued | running | waiting_permission | cancelling | completed | failed | cancelled | interrupted
  user_message_id
  latest_assistant_message_id
  provider
  model
  prompt_preview
  usage_before
  usage_after
  usage_delta
  started_at
  updated_at
  finished_at
  error
```

状态含义：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已创建但尚未开始执行。 |
| `running` | model loop 或 tool scheduler 正在执行。 |
| `waiting_permission` | 等待用户审批。 |
| `cancelling` | 已收到取消请求，正在停止。 |
| `completed` | 正常完成。 |
| `failed` | 失败结束。 |
| `cancelled` | 用户取消。 |
| `interrupted` | runtime 退出或恢复时发现无法继续。 |

## ToolCall

推荐字段：

```text
ToolCall
  id
  turn_id
  session_id
  message_id
  name
  source: builtin | mcp | plugin | shell
  status: pending | waiting_permission | running | completed | failed | cancelled
  input_json
  input_summary
  output_json
  stdout
  stderr
  diff_refs
  artifact_refs
  started_at
  finished_at
  error
```

ToolCall 必须独立于 message 存在。message part 可以引用 tool call，但客户端需要按 tool call 查看详情、权限、输出、diff 和审计。

## PermissionRequest

推荐字段：

```text
PermissionRequest
  id
  session_id
  turn_id
  tool_call_id
  tool_name
  action
  risk
  target_path
  params_summary
  status: pending | allowed | allowed_for_session | denied | expired | cancelled
  created_at
  decided_at
  decision_by
```

当前 `internal/permission.PermissionRequest` 已有基础字段，但缺少 turn id、risk、status、decision record。客户端化后应补齐。

## Task

Task 用于描述 turn 内的可跟踪子工作。

典型场景：

- subagent/background agent
- 长时间 shell command
- 多步骤计划中的步骤
- 批量文件修改
- MCP 长任务

推荐字段：

```text
Task
  id
  parent_turn_id
  parent_task_id
  title
  kind: subagent | tool | plan_step | background
  status
  progress
  owner
  started_at
  updated_at
  finished_at
  result_summary
  error
```

Task 不替代 ToolCall。ToolCall 是模型请求工具，Task 是 runtime 追踪工作。

## Run

Run 是未来 agentic operations 的跨 turn 长生命周期对象。

适用场景：

- “实现一个功能并持续验证”
- “执行迁移计划”
- “后台监控任务”
- “多 agent 协作”

推荐字段：

```text
Run
  id
  title
  workspace_id
  status
  goal
  created_by
  active_session_id
  turn_ids
  task_ids
  started_at
  updated_at
  finished_at
```

当前不建议先实现完整 Run，避免过早复杂化。先把 Turn 和 ToolCall 做稳。

### Phase 6 Design Gate: Minimal Run Contract

Phase 6 defines Run as a future additive business-operation summary, not an
implemented runtime state machine.

Run must not replace these existing primitives:

- `Turn`: the durable unit of user interaction and model/tool execution.
- `ToolCall`: the durable unit of tool evidence, output, refs, and policy
  context.
- `PermissionRequest`: the durable unit of user/policy approval evidence.
- `RuntimeAgentTask`: the existing task persistence foundation.
- `SessionActivity`: the current hydrated source of truth for timeline,
  diagnostics, and interrupted recovery UX.

Minimum Run fields for a future DTO/API:

```text
Run
  id
  workspace_id
  session_ids
  objective
  status
  expected_artifacts[]
  produced_artifacts[]
  verified_artifacts[]
  turn_ids[]
  task_ids[]
  checkpoints[]
  final_verification
  user_actions.resume[]
  user_actions.discard[]
  created_at
  updated_at
  finished_at optional
```

Design constraints:

- Run is a cross-turn index and summary over existing runtime evidence.
- Run references turns, tasks, tool calls, permissions, diagnostics, and
  artifact refs instead of owning or duplicating their state.
- A future React Run UI must hydrate from runtime DTOs. React must not become a
  Run source of truth.
- Runtime events remain refresh triggers only; they are not Run state.
- Resume is always a user-triggered new turn from an explicit checkpoint
  summary. It is not automatic replay.
- Discard acknowledges or closes a checkpoint/run view without deleting the
  underlying evidence.
- No database migration, runtime Run store, or frontend Run UI is part of the
  Phase 6 design gate.

### Phase 7 Design Gate: Claude Code Runtime Mapping

Phase 7 grounds the future Run DTO in the Claude Code runtime model rather than
introducing an independent abstraction.

Claude Code mapping:

| Claude Code concept | Agent Builder counterpart |
| --- | --- |
| `QueryEngine` conversation | runtime `Session` and runtime service |
| `submitMessage()` | `RuntimeTurn` |
| transcript messages | persisted messages and `SessionActivity` |
| tool stream/result | `RuntimeToolCall` and structured refs |
| permission callbacks/denials | `RuntimePermissionRequest` |
| session metadata and recovery | runtime events, audit, replay, recovery status |
| background/local/remote task state | `RuntimeAgentTask` |
| subagent transcript and output file | task/ref/artifact evidence |
| `resumeAgentBackground()` | future explicit checkpoint continuation |

Read-only Run DTO candidate:

```text
RuntimeRunSummary
  id
  workspace_id
  session_ids[]
  primary_session_id
  objective
  status: active | waiting_user | interrupted | completed | failed | cancelled
  turn_ids[]
  task_ids[]
  tool_call_ids[]
  permission_request_ids[]
  expected_artifacts[]
  produced_artifacts[]
  verified_artifacts[]
  checkpoints[]
  diagnostics
  interrupted
  user_actions.resume[]
  user_actions.discard[]
  evidence_cursor
  created_at
  updated_at
  finished_at optional
```

Stability decision:

- Stable as a read-only DTO vocabulary because the fields map to existing
  Agent Builder evidence and to Claude Code runtime concepts.
- Not stable as a database schema yet. Run id assignment, objective ownership,
  checkpoint identity, cross-session grouping, and workspace/worktree metadata
  still need implementation evidence.
- A first implementation should derive the DTO from existing sessions, turns,
  tool calls, permissions, `RuntimeAgentTask`, runtime events, replay, and
  `SessionActivity`.
- `SessionActivity` remains the fallback and parity oracle. Any Run projection
  must prove parity with the corresponding activity evidence for messages,
  tool calls, permissions, diagnostics, artifact evidence, interrupted
  summaries, and terminal permission/MCP semantics.
- Runtime events can trigger a Run DTO refresh, but event payloads must never
  become Run state.
- Resume remains a user-triggered new turn from an explicit checkpoint summary,
  not automatic replay.
- Phase 7 does not add a Run state machine, runtime Run store, Run migration,
  automatic resume, background Run scheduler, or frontend Run UI.

### Phase 7.1 Spike: Internal Read-only Run Projection

Phase 7.1 implements the first read-only projection as an internal runtime
method and test fixture only.

Implemented semantics:

- `RuntimeRunProjection` is assembled from existing `SessionActivity` evidence,
  runtime turns, tool calls, permission requests, runtime events, and
  `RuntimeAgentTask` rows.
- The projection source is marked `session_activity_projection`, `readOnly:
  true`, and `sessionActivityParity: true`.
- `RunProjection(ctx, RuntimeRunProjectionRequest)` is not part of the
  transport-neutral `RuntimeService` interface and is not exposed through
  HTTP, Wails, or React.
- `runtimeAgentTaskStore.ListBySession` is query-only over the existing task
  table. It does not add a migration.
- Checkpoints are derived only from structured interrupted summaries and final
  task evidence. Assistant prose is not a checkpoint source.
- Resume/discard are read-only user-action DTO candidates. They do not execute
  resume, persist acknowledgement, or restore stale permission/MCP/tool
  actionability.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunProjection" -count=1`
- `go test ./internal/runtime -count=1`

### Phase 7.2 Gate: Read-only Transport Contract

Phase 7.2 promotes the read-only projection to transport surfaces without
turning it into persisted Run state.

Implemented semantics:

- `RunProjection(ctx, RuntimeRunProjectionRequest)` is now part of the
  transport-neutral `RuntimeService`.
- HTTP exposes
  `GET /v1/sessions/{session_id}/run-projection?limit=N&cursor=C`.
- Wails exposes `RuntimeBridge.RunProjection(...)`.
- The client bridge has an optional `RunProjection` capability and HTTP
  fallback method, but no React view consumes it.
- The endpoint remains session-scoped and read-only. It is not a persisted Run
  resource.

Boundary:

- No runtime Run store.
- No Run database migration.
- No automatic resume.
- No background Run scheduler.
- No frontend Run UI.
- No executable resume/discard action.
- No event-payload or assistant-prose-derived Run state.

Validation:

- `go test ./internal/runtime -run "TestRuntimeHTTPServerRoutesNarrowActivityToRuntimeService|TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy|TestRuntimeRunProjection" -count=1`
- `go test ./internal/runtimeapi -count=1`
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgePhase62PackagedHandoffRecoveryContract" -count=1`
- `cd client && npx tsc -b --pretty false`

### Phase 7.3 Gate: Frontend Read-only Run Projection Preview

Phase 7.3 adopts the read-only projection in the client as a preview only. It
does not make Run a persisted resource and does not make React the source of
truth.

Implemented boundary:

- `WorkbenchViewModel` now has an optional `RunProjectionViewModel` containing
  aggregate status/count/cursor/source fields.
- The runtime adapter hydrates it from `RunProjection({ sessionId, limit })`
  after resolving the active session.
- The workspace renders a read-only projection preview beside turn diagnostics.
- `userActions`, resume/discard execution, persisted acknowledgement,
  background scheduling, and auto-resume remain out of scope.
- `SessionActivity` remains the parity oracle and the source for timeline,
  diagnostics, permission, artifact, and interrupted recovery state.

## API 影响

最小 API：

```text
POST /v1/sessions/{session_id}/turns
GET  /v1/turns/{turn_id}
POST /v1/turns/{turn_id}/cancel
GET  /v1/turns/{turn_id}/tool-calls
GET  /v1/turns/{turn_id}/permissions
GET  /v1/audit/turns/{turn_id}
```

后续扩展：

```text
GET  /v1/tasks/{task_id}
GET  /v1/runs
POST /v1/runs
GET  /v1/runs/{run_id}
POST /v1/runs/{run_id}/cancel
```

## 事件影响

Turn 和 ToolCall 至少需要这些事件：

```text
turn.started
turn.progress
turn.waiting_permission
turn.completed
turn.failed
turn.cancelled
turn.interrupted

tool.call.started
tool.call.output
tool.call.completed
tool.call.failed
tool.call.cancelled
```

## 存储建议

初始可增加轻量表：

```text
runtime_turns
runtime_tool_calls
runtime_permission_requests
runtime_audit_events
```

message 仍可保留现有 Crush 存储，但 turn/tool/permission/audit 应有 runtime 级索引，避免客户端只能从 message parts 反推状态。

## 迁移步骤

Historical checklist status:

1-5 are implemented as foundations. Step 6 is mostly complete through runtime
turn/recovery APIs, with remaining work around richer task/compact/replay
diagnostics.

1. 保留当前 `requestID == turnID` 过渡行为。
2. 新增 runtime turn store，Chat 创建 turn 时写入。
3. `runtime_events.go` 记录 message/tool/permission 时附带 turn id。
4. `GET /v1/turns/{turn_id}` 从 turn store 读取，而不是只读内存 `requests`。
5. ToolCall 从 message part 反推逐步改为 ToolScheduler 写入。
6. 客户端 active turn 状态从 `RuntimeStatus.requests` 迁移到 turn API。
