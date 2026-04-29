# P1 Context Cache And Memory Depth Implementation Plan

Date: 2026-04-29

## Phase 0: Entry Gate

Confirm P0 and P1.1 contracts:

- shared slash command registry and QueryEngine default path
- SubmitPrompt single input processing
- stable runtime event contracts
- baseline session recovery
- P1.1 continuation snapshot and gateway/TUI projections

Stop on failure.

## Phase 1: Tests First

Add focused failing tests for:

- deterministic multi-layer workspace instruction loading
- read-file state fingerprints
- context cache hit and invalidation
- projected history with preserved tool identities
- history snip/replay after compaction boundary
- compaction memory summary restart recovery
- deterministic context rebuild after restart
- conservative fallback for missing/corrupt state
- P0/P1.1 regressions

## Phase 2: Workspace Instructions

Implement deterministic instruction discovery and ordering in `internal/workspace`.

Persist load fingerprints where needed for context rebuild.

## Phase 3: Read-File State

Record read-file state from model-visible file reads. Keep state in session/runtime-owned storage, not UI state.

## Phase 4: Context Cache

Define cache keys from workspace instruction fingerprints, read-file fingerprints, memory summary fingerprints, and projected history fingerprints.

Implement hit and invalidation paths with explicit diagnostics.

## Phase 5: History Projection

Create projected history helpers that preserve tool_use/tool_result identity and can replay after compaction boundaries.

## Phase 6: Memory Recovery

Ensure compaction memory summaries are saved in memory/session state and recoverable after restart.

## Phase 7: Validation

Run:

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

Commit implementation separately from task docs.
