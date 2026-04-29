# P2.1 Plugin/Extension Lifecycle Hardening Task

Date: 2026-04-29

## Objective

Harden plugin-like extension lifecycle state on top of the P1 extension inventory without introducing a plugin marketplace or full plugin product surface.

P2.1 adds a runtime-owned lifecycle model for runtime commands, configured commands, dynamic tools, skills, MCP servers, and the future LSP placeholder.

## Scope

- Define lifecycle states: `discovered`, `loaded`, `active`, `degraded`, `disabled`, `failed`, `unloaded`, and `reloaded`.
- Add lifecycle metadata: source, type, name, version, capabilities, last error, last updated, and recovery behavior.
- Extend P1 extension inventory without removing or renaming existing inventory fields.
- Implement minimal runtime operations: discover/rebuild inventory, reload extension or source, disable extension, enable extension, and mark degraded/failed with error.
- Preserve runtime/queryengine ownership. Gateway and TUI remain projections.
- Add tests for lifecycle state, inventory projection, operations, recovery, gateway serialization, permissions, and dedupe.

## Non-Goals

- Do not implement a plugin marketplace.
- Do not implement complete LSP behavior.
- Do not rewrite the command registry.
- Do not make gateway or TUI a lifecycle source of truth.
- Do not weaken permission policy semantics.
- Do not introduce client-specific workarounds.

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

## Go Ownership Boundary

- `internal/tools`: lifecycle state model, extension identity, operation result contracts, skill lifecycle projection helpers.
- `internal/queryengine`: runtime-owned lifecycle state overlay, inventory rebuild, lifecycle operations, MCP state integration.
- `internal/runtime`: public runtime API delegating to QueryEngine-owned lifecycle state.
- `internal/gateway`: read-only serialization of runtime lifecycle projections.
- `internal/permissions`: unchanged execution authority. Lifecycle allowed-tools and permission hints are advisory metadata only.

## Implementation Order

1. Add task documents in this folder.
2. Add failing lifecycle state model and inventory projection tests.
3. Add lifecycle model and projection fields.
4. Add failing runtime operation tests.
5. Implement QueryEngine lifecycle operations and runtime wrappers.
6. Add failing gateway serialization tests.
7. Serialize lifecycle fields in gateway projection.
8. Add permission and dedupe regression tests.
9. Run focused validation and `go test ./...`.

## Validation Requirements

Run:

```powershell
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/permissions ./internal/tools ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./...
```

## Completion Output Requirements

Report:

- created or modified docs files
- created or modified code files
- lifecycle state model
- runtime source of truth location
- gateway projection fields
- persistence and recovery rules
- unsupported or deferred behavior
- added tests
- validation commands and results
- whether P2.2 entry criteria are met
- final `git status`
- commit hash if committed
