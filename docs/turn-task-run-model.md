# Turn / Task / Run 数据模型

Status: partially implemented design baseline. Runtime turns, ToolCalls,
permissions, audit, recovery, and `RuntimeAgentTask` persistence now exist as
foundations. This document remains useful for object semantics, but several
items below are now implemented rather than future-only.

Current remaining gaps are tracked in:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

Phase 18.1 note:

- Backend contract coverage now validates task cancellation ownership:
  `CancelAgentTask` terminalizes active task row/result/message evidence,
  preserves parent Run/session/turn/tool/task links, leaves already-final task
  and result evidence unchanged, and keeps scheduler task plan items read-only,
  non-executable, and scope-preserving.
- Scheduler-owned task cancellation execution, task scheduler execution,
  automatic resume, unattended worker loops, frontend Run management UI,
  database migration, and transition-derived lifecycle/actionability remain out
  of scope.

Phase 18.2 note:

- Task cancellation ownership is accepted as stable: `CancelAgentTask` remains
  the entry point, cancellation evidence comes from task row/result/message
  stores, and scheduler task plan items remain read-only planning evidence.
- The next safe boundary is a task scheduler execution design gate. Task
  execution remains unimplemented until a later accepted implementation phase.

Phase 19 note:

- Task scheduler execution is accepted only as a design boundary. A future
  delegate must verify parent Run/session/turn/task ownership, enforce task
  scope, keep cancellation owned by `CancelAgentTask` or recorder terminal
  evidence, and treat completed structured task/tool output as the only
  produced-ref source.
- Event payloads, transition history, assistant prose, and React state remain
  refresh signals or presentation state only. They are not task lifecycle,
  artifact, permission/MCP actionability, or Run status truth.

Phase 19.1 note:

- `runtimeRunSchedulerDelegateTaskTurn` now exists as an internal
  rejection-only contract helper. It reads task plan/task row evidence and
  rejects missing, unowned, terminal, cancelled, interrupted, and currently
  non-accepted owned task candidates without execution side effects.
- Task plan items remain non-executable until a later accepted implementation
  phase changes both plan executability and delegate side-effect coverage.

Phase 19.2 note:

- The rejection-only task delegate contract is accepted. Task plan items remain
  non-executable until a later foreground implementation gate explicitly flips
  owned active candidates to executable with tests for ownership, scope,
  cancellation ordering, artifact evidence, and `SessionActivity` parity.
- Worker, queue, automatic resume, frontend Run UI, migration, and
  event/prose/React-derived truth remain out of scope.

Phase 20 note:

- Owned active task plan items with verified parent Run/session/turn ownership
  may now become foreground-schedulable internally. The delegate can accept
  that candidate, but it does not start execution or write lifecycle evidence
  by itself.
- Final, cancelled, interrupted, missing, or unowned task rows remain
  non-executable. Runtime stores/DTO reads remain the source of truth.

Phase 20.1 note:

- Foreground task schedulability now has parity/evidence coverage: delegate
  preflight creates no refs by itself, recorder completed output creates task
  artifact refs, cursor-window events match the full activity evidence, and
  completed task rows become terminal/non-executable again.

Phase 20.2 note:

- Internal foreground task schedulability is accepted. Transport and UI
  exposure remain unaccepted until a separate design gate defines adapter DTOs,
  refresh behavior, and source-of-truth constraints.

Phase 21 note:

- The accepted transport boundary is read-only scheduler plan DTO exposure
  only. The internal task delegate remains backend-only, and no execute/cancel
  action or frontend Run management UI is accepted.
- Event payloads may select DTO refreshes but must not become lifecycle,
  artifact, permission/MCP actionability, or Run status truth.

Phase 21.1 note:

- `RunSchedulerPlan(ctx, RuntimeRunSchedulerPlanRequest)` is now exposed as a
  read-only RuntimeService/HTTP/dev/Wails DTO read.
- It is planning evidence only: it does not start a worker, execute a task,
  cancel a task, mutate task rows, or replace `SessionActivity`,
  `RunProjection`, persisted Run detail, permission/MCP DTOs, or artifact
  evidence as source of truth.
- No worker, queue, automatic resume, frontend Run UI, or migration is accepted
  by this gate.

Phase 21.2 note:

- Read-only scheduler plan transport is accepted. It can inform planning
  affordances, but it is not an execution command and is not lifecycle truth.
- The next boundary, if pursued, is a foreground execute-action design gate
  with explicit ownership, idempotency, cancellation, artifact, permission/MCP,
  and refresh semantics.

Phase 22 note:

- A future task execute action may exist only as an explicit foreground
  user-triggered backend action. It must revalidate scheduler plan/preflight,
  parent Run/session/turn/task ownership, task scope, and cancellation state
  before starting.
- Task execution must be idempotent by task id and must not duplicate turns,
  task messages/results, refs, or lifecycle events.
- Completed structured task output remains the only produced-ref source.
  Cancelled, partial, unfinished, or disconnected task execution must not
  create artifact evidence.
- Background workers, automatic resume, database migrations, frontend Run UI,
  stale permission/MCP actionability recovery, and event/prose/React-derived
  truth remain rejected.

Phase 22.1 note:

- The backend now has an internal task execute contract that revalidates
  scheduler delegate acceptance and returns backend-only source metadata.
- The contract is intentionally non-executing for this phase:
  `executionStarted=false` and `startsWorker=false`.
- Duplicate calls are idempotent before execution implementation and must not
  duplicate turns, task messages/results, refs, events, or lifecycle evidence.

Phase 22.2 note:

- The internal execute contract is accepted. Future foreground execution must
  preserve its revalidation, idempotency, and no-stale-actionability semantics.
- Transport and frontend exposure remain unaccepted.

Phase 22.3 note:

- Internal foreground task start now moves queued tasks to running and records
  start evidence once.
- Running tasks are treated as idempotent duplicates.
- The start action does not produce task results or artifact refs; those remain
  completion-only evidence.

Phase 22.4 note:

- Internal foreground task start is accepted as durable lifecycle evidence.
- Actual child-agent execution remains unimplemented and unexposed through
  transport/UI.

Phase 22.5 note:

- The child-agent foreground runner direction is accepted as a design only.
- Runtime should call a narrow backend-internal runner contract that reuses
  coordinator sub-agent semantics and `AgentTaskRecorder` evidence instead of
  cloning execution logic in runtime.
- The runner remains foreground/request-scoped; background scheduling,
  automatic resume, transport/frontend exposure, stale actionability recovery,
  and event/prose/React-derived truth remain rejected.

Phase 22.6 note:

- A backend-internal, test-injectable child-agent runner contract now exists
  behind `runtimeRunSchedulerExecuteTask`.
- The contract receives durable run/task ownership and scope evidence after
  scheduler revalidation and start recording; runtime re-reads durable task
  state after runner return.
- No real coordinator adapter, transport exposure, frontend Run UI,
  background worker, automatic resume, database migration, or stale
  actionability recovery is implemented.

Phase 22.7 note:

- The backend-internal runner contract is accepted.
- A future real coordinator adapter must execute an already-started runtime
  task through recorder-compatible evidence without duplicating start evidence
  or restoring process-local child agent state after restart.
- Transport/frontend exposure remains blocked until the real adapter is
  implemented and separately accepted.

Phase 23 note:

- The real coordinator foreground runner adapter is planned as a design gate.
- The adapter must execute an already-started runtime task, skip duplicate
  start evidence, reuse coordinator sub-agent semantics, and write terminal
  evidence through recorder-compatible paths.
- Process-local child agent registration may support active foreground
  follow-up/cancel routing, but it must not become durable resume state.

Phase 23.1 note:

- `internal/agent` now has a started-task execution contract that runs an
  already-started child task foreground-only.
- It requires pre-recorded start evidence, skips duplicate start/progress
  recorder writes, uses process-local child registration only during the active
  call, and writes terminal recorder evidence for completed/failed/cancelled
  outcomes.
- Runtime is not wired to the real adapter yet.

Phase 23.2 note:

- The coordinator-side started-task execution contract is accepted.
- Runtime-to-coordinator wiring is still pending and must first design agent
  selection, prompt sourcing, permission/MCP behavior, cancellation ordering,
  and durable re-read semantics.

Phase 23.3 note:

- Runtime-to-coordinator wiring is designed but not implemented.
- The adapter must resolve a real task agent through backend/workspace/
  coordinator ownership, use a durable structured prompt source, fail unknown
  roles terminally, and keep runtime durable re-reads as the source of truth.

Phase 23.4 note:

- Runtime now has an internal, uninstalled coordinator adapter contract.
- Runtime task start writes a structured durable prompt source into the task
  instruction message payload.
- The adapter reads only explicit request prompt or durable instruction payload,
  supports only `config.AgentTask`, and terminally fails unsupported roles or
  missing prompt source without artifact refs.

Phase 23.5 note:

- The runtime-side coordinator adapter contract is accepted.
- Real backend/coordinator executor installation is still pending and must
  prove workspace/coordinator readiness, task-agent selection, cancellation
  ordering, and completed-output-only refs before any transport/UI exposure.

Phase 24 note:

- Real executor installation is designed but not implemented.
- Task-agent construction must stay in coordinator code through a configured
  started-task executor; runtime must not duplicate `buildAgent` or fall back
  to the coder agent.

Phase 24.1 note:

- Coordinator now owns a configured started-task executor contract.
- It builds the task agent through the existing `config.AgentTask` path,
  rejects unsupported roles terminally, and delegates to the started-task
  executor without duplicating start evidence.
- Runtime adapter installation is still pending.

Phase 24.2 note:

- The coordinator configured executor contract is accepted.
- Backend/runtime wiring may call this contract later, but runtime still must
  not construct agents or expose execution through transport/UI.

Phase 24.3 note:

- Backend/runtime executor wiring is designed but not implemented.
- Backend should resolve workspace/coordinator and call the configured
  coordinator executor; runtime should install only a thin executor adapter and
  keep durable re-read semantics after return.

Phase 24.4 note:

- Backend/runtime executor wiring now exists internally and is installed after
  runtime startup has a live backend, workspace id, and DB-backed stores.
- Backend resolves workspace/coordinator and delegates to the coordinator-owned
  configured started-task executor; runtime installs only a thin adapter and
  terminalizes non-terminal backend/coordinator errors through durable failed
  task evidence.
- Runtime still does not construct agents, choose models, expose transport/UI
  execution actions, auto-resume, or treat events/prose/React state as source
  of truth.

Phase 24.5 note:

- The backend/runtime executor wiring contract is accepted as internal-only.
- It remains a controlled delegate for existing explicit scheduler execution,
  not a user-facing Run/task execution feature.
- The next validation boundary is live/fake child-agent execution smoke and
  cancellation behavior, still without transport/UI exposure, background
  workers, automatic resume, or migrations.

Phase 25 note:

- Internal backend/coordinator runner smoke now validates queued task execution
  through the installed runtime runner and backend workspace coordinator path.
- Failed and cancelled task recorder evidence ignores incoming artifact refs,
  so partial/unfinished child output cannot become artifact evidence.
- Live hosted/provider smoke remains credential-gated and must be redacted or
  manual unless covered by safe local fakes.

Phase 25.1 note:

- Real provider/hosted MCP live smoke remains a credential-gated manual gap.
- A redacted local checklist was recorded under ignored `tmp/runtime-dev`;
  durable runtime evidence and deterministic fake coverage remain the current
  source of validation truth.
- No Run persistence, transport/UI exposure, background execution, automatic
  resume, migration, or stale actionability recovery was added.

Phase 25.2 note:

- The internal backend runner track is accepted as ready for a transport
  exposure design gate only.
- User-facing execution controls are still not accepted; the next gate must
  define idempotent action metadata, durable rereads, refresh targets, and
  no event/prose/React source-of-truth behavior before any implementation.

Phase 26 note:

- Explicit scheduler task execution transport is accepted as a design only:
  `POST /v1/runs/{run_id}/tasks/{task_id}/execute` and a matching Wails bridge
  method may delegate to the existing internal scheduler execute request.
- The action response must remain metadata plus refresh targets; clients must
  re-read durable Run/task/activity DTOs.
- Full Run execution, background scheduling, automatic resume, migrations,
  stale actionability recovery, and frontend execution controls remain out of
  scope.

Phase 26.1 note:

- Backend/service HTTP and Wails transport now expose the explicit scheduler
  task execute action.
- The action delegates to the existing internal scheduler execute contract and
  returns metadata plus refresh targets.
- Browser/Wails workbench adapter exposure and visible frontend controls remain
  unimplemented.

Phase 26.2 note:

- Hidden workbench adapter exposure is accepted as a contract only.
- The adapter must call the explicit execute action and then re-read durable
  DTOs; it must not derive UI state from action responses or runtime events.
- Visible frontend execution controls remain out of scope.

Phase 26.3 note:

- Hidden workbench adapter support for explicit scheduler task execution now
  exists.
- The adapter calls the action and then rehydrates durable DTOs; visible UI
  controls remain unimplemented.

Phase 26.4 note:

- The future scheduler execute control is accepted only after a durable
  scheduler task candidate read model exists.
- The row, not React state/events/action responses, must provide execution
  eligibility, scheduler status, ownership evidence, and non-secret denial
  reason.
- Visible controls remain out of scope until that read model is defined and
  proven to preserve `SessionActivity`/Run projection parity.

Phase 26.5 note:

- Scheduler task candidates are accepted as read-only view-model rows derived
  from durable `RunSchedulerPlan` items.
- `executeEligible` maps only from durable `canSchedule`; terminal or failed
  preflight rows remain non-actionable diagnostics and must not resurrect stale
  permission/MCP actionability.
- Full `SessionActivity` remains the fallback and parity oracle for timeline,
  diagnostics, artifacts, interrupted summaries, and terminal permission/MCP
  semantics.

Phase 26.6 note:

- The frontend now has hidden scheduler candidate DTO/read support under the
  Run projection view model.
- Candidate rows are hydrated from durable Run projection task IDs plus
  `RunSchedulerPlan` reads and are keyed by stable `runID:taskID`.
- No visible execution UI is implemented; future UI must still consume these
  durable rows and re-read after any explicit action.

Phase 26.7 note:

- Event-triggered scheduler candidate refresh is covered as a contract:
  runtime events schedule durable rereads only.
- Duplicate terminal or artifact/ref events must not duplicate candidate rows
  or resurrect stale permission/MCP actionability because candidate state is
  keyed and hydrated from durable reads.

Phase 26.8 note:

- A minimal visible execute control is accepted only as a consumer of durable
  scheduler candidate rows.
- The UI may hold local pending/error affordance for the clicked action, but it
  must not persist or infer task lifecycle, artifact, permission, MCP
  actionability, or Run state.

Phase 26.9 note:

- The first visible scheduler execute affordance exists as a consumer of
  durable scheduler candidate rows.
- Execution clicks delegate to the adapter action and then durable hydration;
  action response metadata and local pending/error affordance are not runtime
  state.

Phase 26.10 note:

- Visible scheduler execute behavior is covered by a fixture smoke for queued,
  terminal/blocked, and duplicate candidate evidence.
- Live click validation remains pending until a runtime-owned durable candidate
  seed exists; React must not fabricate that source state.

Phase 26.11 note:

- Live scheduler click validation should seed durable Run/Turn/AgentTask
  evidence through runtime stores in a temp environment.
- No production seed endpoint, migration, React fixture mode, background
  scheduler, or auto-resume behavior is accepted for this validation.

Phase 26.12 note:

- Runtime-owned durable seed coverage now verifies queued and terminal
  AgentTask candidates through scheduler plan and execute transport.
- The validation preserves Run projection readiness guards and leaves full
  browser clicking for a normal ready runtime setup.

Phase 26.13 note:

- End-to-end browser clicking remains manual/local until runtime readiness can
  be satisfied through an accepted non-secret provider/config fixture.
- The model boundary remains unchanged: candidate state must come from runtime
  Run/Turn/AgentTask evidence, not frontend fixtures.

Phase 26.14 note:

- The scheduler execute track is accepted through explicit transport, durable
  candidate reads, minimal visible UI, and runtime-owned seed smoke.
- Full browser click automation is deferred to a separate provider/config
  readiness gate. It must not introduce Run persistence expansion, migrations,
  auto-resume, background scheduling, or frontend-owned candidate state.

Phase 27 note:

- Provider/config readiness automation is accepted as a separate gate only.
- Any browser click automation must make the runtime ready through normal local
  config and durable runtime evidence, not a readiness bypass or frontend-owned
  fixture state.

Phase 27.1 note:

- Local test-provider readiness is now covered through normal runtime config:
  temp `model.json`, loopback fake provider, and durable Run/Turn/AgentTask
  evidence.
- This closes the RunProjection readiness part of browser click automation
  without adding frontend-owned candidates or new persistence semantics.

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

### Phase 17 Gate: Task Scheduling Ownership

Phase 17 defines task scheduling ownership without implementing task execution.

Accepted model:

- A task requires parent Run/session/turn ownership before it can become
  executable scheduler work.
- Existing `runtime_agent_tasks`, task messages, task results, and completed
  task/tool refs remain structured evidence.
- Task plan items must preserve allowed tools, capability scope, worktree/cwd,
  role, provider/model, and parent tool-call linkage.
- Task cancellation and recovery remain owned by current task stores and
  runtime recovery until a later implementation gate.
- Task lifecycle or artifact evidence must not be inferred from transition
  history, event payloads, assistant prose, or React state.

### Phase 17.1 Gate: Task Scheduler Plan/Preflight

Phase 17.1 implements read-only task scheduler plan/preflight coverage.

Accepted implementation:

- Task plan items read `runtime_agent_tasks`.
- Task plan items verify parent Run/session/turn ownership through the existing
  scheduler preflight.
- Valid ownership sets `ownership_verified=true`, but task items still remain
  non-executable because task scheduler execution is not accepted.
- Task plan items preserve allowed tools, capability scope, worktree/cwd, role,
  provider/model, parent tool-call id, and child session id.
- Planning does not mutate task state, widen scope, produce artifact evidence,
  or change cancellation/recovery ownership.

### Phase 17.2 Gate: Task Plan Acceptance

Phase 17.2 accepts read-only task planning and chooses task cancellation
ownership as the next boundary.

Accepted review:

- `ownership_verified` is not executability.
- Task scheduler execution remains unimplemented.
- `CancelAgentTask` remains the current cancellation entry point.
- Future scheduler task items must not own cancellation actionability until a
  separate cancellation ownership gate is accepted.

### Phase 18 Gate: Task Cancellation Ownership

Phase 18 defines task cancellation ownership without implementing scheduler-owned
task cancellation.

Accepted model:

- `CancelAgentTask` remains the cancellation entry point.
- Task row, task result, and task message evidence are cancellation truth.
- Run task transition audit can record ordering only after terminal task
  evidence exists.
- Cancelling an already-final task must not rewrite final status, refs, or
  result evidence.
- Future scheduler task items may describe cancellation scope, but they must not
  decide cancellation actionability or execute cancellation.

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
