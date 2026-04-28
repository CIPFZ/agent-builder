# P1.3 Test Validation Plan

Date: 2026-04-29

## Focused Tests

- Agent manager: lifecycle, background transition, restore metadata.
- Tools: structured Agent task parsing for background, isolation, cwd, allowed tools, permission mode, remote boundary, output file.
- Runtime: persisted and recovered task metadata, cwd override, worktree isolation, allowed-tools inheritance, permission mode derivation.
- Gateway/TUI: projection consumes runtime task state.

## Regression Tests

- P0 command registry and single-processing tests remain covered by full package runs.
- P1.1 continuation snapshot tests remain covered by runtime/gateway/TUI runs.
- P1.2 context rebuild tests remain covered by workspace/prompt/memory/model/session/runtime/queryengine runs.

## Commands

```powershell
go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./...
```
