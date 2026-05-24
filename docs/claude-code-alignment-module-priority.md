# Claude Code Alignment Module Priority

Status: refreshed on 2026-05-24 after the full runtime parity re-audit.

Use these docs for the next session:

- [`docs/claude-code-full-parity-review.md`](./claude-code-full-parity-review.md)
- [`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md)
- [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
- [`docs/claude-code-next-implementation-plan.md`](./claude-code-next-implementation-plan.md)

Older wording that made either PermissionPolicy or React shell the next main
module is superseded. Current code already contains foundations for scoped
policy, compact boundary/micro compact, tool search, AgentTask messages/results,
worktree lifecycle, replay export, and scenario tests. The next priority is
runtime hardening.

## Current Baseline

| Area | Status | Evidence |
| --- | --- | --- |
| Runtime spine | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_tool_call_store.go`, `runtime_events.go`, `runtime_audit.go`, `runtime_recovery.go` |
| Tool scheduler | Completed foundation | `internal/tools/scheduler/*`, `internal/agent/scheduler_tool.go`, `runtime_scheduler_recorder.go` |
| Permission policy | Partial implemented | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go` |
| Compact and budget | Partial implemented | `internal/runtime/runtime_compact*.go`, `runtime_budget.go` |
| Tool search | Partial implemented | `internal/agent/tool_search.go`, `internal/runtime/runtime_tool_search.go` |
| MCP/skills/capabilities | Partial implemented | `internal/runtime/runtime_mcp*.go`, `runtime_skills.go`, `runtime_skill_activation.go`, `runtime_capabilities.go` |
| AgentTask/coordinator base | Partial implemented | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_roles.go`, `runtime_agent_task_scope.go`, `runtime_agent_task_comm_store.go`, `internal/agent/coordinator.go` |
| Worktree | Partial implemented | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go` |
| Replay/eval harness | Partial implemented | `internal/runtime/runtime_replay_export.go`, `runtime_scenario_harness_test.go` |
| React runtime client | Partial diagnostics | `client/src/runtime/*`, `client/src/features/*` |

## Recommended Next Module

```text
runtime: full compact and post-compact reinjection
```

This is first because current code has compact boundaries, budget reporting, and
micro compact but still lacks Claude Code's full compact lifecycle:

- full compact summaries,
- auto compact trigger,
- session memory compact,
- post-compact cleanup/reinjection,
- compact-aware recovery,
- broader compact replay/eval coverage.

React/page work should wait because compact, replay, task mailbox, artifact, and
policy diagnostics must render runtime state. The client must not infer compact
or task truth from messages.

## Next Priority Order

1. Full compact and post-compact reinjection.
2. Persisted event replay and expanded scenario harness.
3. Tool discovery guardrails and scheduler recursion/concurrency hardening.
4. Policy profiles, headless semantics, and shell parser hardening.
5. AgentTask coordinator mailbox and task-tool parity.
6. Output/artifact refs and durable background job entity.
7. MCP auth/elicitation lifecycle.
8. Worktree task/cwd cleanup hardening.
9. React compact/replay/task/policy diagnostics after runtime APIs.
10. Sandbox/remote runtime.
11. Capability package/plugin governance.
12. Advisory permission advisor.

## Not Needed

- Terminal UI / Ink / terminal layout.
- Keybindings / Vim input state.
- Slash command UI.
- CLI argument UX.
- Anthropic subscription/pass/growth surfaces.
- Claude.ai OAuth/product login surfaces.
- First-party telemetry sinks / GrowthBook / Datadog.
- Marketplace-first plugin browsing/install.
- Provider/model protocol rewrite.
- Changes to `charm.land/fantasy`.
- TUI/CLI main-path restoration.
