# Claude Code Alignment Module Priority

This document is the short entry point for the next Agent Builder planning
phase. The detailed roadmap is now maintained in
[`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md).

This session is docs-only. Do not modify Go or React product code when updating
this plan.

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
| PermissionPolicy | Next | `internal/permission/policy.go` exists, but mode/rule/audit/API semantics are still minimal; the next pass is deterministic runtime enforcement, with model-assisted policy reserved for a later advisory layer |
| Skills/MCP/capability inventory | Parallel foundation | runtime panels and APIs exist; activation/lazy loading remain future work |
| React runtime UI | Parallel foundation | React consumes runtime DTOs, but tool cards/timeline/policy UI need deeper runtime objects |

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
PermissionPolicy baseline
```

Reason: Tool Scheduler integration is now present, and the next set of modules
all need a single runtime policy boundary before they can be implemented safely:

- Tool / Capability Lazy Loading must ask policy before activating executable,
  MCP, skill, or subagent capabilities.
- Plan mode is a permission mode, not a React label.
- Shell/background job safety needs mode-aware allow/ask/deny decisions.
- MCP and skills activation need policy-controlled capability visibility.
- Subagent / AgentTask needs scoped tools, model, cwd, and capability rules.

Claude Code's permission system includes more adaptive/model-assisted behavior
than original Crush. Agent Builder should model that as a later advisor layer:
the model or classifier may summarize intent, explain risk, or propose leaving
plan mode, but final `allow`/`ask`/`deny` decisions must remain enforced by Go
runtime policy. The baseline implementation must not let the model approve its
own high-risk tool use.

Lazy Loading should follow the PermissionPolicy baseline. It can start in
parallel as a design-only capability registry refinement, but executable lazy
activation should wait until policy decisions are stable.

## Roadmap Pointer

Use [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
for the next session. It contains:

- completed capability baseline,
- remaining Claude Code gaps,
- Tool / Capability Lazy Loading design,
- P0/P1/P2/P3 priority map,
- module boundaries and acceptance criteria,
- dependency graph,
- commit-by-commit implementation sequence.
