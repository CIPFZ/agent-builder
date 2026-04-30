# P2.1 Review Checklist

Date: 2026-04-29

## Scope

- [x] Lifecycle states cover discovered, loaded, active, degraded, disabled, failed, unloaded, and reloaded.
- [x] Inventory covers runtime commands, configured commands, dynamic tools, skills, MCP servers, and LSP placeholder.
- [x] Marketplace and full LSP behavior remain deferred.

## Ownership

- [x] QueryEngine/runtime shared layer owns lifecycle state.
- [x] Gateway and TUI are projections only.
- [x] Existing P1 inventory fields remain compatible.

## Operations

- [x] Rebuild, reload, disable, enable, mark degraded, and mark failed APIs exist.
- [x] Unsupported source behavior returns explicit errors.
- [x] Disabled/degraded/failed recovery behavior is documented and tested.

## Permission Boundary

- [x] Allowed-tools metadata remains advisory.
- [x] Runtime permission policy remains the execution authority.
- [x] Tests prove lifecycle metadata cannot bypass denied tools.

## Validation

- [x] Focused package tests pass.
- [x] Full `go test ./...` passes.
- [x] Completion output includes git status and commit hash.
