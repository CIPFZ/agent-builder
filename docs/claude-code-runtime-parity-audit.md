# Claude Code Runtime Parity Audit

Status: refreshed on 2026-05-27 from current `main` code and the local Claude
Code source snapshot at `C:\Users\ytq\work\ai\myclaw\claude-code`.

This audit is now a short current-state entry point. The full module-by-module
re-audit, evidence matrix, dependency graph, gap classification, and next
implementation batch are maintained in:

- [`docs/claude-code-full-parity-review.md`](./claude-code-full-parity-review.md)
- [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
- [`docs/claude-code-next-implementation-plan.md`](./claude-code-next-implementation-plan.md)
- [`docs/claude-code-alignment-module-priority.md`](./claude-code-alignment-module-priority.md)

## Fixed Constraints

- Go runtime is the source of truth for sessions, turns, tools, permissions,
  capabilities, context, compact, AgentTask, worktree, events, audit, replay,
  and recovery.
- React is display, input, diagnostics, and product workflow only.
- Wails and HTTP are adapters.
- `charm.land/fantasy` remains the provider/model/tool protocol abstraction.
- Compact, tool discovery, policy, AgentTask, MCP/skills, worktree, hooks,
  audit, and replay are runtime primitives.
- Model-assisted permission can only be advisory and cannot approve its own
  high-risk tool use.
- Coordinator and agent communication are runtime primitives, not UI event
  stitching.
- Provider/model config belongs above fantasy as product policy/config, not as a
  second provider abstraction.

## Inputs Checked

Agent Builder docs:

- `AGENTS.md`
- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`
- `docs/claude-code-next-implementation-plan.md`
- `docs/claude-code-alignment-module-priority.md`
- `docs/client-runtime-architecture-review.md`
- `docs/turn-task-run-model.md`
- `docs/tool-scheduler-design.md`
- `docs/permission-policy-model.md`
- `docs/client-state-recovery.md`
- `docs/archive/phase-2-runtime-api-boundary.md`
- `docs/client-architecture-and-core-flow.md`

Agent Builder code:

- `internal/runtime/*`
- `internal/agent/*`
- `internal/agent/prompt/*`
- `internal/agent/tools/*`
- `internal/agent/tools/mcp/*`
- `internal/tools/scheduler/*`
- `internal/permission/*`
- `internal/skills/*`
- `internal/hooks/*`
- `internal/session/*`
- `internal/message/*`
- `internal/db/*`
- `client/src/runtime/*`
- `client/src/features/*`

Claude Code runtime reference:

- `src/QueryEngine.ts`, `src/query.ts`, `src/Tool.ts`, `src/tools.ts`
- `src/services/tools/*`, `src/tools/*`
- `src/tools/AgentTool/*`, `src/tools/Task*Tool/*`
- `src/tools/SendMessageTool/*`, `src/tools/ToolSearchTool/*`
- `src/tools/BashTool/*`, `src/tools/PowerShellTool/*`
- `src/tools/MCPTool/*`, `src/tools/ListMcpResourcesTool/*`
- `src/tools/ReadMcpResourceTool/*`
- `src/services/compact/*`, `src/context.ts`, `src/utils/claudemd.ts`
- `src/memdir/*`, `src/utils/permissions/*`, `src/services/mcp/*`
- `src/skills/*`, `src/utils/plugins/*`, `src/tasks/*`
- `src/coordinator/*`, `src/utils/worktree.ts`, `src/utils/sandbox/*`
- `src/services/analytics/*`, `src/services/vcr.ts`
- `docs/20-coordinator-swarm-and-teammate-collaboration.md`
- `docs/32-harness-and-eval-runtime.md`
- `docs/45-telemetry-and-reporting-rules-audit.md`

## Current Capability Matrix

| Module | Status | Agent Builder code evidence | Claude Code comparison / remaining gap |
| --- | --- | --- | --- |
| Query engine / turn lifecycle | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_turn_store.go`, `runtime_service.go`, `internal/agent/agent.go` | Claude Code `QueryEngine.ts` has mature abort/continuation/headless projections. Agent Builder still needs stronger continuation after interruption. |
| Tool protocol / scheduler / normalization | Completed foundation | `internal/tools/scheduler/*`, `internal/agent/scheduler_tool.go`, `internal/runtime/runtime_scheduler_recorder.go`, `runtime_tool_call_store.go` | Output refs/artifacts are partial; model-visible vs UI-visible output policy needs hardening. |
| Tool search / discovery / deadlock avoidance | Partial implemented | `internal/agent/tool_search.go`, `internal/runtime/runtime_tool_search.go`, `internal/agent/loop_detection.go` | Runtime search, disclosure budget, repeated-search and concurrency guardrails exist. Needs broader scenario coverage and richer per-source recursion policy. |
| Compact / context budget lifecycle | Implemented foundation | `internal/runtime/runtime_compact*.go`, `runtime_budget.go`, migration `20260524020000_add_runtime_compact_boundaries.sql` | Boundary, budget, compact summaries, replay/recovery refs, and post-compact reinjection exist. Remaining work is broader fixture coverage and product diagnostics. |
| Context / memory / AGENTS / read-file state | Implemented foundation | `internal/agent/prompt/prompt.go`, `internal/runtime/runtime_context.go`, `internal/db/read_files.sql.go` | AGENTS/CLAUDE loading, context audit, read-file state, and compact reinjection exist. Remaining work is broader Claude Code memory taxonomy and diagnostics. |
| Permission policy / plan / scoped rules / shell safety | Implemented foundation | `internal/permission/policy.go`, `internal/runtime/runtime_policy.go`, `runtime_permission_store.go` | Scoped rules, profiles, headless fail-closed semantics, scheduler integration, and shell/sandbox policy metadata exist. Remaining work is broader shell parser parity fixtures and later advisory explanations. |
| Model-assisted permission advisor | Missing | No advisor runtime package or events | Must remain later and advisory-only after deterministic scopes and evals are stable. |
| MCP lifecycle / resources / prompts / auth / elicitation | Implemented foundation | `internal/runtime/runtime_mcp*.go`, `internal/agent/tools/mcp/*`, `internal/agent/tools/list_mcp_resources.go`, `read_mcp_resource.go` | Server/tool/resource/prompt APIs, lazy refresh, policy filtering, auth/elicitation lifecycle records, and redaction exist. Remaining work is broader protocol/provider fixture coverage. |
| Skills / allowed tools / activation / plugin metadata | Partial implemented | `internal/skills/*`, `internal/runtime/runtime_skills.go`, `runtime_skill_activation.go`, `runtime_capabilities.go` | Skill metadata and allowed_tools hints are preserved; allowed_tools does not grant permissions. Plugin package governance is still missing. |
| AgentTask / subagent roles / parent-child messaging | Completed local runtime primitive | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_task_tools.go`, `runtime_agent_roles.go`, `runtime_agent_task_scope.go`, `runtime_agent_task_comm_store.go`, `internal/agent/agent_tool.go`, `internal/agent/task_tools.go`, `internal/agent/coordinator.go` | Local SendMessage/coordinator semantics are runtime-owned: follow-up delivery, stop/cancel, output refs, message sequence/status, replay, recovery, HTTP/Wails DTOs, and model-facing task tools exist. Remote teammate/fleet remains P3/later. |
| Worktree / cwd isolation / sandbox / remote | Implemented foundation | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go`, `runtime_agent_task_scope.go`, sandbox runtime records | Git worktree lifecycle, recovery/cleanup, task cwd scope, and sandbox execution boundary records exist. Remote runtime remains later. |
| Audit / replay / eval harness / observability | Implemented foundation | `internal/runtime/runtime_audit*.go`, `runtime_replay_export.go`, `runtime_scenario_harness_test.go` | Persisted replay events, replay export, audit summaries, and scenario harness exist. Remaining work is broader fixture packs. |
| Provider/model configuration on fantasy | Partial | `internal/runtime/runtime_model*.go`, `client/src/features/settings/ModelSettingsDrawer.tsx` | Correctly stays above fantasy. Needs health/capability diagnostics and per-mode model policy. |
| React runtime boundary / diagnostics / recovery | Partial | `client/src/runtime/*`, `client/src/features/*`, `internal/runtime/runtime_http.go`, `runtime_sse.go`, `runtime_recovery.go` | React consumes runtime APIs. It should wait for runtime APIs before compact/task/replay/policy diagnostics deepen. |
| Data model / migrations / persistence / recovery | Implemented foundation | `internal/db/migrations/20260523*.sql`, `202605240*.sql`, `20260524100000_add_runtime_hook_executions.sql`, `internal/runtime/*_store.go` | Turns, tool calls, permissions, compact, AgentTask, worktree, hooks, audit, refs, and replay events are persisted and recoverable. |
| Hooks integration | Implemented foundation | `internal/hooks/*`, `internal/agent/hooked_tool.go`, `internal/runtime/runtime_hooks.go`, `runtime_scheduler_recorder.go` | Hooks are runtime-owned lifecycle records for pre-tool, post-tool, and post-error paths. Policy remains final authority; hook allow cannot bypass deny/ask/headless/scope/sandbox/MCP gates. |
| Background jobs / shell jobs / artifacts / output refs | Implemented foundation | `internal/agent/tools/bash.go`, `job_output.go`, `job_kill.go`, scheduler shell metadata, `internal/runtime/runtime_refs.go` | Job output/kill, output refs, artifact refs, replay summaries, and redaction exist. Remaining work is richer artifact diagnostics. |

## Current Overall Judgment

Agent Builder is no longer missing the runtime spine. It now has durable turns,
ToolCalls, permissions, policy modes and scoped rules, context audit, compact
boundaries and micro compact, tool search/discovery, AgentTask records with
messages/results, worktree lifecycle, audit, replay export, and scenario tests.

The real gap has moved to stabilization, fixture breadth, and product
diagnostics:

- broader eval fixtures,
- runtime parity re-audit over completed primitives,
- richer shell parser coverage and remote isolation later,
- product diagnostics after runtime APIs are stable.

## Explicit Not Needed

These Claude Code surfaces remain excluded:

- terminal UI / Ink / terminal layout,
- keybindings and Vim input state,
- slash command UI,
- CLI argument UX,
- Anthropic subscription/pass/product growth surfaces,
- Claude.ai OAuth/product login surfaces,
- first-party telemetry sinks, GrowthBook, Datadog integrations,
- marketplace-first plugin browsing/install,
- provider/model protocol rewrite,
- changes to `charm.land/fantasy`,
- TUI/CLI main-path restoration.

## Gap Classification

Runtime blockers:

- None for the local Claude Code runtime parity scope. AgentTask/coordinator
  communication is complete as a local runtime primitive.

Runtime hardening:

- Tool search and scheduler guardrails need more scenario coverage and
  per-source recursion/concurrency policy.
- Shell parser parity needs broader Bash/PowerShell/cmd fixtures.
- Hooks, MCP, worktree, sandbox, refs, replay, compact, and AgentTask
  communication should keep gaining scenario coverage as product diagnostics
  are added.
- Remote runtime remains a later product optimization.

Page/React diagnostics later:

- Compact budget panels, replay views, policy rule diagnostics, task mailbox
  views, worktree cleanup UI, and artifact drawers should wait for runtime APIs
  and persisted state to stabilize.

Product optimization later:

- Provider/model health diagnostics over fantasy.
- Local capability package governance and signed package trust.
- Advisory permission explanations.
- Advanced memory taxonomy/session memory compact.

Not needed:

- All explicitly excluded Claude Code product/UI/provider surfaces listed
  above.

## Next Priority

First recommended runtime module after hook lifecycle hardening:

```text
runtime: parity stabilization and re-audit
```

Why first:

- Core Claude Code runtime primitives now exist as Go-owned records, including
  AgentTask/coordinator communication.
- The remaining risk is regression across cross-cutting boundaries: hooks,
  policy, headless, AgentTask scope, MCP auth/elicitation, sandbox/worktree,
  compact/replay, and refs.
- React work should still remain read-only diagnostics over runtime APIs.

See the detailed scope in
[`docs/claude-code-next-implementation-plan.md`](./claude-code-next-implementation-plan.md).
