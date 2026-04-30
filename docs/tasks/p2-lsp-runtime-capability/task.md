# P2.2 LSP Runtime Capability Task

Date: 2026-04-30

## Objective

Implement the first runtime-owned LSP capability layer on top of P1 extension inventory and P2.1 lifecycle persistence.

P2.2 defines LSP server configuration, lifecycle projection, minimal read-only tool contracts, permission classification, gateway serialization, and restart recovery. It does not implement a full IDE, full LSP protocol client, React UI, plugin marketplace, or remote trusted-device behavior.

## Scope

- Define LSP states: `discovered`, `configured`, `starting`, `active`, `degraded`, `failed`, `disabled`, and `stopped`.
- Map shared states to the existing P2.1 lifecycle constants where possible.
- Add a minimal LSP server config model: name, language IDs, file patterns, command, args, env, cwd, workspace root, enabled flag, capability hints, and read-only/mutating classification.
- Keep QueryEngine/runtime as the source of truth for LSP state. Gateway and TUI consume projections only.
- Replace the single deferred LSP placeholder with inventory entries for configured LSP servers while preserving the `lsp_boundaries` payload shape.
- Register minimal LSP-backed read-only tool contracts that report explicit unavailable/degraded status unless a mockable runtime handler is configured.
- Route LSP read-only tools through `internal/permissions`; defer mutating LSP actions.
- Support disable, enable, degraded, failed, rebuild, and explicit unsupported reload/start behavior through P2.1 lifecycle APIs.
- Persist disabled, degraded, and failed LSP overlays through session metadata and recover them through the existing session/store path.
- Add gateway projection for LSP lifecycle fields and LSP-specific coverage/classification fields.

## Non-Goals

- Do not implement a full LSP protocol client or external process lifecycle.
- Do not implement code action apply, edit, rename, or mutating LSP tools.
- Do not build React/operator UI.
- Do not implement a plugin marketplace.
- Do not make gateway, TUI, or SDK paths infer or own LSP state.
- Do not weaken P0/P1/P2.1 command, permission, recovery, or inventory contracts.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p1-runtime-maturation-closure/task.md`
5. `docs/tasks/p2-runtime-expansion-roadmap/task.md`
6. `docs/tasks/p2-runtime-expansion-roadmap/design.md`
7. `docs/tasks/p2-runtime-expansion-roadmap/source-alignment.md`
8. `docs/tasks/p2-runtime-expansion-roadmap/implementation-plan.md`
9. `docs/tasks/p2-runtime-expansion-roadmap/test-validation-plan.md`
10. `docs/tasks/p2-runtime-expansion-roadmap/review-checklist.md`
11. `docs/tasks/p2-plugin-extension-lifecycle/task.md`
12. `docs/tasks/p2-plugin-extension-lifecycle/design.md`
13. `docs/tasks/p2-plugin-extension-lifecycle/review-checklist.md`

## Claude Semantic Alignment

Claude Code treats LSP-like capability as a runtime extension surface with conservative lifecycle state, permission-gated tool access, and client-neutral projection. P2.2 follows that shape without claiming full protocol parity.

## Go Ownership Boundary

- `internal/tools`: LSP config model, lifecycle constants mapping, tool contracts, mockable handler boundary.
- `internal/queryengine`: runtime-owned LSP server state, lifecycle overlay application, inventory assembly.
- `internal/runtime`: runner options and public projection wrappers.
- `internal/gateway`: read-only serialization of runtime projections.
- `internal/permissions`: unchanged permission authority for read-only and future mutating actions.

## Implementation Order

1. Create this task document set.
2. Add failing tests for LSP config normalization and tool contracts.
3. Add failing QueryEngine/runtime inventory and lifecycle tests.
4. Add failing session/store recovery tests for LSP overlays.
5. Add failing gateway serialization tests.
6. Implement LSP model and read-only tool contracts.
7. Wire QueryEngine/runtime config, inventory projection, lifecycle overlay, and persistence.
8. Wire gateway payload fields.
9. Run focused tests, full validation, whitespace check, status, and commit.

## Validation Requirements

Run:

```powershell
git diff --check origin/main..HEAD
go test ./internal/queryengine
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/permissions ./internal/tools ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./...
```

## Completion Output Requirements

Report:

- created or modified docs files
- created or modified code files
- LSP lifecycle model and P2.1 mapping
- runtime source of truth location
- LSP inventory and gateway projection fields
- LSP tool contracts and permission classification
- persistence and recovery rules
- unsupported or deferred behavior
- added or updated tests
- queryengine production file line counts
- validation command results
- whether P2.3 entry criteria are met
- final `git status --short --branch`
- commit hash if committed

## Starter Prompt

Implement P2.2 LSP Runtime Capability in the current branch. Read this task and companion docs first, then use TDD to add the runtime-owned LSP model, inventory projection, lifecycle recovery, read-only tool contracts, permission classification, gateway serialization, validation, and commit.
