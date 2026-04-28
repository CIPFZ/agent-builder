# P1 AppState Session Continuity Task

Date: 2026-04-28

## Objective

Implement P1.1 AppState And Session Continuity.

The goal is to let session, runtime, approval, task/subagent, and client-visible state form a recoverable continuation model after process restart or client reconnect.

## Scope

Implement:

- runtime state snapshot creation
- runtime state recovery
- pending approval recovery
- task/subagent visible state recovery
- client reconnect state read
- TUI and gateway consistency over the same recovered projection

## Non-Goals

Do not implement P1.2 context cache and memory depth.

Do not implement P1.3 subagent task isolation beyond preserving existing security boundaries while recovering visible task state.

Do not implement P1.4 extension platform foundation.

Do not reopen P0 contracts unless a blocking bug is found.

## Required Reading

Read in order:

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p0-runtime-parity-roadmap/task.md`
5. `docs/tasks/p1-runtime-maturation-roadmap/task.md`
6. `docs/tasks/p1-runtime-maturation-roadmap/design.md`
7. `docs/tasks/p1-runtime-maturation-roadmap/source-alignment.md`
8. `docs/tasks/p1-runtime-maturation-roadmap/implementation-plan.md`
9. `docs/tasks/p1-runtime-maturation-roadmap/test-validation-plan.md`
10. `docs/tasks/p1-runtime-maturation-roadmap/review-checklist.md`
11. `docs/tasks/p1-appstate-session-continuity/design.md`
12. `docs/tasks/p1-appstate-session-continuity/source-alignment.md`
13. `docs/tasks/p1-appstate-session-continuity/implementation-plan.md`
14. `docs/tasks/p1-appstate-session-continuity/test-validation-plan.md`
15. `docs/tasks/p1-appstate-session-continuity/review-checklist.md`

## P0 Entry Contract

Before implementation, confirm:

- core tool identity and result semantics are stable
- shared runtime command registry exists
- baseline session recovery contract exists
- runtime event names and payload expectations are stable
- QueryEngine default input path uses the shared command registry
- SubmitPrompt/SubmitMessage do not process input twice
- TUI footer no longer contains `????navigate`

If any contract is missing, stop and record a blocking note under the affected P0 workstream.

## Go Ownership

Primary ownership:

- `internal/session`
- `internal/store`
- `internal/runtime`
- `internal/queryengine`
- `internal/approval`
- `internal/agent`
- `internal/tui`
- `internal/gateway`

## Implementation Order

1. Add failing tests for runtime snapshot and recovery.
2. Add failing tests for pending approval recovery and conservative ready state.
3. Add failing tests for task/subagent visible state recovery.
4. Add failing tests for gateway reconnect `session_status`.
5. Add failing tests for TUI/client store consumption of the same projection.
6. Implement the minimal shared snapshot schema and projection.
7. Wire gateway and TUI to consume the projection.
8. Run focused tests.
9. Run `go test ./...`.

## Validation Requirements

Run:

```powershell
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

If package paths change, record the actual package structure, command, reason, and result.

## Completion Output Requirements

When complete, report:

- changed files
- added or updated tests
- test commands and results
- unresolved issues
- remaining gaps versus real Claude Code semantics
- review checklist result for each item
- commit hash
- current git status
- whether P1.2 should start
