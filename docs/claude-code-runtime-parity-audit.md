# Claude Code Runtime Parity Audit

Status: refreshed on 2026-05-27 from current `main`.

Agent Builder baseline: `d51590b76a680e683b9d5335797c7076c16a5b05`

Claude Code comparison source: `C:\Users\ytq\work\ai\myclaw\claude-code`

Authoritative closure audit:

- [`docs/claude-code-runtime-parity-closure-review.md`](./claude-code-runtime-parity-closure-review.md)

This document is the short audit entry point. The closure review contains the
full module matrix, Claude Code semantic comparison, blocker classification,
Mermaid dependency graph, hooks and AgentTask closure sections, and the next
session implementation brief.

## Current Runtime Parity Judgment

| Scope | Current state |
| --- | --- |
| Single main-agent runtime | 92 percent complete. Core local execution, policy, scheduler, compact, context, MCP, hooks, replay, recovery, and adapters exist. |
| Local multi-agent / AgentTask / coordinator runtime | 84 percent complete. Local runtime-owned task tools, messages, follow-up delivery, stop/cancel, output/artifact refs, replay, recovery, worktree/cwd scope, and rejection semantics exist. |
| Core Claude Code runtime parity excluding TUI/CLI/remote/product surfaces | 87 percent complete. Remaining work is scenario coverage and hardening. |

There is no current core runtime blocker for the local desktop runtime parity
scope. The next phase is runtime closure/stabilization.

## Fixed Constraints

- Go runtime is the source of truth.
- React is display, input, diagnostics, and product workflow only.
- Wails and HTTP are adapters.
- `charm.land/fantasy` remains the provider/model/tool protocol abstraction.
- Provider/model config belongs above fantasy as product policy/config.
- Compact, tool discovery, policy, hooks, AgentTask, coordinator communication,
  MCP/skills, worktree, sandbox, audit/replay, and recovery are runtime
  primitives.
- Model-assisted permission advice is advisory-only and cannot self-approve.
- Remote runtime and remote teammate/cloud execution are P3/product later, not
  current blockers.

## Evidence Inputs

Agent Builder current code evidence is concentrated in:

- `internal/runtime/`
- `internal/runtimeapi/`
- `internal/workbench/`
- `internal/session/`
- `internal/message/`
- `internal/db/`
- `internal/db/migrations/`
- `internal/agent/`
- `internal/agent/prompt/`
- `internal/agent/tools/`
- `internal/agent/tools/mcp/`
- `internal/tools/scheduler/`
- `internal/permission/`
- `internal/hooks/`
- `internal/skills/`
- `client/src/runtime/`
- `client/src/features/`
- `desktop/`

Claude Code comparison evidence is concentrated in:

- `src/QueryEngine.ts`
- `src/query.ts`
- `src/context.ts`
- `src/utils/claudemd.ts`
- `src/memdir/`
- `src/services/compact/`
- `src/Tool.ts`
- `src/tools.ts`
- `src/services/tools/`
- `src/tools/`
- `src/tools/ToolSearchTool/`
- `src/tools/BashTool/`
- `src/tools/PowerShellTool/`
- `src/utils/permissions/`
- `src/hooks/toolPermission/`
- `src/utils/hooks/`
- `src/types/hooks.ts`
- `src/schemas/hooks.ts`
- `src/tools/MCPTool/`
- `src/tools/ListMcpResourcesTool/`
- `src/tools/ReadMcpResourceTool/`
- `src/services/mcp/`
- `src/skills/`
- `src/utils/plugins/`
- `src/tools/AgentTool/`
- `src/tools/TaskCreateTool/`
- `src/tools/TaskGetTool/`
- `src/tools/TaskListTool/`
- `src/tools/TaskUpdateTool/`
- `src/tools/TaskStopTool/`
- `src/tools/TaskOutputTool/`
- `src/tools/SendMessageTool/`
- `src/tasks/`
- `src/coordinator/`
- `src/utils/worktree.ts`
- `src/utils/sandbox/`
- `src/services/vcr.ts`
- `src/services/analytics/`

## Classification

| Class | Audit result |
| --- | --- |
| Runtime blocker | None for local desktop parity. |
| Runtime hardening | Closure scenarios, shell parser fixtures, hook/policy/scope/MCP/sandbox/AgentTask bypass coverage, compact/replay/ref restart coverage, migration/replay contract checks. |
| React/page diagnostics later | Compact, replay, policy, task mailbox, artifact/output, hook, and worktree diagnostics over stable runtime APIs. |
| Product optimization later / P3 | Remote runtime, remote agent, SSH/cloud teammate, advisory permission advisor, plugin package governance, advanced memory/session memory compact, OS sandbox executor maturity. |
| Not needed | Terminal UI, Ink, keybindings, Vim input, slash command UI, CLI argument UX, Anthropic product growth/login, first-party telemetry sinks, GrowthBook, Datadog, marketplace-first install, provider protocol rewrite, `charm.land/fantasy` changes. |

## Hooks And AgentTask Special Findings

Hooks: core local lifecycle is implemented. `PreToolUse`, `PostToolUse`, and
`PostToolUseFailure` are runtime-owned, persisted, audited, replayable, and
recoverable. Hook allow cannot override deterministic deny, headless
fail-closed, task scope, sandbox, or MCP gates. Claude Code's broader hook event
catalog is later/not needed unless a desktop runtime workflow requires it.

AgentTask/coordinator: local runtime communication is implemented. It includes
model-facing task tools, runtime-owned parent-child messages, delivery through
child sessions, delivered/processed/rejected status, stop/cancel, output and
artifact refs, replay, recovery, worktree/cwd scope, and adapter DTOs. Remote
teammate/fleet/cloud semantics remain P3.

## Next Priority

Recommended unique next task:

```text
runtime: run parity closure stabilization and scenario coverage
```

Reason: current primitives exist, but they cross several authority boundaries.
Runtime scenario proof should come before React diagnostics so React remains a
consumer of runtime truth.
