# P1 AppState Session Continuity Test Validation Plan

Date: 2026-04-28

## Required Tests

1. Session restart after runtime state snapshot:
   - rebuild session manager and runner from persisted session data
   - read continuation snapshot without a model call

2. Pending approval recovery:
   - pending approval remains visible after recovery
   - continuation is not `ready_for_prompt`

3. Task/subagent visible state recovery:
   - visible run metadata is persisted
   - rebuilt runner exposes the same run metadata

4. Gateway reconnect:
   - `session_status` returns recovered continuation state
   - response includes pending approval and task/subagent projections

5. TUI/client store consistency:
   - the client store can apply the gateway projection
   - pending approval and task views match event-driven state shape

6. Error path:
   - corrupted or missing recovery state does not panic
   - recovery returns a clear error or conservative fallback

7. P0 regressions:
   - slash command registry still owns default slash command execution
   - SubmitPrompt does not double-run InputProcessor through SubmitMessage

## Required Commands

```powershell
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

## Expected Result

All packages pass or report `[no test files]`.
