# Claude Code Alignment Next Roadmap

Status: refreshed on 2026-05-27 after AgentTask/coordinator communication
completion against current Agent Builder `main` and the local Claude Code
source snapshot.

Primary reference:

- [`docs/claude-code-full-parity-review.md`](./claude-code-full-parity-review.md)

Supporting docs:

- [`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md)
- [`docs/claude-code-next-implementation-plan.md`](./claude-code-next-implementation-plan.md)
- [`docs/claude-code-alignment-module-priority.md`](./claude-code-alignment-module-priority.md)
- [`docs/client-runtime-architecture-review.md`](./client-runtime-architecture-review.md)
- [`docs/turn-task-run-model.md`](./turn-task-run-model.md)
- [`docs/tool-scheduler-design.md`](./tool-scheduler-design.md)
- [`docs/permission-policy-model.md`](./permission-policy-model.md)
- [`docs/client-state-recovery.md`](./client-state-recovery.md)
- [`docs/archive/phase-2-runtime-api-boundary.md`](./archive/phase-2-runtime-api-boundary.md)
- [`docs/client-architecture-and-core-flow.md`](./client-architecture-and-core-flow.md)

## Fixed Principles

- Runtime first, then pages.
- Go runtime is the source of truth.
- React is presentation, input, diagnostics, and product workflow only.
- Wails and HTTP are adapters.
- `charm.land/fantasy` is the provider/model/tool protocol abstraction and must
  not be rewritten.
- Compact, tool discovery, policy, AgentTask, MCP/skills, worktree, hooks,
  audit, and replay are runtime primitives.
- Model-assisted permission is advisory-only and cannot self-approve high-risk
  tool use.
- Coordinator/agent communication is a runtime primitive, not UI event
  stitching.
- Provider/model config belongs above fantasy as product policy/config.

## Current Baseline

The current code is ahead of the older roadmap. Several items previously listed
as missing are now runtime foundations.

| Area | Status | Evidence |
| --- | --- | --- |
| Runtime spine: turns, ToolCalls, events, audit, recovery | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_tool_call_store.go`, `runtime_events.go`, `runtime_audit.go`, `runtime_recovery.go` |
| Tool scheduler integration | Completed foundation | `internal/tools/scheduler/*`, `internal/agent/scheduler_tool.go`, `runtime_scheduler_recorder.go` |
| Permission policy with scoped rules and shell safety | Implemented foundation | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go` |
| Compact boundary, budget, reinjection | Implemented foundation | `internal/runtime/runtime_compact*.go`, `runtime_budget.go` |
| Tool search/discovery and guardrails | Implemented foundation | `internal/agent/tool_search.go`, `internal/runtime/runtime_tool_search.go`, `internal/agent/loop_detection.go` |
| Capability registry and lazy refresh state | Implemented foundation | `internal/runtime/runtime_capabilities.go` |
| MCP lifecycle, auth/elicitation, policy filtering | Implemented foundation | `internal/runtime/runtime_mcp*.go`, `internal/agent/tools/mcp/*` |
| Skills activation metadata | Implemented foundation | `internal/skills/*`, `internal/runtime/runtime_skill_activation.go` |
| AgentTask roles, scopes, messages, results, follow-up, stop, output | Completed local runtime primitive | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_task_tools.go`, `runtime_agent_roles.go`, `runtime_agent_task_scope.go`, `runtime_agent_task_comm_store.go`, `internal/agent/task_tools.go`, `internal/agent/coordinator.go` |
| Worktree lifecycle and sandbox records | Implemented foundation | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go` |
| Hooks lifecycle | Implemented foundation | `internal/hooks/*`, `internal/agent/hooked_tool.go`, `internal/runtime/runtime_hooks.go` |
| Replay export and scenario harness | Implemented foundation | `internal/runtime/runtime_replay_export.go`, `runtime_scenario_harness_test.go` |
| React runtime boundary | Partial | `client/src/runtime/*`, `client/src/features/*` |

## Reordered Roadmap

The next phase is runtime parity stabilization/re-audit, not a React shell-first
phase. React diagnostics should follow stable runtime APIs.

| Priority | Module | Status | Why |
| --- | --- | --- | --- |
| P0 Completed | Runtime spine: Turn, ToolCall, Event, Audit, Recovery | Completed | Stable foundation already present. |
| P0 Completed | Tool scheduler baseline | Completed | Scheduler records lifecycle, policy, events, audit. |
| P0 Completed | Deterministic policy baseline | Completed foundation | Modes and scoped rules exist. |
| P1 Next | Runtime parity stabilization and re-audit | Next runtime | Cross-boundary primitives exist, including AgentTask communication; regression coverage should expand before product UI depends on them. |
| P1 Parallel | Shell parser fixture expansion | Parallel runtime | Deterministic policy exists; Bash/PowerShell/cmd coverage should keep growing. |
| P2 | React compact/task/policy/replay/hook diagnostics | After stabilization | React should expose runtime facts only. |
| P3 Later | Sandbox and remote runtime | Later | Depends on shell policy, worktree, task scope. |
| P3 Later | Capability package/plugin governance | Later | Needs stable registry, tool search, scoped policy, MCP/skills lifecycle. |
| P3 Later | Advisory permission advisor | Later | Must wait for deterministic scopes and evals. |
| Not needed | TUI/CLI UI, slash UI, provider rewrite, marketplace-first | Not needed | Explicitly excluded. |

## Dependency Graph

```mermaid
graph TD
  SP["Completed: runtime spine"] --> FX["Next: runtime parity stabilization/re-audit"]
  SP --> DIAGDTO["Next: diagnostics DTO mirror"]

  HOOK["Implemented: hooks lifecycle"] --> FX
  POL["Implemented: policy/headless"] --> FX
  MCP["Implemented: MCP auth/elicitation"] --> FX
  WT["Implemented: worktree/sandbox boundary"] --> FX
  CTX["Implemented: context + compact reinjection"] --> FX
  REF["Implemented: output/artifact refs"] --> FX

  AT["Completed: AgentTask communication"] --> FX

  DIAGDTO --> DIAG["P2: React diagnostics"]
  AT --> DIAG

  CAP --> PKG["P3: package/plugin governance"]
  PH --> ADV["P3: advisory permission advisor"]

  TUI["Not needed: terminal UI/CLI main path"]:::notneeded
  FANTASY["Not needed: fantasy/provider rewrite"]:::notneeded
  MARKET["Not needed: marketplace-first install"]:::notneeded

  classDef notneeded fill:#eee,stroke:#999,color:#555;
```

## First Recommended Module

```text
runtime: parity stabilization and re-audit
```

This is the best next module because:

- core runtime primitives now exist as Go-owned records and APIs, including
  AgentTask/coordinator communication;
- hooks, policy/headless, MCP, sandbox/worktree, compact/replay, and refs are
  cross-cutting enough that fixture breadth matters more than another UI pass;
- React diagnostics should render runtime state, not infer it.

## Page / React Deferral

React/page work is not the next priority. The current client can consume runtime
DTOs, but compact, replay, task mailbox, artifact refs, and policy diagnostics
need runtime-owned APIs first. Page work can proceed only as diagnostics over
existing APIs:

- compact panels after full compact/reinjection APIs,
- replay views after persisted event replay,
- task mailbox UI over runtime mailbox APIs,
- artifact drawer after durable output/artifact refs,
- policy rule editor after profile/headless semantics.

## Not Needed / Later

Not needed:

- Claude Code terminal UI / Ink / terminal layout,
- keybindings / Vim input state,
- slash command UI,
- CLI argument UX,
- Anthropic subscription/pass/growth surfaces,
- Claude.ai OAuth/product login surfaces,
- first-party telemetry sinks / GrowthBook / Datadog,
- marketplace-first plugin browsing/install,
- provider/model protocol rewrite,
- TUI/CLI main-path restoration.

Later/P3:

- model-assisted permission advisor,
- sandbox/remote runtime,
- local/signed plugin package governance,
- advanced memory taxonomy and session memory compact,
- provider/model health dashboards over fantasy.
