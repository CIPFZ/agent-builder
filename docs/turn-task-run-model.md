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

### Phase 8 Gate: Durable Run Identity And Persistence Design

Phase 8 defines the persistence contract for a future durable Run without
implementing it. The key decision is to separate durable identity from runtime
truth: a persisted Run can group sessions, turns, tasks, worktrees, and
checkpoints, but `SessionActivity` and structured runtime evidence remain the
authority for timeline, diagnostics, permission, artifact, interrupted, and
MCP terminal semantics.

Design decisions:

- New persisted Runs should use generated durable ids such as `run_<id>`.
  Legacy backfill can use deterministic compatibility ids like
  `run:session:<session_id>` for idempotency.
- A Run must keep `primary_session_id` and use a link table for additional
  sessions, child sessions, task sessions, and recovery sessions.
- Proposed tables are `runtime_runs`, `runtime_run_sessions`, and
  `runtime_run_checkpoints`. Phase 8 does not add those migrations.
- Backfill must not infer grouping, artifacts, or checkpoints from assistant
  prose. It can use existing `sessions`, `runtime_turns`,
  `runtime_tool_calls`, `runtime_permission_requests`, `runtime_agent_tasks`,
  `runtime_worktrees`, runtime events, and diagnostics.
- Resume is a new user-triggered turn linked to a checkpoint summary and the
  current workspace state. It is not automatic replay.
- Discard acknowledges product UX state only; it must not delete evidence or
  rewrite terminal turn/tool/permission/MCP records.

Phase 8.1 entry criteria:

- Migrations and stores must be accepted separately.
- Backfill must be idempotent.
- Restart tests must prove stale running/waiting tools, permission gates, MCP
  auth requests, and MCP elicitation requests do not become actionable.
- Persisted Run summaries must prove parity with the corresponding
  `SessionActivity`/`RunProjection` subset.
- No automatic resume, background Run scheduler, or executable resume/discard
  controls should be implemented in the persistence foundation phase.

### Phase 11 Gate: Run Lifecycle Source-of-Truth Cutover

Phase 11 narrows the first source-of-truth cutover to persisted Run detail
reconciliation.

Accepted model:

- Persisted Run detail is a durable read cache for runtime-owned evidence, not
  an independent scheduler state machine.
- `RunProjection` remains the parity oracle for status, timestamps, counts,
  checkpoints, diagnostics, artifacts, and user-action eligibility.
- `SessionActivity` remains the fallback/parity oracle for timeline, messages,
  tool calls, permissions, diagnostics, artifact evidence, interrupted
  summaries, and terminal MCP semantics.
- Transition history remains audit evidence. It can validate replay/order, but
  it cannot decide current lifecycle or actionability.
- Permission, MCP auth, and elicitation actionability still come from current
  runtime stores.

Phase 11.1 should harden persisted Run detail reconciliation after turn start,
turn finish, cancellation, interrupted acknowledgement, startup recovery,
explicit checkpoint resume, and checkpoint acknowledgement/discard. It should
not add a migration, scheduler, automatic resume, frontend Run management UI,
or transition-derived actionability.

### Phase 12 Gate: Run Execution Ownership

Phase 12 keeps the current session-first execution path and accepts only
contract-first Run ownership hardening.

Accepted model:

- `Chat` remains the execution entry point.
- Runtime must ensure a durable Run for the session before execution and link
  each persisted turn to that Run before transition audit rows are treated as
  useful ordering evidence.
- `runChat`, cancellation, startup recovery, interrupted acknowledgement, and
  explicit checkpoint resume remain structured evidence writers before any
  transition row or frontend event can be interpreted.
- A future scheduler may use reconciled Run detail for ownership/grouping and
  cancellation scope, but permission/MCP actionability must stay in current
  runtime stores.
- Checkpoint resume remains an explicit user-triggered new turn, never
  automatic replay.

Phase 12.1 should add ownership preflight/link stability tests only. It should
not add a scheduler implementation, migration, automatic resume, frontend Run
management UI, background worker, or transition-derived actionability.

### Phase 13 Gate: Run Scheduler Boundary

Phase 13 defines scheduler ownership without introducing scheduler behavior.
The current implementation remains session-first: `Chat`, cancellation,
startup recovery, interrupted acknowledgement, and explicit checkpoint resume
continue to write structured runtime evidence before Run transition audit or
frontend refresh events can be interpreted.

Accepted future scheduler responsibilities:

- Create and manage Run-level execution plans only after a durable Run and
  Run/session link exist.
- Group future turns/tasks/checkpoint-resume turns under a Run for ownership,
  cancellation scope, and diagnostics routing.
- Use reconciled Run detail as a read model for display status and grouping.
- Emit refresh-trigger events that select which runtime DTO should be read.
- Write audit/diagnostic evidence after durable turn/tool/runtime evidence is
  persisted.

Responsibilities that remain outside scheduler ownership:

- Permission request actionability.
- MCP auth and elicitation actionability.
- Checkpoint actionability and checkpoint acknowledgement/discard state.
- Artifact evidence and produced refs.
- Session timeline, diagnostics, interrupted summaries, terminal permission
  semantics, and terminal MCP semantics.
- Current lifecycle truth for partial/bounded projection windows.

Required before implementing scheduler code:

- A concrete scheduler-facing DTO/API contract.
- Tests proving scheduler-created work cannot execute without durable
  Run/session/turn links.
- Tests proving cancellation and startup recovery terminalize stale structured
  evidence before transition audit or replay.
- Tests proving explicit checkpoint resume remains a user-triggered new turn,
  never automatic replay.
- Parity tests proving scheduler reads match the corresponding
  `SessionActivity`/`RunProjection` subset for messages, tool calls,
  permissions, diagnostics, artifact evidence, interrupted summaries, and
  terminal permission/MCP semantics.

Phase 13 does not add a migration, background scheduler, automatic resume,
frontend Run UI, transition-derived lifecycle/actionability, React-owned Run
state, or prose-derived artifact/checkpoint/lifecycle inference.

### Phase 13.1 Gate: Scheduler Preflight Contract

Phase 13.1 adds an internal read-only scheduler preflight contract. It is not a
scheduler worker and it is not exposed as a frontend capability.

The preflight can return `canSchedule=true` only when all durable ownership
evidence is present:

- The turn exists in `runtime_turns`.
- The request session matches the turn session.
- The Run exists in `runtime_runs`.
- The Run contains the session in `runtime_run_sessions`.
- The Run/session link points at the same turn.
- The turn is not terminal.

The preflight reads `runtime_runs`, `runtime_run_sessions`, and `runtime_turns`
only. It does not decide permission, MCP auth, MCP elicitation, checkpoint,
artifact, interrupted, timeline, diagnostics, or lifecycle truth. It does not
start a worker, resume work, write transitions, or emit frontend state.

### Phase 13.2 Gate: Scheduler Preflight Acceptance

Phase 13.2 accepts the internal preflight as the first execution gate for a
future scheduler. No additional runtime behavior is introduced.

Remaining before scheduler implementation:

- Define a read-only scheduler plan DTO.
- The plan DTO must describe intended ownership, ordering, cancellation scope,
  diagnostics routing, refresh targets, and required preflight checks.
- The plan DTO must not start work, auto-resume checkpoints, write transitions,
  decide actionability, or become frontend lifecycle state.
- Any future worker must check the Phase 13.1 preflight before executing a
  planned turn.

### Phase 14 Gate: Scheduler Plan DTO

Phase 14 defines the read-only scheduler plan DTO contract. It does not
implement the DTO or a worker.

The future plan model is:

- `RuntimeRunSchedulerPlanRequest`: run/session plus optional turn,
  checkpoint, task, cursor, and limit fields.
- `RuntimeRunSchedulerPlanResponse`: plan plus source metadata.
- `RuntimeRunSchedulerPlan`: Run identity, linked sessions, objective,
  status-from-Run-detail, plan items, cancellation scope, diagnostics route,
  refresh targets, and optional activity window.
- `RuntimeRunSchedulerPlanItem`: item kind, order key, session/turn/checkpoint
  or task ids, `can_schedule`, preflight reason, required preflight,
  cancellation scope, diagnostics route, and refresh targets.
- `RuntimeRunSchedulerPlanSource`: `kind=run_scheduler_plan`,
  `read_only=true`, `starts_worker=false`, and parity/evidence metadata.

Plan item executability must come from the Phase 13.1 preflight. The plan
cannot decide permission, MCP auth, MCP elicitation, checkpoint, artifact,
interrupted, diagnostics, timeline, or lifecycle truth. It cannot start work,
write transitions, auto-resume, or mutate checkpoint evidence.

### Phase 14.1 Gate: Internal Scheduler Plan DTO

Phase 14.1 implements the plan DTO internally. It remains read-only and backend
only.

Accepted implementation:

- Plan source has `read_only=true` and `starts_worker=false`.
- Plan item `can_schedule` is derived through Phase 13.1 preflight.
- Missing Run/session/turn links and terminal turns remain non-executable.
- Checkpoint resume items remain non-executable until an explicit resumed turn
  exists.
- Checkpoint planning does not acknowledge, discard, resume, or mutate source
  checkpoint evidence.
- Task items stay non-executable until a later task scheduling ownership gate.

### Phase 14.2 Gate: Scheduler Plan Acceptance

Phase 14.2 accepts the internal plan DTO as sufficient before worker design. It
does not expose plan transport.

Next worker design boundary:

- The first worker design must be user-triggered and foreground only.
- It should apply the internal plan and Phase 13.1 preflight to the existing
  session-first `Chat` path.
- It must not auto-resume checkpoints, restore stale actionability, run
  unattended in the background, or make transition rows/current events into
  lifecycle truth.
- It must not move Run or scheduler state into React.

### Phase 15 Gate: Foreground User-turn Scheduler Worker Design

Phase 15 accepts the first worker only as a design. The worker is a foreground
backend delegate for user-triggered `Chat`, not a background queue.

Designed flow:

- `Chat` ensures/selects a session.
- Runtime ensures a durable Run.
- Runtime persists the queued turn.
- Runtime links Run/session/turn.
- Runtime builds an internal scheduler plan for that turn.
- Runtime requires Phase 13.1 preflight to pass.
- Runtime records `turn_started` transition audit.
- Runtime delegates to existing `runChat`.

Rejected behavior:

- Automatic resume.
- Unattended background execution.
- Scheduler-owned permission/MCP/checkpoint/artifact actionability.
- Task scheduling execution.
- Frontend Run management UI.
- Transition/event/React-derived lifecycle truth.

### Phase 15.1 Gate: Foreground Scheduler Delegate

Phase 15.1 wires the designed foreground delegate into `Chat`.

Accepted implementation:

- `Chat` remains the public user-triggered entry point.
- Runtime links Run/session/turn before scheduler plan/preflight.
- `turn_started` transition audit is recorded only after plan/preflight accepts
  the queued turn.
- Accepted turns still delegate to existing `runChat`.
- Rejected turns are marked `failed` before execution starts.
- Rejected turns also mark the in-memory request state finished/failed.
- No queue, poller, automatic resume, task scheduler execution, or frontend Run
  management UI is added.

### Phase 15.2 Gate: Foreground Delegate Acceptance

Phase 15.2 accepts the foreground delegate and chooses checkpoint-resume
hardening as the next boundary.

Next boundary:

- `ResumeRunCheckpoint` must remain an explicit user action.
- The resumed turn should inherit the foreground scheduler delegate through
  `Chat`.
- Source checkpoint evidence must not be acknowledged, discarded, resumed, or
  otherwise mutated by planning.
- No automatic resume or unattended background execution is accepted.

### Phase 16 Gate: Checkpoint Resume Scheduler Delegate

Phase 16 proves explicit checkpoint resume can use the foreground scheduler
delegate without becoming automatic resume.

Accepted implementation:

- Checkpoint plan items are not executable without a concrete explicit resumed
  turn.
- A resumed turn linked through Run/session/turn passes the same foreground
  delegate preflight as other user turns.
- Linking the resumed turn to checkpoint metadata does not acknowledge,
  discard, or mutate source checkpoint evidence.
- No automatic resume, unattended background execution, or frontend Run
  management UI is added.

### Phase 16.1 Gate: Scheduler Delegate Acceptance

Phase 16.1 accepts the foreground delegate slice for user turns and explicit
checkpoint resume.

Next boundary:

- Task scheduling ownership requires its own design gate.
- Task scheduling must not be inferred from transition history, event payloads,
  assistant prose, or React state.
- Agent task stores, Run ownership, cancellation, diagnostics, permission/MCP
  actionability, and artifact evidence boundaries must be defined before
  executable task scheduling is added.

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
