# Claude Code Alignment Module Priority

Status: refreshed on 2026-05-27 after AgentTask/coordinator communication
completion.

Use these docs for the next session:

- [`docs/claude-code-full-parity-review.md`](./claude-code-full-parity-review.md)
- [`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md)
- [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
- [`docs/claude-code-next-implementation-plan.md`](./claude-code-next-implementation-plan.md)

Older wording that made PermissionPolicy, compact, replay, MCP auth, worktree,
or React shell the next main module is superseded. Current code already contains
runtime foundations for scoped policy/headless, compact/reinjection, persisted
replay events, tool discovery, completed AgentTask/coordinator communication,
output/artifact refs, MCP auth/elicitation, worktree recovery/cleanup, sandbox
records, context loading, hooks lifecycle records, and scenario tests. The next
priority is runtime parity stabilization/re-audit, followed by read-only
diagnostics contracts.

## Current Baseline

| Area | Status | Evidence |
| --- | --- | --- |
| Runtime spine | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_tool_call_store.go`, `runtime_events.go`, `runtime_audit.go`, `runtime_recovery.go` |
| Tool scheduler | Completed foundation | `internal/tools/scheduler/*`, `internal/agent/scheduler_tool.go`, `runtime_scheduler_recorder.go` |
| Permission policy | Implemented foundation | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go` |
| Compact and budget | Implemented foundation | `internal/runtime/runtime_compact*.go`, `runtime_budget.go` |
| Tool search | Implemented foundation | `internal/agent/tool_search.go`, `internal/runtime/runtime_tool_search.go` |
| MCP/skills/capabilities | Implemented foundation | `internal/runtime/runtime_mcp*.go`, `runtime_skills.go`, `runtime_skill_activation.go`, `runtime_capabilities.go` |
| AgentTask/coordinator communication | Completed local runtime primitive | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_task_tools.go`, `runtime_agent_roles.go`, `runtime_agent_task_scope.go`, `runtime_agent_task_comm_store.go`, `internal/agent/task_tools.go`, `internal/agent/coordinator.go` |
| Worktree/sandbox | Implemented foundation | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go`, sandbox execution records |
| Hooks lifecycle | Implemented foundation | `internal/hooks/*`, `internal/agent/hooked_tool.go`, `internal/runtime/runtime_hooks.go` |
| Replay/eval harness | Implemented foundation | `internal/runtime/runtime_replay_export.go`, `runtime_scenario_harness_test.go` |
| React runtime client | Partial diagnostics | `client/src/runtime/*`, `client/src/features/*` |

## Recommended Next Module

```text
runtime: parity stabilization and re-audit
```

This is first because the core runtime primitive sequence is now in place,
including AgentTask/coordinator communication. The remaining highest-value work
is proving boundary combinations with fixtures and mirroring read-only DTOs for
diagnostics. React/page work should still wait unless it mirrors runtime
contracts; the client must not infer hook, compact, policy, task, artifact, or
worktree truth from messages.

## Next Priority Order

1. Runtime parity stabilization/re-audit across hooks, policy/headless, MCP,
   AgentTask communication, sandbox/worktree, compact/replay, and refs.
2. React diagnostics DTO/API mirror only where runtime contracts already exist.
3. Shell parser fixture expansion for Bash/PowerShell/cmd.
4. Capability package/plugin governance.
5. Sandbox/remote runtime.
6. Advisory permission advisor.

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
