# P1 Context Cache And Memory Depth Task

Date: 2026-04-29

## Objective

Implement P1.2 Context Cache And Memory Depth.

The goal is to make long-session context rebuild deterministic after session restart or runtime rebuild by covering workspace instructions, read-file state, context cache, projected history, history snip/replay, and compaction memory save/recovery.

## Scope

Implement:

- deterministic `CLAUDE.md` and workspace instruction loading
- workspace instruction loading order
- read-file state tracking
- context cache hit and invalidation
- projected history view
- history snip and replay
- compaction memory save and recovery
- deterministic context rebuild after restart

## Non-Goals

Do not implement P1.3 subagent task isolation.

Do not implement P1.4 extension platform foundation.

Do not reopen P0 or P1.1 contracts unless implementation proves a blocking bug.

Do not make TUI or gateway own context rebuild truth.

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
11. `docs/tasks/p1-appstate-session-continuity/task.md`
12. `docs/tasks/p1-appstate-session-continuity/review-checklist.md`
13. `docs/tasks/p1-context-cache-memory-depth/design.md`
14. `docs/tasks/p1-context-cache-memory-depth/source-alignment.md`
15. `docs/tasks/p1-context-cache-memory-depth/implementation-plan.md`
16. `docs/tasks/p1-context-cache-memory-depth/test-validation-plan.md`
17. `docs/tasks/p1-context-cache-memory-depth/review-checklist.md`

## Entry Contracts

Before implementation, confirm:

- core tool identity and result semantics are stable
- shared runtime command registry exists
- baseline session recovery contract exists
- runtime event names and payload expectations are stable
- QueryEngine default input path uses shared command registry
- SubmitPrompt/SubmitMessage do not process input twice
- P1.1 runtime continuation snapshot exists
- pending approval recovery is visible and not ready-for-prompt
- task/subagent visible metadata recovery exists
- gateway `session_status` exposes continuation projection
- TUI/client store consumes continuation projection

If any contract is missing, stop and record a blocking note against the relevant P0 or P1.1 workstream.

## Go Ownership

Primary ownership:

- `internal/workspace`
- `internal/prompt`
- `internal/memory`
- `internal/model`
- `internal/session`
- `internal/runtime`
- `internal/queryengine`
- `internal/store`
- `internal/tools`

## Implementation Order

1. Add failing tests for deterministic workspace instruction loading.
2. Add failing tests for read-file state and context cache hit/invalidation.
3. Add failing tests for projected history and history snip/replay.
4. Add failing tests for compaction memory save/recovery.
5. Add failing tests for deterministic context rebuild after restart and error paths.
6. Implement minimal context state/cache schema in the right ownership packages.
7. Wire read-file state recording through tool/runtime/queryengine paths.
8. Wire context rebuild through prompt/queryengine/runtime paths.
9. Run focused tests.
10. Run P0/P1.1 regression command.
11. Run `go test ./...`.

## Validation Requirements

Run:

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

If package paths change, record actual commands, reasons, and results.

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
- whether P1.3 should start
