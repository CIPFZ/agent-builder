# Claude Code Next Implementation Plan

Status: refreshed on 2026-05-27. This plan is based on the full parity review
and the AgentTask/coordinator communication pass:

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
- Compact, tool discovery, policy, AgentTask, MCP/skills, worktree, hooks,
  audit, and replay are runtime primitives.
- Model-assisted permission is advisory-only and cannot approve actions.
- UI features missing runtime APIs must be marked blocked by runtime API.

## First Module To Implement

```text
runtime: parity stabilization and re-audit
```

The older plan said the next product module could be a Claude-client-inspired
React shell or compact-first hardening. The current code has since landed
compact/reinjection, persisted replay events, tool discovery guardrails,
policy/headless semantics, completed AgentTask/coordinator communication,
output/artifact refs, MCP auth/elicitation, worktree recovery/cleanup, sandbox
boundaries, context loading, and hook lifecycle records. Page work should wait
unless it is only mirroring runtime DTOs.

### Why This Beats Other Candidates

- Cross-boundary regressions are now the main risk: hooks must not bypass
  policy/headless/scope/sandbox/MCP, compact/replay must preserve refs and
  provenance, and AgentTask/worktree scope must remain deterministic.
- Runtime APIs exist for most diagnostics; React should mirror them instead of
  deriving state from messages.
- React diagnostics are safer after broader fixture coverage proves the current
  runtime boundaries.

## Phase Map

| Phase | Status | Modules |
| --- | --- | --- |
| P0 | Completed foundation | Runtime spine, scheduler, event cursor, audit, recovery, ToolCall store, policy baseline, capability inventory. |
| P1 Next | Runtime | Runtime parity stabilization/re-audit with broader scenario fixtures across hooks, policy/headless, MCP, AgentTask communication, sandbox/worktree, compact/replay, and refs. |
| P1 Parallel | Runtime | Shell parser fixture expansion and diagnostics DTO gap checks. |
| P2 | React diagnostics | Compact/replay/task/policy/artifact/worktree/hook diagnostics after stabilization. |
| P3 Later | Runtime/product | Sandbox/remote, plugin governance, advisory permission advisor, advanced memory lifecycle. |
| Not needed | Excluded | TUI/CLI UI, slash UI, fantasy/provider rewrite, marketplace-first distribution. |

## Module Boundaries

### Runtime Parity Stabilization And Re-Audit

| Field | Plan |
| --- | --- |
| Goal | Re-audit runtime parity and expand golden scenarios for hooks, policy/headless, tool search, MCP, skills, AgentTask communication, sandbox/worktree, compact/replay, refs, and recovery. |
| Non-goals | No external telemetry sink, no first-party analytics import, no replay as live runtime state. |
| Go packages | `internal/runtime/runtime_scenario_harness_test.go`, `runtime_replay_export.go`, `runtime_audit*.go`, `internal/agent/*`, `internal/hooks/*`, `internal/permission/*`. |
| React packages | None unless mirroring DTOs. |
| Runtime API / event schema | Keep event types stable; add fields only when a scenario cannot explain a runtime decision. |
| Data model changes | Avoid new tables unless fixture gaps expose missing runtime-owned state. |
| Tests | Hook bypass attempts, headless ask fail-closed, MCP pending/denied, AgentTask follow-up/stop/message ordering, AgentTask cwd/worktree scope, sandbox denial, compact reinjection, output/artifact refs, replay redaction. |
| Acceptance | Scenario harness fails on regressions across authority boundaries and replay/recovery explain the decision path with redacted summaries and ordered task communication. |
| Risks | Duplicating audit semantics, storing sensitive payloads, brittle golden fixtures. |
| Blocked by | Existing runtime primitives are now present. |
| Unlocks | React diagnostics and safer advisory permission work. |

### Tool Discovery Guardrails

| Field | Plan |
| --- | --- |
| Goal | Continue hardening existing tool search and disclosure with scenario coverage for built-in/MCP/skill/AgentTask scopes. |
| Non-goals | No React-selected tool sets, no marketplace flow, no policy bypass. |
| Go packages | `internal/runtime/runtime_tool_search.go`, `internal/agent/tool_search.go`, `internal/agent/loop_detection.go`, `internal/tools/scheduler`, `internal/permission`. |
| React packages | Later capability diagnostics only. |
| Runtime API / event schema | Existing `SearchTools`, `tool.search.performed`, `tool.discovery.selected`, `tool.discovery.omitted`, `scheduler.deadlock.prevented`. Add reason fields only if needed. |
| Data model changes | None expected; audit can carry guardrail evidence. |
| Tests | Repeated search, max searches, concurrent tool limit, tool search recursion, MCP disabled/policy denied, AgentTask scope denial. |
| Acceptance | Deferred tools are discoverable; denied/disabled tools are omitted with reasons; recursion/deadlock guardrails are auditable. |
| Risks | Hiding required tools, over-blocking valid nested flows, exposing sensitive metadata. |
| Blocked by | Existing capability registry, policy, and tool search foundation exist. |
| Unlocks | Larger MCP/skills/plugin surfaces and safer coordinator workflows. |

### Shell Parser Fixture Expansion

| Field | Plan |
| --- | --- |
| Goal | Expand Bash/PowerShell/cmd destructive/read-only fixtures around the deterministic policy engine. |
| Non-goals | No model-approved permissions, no enterprise RBAC first pass, no UI risk classification. |
| Go packages | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go`, `runtime_permissions.go`, `runtime_scheduler_recorder.go`, shell tools. |
| React packages | Later policy diagnostics/rule editor only after runtime semantics settle. |
| Runtime API / event schema | Existing `/v1/policy` shape has rules/diagnostics/profile. Add fields only if fixtures expose missing explanation. |
| Data model changes | Policy config file may be enough; add persisted policy decision snapshots only if replay needs them beyond audit. |
| Tests | Policy table tests, headless ask fail-closed tests, shell fixture pack for Bash/PowerShell/cmd, scoped MCP/skill/subagent/cwd/path rules. |
| Acceptance | Destructive shell is reliably denied/asked per mode; diagnostics explain matched rules; model assistance remains absent or advisory-only. |
| Risks | Shell parser false negatives, rule precedence surprises, allowing skills/MCP to expand permissions. |
| Blocked by | Existing scoped policy/headless foundation exists. |
| Unlocks | AgentTask fixture confidence and advisory permission advisor later. |

### AgentTask Coordinator Communication

Completed as a local runtime primitive. The runtime now owns task messages,
delivery/processed/rejected status, follow-up delivery to child sessions,
stop/cancel control records, task output/artifact refs, replay/recovery, HTTP
and Wails DTOs, and model-facing `task_list`, `task_get`, `task_message`,
`task_stop`, and `task_output` tools. Remote teammate/fleet semantics remain
P3/later and are not a current parity blocker.

### React Diagnostics Later

| Field | Plan |
| --- | --- |
| Goal | Expose compact, replay, policy, task mailbox, hooks, worktree, and artifact diagnostics from runtime APIs. |
| Non-goals | No React-owned business state, no frontend-derived audit, no UI permission classifier. |
| Go packages | Runtime APIs above must exist first. |
| React packages | `client/src/runtime/*`, `client/src/features/audit/*`, `client/src/features/chat/*`, `client/src/features/capabilities/*`, `client/src/features/permissions/*`, `client/src/features/mcp/*`, `client/src/features/skills/*`. |
| Runtime API / event schema | Consume runtime APIs/events only. Refresh from API after events. |
| Data model changes | None in React. TypeScript DTOs mirror Go contracts. |
| Tests | Client build/type checks, reload/recovery smoke, blocked-state rendering for unavailable APIs. |
| Acceptance | UI can be rebuilt from runtime APIs after reload; unsupported surfaces are explicitly blocked. |
| Risks | Reducers becoming source of truth, event-only reconstruction, invented artifacts/task state. |
| Blocked by | DTO mirror and scenario confidence, not React-owned state. |
| Unlocks | Product diagnostics and review workflows. |

## Commit Plan

Recommended future implementation sequence:

1. `runtime: parity stabilization and re-audit`
2. `runtime: mirror diagnostics DTO gaps`
3. `runtime: expand shell safety fixtures`
4. `client: expose runtime diagnostics read-only`

## Acceptance Checklist For This Planning Phase

- Roadmap points to the full parity review.
- Audit reflects current code instead of older missing-module conclusions.
- Every required module is covered.
- Completed, partial, missing, not-needed, and later items are separated.
- Runtime blockers are distinguished from React diagnostics and P3 product
  optimization.
- Runtime continues to be prioritized before page work.
- Mermaid dependency graph exists in the roadmap/full review.
- AgentTask/coordinator communication is recorded as completed or any future
  gap is explicitly scoped to P3 remote/product semantics.
