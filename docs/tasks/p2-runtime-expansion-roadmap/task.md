# P2 Runtime Expansion Roadmap Task

Date: 2026-04-29

## Objective

Plan P2 runtime expansion from the completed P1 foundation without destabilizing P0/P1 contracts.

P2 should add lifecycle depth and external integration surfaces only where shared runtime contracts are strong enough to support them.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p1-runtime-maturation-roadmap/task.md`
5. `docs/tasks/p1-runtime-maturation-closure/task.md`
6. `docs/tasks/p2-runtime-expansion-roadmap/design.md`
7. `docs/tasks/p2-runtime-expansion-roadmap/source-alignment.md`
8. `docs/tasks/p2-runtime-expansion-roadmap/implementation-plan.md`
9. `docs/tasks/p2-runtime-expansion-roadmap/test-validation-plan.md`
10. `docs/tasks/p2-runtime-expansion-roadmap/review-checklist.md`

## Stable Contracts

P2 must preserve:

- shared runtime slash command registry
- QueryEngine single input processing
- session/appstate recovery projection
- approval continuation semantics
- context cache invalidation and deterministic rebuild
- subagent allowed-tools, permission, cwd, worktree, output file, and background state recovery
- runtime-owned extension inventory and gateway/TUI projection boundaries

## Workstreams

### P2.1 Plugin/Extension Lifecycle Hardening

Entry criteria:

- P1 extension inventory rebuilds deterministically.
- Runtime commands and configured commands dedupe correctly.
- Skills and MCP state are visible through runtime inventory.

Acceptance criteria:

- plugin-like extension install/reload/unload lifecycle has runtime state
- lifecycle events are persisted or recoverable
- permission and allowed-tools metadata remains advisory until enforced by runtime policy
- gateway exposes lifecycle projection without owning truth

Test requirements:

- lifecycle state transition tests
- restart/reconnect recovery tests
- command/tool/skill dedupe tests
- permission regression tests

### P2.2 LSP Runtime Capability

Entry criteria:

- P1 LSP inventory boundary exists.
- Tool contract and permission policy paths are stable.

Acceptance criteria:

- LSP server discovery and lifecycle state are modeled
- LSP-backed tools expose explicit contracts
- read-only versus mutating LSP actions are permission classified
- unavailable LSP remains a degraded, visible state

Test requirements:

- LSP lifecycle unit tests
- tool contract projection tests
- permission classification tests
- gateway inventory serialization tests

### P2.3 Remote/Bridge/Trusted-Device Foundation

Entry criteria:

- session recovery and approval continuation are stable.
- gateway projection contracts are stable.

Acceptance criteria:

- remote/bridge sessions have explicit identity and trust state
- approval forwarding has durable correlation IDs
- reconnect/liveness semantics are defined
- trusted-device state is modeled as foundation only, not full enterprise management

Test requirements:

- reconnect/liveness tests
- approval forwarding correlation tests
- trust-state persistence tests
- gateway protocol compatibility tests

### P2.4 Advanced Execution Surfaces

Entry criteria:

- execution surface still aligns with roadmap priorities.
- permission and sandbox boundaries are explicit.

Acceptance criteria:

- each execution surface has runtime contract, permission integration, progress/result semantics, and control-plane visibility
- no execution surface remains only a shell snippet

Test requirements:

- contract tests per surface
- permission and approval tests
- progress/result event tests
- recovery tests for long-running work

### P2.5 Operator/UI Integration Readiness

Entry criteria:

- gateway projection contracts remain stable.
- runtime inventory and continuation payloads are complete enough for a UI host.

Acceptance criteria:

- operator-facing payloads are versioned or compatibility-safe
- UI consumes runtime/gateway projection only
- no React/operator feature becomes a backend source of truth

Test requirements:

- gateway payload compatibility tests
- reconnect projection tests
- TUI regression tests
- snapshot or contract tests for payload shape

## Non-Goals

P2 planning alone does not implement:

- full React operator UI
- enterprise managed settings
- plugin marketplace billing/discovery product flows
- full Claude Code remote parity
- telemetry/GrowthBook clone

## Completion Output

For each P2 child task, report:

- entry criteria status
- changed runtime contracts
- tests added
- regression commands
- deferred P3 items

