# Claude Code Next Implementation Plan

This document turns the latest parity audit and roadmap into implementation
boundaries for the next runtime phases. It is scoped to Agent Builder runtime
planning only. Do not use it to justify React-owned business state, fantasy
changes, TUI/CLI restoration, or provider rewrites.

Primary inputs:

- [`docs/claude-code-runtime-parity-audit.md`](./claude-code-runtime-parity-audit.md)
- [`docs/claude-code-alignment-next-roadmap.md`](./claude-code-alignment-next-roadmap.md)
- [`docs/client-runtime-architecture-review.md`](./client-runtime-architecture-review.md)
- [`docs/turn-task-run-model.md`](./turn-task-run-model.md)
- [`docs/tool-scheduler-design.md`](./tool-scheduler-design.md)
- [`docs/permission-policy-model.md`](./permission-policy-model.md)
- [`docs/client-state-recovery.md`](./client-state-recovery.md)
- [`docs/archive/phase-2-runtime-api-boundary.md`](./archive/phase-2-runtime-api-boundary.md)
- [`docs/client-architecture-and-core-flow.md`](./client-architecture-and-core-flow.md)

## Implementation Principles

- `charm.land/fantasy` must not be modified.
- Go runtime is the source of truth.
- React renders runtime facts and holds UI-only state.
- Wails and HTTP remain adapters.
- TUI/CLI must not return as the product main path.
- Compact is runtime lifecycle, not a UI command.
- Tool search and deadlock avoidance live in scheduler/runtime.
- Agent communication/coordinator is a runtime primitive.
- Provider/model config is a product policy/config layer above fantasy.
- Model-assisted permission is advisory only and cannot approve actions.

## First Module To Implement

Next module: `Compact / context budget lifecycle foundation`.

The first implementation commit should be:

```text
runtime: record compact boundaries
```

This first commit should not change model-loop behavior. It should add durable
compact boundary records, DTOs, event names, audit entries, and read APIs so
later commits can safely add budget accounting, micro compact, full compact,
auto compact, and reinjection.

### Why This Beats Other Candidates

Compact/context budget is the missing lifecycle that makes every long-running
runtime feature safer. Tool search needs budget numbers, subagents need
compact-aware transcripts, recovery needs compact boundaries, and evals need
observable compact events. Starting with coordinator, plugin governance, or
worktree isolation first would add more runtime volume and risk before context
governance exists.

## Phase Map

| Phase | Status | Modules |
| --- | --- | --- |
| P0 | Completed foundation | Runtime spine, event cursor, audit, recovery, ToolCall store, PermissionPolicy baseline, capability inventory. |
| P1 Next | Immediate | Compact boundary, context/prompt budget, micro compact. |
| P1 Parallel | Parallel | Tool search/discovery/deadlock avoidance, scoped policy/shell safety, scenario eval/replay harness. |
| P2 | Blocked by P1 | AgentTask roles/communication, MCP/skills scoped activation, provider/model policy diagnostics, worktree isolation, React diagnostics. |
| P3 Later | Later | Plugin governance, advisory permission advisor, advanced memory lifecycle, sandbox/remote runtime. |
| Not needed | Excluded | TUI/CLI UI, slash-command UI, fantasy/provider rewrite, marketplace-first distribution. |

## Module Boundaries

### Compact / Context Budget Lifecycle

| Field | Plan |
| --- | --- |
| Goal | Add runtime-owned compact boundaries, budget accounting, micro compact, full compact, auto trigger metadata, session-memory compact hooks, and post-compact reinjection path. |
| Non-goals | No fantasy changes, no React-owned compact state, no UI-only compact button as source of truth, no remote memory service, no immediate full summarization in the first commit. |
| Go packages | Add `internal/runtime/runtime_compact*.go`; touch `internal/runtime/runtime_contract_types.go`, `runtime_service_types.go`, `runtime_http.go`, `runtime_events.go`, `runtime_audit.go`, `runtime_recovery.go`, `internal/agent/prompt`, `internal/message`, `internal/tools/scheduler`, `internal/db` migrations as needed. |
| React packages | Later read-only diagnostics in `client/src/runtime/types.ts`, `client/src/runtime/api.ts`, `client/src/features/audit`, `client/src/features/chat`; no business logic. |
| Runtime API / event schema | Add `GET /v1/turns/{turn_id}/compact`, `GET /v1/sessions/{session_id}/compact`, optional `POST /v1/turns/{turn_id}/compact` only after policy is clear. Events: `compact.boundary.recorded`, `compact.micro.completed`, `compact.full.completed`, `compact.failed`, `budget.updated`. |
| Data model changes | Add `runtime_compact_boundaries` with id, session_id, turn_id, kind, trigger, status, budget_before_json, budget_after_json, summary_ref, message_refs_json, tool_call_refs_json, reinjected_refs_json, error, created_at, completed_at. Add output ref columns only when micro compact needs them. |
| Tests | Store tests, event/audit redaction tests, recovery snapshot tests, message ToolCall invariant tests, budget accounting tests, micro compact replacement tests. |
| Acceptance | Compact boundary records survive restart; audit explains trigger/status; events are cursor-compatible; full tool outputs are not silently lost; React can refresh from API; no provider/fantasy changes. |
| Risks | Breaking model transcript tool-use/tool-result pairing, summarizing away instructions/files, storing secrets in compact summaries, making compact UI-driven. |
| Blocked by | Existing context source audit, read-file state, ToolCall store, audit/event cursor. These foundations exist. |
| Unlocks | Tool search budget, auto compact, full compact, session memory compact, post-compact reinjection, compact-aware recovery, long-running AgentTasks. |

Implementation split:

1. Compact boundary record and audit only.
2. Budget accounting for messages/context/tool schemas/tool results.
3. Micro compact for old/high-cost tool outputs.
4. Full compact model summary path.
5. Auto trigger thresholds and circuit breaker.
6. Post-compact reinjection from context sources and read-file state.
7. Session memory compact and advanced memory lifecycle.

### Tool Search / Discovery / Deadlock Avoidance

| Field | Plan |
| --- | --- |
| Goal | Keep large tool/capability surfaces out of every prompt, let the model discover tools through runtime-mediated metadata, and add recursion/deadlock/concurrency guardrails. |
| Non-goals | No React-selected tool set, no marketplace, no policy bypass, no model self-approval. |
| Go packages | `internal/runtime/runtime_capabilities.go`, future `runtime_tool_search.go`, `internal/tools/scheduler`, `internal/agent/tools/tools.go`, `internal/permission`. |
| React packages | `client/src/features/capabilities` diagnostics only after runtime API exists. |
| Runtime API / event schema | Add searchable metadata in `RuntimeCapability`; possible `POST /v1/tools/search`; events `tool.search.performed`, `tool.discovery.selected`, `scheduler.deadlock.prevented`. |
| Data model changes | Add search metadata fields if needed: description, keywords, source, risk, input schema digest, load state. Durable search audit can live in runtime audit first. |
| Tests | Selection tests, denied capability tests, budget omission tests, recursion/deadlock tests, MCP/skill capability discovery tests. |
| Acceptance | Model-facing prompt can omit non-selected tool schemas; selected tools are auditable; disabled or denied capabilities are not selected; recursion limits fail closed with audit. |
| Risks | Hiding a necessary tool, exposing disabled MCP/skills, adding model-visible metadata that leaks secrets, blocking valid nested tool flows. |
| Blocked by | Capability registry exists; budget accounting should land first for prompt pressure decisions; scoped policy improves enforcement. |
| Unlocks | Plugin governance, larger MCP surfaces, safer AgentTask role tool scopes. |

### Agent Coordinator / Agent Communication

| Field | Plan |
| --- | --- |
| Goal | Upgrade AgentTask from persisted child work into scoped agent roles with structured parent/child messaging, results, artifacts, and later coordinator/team semantics. |
| Non-goals | No swarm product UI first, no remote fleet, no UI event stitching as communication, no marketplace agents. |
| Go packages | `internal/runtime/runtime_agent_tasks.go`, `runtime_agent_task_store.go`, `internal/agent/agent_tool.go`, `internal/agent/coordinator.go`, future role registry package. |
| React packages | Later `client/src/features/chat`, possible task panel in `client/src/features/*`; render runtime state only. |
| Runtime API / event schema | Extend task APIs with message/result/artifact endpoints. Events: `task.message.created`, `task.result.updated`, `task.artifact.created`, `coordinator.message.routed`. |
| Data model changes | Add role definitions, task messages/mailbox, task artifacts, parent/child message refs. Reuse `RuntimeAgentTask` fields for allowed_tools, capability_scope, cwd, worktree. |
| Tests | Scope enforcement, parent/child linkage, cancellation, interrupted recovery, artifact refs, compact-aware transcript handoff. |
| Acceptance | Child agents cannot exceed allowed tools/model/cwd/capabilities; parent sees structured results; task cancellation and recovery are auditable; no React-derived task state. |
| Risks | Agent recursion loops, unbounded context growth, unclear ownership of child transcript, artifacts without durable refs. |
| Blocked by | Compact/budget, scoped policy, scheduler deadlock limits. |
| Unlocks | Coordinator mode, teammate communication, worktree-isolated tasks, richer task panels. |

### Provider / Model Configuration On Fantasy

| Field | Plan |
| --- | --- |
| Goal | Harden product-level provider/model policy, health, verification, redaction, per-mode model selection, and model capability display while keeping fantasy as the provider abstraction. |
| Non-goals | No provider client rewrite, no model streaming rewrite, no model-facing message/tool protocol outside fantasy. |
| Go packages | `internal/runtime/runtime_model*.go`, config packages, audit/redaction helpers. |
| React packages | `client/src/features/settings/ModelSettingsDrawer.tsx`, runtime types/API. |
| Runtime API / event schema | Existing model config APIs continue. Add health/capability diagnostics and events such as `model.config.updated`, `model.health.checked` if needed. |
| Data model changes | Possibly add model capability cache and per-mode model policy config; keep API keys redacted. |
| Tests | Redaction tests, verify/discover model tests, config persistence tests, policy-aware model selection tests. |
| Acceptance | UI can show model health/capabilities from runtime; config is persisted/redacted; fantasy remains untouched. |
| Risks | Duplicating fantasy provider logic, leaking credentials, mixing UI preferences with runtime policy. |
| Blocked by | Existing model config foundation is enough; not blocking compact. |
| Unlocks | Per-mode model policy, cheaper compact model selection, advisory permission classifier model choice. |

### Worktree / Sandbox / Remote Isolation

| Field | Plan |
| --- | --- |
| Goal | Add explicit isolation primitives while keeping cwd override, worktree, sandbox, and remote runtime separate in API, policy, and audit. |
| Non-goals | No first-pass full sandbox parity, no remote runtime before local semantics, no single boolean `isolated` flag. |
| Go packages | Future runtime isolation package, `internal/runtime/runtime_agent_tasks.go`, `internal/permission`, `internal/tools/scheduler`, shell tools. |
| React packages | Later task/isolation diagnostics only. |
| Runtime API / event schema | Events: `worktree.created`, `worktree.entered`, `worktree.exited`, `worktree.cleaned`, `sandbox.denied`, `remote.runtime.connected` later. |
| Data model changes | Worktree table or task worktree fields plus cleanup/preserve status, base branch/ref, cwd, policy decision, audit refs. |
| Tests | Path safety, cleanup/preserve, cancellation cleanup, shell policy enforcement, cross-platform path handling. |
| Acceptance | Worktree lifecycle is durable and auditable; tasks/tools know cwd/worktree; cleanup is explicit; sandbox/remote remain separate later phases. |
| Risks | Data loss through cleanup, path traversal, unclear current working directory, Windows shell differences. |
| Blocked by | AgentTask scopes, shell safety, scoped policy. |
| Unlocks | Safer high-risk implementation tasks, remote/sandbox later. |

### Observability / Evals / Replay

| Field | Plan |
| --- | --- |
| Goal | Build local scenario fixtures and replay/export paths for compact, policy, scheduler, MCP, skills, AgentTask, and recovery regressions. |
| Non-goals | No first-party analytics import, no external telemetry dependency, no product growth reporting. |
| Go packages | `internal/runtime/runtime_audit*.go`, `runtime_events.go`, test harness packages, possibly `internal/evals` later. |
| React packages | Later `client/src/features/audit` diagnostics and export UI. |
| Runtime API / event schema | Add replay/export read paths only after fixture format stabilizes. Events remain runtime events; replay should not fabricate source-of-truth state. |
| Data model changes | Initially none beyond audit/event data; later persisted event log/export artifacts. |
| Tests | Golden scenario runner for plan mode, auto_read, shell denial, MCP disabled/refresh, skill allowed_tools hint, compact boundary, event cursor, AgentTask cancellation. |
| Acceptance | Scenario harness catches policy/compact/tool regressions locally; audit exports redact secrets; replay identifies snapshot-required gaps. |
| Risks | Brittle golden fixtures, storing secrets in fixtures, confusing replay with live runtime state. |
| Blocked by | Can start now using existing audit/events; compact scenarios wait for compact boundary. |
| Unlocks | Safer P1/P2 expansion, policy advisor validation, regression confidence. |

### Capability Package / Plugin Governance

| Field | Plan |
| --- | --- |
| Goal | Add local/managed capability package manifests, trust state, enable/disable, version/source metadata, and audit. |
| Non-goals | No marketplace-first distribution, no remote install flow first, no plugin permissions outside runtime policy. |
| Go packages | `internal/runtime/runtime_capabilities.go`, `internal/skills`, MCP config, future package registry. |
| React packages | Later capability governance panels. |
| Runtime API / event schema | Events: `capability.package.discovered`, `capability.package.enabled`, `capability.package.disabled`, `capability.package.failed`. |
| Data model changes | Package manifest table or config section with id, source, version, trust, enabled, capabilities, policy hints. |
| Tests | Manifest parsing, trust enforcement, disabled package exclusion, audit redaction, policy scope integration. |
| Acceptance | Packages cannot silently grant permissions; local trust state is visible; capability inventory reflects package state. |
| Risks | Permission bypass, duplicate capability IDs, premature marketplace coupling. |
| Blocked by | Tool search, scoped policy, MCP/skills activation enforcement. |
| Unlocks | Managed extensions and signed package flow later. |

### Adaptive / Model-Assisted Permission Advisor

| Field | Plan |
| --- | --- |
| Goal | Add an advisory classifier that can summarize intent, explain risk, suggest policy outcomes, and improve review UX while deterministic Go policy remains final. |
| Non-goals | No model-approved permissions, no automatic high-risk allow, no hidden policy changes, no replacing scoped policy. |
| Go packages | `internal/permission`, runtime policy/audit files, model config policy layer. |
| React packages | Later permission diagnostics display advisory explanation. |
| Runtime API / event schema | Events/audit may include `permission.advisor.evaluated`; decisions remain `allow/ask/deny` from deterministic policy/user decision. |
| Data model changes | Advisory result fields: risk_explanation, suggested_decision, confidence, model, prompt hash, redacted inputs. |
| Tests | Advisor cannot override deny/ask, redaction tests, regression fixtures comparing deterministic outcome and advisory text. |
| Acceptance | High-risk actions still require deterministic allow or user approval; advisor output is auditable and redacted. |
| Risks | Users overtrusting advisor, prompt injection, leaking tool inputs, accidental auto-approval. |
| Blocked by | Scoped policy rules, shell safety, scenario eval harness. |
| Unlocks | Better permission UX, plan-exit explanations, policy tuning diagnostics. |

### Advanced Context / Memory Lifecycle Beyond Baseline

| Field | Plan |
| --- | --- |
| Goal | Build on compact foundation with session memory compact, memory taxonomy, include/frontmatter support, read-file reinjection, and compact-aware recovery. |
| Non-goals | No cloud memory service, no React memory ownership, no opaque summaries without source refs. |
| Go packages | `internal/agent/prompt`, `internal/runtime/runtime_context.go`, future compact/memory files, read-file tracker. |
| React packages | Diagnostics only after runtime read APIs exist. |
| Runtime API / event schema | Events: `memory.compact.completed`, `context.reinjected`, `context.source.skipped`, `context.source.failed`. |
| Data model changes | Memory compact records, source refs, reinjection refs, summary provenance. |
| Tests | Include/frontmatter precedence, read-file reinjection, compact source replay, summary redaction. |
| Acceptance | Compacted sessions retain required instructions and referenced files; source provenance is auditable; recovery can explain reinjection. |
| Risks | Losing instructions, stale file reinjection, token budget oscillation, source precedence bugs. |
| Blocked by | Compact boundary, full compact, read-file state, context source audit. |
| Unlocks | Long autonomous sessions and richer resume semantics. |

### React Diagnostics / Audit Deepening

| Field | Plan |
| --- | --- |
| Goal | Expose runtime-owned compact, budget, task, policy, search, and replay diagnostics in React without making React a state source. |
| Non-goals | No frontend-derived audit, no frontend permission risk decisions, no event-only reconstruction as truth. |
| Go packages | Runtime APIs must exist first. |
| React packages | `client/src/runtime/types.ts`, `client/src/runtime/api.ts`, `client/src/features/audit`, `client/src/features/chat`, `client/src/features/capabilities`, `client/src/features/permissions`. |
| Runtime API / event schema | Consume compact/budget/task/policy/replay APIs and events; refresh facts from API. |
| Data model changes | None in React; TypeScript DTOs mirror runtime contracts. |
| Tests | Type/build tests, component tests if present, Playwright/smoke for diagnostics flows. |
| Acceptance | Refresh after reload shows same compact/task/policy facts; UI cannot approve or synthesize runtime decisions. |
| Risks | Reducers becoming source of truth, stale event-derived state, overwhelming chat timeline. |
| Blocked by | Runtime APIs for compact, task communication, policy scopes, replay. |
| Unlocks | Usable product diagnostics and audit review. |

## Commit Plan

1. `runtime: record compact boundaries`
   - Scope: compact boundary DTO/store/events/audit/read APIs.
   - Tests: compact store, audit redaction, event cursor, recovery no-op.

2. `runtime: add context and prompt budget accounting`
   - Scope: budget data for context sources, messages, tool schemas, skills,
     MCP, tool outputs.
   - Tests: budget table tests, audit summary tests.

3. `runtime: add micro compact output replacement`
   - Scope: deterministic old/high-cost tool output replacement with refs.
   - Tests: ToolCall/message invariants, output ref preservation, audit.

4. `runtime: expose tool search metadata`
   - Scope: capability metadata, discovery/search path, selected-tool audit.
   - Tests: search selection, disabled capability exclusion, policy denied.

5. `runtime: add scheduler deadlock limits`
   - Scope: recursion, nested tools, agent recursion, concurrency policy.
   - Tests: scheduler deadlock and cancellation scenarios.

6. `runtime: add scoped permission rules`
   - Scope: deterministic tool/MCP/skill/subagent/cwd/shell rule matcher.
   - Tests: policy tables and runtime scenarios.

7. `runtime: harden shell policy classification`
   - Scope: Bash/PowerShell destructive/read-only parsing improvements.
   - Tests: shell command fixture pack.

8. `runtime: enforce agent task scopes`
   - Scope: role definitions and allowed tools/model/cwd/capability scopes.
   - Tests: scope enforcement, child session linkage, cancellation/recovery.

9. `runtime: add parent child agent messaging`
   - Scope: message/result/artifact protocol for task communication.
   - Tests: parent/child transcript, artifact refs, task notifications.

10. `runtime: add scenario eval replay harness`
    - Scope: local golden scenario runner and redacted audit/export fixtures.
    - Tests: compact, policy, MCP, skills, tasks, recovery, event cursor.

## Acceptance Checklist For This Planning Phase

- Roadmap points to this implementation plan.
- Every referenced docs path exists.
- Priority order starts with compact/context budget and explains why.
- All required candidate modules are covered with boundaries.
- Not-needed and later/P3 surfaces are explicit.
- Mermaid graph exists in the roadmap.
- Future commits have scope and main tests.
- No Go/React business code changes are required by this docs update.
