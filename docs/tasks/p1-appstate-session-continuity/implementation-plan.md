# P1 AppState Session Continuity Implementation Plan

Date: 2026-04-28

## Phase 0: P0 Gate

Confirm:

- command registry exists and is used by QueryEngine default input processing
- SubmitPrompt does not process input twice through SubmitMessage
- runtime events have stable names and payloads
- baseline recovery and pending approval metadata exist
- TUI footer text has no `????navigate`

Stop on failure.

## Phase 1: Tests First

Add focused failing tests for:

- runtime continuation snapshot after session manager and runner rebuild
- pending approval recovery that is not ready for prompt
- task/subagent visible state recovery
- gateway `session_status` returning recovered continuation
- TUI/client store consuming the same projection
- conservative error path for invalid recovery data
- P0 regressions for slash command registry and SubmitPrompt single processing

## Phase 2: Snapshot Schema

Add a minimal shared runtime continuation snapshot type.

The snapshot should derive from:

- `session.RecoverySnapshot`
- `approval.Manager`
- `agent.Manager`
- runtime policy/model helpers

## Phase 3: Recovery Wiring

Ensure runner construction restores:

- pending approvals from persisted session metadata
- visible agent runs from persisted session metadata

No model call should be required for the restored snapshot.

## Phase 4: Gateway Projection

Extend `session_status` to include:

- continuation status
- `ready_for_prompt`
- resume anchor
- pending approval
- visible tasks/subagents
- recovery error or conservative reason when present

## Phase 5: TUI Consumption

Add a client-store path that applies the continuation projection and produces the same pending approval/task state used by event-driven updates.

## Phase 6: Validation

Run:

```powershell
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

Commit implementation separately from these task docs.
