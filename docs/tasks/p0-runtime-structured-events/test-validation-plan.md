# P0 Runtime Structured Events Test Validation Plan

Required coverage: QueryEngine to runtime event mapping, runtime to gateway serialization, websocket payload stability, TUI runtime bridge consumption, approval event shape, tool lifecycle event shape, command lifecycle event shape, compaction event shape, and agent/task lifecycle event shape.

```powershell
go test ./internal/runtime ./internal/queryengine ./internal/gateway ./internal/protocol/ws ./internal/tui
```