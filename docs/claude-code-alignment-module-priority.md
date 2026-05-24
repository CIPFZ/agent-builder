# Claude Code Alignment Module Priority

This document is a short pointer to the current Claude Code alignment plan.
The detailed roadmap is maintained in
[`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md),
and the current full parity source is
[`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md).

Status: updated after the 2026-05-24 runtime parity audit. Older wording that
made PermissionPolicy the next implementation module is superseded.

## Current Baseline

The current `main` baseline has moved beyond the older P0/P1 planning state:

| Area | Status | Evidence |
| --- | --- | --- |
| Durable Turn lifecycle | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_turn_store.go`, `runtime_turn_store_test.go` |
| Durable ToolCall lifecycle | Completed foundation | `internal/runtime/runtime_tool_calls.go`, `runtime_tool_call_store.go`, `internal/tools/scheduler` |
| Runtime event cursor | Completed foundation | `internal/runtime/runtime_events.go`, `runtime_sse.go`, `client/src/runtime/types.ts` |
| Runtime audit trail | Completed foundation | `internal/runtime/runtime_audit*.go`, `runtime_audit_test.go` |
| Session recovery foundation | Completed foundation | `internal/runtime/runtime_recovery.go`, `client/src/runtime/types.ts` |
| Tool Scheduler integration | Completed P1 baseline | `internal/agent/scheduler_tool.go`, `internal/runtime/runtime_scheduler_recorder.go` |
| PermissionPolicy | Partial foundation | `internal/permission/policy.go` and `/v1/policy` exist with `ask`, `auto_read`, `plan`, and `deny_all`; scoped rules, shell hardening, headless profiles, and advisory-only model assistance remain future work |
| Context source audit | Partial foundation | `internal/runtime/runtime_context.go`, `internal/agent/prompt`, context source events/audit |
| Skills/MCP/capability inventory | Partial foundation | runtime panels, APIs, capability states, and refresh events exist; scoped activation/lazy enforcement remain future work |
| AgentTask persistence | Partial foundation | `internal/runtime/runtime_agent_tasks.go`, task store/events/API exist; roles, scoped enforcement, communication, and artifacts remain partial |
| React runtime UI | Partial foundation | React consumes runtime DTOs; compact/task/policy diagnostics and richer replay views remain future work |

`charm.land/fantasy` remains the provider/model/tool protocol abstraction. Do
not modify fantasy or recreate provider clients, model-facing message formats,
tool-call protocol, or stream handling as part of Agent Builder roadmap work.
Agent Builder owns runtime orchestration above fantasy: turns, tool calls,
permission policy, capability inventory, audit, recovery, and client API.

React is a thin client surface. It must not become the business state source.
Go runtime is the source of truth. Wails is an adapter. CLI/TUI compatibility is
legacy and must not be restored as the product main path.

## Recommended Next Module

The next implementation module should be:

```text
Compact lifecycle foundation
```

Reason: Tool Scheduler integration, runtime turns, audit, recovery, policy
baseline, capability inventory, context source audit, and AgentTask persistence
are now present as foundations. The largest Claude Code parity gap is
long-session governance:

- micro compact,
- full compact,
- session memory compact,
- auto compact trigger,
- post-compact reinjection,
- prompt/tool budget accounting.

Compact should start as a conservative runtime primitive: durable compact
boundary records, events, audit, and tests before changing model-loop behavior.

## Other P1 Modules

These remain high priority after or alongside the compact boundary:

- Tool search and prompt/tool budget.
- Scoped policy rules and shell safety hardening.
- AgentTask scope, role definitions, and parent/child messaging.
- Scenario/eval harnesses for policy, tools, MCP, skills, tasks, compact, and
  recovery.

Claude Code's permission system includes more adaptive/model-assisted behavior
than original Crush. Agent Builder should model that as a later advisor layer:
the model or classifier may summarize intent, explain risk, or propose leaving
plan mode, but final `allow`/`ask`/`deny` decisions must remain enforced by Go
runtime policy. The baseline implementation must not let the model approve its
own high-risk tool use.

Tool/capability lazy exposure should now be handled through the combination of
capability registry metadata, scoped policy, and model-facing tool search.

## Roadmap Pointer

Use [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
for the next session. It contains:

- completed capability baseline,
- remaining Claude Code gaps,
- compact/tool search/policy/task priority order,
- P0/P1/P2/P3 priority map,
- module boundaries and acceptance criteria,
- dependency graph,
- commit-by-commit implementation sequence.
