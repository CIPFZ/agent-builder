# P2.2 LSP Runtime Capability Test Validation Plan

Date: 2026-04-30

## Unit Tests

- LSP config normalization handles names, language IDs, file patterns, env, cwd, workspace root, enabled defaults, and capability classification.
- LSP lifecycle state normalization preserves LSP-specific states and maps shared lifecycle states.
- LSP read-only tools expose stable contracts, input schemas, read-only classification, and explicit unavailable errors.

## QueryEngine And Runtime Tests

- Configured LSP servers replace the legacy single placeholder in inventory.
- Inventory includes lifecycle fields, language coverage, file pattern coverage, workspace boundary, command summary, and permission classification.
- Disable/enable/degraded/failed operations apply to LSP servers.
- Disabled/degraded/failed overlays recover through session/store metadata without directly injecting lifecycle records into options.
- Enable clears persisted LSP overlay and restart returns to config-derived state.
- Deterministic rebuild produces stable inventory after restart.
- P2.1 command/tool/skill/MCP lifecycle regressions continue passing.

## Permission Tests

- LSP read-only tools are read-only and non-destructive.
- Permission policy blanket deny removes or blocks LSP tools.
- Mutating LSP actions remain absent or explicitly deferred.

## Gateway Tests

- `extension_inventory` payload includes LSP lifecycle fields.
- `lsp_boundaries` remains compatible.
- New LSP-specific fields serialize as stable arrays and strings.

## Required Validation

```powershell
git diff --check origin/main..HEAD
go test ./internal/queryengine
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/permissions ./internal/tools ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./...
```
