# Usage-Driven Remaining Roadmap

This document records the remaining work after Phase 51 so future optimization
can proceed from user-visible behavior and concrete usage questions instead of
expanding runtime authority prematurely.

Current baseline:

- Narrow activity, cursor reads, hosted MCP replay/auth/elicitation hardening,
  persisted Run summary reads, and checkpoint marker reads are complete as
  bounded workstreams.
- Full `SessionActivity` and full `RunProjection` remain the parity oracles for
  lifecycle, artifacts, diagnostics, interrupted summaries, permission state,
  MCP actionability, scheduler state, and terminal/recovery decisions.
- Runtime events, action metadata, transition rows, summary DTOs, marker DTOs,
  assistant prose, and browser memory are refresh hints or read-only evidence,
  not sources of lifecycle/actionability truth.

Non-goals that remain in force:

- No full Run state machine.
- No runtime Run store expansion beyond accepted narrow helpers.
- No database migration.
- No automatic resume.
- No background scheduler loop.
- No stale running/waiting tool recovery.
- No stale permission recovery.
- No stale MCP auth/elicitation recovery.
- No frontend Run UI without a separate ownership gate.
- No React-owned runtime state.

## Remaining Phase Outline

### Phase 52: Run Lifecycle Conflict Display Authority Design Gate

Define how read surfaces should present conflicts between persisted Run status
and full `RunProjection` / `SessionActivity` evidence.

Expected outcome:

- A display-only conflict policy.
- Explicit rules for active/interrupted/terminal conflicts.
- Full projection parity remains required for terminal/recovery lifecycle.
- Persisted Run status is not accepted as sole lifecycle truth.

### Phase 52.x: Lifecycle Conflict Contracts And Narrow Read Implementation

Add tests before implementation.

Expected coverage:

- Persisted active vs full terminal evidence.
- Persisted terminal vs full active/interrupted evidence.
- Interrupted acknowledgement semantics remain cancelled terminal semantics.
- Bounded/windowed reads cannot resolve lifecycle conflicts.
- Events/action metadata/transition history alone cannot resolve lifecycle
  conflicts.

Implementation, if accepted, should be read-only and DTO-scoped.

### Phase 53: Transport And Adapter Authority Smoke

Validate that lifecycle conflict display reads remain backend DTO rereads.

Expected coverage:

- HTTP/dev-module route contracts.
- Low-level adapter smoke.
- No hydration merge from event payloads or action metadata.
- No `WorkbenchViewModel` Run UI state unless separately accepted.

### Phase 54: Product Binding And UI Ownership Gate

Only after backend/transport authority is stable, decide whether any user-facing
UI should expose these read-only Run surfaces.

Questions to answer from user experience:

- What problem does the UI solve for the user?
- Is it diagnostic-only, recovery-oriented, or primary workflow state?
- Which full runtime DTO is the source of truth?
- What stale actionability must remain impossible after reload/restart?

Expected default:

- No frontend Run UI until a concrete user workflow justifies it.

### Phase 55: Usage-Driven Workflow Review

Review real app behavior around the workflows the user asks about:

- Long conversation reload.
- Restart after interrupted turn.
- Tool result truncation/persistence behavior.
- Artifact/ref visibility.
- Permission and MCP request recovery.
- Scheduler task visibility.
- Checkpoint acknowledgement/discard/resume marker visibility.

Each workflow should be handled with a small gate:

- describe the user problem;
- identify the runtime source of truth;
- add contract/smoke coverage;
- implement only the smallest read or action surface needed.

### Phase 56: Packaged And Browser Smoke Consolidation

Consolidate the critical browser/Vite and packaged Wails smoke tests that prove
the user-visible app still follows backend DTO authority.

Expected coverage:

- Event-triggered refresh rereads runtime DTOs.
- Reload/reconnect does not resurrect stale permission/MCP/checkpoint
  actionability.
- Tool result guard/persistence behavior is visible without leaking secrets or
  relying on assistant prose.
- Marker/summary DTOs remain explicit reads, not timeline/actionability state.

### Phase 57+: Final Closure Gates

Close the long-conversation hardening workstream only after the usage-driven
questions no longer expose missing authority contracts.

Closure criteria:

- Full and narrow reads have parity tests where needed.
- Hosted MCP and local tool flows have restart/replay coverage.
- Frontend state ownership is documented and smoke-tested.
- Remaining risks are explicitly listed as product decisions rather than hidden
  runtime ambiguity.

## How To Use This Roadmap

For each new user-facing concern:

1. Start from the user-visible behavior.
2. Identify which backend DTO or store is authoritative.
3. Add a design gate if authority is unclear.
4. Add contract tests before implementation.
5. Add transport/adapter smoke before UI use.
6. Keep UI state derived from backend DTO rereads.

This roadmap is intentionally conservative. It keeps the project aligned with
Claude Code-inspired runtime discipline while avoiding premature persistence,
automatic recovery, or frontend-owned orchestration.
