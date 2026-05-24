# Claude Code Full Parity Review

Date: 2026-05-24

Scope: full runtime parity re-audit of Agent Builder current `main` against the
local Claude Code source snapshot at
`C:\Users\ytq\work\ai\myclaw\claude-code`.

This review intentionally excludes Claude Code terminal UI, Ink layout,
keybindings, Vim input, slash command UI, CLI argument UX, Anthropic
subscription/pass/growth surfaces, Claude.ai OAuth/product login surfaces,
first-party telemetry sinks, GrowthBook, Datadog, marketplace-first plugin
install, provider/model protocol rewrites, `charm.land/fantasy` changes, and
TUI/CLI main-path restoration.

## Executive Summary

Agent Builder has materially advanced beyond the older roadmap. The runtime
spine exists and several former gaps are now partial foundations:

- compact boundary, budget, and micro compact,
- tool search/discovery,
- scoped policy rules and shell destructive classification,
- AgentTask roles/scopes/messages/results,
- worktree lifecycle,
- replay export,
- scenario harness coverage.

The remaining parity work is no longer "create the runtime boundary." It is
hardening and completing the runtime primitives so React can later expose them
as diagnostics:

- full compact and post-compact reinjection,
- persisted event replay,
- broader scenario/eval fixtures,
- coordinator mailbox semantics,
- durable output/artifact refs,
- MCP auth/elicitation,
- policy profile/headless semantics,
- sandbox/remote later.

First recommended module:

```text
runtime: full compact and post-compact reinjection
```

Runtime remains prior to page work because React must display runtime facts. It
must not infer compact, task, artifact, permission, replay, or worktree state
from local reducers or message parsing.

## Evidence Inputs

Agent Builder docs reviewed:

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

Agent Builder code sampled:

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

Claude Code runtime reference sampled:

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

| Area | Status | Agent Builder evidence | Claude Code evidence | Gap / decision |
| --- | --- | --- | --- | --- |
| Query engine / turn lifecycle | Completed foundation | `internal/runtime/runtime_turns.go`, `runtime_turn_store.go`, `runtime_lifecycle.go`, `runtime_service.go`, `internal/agent/agent.go` | `src/QueryEngine.ts`, `src/query.ts` | Durable turn lifecycle exists. Remaining gap is full continuation after interruption and richer headless projection. |
| Tool protocol / scheduler / output normalization | Completed foundation | `internal/tools/scheduler/*`, `internal/agent/scheduler_tool.go`, `internal/runtime/runtime_scheduler_recorder.go`, `runtime_tool_call_store.go` | `src/Tool.ts`, `src/services/tools/*`, `src/tools.ts` | Lifecycle/audit foundation exists. Need durable output/artifact refs and stronger model-visible vs UI-visible output policy. |
| Tool search / discovery / deadlock avoidance | Partial implemented | `internal/agent/tool_search.go`, `internal/runtime/runtime_tool_search.go`, `internal/agent/loop_detection.go`, `internal/runtime/runtime_scenario_harness_test.go` | `src/tools/ToolSearchTool/ToolSearchTool.ts`, `src/utils/toolSearch.ts`, `src/tools.ts` | Runtime search, selected/omitted budgets, repeated-search and concurrency guardrails exist. Need broader scenario coverage and per-source recursion rules. |
| Compact / context budget lifecycle | Partial implemented | `internal/runtime/runtime_compact.go`, `runtime_compact_store.go`, `runtime_budget.go`, `runtime_compact_test.go`, migration `20260524020000_add_runtime_compact_boundaries.sql` | `src/services/compact/*` | Boundary, budget, and micro compact exist. Full compact, auto compact, session memory compact, and post-compact reinjection are missing. |
| Context / memory / CLAUDE.md / AGENTS / read-file state | Partial | `internal/agent/prompt/prompt.go`, `internal/agent/prompt/context_sources_test.go`, `internal/runtime/runtime_context.go`, `internal/db/read_files.sql.go` | `src/context.ts`, `src/utils/claudemd.ts`, `src/memdir/*` | Context audit and read-file table exist. Need include/frontmatter/rules/memdir parity and compact-aware reinjection. |
| Permission policy / plan mode / scoped rules / shell safety | Partial implemented | `internal/permission/policy.go`, `policy_test.go`, `internal/runtime/runtime_policy.go`, `runtime_permission_store.go` | `src/utils/permissions/*`, `src/tools/EnterPlanModeTool/*`, `src/tools/ExitPlanModeTool/*`, Bash/PowerShell tools | Scoped rules and shell destructive classification exist. Need plan-exit lifecycle, profiles/headless semantics, fuller Bash/PowerShell parser parity. |
| Model-assisted permission advisor | Missing | No advisor package/events | Claude Code permission classifier/adaptive behavior in permission utilities | Later only. Must remain advisory and never approve high-risk actions. |
| MCP lifecycle / resources / prompts / tools / auth / elicitation | Partial implemented | `internal/runtime/runtime_mcp.go`, `runtime_mcp_config.go`, `internal/agent/tools/mcp/*`, `list_mcp_resources.go`, `read_mcp_resource.go` | `src/services/mcp/*`, `src/tools/MCPTool/*`, `McpAuthTool`, list/read resource tools | Server/tool/resource/prompt APIs, policy filtering, lazy refresh, redaction exist. Auth/elicitation lifecycle missing. |
| Skills / allowed tools / activation / plugin metadata | Partial implemented | `internal/skills/*`, `internal/runtime/runtime_skills.go`, `runtime_skill_activation.go`, `runtime_capabilities.go` | `src/skills/*`, `src/utils/plugins/*` | Skill metadata and allowed_tools hints are preserved. Plugin/package governance is missing; marketplace-first is not needed. |
| AgentTask / subagent roles / parent-child messaging / coordinator | Partial implemented | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_roles.go`, `runtime_agent_task_scope.go`, `runtime_agent_task_comm_store.go`, `internal/agent/agent_tool.go`, `internal/agent/coordinator.go` | `src/tools/AgentTool/*`, `src/tools/Task*Tool/*`, `src/tools/SendMessageTool/*`, `src/tasks/*`, `src/coordinator/*` | AgentTask store, roles, scope, messages/results exist. Full mailbox, SendMessage, teammate/coordinator semantics remain incomplete. |
| Worktree / cwd isolation / sandbox / remote | Partial implemented | `internal/runtime/runtime_worktrees.go`, `runtime_worktree_store.go`, `runtime_agent_task_scope.go` | `src/utils/worktree.ts`, `src/tools/EnterWorktreeTool/*`, `ExitWorktreeTool`, `src/utils/sandbox/*` | Git worktree lifecycle exists. Sandbox and remote remain later; worktree/task cleanup recovery needs hardening. |
| Audit / replay / eval harness / local observability | Partial implemented | `internal/runtime/runtime_audit*.go`, `runtime_replay_export.go`, `runtime_scenario_harness_test.go` | `src/services/vcr.ts`, `docs/32-harness-and-eval-runtime.md`, `docs/45-telemetry-and-reporting-rules-audit.md` | Audit/replay/scenarios exist. Need persisted event replay and broader fixture packs. Do not import first-party telemetry sinks. |
| Provider/model configuration on fantasy | Partial | `internal/runtime/runtime_model*.go`, `client/src/features/settings/ModelSettingsDrawer.tsx` | Claude Code provider/product config modules | Correct layer: above fantasy. Need health/capability/per-mode policy; no provider rewrite. |
| React runtime boundary / diagnostics / recovery | Partial | `client/src/runtime/*`, `client/src/features/*`, `internal/runtime/runtime_http.go`, `runtime_sse.go`, `runtime_recovery.go` | Claude Code structured/headless output and message components as runtime references only | React consumes runtime DTOs. Diagnostics must wait for runtime compact/replay/task/artifact APIs. |
| Data model / migrations / persistence / recovery | Partial implemented | `internal/db/migrations/20260523*.sql`, `202605240*.sql`, `internal/runtime/*_store.go` | Claude Code task/memory/VCR persistence patterns | Runtime stores exist for main primitives. Persisted event replay and artifact refs remain gaps. |
| Hooks integration | Partial | `internal/hooks/*`, `internal/agent/hooked_tool.go` | Claude Code hooks/plugin integrations | Hooks exist. Need clearer runtime hook lifecycle events and policy precedence diagnostics. |
| Background jobs / shell jobs / artifacts / output refs | Partial | `internal/agent/tools/bash.go`, `job_output.go`, `job_kill.go`, scheduler shell metadata, migration `20260524000000_add_shell_job_tool_call_metadata.sql` | `src/tasks/LocalShellTask/*`, `src/tools/BashTool/*`, `PowerShellTool/*` | Shell job tools exist. Durable background job entity and output/artifact ref store remain partial. |

## Completed Foundations

- Runtime Turn and ToolCall lifecycle.
- Runtime event cursor, SSE, audit store, and recovery status.
- Tool scheduler wrapper with permission/audit integration.
- Permission modes: `ask`, `auto_read`, `plan`, `deny_all`.
- Scoped policy rules for tool, capability, MCP, skill, subagent/task, cwd/path,
  shell prefix/regex.
- Shell destructive classifier for common Bash/PowerShell/cmd/git/delete
  patterns.
- Capability registry with states, schema summaries, search text, policy
  filtering, and lazy refresh.
- Tool search and deferred disclosure.
- Compact boundaries, budget reports, and micro compact for old high-cost tool
  outputs.
- Context source audit and read-file table foundation.
- Skills/MCP inventory, enablement, refresh, redaction, and policy filtering.
- AgentTask store, role definitions, scope checks, task messages/results,
  artifact refs, cancellation.
- Git worktree lifecycle store/API/events/audit.
- Replay export over audit plus event buffer.
- Runtime scenario harness covering policy, shell, tool discovery, compact,
  recovery, cancellation, worktree, redaction.
- React runtime DTO consumption for chat, permissions, audit, settings,
  capabilities, skills, MCP, tasks, compact/worktree API bindings.

## Partial / Hardening Areas

- Full compact, auto compact, session memory compact, and reinjection.
- Persisted event replay beyond bounded runtime event buffer.
- Tool search guardrail breadth and per-source recursion/concurrency policy.
- Headless/trusted policy profiles and full plan-exit lifecycle.
- Bash/PowerShell parser parity beyond destructive heuristics.
- MCP auth/OAuth and elicitation.
- Skill allowed_tools enforcement through policy scopes rather than prompt hints.
- AgentTask mailbox, SendMessage equivalent, coordinator/team semantics.
- Durable output/artifact refs and background job entity.
- Worktree recovery/cleanup and task integration.
- React diagnostics over compact/replay/task/policy/artifacts after runtime APIs.

## Missing Runtime Primitives

- Model-assisted permission advisor, intentionally later and advisory-only.
- Sandbox runtime.
- Remote runtime.
- Capability package/plugin governance with trust/signing.
- Full Claude Code memory taxonomy/session memory compact.

## Not Needed

- Terminal UI / Ink / terminal layout.
- Keybindings / Vim input state.
- Slash command UI.
- CLI argument UX.
- Anthropic subscription/pass/product growth surfaces.
- Claude.ai OAuth/product login surfaces.
- First-party telemetry sinks / GrowthBook / Datadog integrations.
- Marketplace-first plugin browsing/install.
- Provider/model protocol rewrite.
- Changes to `charm.land/fantasy`.
- TUI/CLI main-path restoration.

## Gap Classification

### Runtime Blockers

| Gap | Why it is real | Risk | Dependencies | Acceptance |
| --- | --- | --- | --- | --- |
| Full compact and reinjection | Agent Builder has boundary/budget/micro compact, but no full summary/reinjection equivalent to Claude Code compact services. | Long sessions can lose context governance and cannot explain what was summarized or restored. | Existing compact boundary, context audit, read-file state. | Full compact boundary survives restart; summary/source refs are auditable; instructions/read files can be reinjected with provenance. |
| Persisted event replay | `runtime_replay_export.go` combines audit store with bounded event buffer. | Long-running sessions lose event-level replay after buffer rollover. | Existing event cursor/audit/replay export. | Replay after restart/buffer rollover can reconstruct redacted event summary for a turn/session. |
| AgentTask coordinator mailbox | Task messages/results exist, but SendMessage/coordinator semantics are not complete. | Multi-agent work remains recordable but not fully orchestrated. | Compact/replay/policy hardening. | Parent/child messages, delivery state, results, artifacts, cancellation, and replay are durable runtime state. |

### Runtime Hardening

| Gap | Risk | Acceptance |
| --- | --- | --- |
| Tool discovery guardrails | Repeated/nested tool search or large MCP/skill surfaces can still stress prompt/scheduler behavior. | Per-source recursion/concurrency limits and scenario tests pass. |
| Policy profiles/headless semantics | Ask flows may be ambiguous outside interactive UI. | Headless ask fails closed; profile/rule diagnostics are replayable. |
| Shell parser parity | Regex/token heuristics can miss shell edge cases. | Bash/PowerShell/cmd fixture pack covers destructive/read-only patterns. |
| MCP auth/elicitation | Some MCP servers cannot complete lifecycle through runtime. | Auth/elicitation requests are recoverable, redacted, and auditable. |
| Output/artifact refs | Large output is summarized but not consistently durable as user-inspectable refs. | Runtime can fetch output/artifact refs after compact/replay. |
| Worktree cleanup/recovery | Isolation state may become stale after failures/restarts. | Runtime recovers missing/preserved/cleanup-pending worktrees with audit. |

### Page / React Diagnostics Later

- Compact budget and reinjection panels.
- Replay/export diagnostics.
- Policy profile/rule diagnostics.
- Task mailbox/coordinator panel.
- Artifact/output detail drawer.
- Worktree cleanup/status panel.

These depend on runtime APIs and persisted state. React must not synthesize them.

### Product Optimization Later

- Provider/model health and per-mode model policy over fantasy.
- Local/signed plugin package governance.
- Advisory permission explanations.
- Advanced memory taxonomy and session memory compact.

## Reordered Roadmap

| Priority | Module | Status |
| --- | --- | --- |
| P0 | Runtime spine, scheduler, event cursor, audit, recovery | Completed |
| P0 | Deterministic policy baseline and scoped rules | Completed foundation |
| P0 | Capability registry and MCP/skills inventory | Completed foundation |
| P1 Next | Full compact and post-compact reinjection | Next |
| P1 Next | Persisted event replay and expanded scenario harness | Next |
| P1 Parallel | Tool discovery guardrails | Parallel |
| P1 Parallel | Policy profiles/headless and shell parser hardening | Parallel |
| P2 | AgentTask coordinator mailbox / SendMessage semantics | Blocked by P1 |
| P2 | Output/artifact refs and durable background job entity | Blocked by compact/replay |
| P2 | MCP auth/elicitation | Blocked by policy/replay |
| P2 | Worktree hardening | Blocked by policy/task hardening |
| P2 Later | React diagnostics | Blocked by runtime APIs |
| P3 | Sandbox/remote runtime | Later |
| P3 | Plugin/package governance | Later |
| P3 | Advisory permission advisor | Later |
| Not needed | Terminal UI, slash UI, provider rewrite, marketplace-first | Not needed |

## Dependency Graph

```mermaid
graph TD
  SP["Completed: Runtime spine"] --> FC["P1: Full compact"]
  SP --> PER["P1: Persisted event replay"]
  SP --> TD["P1: Tool discovery guardrails"]
  SP --> POL["P1: Policy profiles + shell parser"]

  CB["Partial: Boundary + budget + micro compact"] --> FC
  CTX["Partial: Context audit + read-file state"] --> REINJ["P1: Post-compact reinjection"]
  FC --> REINJ
  FC --> AUTO["P2: Auto compact"]
  FC --> MEM["P3: Session memory compact"]

  CAP["Partial: Capability registry"] --> TD
  TS["Partial: Tool search"] --> TD
  POL --> TD

  AT["Partial: AgentTask roles/scopes/messages/results"] --> MAIL["P2: Coordinator mailbox"]
  FC --> MAIL
  PER --> MAIL
  POL --> MAIL

  OUT["Partial: Tool output summaries"] --> ART["P2: Output/artifact refs"]
  FC --> ART
  PER --> ART

  MCP["Partial: MCP lifecycle"] --> AUTH["P2: MCP auth/elicitation"]
  POL --> AUTH
  PER --> AUTH

  WT["Partial: Worktree lifecycle"] --> WTH["P2: Worktree hardening"]
  POL --> WTH
  MAIL --> WTH
  WTH --> SB["P3: Sandbox/remote"]

  FC --> RUI["P2 later: React compact diagnostics"]
  PER --> RUI
  MAIL --> RUI
  ART --> RUI
  AUTH --> RUI

  TD --> PKG["P3: Plugin/package governance"]
  POL --> ADV["P3: Advisory permission advisor"]

  TUI["Not needed: terminal UI/CLI main path"]:::notneeded
  FANTASY["Not needed: fantasy/provider rewrite"]:::notneeded
  MARKET["Not needed: marketplace-first install"]:::notneeded

  classDef notneeded fill:#eee,stroke:#999,color:#555;
```

## Next Batch Recommendation

### 1. Full Compact And Reinjection

- Goal: full compact summary, reinjection refs, compact-aware recovery.
- Not doing: React compact UI, fantasy/provider changes, cloud memory.
- Go: `internal/runtime/runtime_compact*.go`, `runtime_budget.go`,
  `runtime_context.go`, `runtime_recovery.go`, `runtime_replay_export.go`,
  `internal/agent/prompt`, `internal/db/read_files.sql.go`.
- React: none first; later DTO/display only.
- API/events: extend compact boundary APIs; add/complete
  `compact.full.completed`, `compact.auto.triggered`, `context.reinjected`,
  `compact.failed`.
- Data model: extend `runtime_compact_boundaries` only if current refs are
  insufficient.
- Tests: compact invariant, reinjection, redaction, replay/recovery.
- Acceptance: compact state is durable, replayable, and preserves transcript
  invariants.
- Risk: lost instructions, stale files, secret leakage.
- Blocked by: existing compact/context/read-file foundations.
- Unlocks: auto compact, session memory compact, React compact diagnostics,
  safer AgentTask transcripts.

### 2. Persisted Event Replay And Scenario Harness

- Goal: durable event replay and broader golden scenarios.
- Not doing: external analytics, first-party telemetry sinks.
- Go: `runtime_events.go`, `runtime_replay_export.go`, `runtime_audit*.go`,
  `runtime_scenario_harness_test.go`, possible `internal/db` migration.
- React: later audit/replay diagnostics.
- API/events: stable replay/export over persisted event source.
- Data model: add `runtime_events` table if bounded buffer remains insufficient.
- Tests: restart replay, cursor gaps, redaction, compact/policy/MCP/task/worktree
  fixtures.
- Acceptance: replay after buffer rollover explains turn/session state.
- Risk: duplicated audit semantics, sensitive payload persistence.
- Blocked by: existing audit/event/replay foundation.
- Unlocks: diagnostics and safer runtime hardening.

### 3. Policy Profiles And Shell Hardening

- Goal: deterministic headless/profile behavior and stronger shell parser.
- Not doing: model self-approval or enterprise RBAC.
- Go: `internal/permission/policy.go`, `internal/runtime/runtime_policy.go`,
  `runtime_permissions.go`, `runtime_scheduler_recorder.go`.
- React: later diagnostics/rule editor only.
- API/events: policy decision events include profile, rule, scope, shell risk.
- Data model: policy config plus audit should remain enough initially.
- Tests: policy tables, headless ask fail-closed, Bash/PowerShell/cmd fixtures.
- Acceptance: ambiguous non-interactive approvals fail closed and are replayable.
- Risk: shell false negatives, confusing rule precedence.
- Blocked by: scoped policy foundation exists.
- Unlocks: MCP auth, AgentTask mailbox, advisory permission later.

### 4. Tool Discovery Guardrails

- Goal: stronger recursion/concurrency/deadlock coverage for tool search.
- Not doing: React-selected tools or marketplace.
- Go: `runtime_tool_search.go`, `internal/agent/tool_search.go`,
  `internal/agent/loop_detection.go`, `internal/tools/scheduler`.
- React: later capability diagnostics.
- API/events: keep existing search/discovery/deadlock events.
- Data model: none expected.
- Tests: repeated search, max search, recursion, disabled/denied MCP, task
  scope.
- Acceptance: search omissions and guardrails are deterministic and auditable.
- Risk: over-blocking valid nested tool flows.
- Blocked by: capability/tool search/policy foundations exist.
- Unlocks: plugin governance and safer coordinator work.

### 5. React Diagnostics Later

- Goal: expose runtime compact/replay/task/policy/artifact/worktree facts.
- Not doing: frontend source of truth, frontend permission classifier.
- Go: runtime APIs above must exist first.
- React: `client/src/runtime/*`, `client/src/features/audit/*`,
  `client/src/features/chat/*`, `client/src/features/capabilities/*`,
  `client/src/features/permissions/*`.
- API/events: consume only runtime-owned APIs/events.
- Data model: none in React.
- Tests: client build and reload/recovery smoke.
- Acceptance: UI can be rebuilt from runtime APIs after reload.
- Risk: local reducers becoming business state.
- Blocked by: compact/replay/task/artifact/policy runtime APIs.
- Unlocks: product diagnostics.

## Self-Review Checklist

- All docs paths referenced above exist.
- Roadmap and audit point to this full review.
- Current code evidence is used instead of stale conclusions.
- Completed, Partial, Missing, Later, and Not needed are separated.
- Runtime blockers are separated from React diagnostics and P3 optimizations.
- Runtime remains prior to page work.
- `charm.land/fantasy` remains untouched.
