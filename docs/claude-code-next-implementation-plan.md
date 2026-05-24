# Claude Code Next Implementation Plan

Status: refreshed on 2026-05-24. This plan is based on the full parity review:

- [`docs/claude-code-full-parity-review.md`](./claude-code-full-parity-review.md)
- [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
- [`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md)

This is a planning document only. Do not use it to justify React-owned runtime
state, `charm.land/fantasy` changes, provider rewrites, or TUI/CLI restoration.

## Implementation Principles

- Runtime first, then pages.
- Go runtime owns business state and lifecycle.
- React renders runtime facts and holds UI-only state.
- Wails and HTTP remain adapters.
- `charm.land/fantasy` remains the provider/model/tool protocol abstraction.
- Compact, tool discovery, policy, AgentTask, MCP/skills, worktree, audit, and
  replay are runtime primitives.
- Model-assisted permission is advisory-only and cannot approve actions.
- UI features missing runtime APIs must be marked blocked by runtime API.

## First Module To Implement

```text
runtime: full compact and post-compact reinjection
```

The older plan said the next product module could be a Claude-client-inspired
React shell. The current code review changes that priority. Agent Builder now
has enough runtime UI surfaces, but long-session runtime parity still depends
on full compact, reinjection, and replay hardening. Page work should wait.

### Why This Beats Other Candidates

- Compact boundary, budget accounting, and micro compact already exist in
  `internal/runtime/runtime_compact*.go` and `runtime_budget.go`.
- Claude Code still has a richer lifecycle in `src/services/compact/*`:
  micro/full/session-memory/auto compact and post-compact cleanup/reinjection.
- Tool search, AgentTask, worktree, replay, and React diagnostics all benefit
  from compact-aware transcript and context state.
- Coordinator work before compact hardening would increase recursion and
  transcript volume risk.
- React diagnostics before runtime compact APIs would force the client to infer
  state from messages, violating the source-of-truth rule.

## Phase Map

| Phase | Status | Modules |
| --- | --- | --- |
| P0 | Completed foundation | Runtime spine, scheduler, event cursor, audit, recovery, ToolCall store, policy baseline, capability inventory. |
| P1 Next | Runtime | Full compact, post-compact reinjection, compact-aware recovery. |
| P1 Next | Runtime | Persisted event replay/export and broader scenario harness. |
| P1 Parallel | Runtime | Tool discovery guardrails, policy profiles/headless semantics, shell parser hardening. |
| P2 | Runtime | AgentTask coordinator mailbox, output/artifact refs, background job entity, MCP auth/elicitation, worktree hardening. |
| P2 Later | React diagnostics | Compact/replay/task/policy/artifact/worktree diagnostics after runtime APIs. |
| P3 Later | Runtime/product | Sandbox/remote, plugin governance, advisory permission advisor, advanced memory lifecycle. |
| Not needed | Excluded | TUI/CLI UI, slash UI, fantasy/provider rewrite, marketplace-first distribution. |

## Module Boundaries

### Full Compact And Post-Compact Reinjection

| Field | Plan |
| --- | --- |
| Goal | Extend the existing compact boundary/budget/micro compact foundation into full compact summaries, auto-trigger metadata, compact-aware recovery, and reinjection of required instructions/read files/context sources. |
| Non-goals | No React-owned compact state, no fantasy changes, no opaque summaries without provenance, no cloud memory service. |
| Go packages | `internal/runtime/runtime_compact*.go`, `runtime_budget.go`, `runtime_context.go`, `runtime_recovery.go`, `runtime_replay_export.go`, `internal/agent/prompt`, `internal/db/read_files.sql.go`, `internal/message`. |
| React packages | Later read-only diagnostics in `client/src/runtime/types.ts`, `client/src/runtime/api.ts`, `client/src/features/audit`, `client/src/features/chat`. |
| Runtime API / event schema | Existing `TurnCompactBoundaries` and `SessionCompactBoundaries` should expand with full compact and reinjection fields. Add or complete events `compact.full.completed`, `compact.auto.triggered`, `context.reinjected`, `context.source.skipped`, `compact.failed`. |
| Data model changes | Extend `runtime_compact_boundaries` only if current `summary_ref`, `message_refs_json`, `tool_call_refs_json`, and `reinjected_refs_json` are insufficient. Avoid a second compact table unless summaries need durable blob refs. |
| Tests | Compact store tests, message/tool-call invariant tests, read-file reinjection tests, context precedence tests, recovery snapshot tests, replay export tests with redaction. |
| Acceptance | Full compact boundaries survive restart; source refs explain what was summarized and reinjected; recovery/replay exposes compact state; tool-use/tool-result pairing is preserved; no secrets leak into summaries. |
| Risks | Losing instructions, stale file reinjection, breaking provider transcript invariants, leaking secrets in summaries. |
| Blocked by | Existing compact boundary, budget, micro compact, context audit, read-file state. These foundations exist. |
| Unlocks | Auto compact, session memory compact, compact-aware AgentTask transcripts, compact diagnostics UI. |

### Persisted Event Replay And Scenario Harness

| Field | Plan |
| --- | --- |
| Goal | Move replay beyond bounded in-memory events by adding a durable event replay source and expanding golden scenarios for compact, policy, tool search, MCP, skills, AgentTask, worktree, and recovery. |
| Non-goals | No external telemetry sink, no first-party analytics import, no replay as live runtime state. |
| Go packages | `internal/runtime/runtime_events.go`, `runtime_replay_export.go`, `runtime_audit*.go`, `runtime_scenario_harness_test.go`, `internal/db` migrations if persisted events need a table. |
| React packages | Later `client/src/features/audit` replay/export diagnostics. |
| Runtime API / event schema | Keep event types stable. Add persisted replay/export read paths if current `ReplayExport` cannot guarantee long-session coverage. |
| Data model changes | Prefer `runtime_events` append table with sequence, ids, type, refs, redacted payload, created_at, if bounded buffer remains insufficient. |
| Tests | Cursor gap tests, persisted replay after service restart, redaction fixtures, golden scenarios covering policy/compact/MCP/skills/tasks/worktree. |
| Acceptance | Replay export can explain a completed turn after event buffer rollover; secrets are redacted; scenario harness fails on regressions in compact/policy/tool discovery/recovery. |
| Risks | Duplicating audit semantics, storing sensitive payloads, brittle golden fixtures. |
| Blocked by | Existing audit/events/replay export foundation. |
| Unlocks | React replay diagnostics, safer advisory permission work, confidence for coordinator/task expansion. |

### Tool Discovery Guardrails

| Field | Plan |
| --- | --- |
| Goal | Harden existing tool search and disclosure with per-source recursion limits, clearer concurrency policy, and scenario coverage for built-in/MCP/skill/AgentTask scopes. |
| Non-goals | No React-selected tool sets, no marketplace flow, no policy bypass. |
| Go packages | `internal/runtime/runtime_tool_search.go`, `internal/agent/tool_search.go`, `internal/agent/loop_detection.go`, `internal/tools/scheduler`, `internal/permission`. |
| React packages | Later capability diagnostics only. |
| Runtime API / event schema | Existing `SearchTools`, `tool.search.performed`, `tool.discovery.selected`, `tool.discovery.omitted`, `scheduler.deadlock.prevented`. Add reason fields only if needed. |
| Data model changes | None expected; audit can carry guardrail evidence. |
| Tests | Repeated search, max searches, concurrent tool limit, tool search recursion, MCP disabled/policy denied, AgentTask scope denial. |
| Acceptance | Deferred tools are discoverable; denied/disabled tools are omitted with reasons; recursion/deadlock guardrails are auditable. |
| Risks | Hiding required tools, over-blocking valid nested flows, exposing sensitive metadata. |
| Blocked by | Existing capability registry, policy, tool search foundation. |
| Unlocks | Larger MCP/skills/plugin surfaces and safer coordinator workflows. |

### Policy Profiles, Headless Semantics, Shell Parser Hardening

| Field | Plan |
| --- | --- |
| Goal | Complete deterministic policy semantics around profiles, headless behavior, rule diagnostics, and Bash/PowerShell destructive/read-only parsing. |
| Non-goals | No model-approved permissions, no enterprise RBAC first pass, no UI risk classification. |
| Go packages | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go`, `runtime_permissions.go`, `runtime_scheduler_recorder.go`, shell tools. |
| React packages | Later policy diagnostics/rule editor only after runtime semantics settle. |
| Runtime API / event schema | Existing `/v1/policy` shape has rules/diagnostics/profile. Ensure policy events include matched rule, scope, shell risk, and headless failure reason. |
| Data model changes | Policy config file may be enough; add persisted policy decision snapshots only if replay needs them beyond audit. |
| Tests | Policy table tests, headless ask fail-closed tests, shell fixture pack for Bash/PowerShell/cmd, scoped MCP/skill/subagent/cwd/path rules. |
| Acceptance | Ask in headless contexts fails closed; destructive shell is reliably denied/asked per mode; diagnostics explain invalid/duplicate rules; model assistance remains absent or advisory-only. |
| Risks | Shell parser false negatives, rule precedence surprises, allowing skills/MCP to expand permissions. |
| Blocked by | Existing scoped policy foundation. |
| Unlocks | MCP auth/elicitation, AgentTask coordinator safety, advisory permission advisor later. |

### AgentTask Coordinator Mailbox

| Field | Plan |
| --- | --- |
| Goal | Upgrade current AgentTask messages/results into a full runtime mailbox compatible with parent/child and coordinator/teammate communication semantics. |
| Non-goals | No swarm UI first, no UI event stitching, no remote fleet. |
| Go packages | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_task_comm_store.go`, `runtime_agent_task_scope.go`, `runtime_agent_roles.go`, `internal/agent/agent_tool.go`, `internal/agent/coordinator.go`. |
| React packages | Later task panel in `client/src/features/chat` or a runtime diagnostics feature. |
| Runtime API / event schema | Existing task message/result APIs and events should add delivery/ack semantics if required. Add coordinator routing events only in runtime. |
| Data model changes | Extend task messages with mailbox delivery state if current `status`/`delivered_at` is insufficient. |
| Tests | Parent-to-child messages, child-to-parent results, cancellation, scope denial, compact refs, replay export, task notification scenarios. |
| Acceptance | Parent and child sessions communicate through durable runtime records; task output is auditable/replayable; scopes cannot be exceeded. |
| Risks | Recursion loops, unbounded context, ambiguous transcript ownership. |
| Blocked by | Full compact/reinjection, persisted replay, policy hardening. |
| Unlocks | Coordinator mode, teammate workflows, task panels. |

### Output / Artifact Refs And Background Job Entity

| Field | Plan |
| --- | --- |
| Goal | Store large outputs, diffs, artifacts, and background shell job state as durable runtime refs instead of only summaries. |
| Non-goals | No frontend artifact inference, no direct React tool execution. |
| Go packages | `internal/tools/scheduler`, `internal/runtime/runtime_tool_calls.go`, `runtime_tool_call_store.go`, shell tools `internal/agent/tools/bash.go`, `job_output.go`, `job_kill.go`, `internal/db`. |
| React packages | Later artifact/detail drawer. |
| Runtime API / event schema | Add read APIs for output/artifact refs and events for job started/output/completed/failed if current tool events are insufficient. |
| Data model changes | Add artifact/output ref table or extend ToolCall metadata with durable blob/file refs. |
| Tests | Large output storage, compact preserves original refs, job output after restart where feasible, redaction. |
| Acceptance | Large output can be compacted from model view while still inspectable by runtime API; artifacts are durable and replayable. |
| Risks | Secret leakage, path safety, unbounded storage growth. |
| Blocked by | Compact/replay hardening. |
| Unlocks | React artifact drawer and robust background job diagnostics. |

### MCP Auth / Elicitation

| Field | Plan |
| --- | --- |
| Goal | Add runtime lifecycle for MCP auth/OAuth-like flows and elicitation without making React the business owner. |
| Non-goals | No Claude.ai product login, no marketplace-first install. |
| Go packages | `internal/runtime/runtime_mcp*.go`, `internal/agent/tools/mcp/*`, `internal/permission`. |
| React packages | Later auth/elicitation approval UI. |
| Runtime API / event schema | Add `mcp.auth.requested`, `mcp.auth.completed`, `mcp.elicitation.requested`, `mcp.elicitation.completed`, with redacted payloads. |
| Data model changes | Add auth state records only if credentials/session state must survive restart; secrets must stay in configured secret storage or redacted config. |
| Tests | Auth required, denied auth, elicitation prompt/response, redaction, disabled server policy. |
| Acceptance | MCP auth and elicitation are recoverable and auditable; React only submits user responses. |
| Risks | Credential leakage, hanging MCP sessions, policy bypass. |
| Blocked by | Policy hardening and persisted replay. |
| Unlocks | Broader MCP parity and diagnostics. |

### Worktree Hardening

| Field | Plan |
| --- | --- |
| Goal | Deepen the existing worktree lifecycle into task-aware cwd isolation with cleanup recovery and policy/audit coverage. |
| Non-goals | No OS sandbox in this phase, no remote runtime, no single `isolated` boolean. |
| Go packages | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go`, `runtime_agent_task_scope.go`, `internal/permission`, shell tools. |
| React packages | Later worktree diagnostics/cleanup UI. |
| Runtime API / event schema | Existing worktree events should remain. Add recovery/cleanup diagnostics if needed. |
| Data model changes | Existing `runtime_worktrees` table is the baseline; extend only for cleanup attempts or failure provenance. |
| Tests | Path traversal, owner validation, cleanup/preserve, task scope CWD/worktree denial, recovery of missing worktree path. |
| Acceptance | Runtime-owned worktrees are safe to create/enter/exit/cleanup; task effective scope is consistent after restart. |
| Risks | Data loss through cleanup, Windows path edge cases, stale cwd. |
| Blocked by | Policy/shell hardening and AgentTask mailbox direction. |
| Unlocks | Sandbox/remote later. |

### React Diagnostics Later

| Field | Plan |
| --- | --- |
| Goal | Expose compact, replay, policy, task mailbox, worktree, and artifact diagnostics after runtime APIs exist. |
| Non-goals | No React-owned business state, no frontend-derived audit, no UI permission classifier. |
| Go packages | Runtime APIs above must exist first. |
| React packages | `client/src/runtime/*`, `client/src/features/audit/*`, `client/src/features/chat/*`, `client/src/features/capabilities/*`, `client/src/features/permissions/*`, `client/src/features/mcp/*`, `client/src/features/skills/*`. |
| Runtime API / event schema | Consume runtime APIs/events only. Refresh from API after events. |
| Data model changes | None in React. TypeScript DTOs mirror Go contracts. |
| Tests | Client build/type checks, reload/recovery smoke, blocked-state rendering for unavailable APIs. |
| Acceptance | UI can be rebuilt from runtime APIs after reload; unsupported surfaces are explicitly blocked. |
| Risks | Reducers becoming source of truth, event-only reconstruction, invented artifacts/task state. |
| Blocked by | Runtime compact/replay/task/artifact/policy APIs. |
| Unlocks | Product diagnostics and review workflows. |

## Commit Plan

Recommended future implementation sequence:

1. `runtime: add full compact summaries`
2. `runtime: reinject compacted context sources`
3. `runtime: persist replay events`
4. `runtime: expand scenario replay harness`
5. `runtime: harden tool discovery guardrails`
6. `runtime: add policy profiles and headless semantics`
7. `runtime: expand shell safety fixtures`
8. `runtime: add agent task mailbox delivery`
9. `runtime: add output artifact refs`
10. `runtime: add mcp auth elicitation lifecycle`
11. `runtime: harden worktree recovery and cleanup`
12. `client: expose compact replay task diagnostics`

## Acceptance Checklist For This Planning Phase

- Roadmap points to the full parity review.
- Audit reflects current code instead of older missing-module conclusions.
- Every required module is covered.
- Completed, partial, missing, not-needed, and later items are separated.
- Runtime blockers are distinguished from React diagnostics and P3 product
  optimization.
- Runtime continues to be prioritized before page work.
- Mermaid dependency graph exists in the roadmap/full review.
- Only docs are changed in this session.
