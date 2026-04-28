# P0 Runtime Parity End-To-End Validation

Date: 2026-04-28

## Scenario Coverage

This P0 branch validates the representative runtime scenario through focused Go tests and the final `go test ./...` gate:

1. start a session: covered by `internal/queryengine` and `internal/gateway` session tests.
2. load workspace context: covered by `go test ./internal/workspace ./internal/prompt`.
3. execute read/file and shell tools: covered by `go test ./internal/tools ./internal/tools/system ./internal/queryengine`.
4. request approval for a shell/write action: covered by QueryEngine approval tests and gateway approval websocket tests.
5. approve or reject and continue: covered by `internal/gateway` approval decision tests.
6. update todo/tool state: covered by tool parity focused tests in `internal/tools` and QueryEngine tool lifecycle tests.
7. trigger compaction/recovery anchors: covered by `internal/runtime`, `internal/session`, and `internal/queryengine` compaction/recovery tests.
8. recover session baseline: covered by `TestRecoverySnapshotBaselineSummarizesRestartContract`.
9. preserve tool identities after recovery: covered by recovery baseline and QueryEngine tool identity tests.
10. emit stable runtime events through gateway/TUI: covered by `internal/runtime` event payload tests, `internal/gateway` runtimeSink/session status tests, and `internal/tui` runtime bridge tests.

## Validation Commands

```powershell
go test ./internal/tools ./internal/tools/system ./internal/queryengine ./internal/runtime ./internal/permissions
go test ./internal/commands ./internal/app ./internal/tui ./internal/queryengine
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/session ./internal/store/... ./internal/model ./internal/runtime ./internal/queryengine
go test ./internal/runtime ./internal/queryengine ./internal/gateway ./internal/protocol/ws ./internal/tui
go test ./...
```

## Known Limits

P0 now defines explicit read-file state and context-cache boundaries through `ToolUseContext.ReadFileState` and recovery baseline documentation, but it does not implement Claude Code's full read-cache mechanics. Full read-file cache invalidation/projection remains P1 work. This is a known semantic gap, not a hidden completion claim.
