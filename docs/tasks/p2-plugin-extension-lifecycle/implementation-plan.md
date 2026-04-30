# P2.1 Implementation Plan

Date: 2026-04-29

## Phase 1: Lifecycle Contracts

- Add lifecycle types in `internal/tools`.
- Add unit tests for state normalization, identity keys, and operation results.
- Extend inventory projection structs with lifecycle fields while preserving existing fields.

## Phase 2: QueryEngine Ownership

- Add a lifecycle overlay map to QueryEngine.
- Initialize it from runtime options.
- Rebuild inventory deterministically by merging discovery data with overlay state.
- Ensure disabled tools are not exposed when the tool source can be enforced by registry policy or lifecycle filtering.

## Phase 3: Runtime Operations

- Add runtime methods for rebuild, reload, disable, enable, and mark degraded/failed.
- Implement explicit unsupported behavior for sources that cannot be safely reloaded or disabled yet.
- Keep command registry unchanged.

## Phase 4: Gateway Projection

- Add lifecycle fields to existing inventory item payloads.
- Preserve P1 payload fields.
- Add gateway serialization tests for lifecycle state and errors.

## Phase 5: Regression Coverage

- Add tests for deterministic rebuild after restart with recovered lifecycle state.
- Add permission boundary regression tests.
- Add command/tool/skill dedupe regression tests.
- Run all required validation commands.
