# P2.2 LSP Runtime Capability Implementation Plan

Date: 2026-04-30

## Phase 1 Documentation

- Create the P2.2 task document set.
- Record scope, non-goals, lifecycle mapping, source ownership, and validation.

## Phase 2 Tests First

- Add `internal/tools` tests for LSP config normalization and read-only tool contracts.
- Add `internal/queryengine` tests for inventory projection, lifecycle operations, deterministic rebuild, and session/store recovery.
- Add `internal/runtime` tests for runner pass-through and P2.1 regression coverage where needed.
- Add `internal/gateway` tests for `extension_inventory` serialization.

## Phase 3 LSP Model And Tools

- Add LSP config and normalized server model in `internal/tools`.
- Add LSP read-only tool contracts with explicit unavailable/degraded errors.
- Register tools when LSP servers are configured or when default runtime options provide LSP contracts.

## Phase 4 Runtime Projection

- Add LSP server options to runtime and QueryEngine config.
- Add QueryEngine-owned LSP state assembly in a separate `lsp_runtime.go`.
- Apply lifecycle overlays to LSP boundaries.
- Preserve placeholder behavior when no LSP servers exist.

## Phase 5 Gateway And Recovery

- Extend gateway boundary serialization with LSP-specific fields while preserving existing keys.
- Reuse lifecycle metadata persistence and recovery for LSP overlays.
- Ensure enable clears persisted disabled state.

## Phase 6 Validation

- Run focused packages.
- Run required validation commands.
- Run whitespace check.
- Check queryengine production file line counts.
- Commit if all checks pass.
