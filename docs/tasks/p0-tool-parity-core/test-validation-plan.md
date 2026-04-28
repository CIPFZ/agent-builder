# P0 Tool Parity Core Test Validation Plan

Required coverage: shell success/failure, shell approval-required, file tool success/failure, path normalization, workspace boundaries, TodoWrite structured results, Agent lifecycle, Skill discovery/invocation, MCP dynamic calls, and tool identity preservation.

```powershell
go test ./internal/tools ./internal/tools/system ./internal/queryengine ./internal/runtime ./internal/permissions
```