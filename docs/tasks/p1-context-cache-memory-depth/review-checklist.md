# P1 Context Cache And Memory Depth Review Checklist

Date: 2026-04-29

## Entry Gate

- [x] Core tool identity and result semantics remained stable.
- [x] Shared runtime command registry remains the slash command source.
- [x] Baseline session recovery contract remains intact.
- [x] Runtime event names and payload expectations remain stable.
- [x] QueryEngine default input path still uses the shared command registry.
- [x] SubmitPrompt/SubmitMessage still avoid double input processing.
- [x] P1.1 continuation snapshot remains available.
- [x] Pending approval recovery remains visible and not ready-for-prompt.
- [x] Task/subagent visible metadata recovery remains available.
- [x] Gateway `session_status` still exposes continuation projection.
- [x] TUI/client store still consumes continuation projection.

## Scope

- [x] P1.2 only; P1.3/P1.4 not mixed in.
- [x] No P0 or P1.1 contract was redefined.
- [x] No placeholder-only implementation.

## Workspace Instructions

- [x] Multi-layer `CLAUDE.md` loading is deterministic.
- [x] Workspace instruction loading order is tested.
- [x] Instruction fingerprints are available for cache invalidation.

## Read-File State

- [x] Read-file state records path and invalidation metadata.
- [x] Read-file state is owned by runtime/session/queryengine paths, not UI.
- [x] Missing or changed files invalidate context conservatively.

## Context Cache

- [x] Context cache has stable keys.
- [x] Cache hit is tested.
- [x] Cache invalidation is tested.
- [x] Corrupt cache is explicit or conservatively bypassed.

## History Projection

- [x] Projected history view is implemented and tested.
- [x] Tool use/result identity is preserved.
- [x] History snip/replay after compaction boundary is tested.

## Memory

- [x] Compaction memory summary save/recovery is tested.
- [x] Invalid memory summary does not panic.
- [x] Restart recovery can use persisted memory summaries.

## Rebuild

- [x] Restart context rebuild is deterministic.
- [x] Rebuild failures are explicit or conservative.

## Tests

- [x] Focused P1.2 command passes.
- [x] P0/P1.1 regression command passes.
- [x] `go test ./...` passes.
