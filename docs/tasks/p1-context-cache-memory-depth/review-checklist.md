# P1 Context Cache And Memory Depth Review Checklist

Date: 2026-04-29

## Entry Gate

- [ ] Core tool identity and result semantics remained stable.
- [ ] Shared runtime command registry remains the slash command source.
- [ ] Baseline session recovery contract remains intact.
- [ ] Runtime event names and payload expectations remain stable.
- [ ] QueryEngine default input path still uses the shared command registry.
- [ ] SubmitPrompt/SubmitMessage still avoid double input processing.
- [ ] P1.1 continuation snapshot remains available.
- [ ] Pending approval recovery remains visible and not ready-for-prompt.
- [ ] Task/subagent visible metadata recovery remains available.
- [ ] Gateway `session_status` still exposes continuation projection.
- [ ] TUI/client store still consumes continuation projection.

## Scope

- [ ] P1.2 only; P1.3/P1.4 not mixed in.
- [ ] No P0 or P1.1 contract was redefined.
- [ ] No placeholder-only implementation.

## Workspace Instructions

- [ ] Multi-layer `CLAUDE.md` loading is deterministic.
- [ ] Workspace instruction loading order is tested.
- [ ] Instruction fingerprints are available for cache invalidation.

## Read-File State

- [ ] Read-file state records path and invalidation metadata.
- [ ] Read-file state is owned by runtime/session/queryengine paths, not UI.
- [ ] Missing or changed files invalidate context conservatively.

## Context Cache

- [ ] Context cache has stable keys.
- [ ] Cache hit is tested.
- [ ] Cache invalidation is tested.
- [ ] Corrupt cache is explicit or conservatively bypassed.

## History Projection

- [ ] Projected history view is implemented and tested.
- [ ] Tool use/result identity is preserved.
- [ ] History snip/replay after compaction boundary is tested.

## Memory

- [ ] Compaction memory summary save/recovery is tested.
- [ ] Invalid memory summary does not panic.
- [ ] Restart recovery can use persisted memory summaries.

## Rebuild

- [ ] Restart context rebuild is deterministic.
- [ ] Rebuild failures are explicit or conservative.

## Tests

- [ ] Focused P1.2 command passes.
- [ ] P0/P1.1 regression command passes.
- [ ] `go test ./...` passes.
