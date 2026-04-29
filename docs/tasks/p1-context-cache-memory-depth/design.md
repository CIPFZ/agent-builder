# P1 Context Cache And Memory Depth Design

Date: 2026-04-29

## Goal

Create a deterministic context rebuild model that survives session restart and runtime rebuild without relying on hidden UI state.

## Design Principles

- `internal/workspace` owns instruction discovery and deterministic loading order.
- `internal/tools` and `internal/queryengine` record read-file state when model-visible file reads occur.
- `internal/session` persists context state needed for rebuild.
- `internal/prompt` owns context assembly and cache projection.
- `internal/memory` owns memory records, including compaction summaries restored after restart.
- `internal/runtime` exposes rebuild helpers and preserves P1.1 continuation state.

## Minimal Context State

The minimum state should include:

- ordered workspace instruction files with path, content hash, size, mtime, and load order
- read-file state entries with path, hash, size, mtime, last read time, and invalidation key
- context cache entries keyed by deterministic input fingerprints
- projected history metadata that preserves tool identity pairs
- compaction memory summary references
- rebuild diagnostics for conservative fallback

## Cache Semantics

Context cache hits require the same session, same workspace instruction fingerprints, same read-file fingerprints, same memory summary fingerprints, and same projected history fingerprint.

Any missing, changed, corrupt, or unreadable input invalidates the cache. Correctness wins over hit rate.

## History Projection

Projected history should trim or summarize irrelevant raw transcript material while preserving provider-critical relationships such as `tool_use_id` and tool result pairing.

History snip/replay must keep identity stable so replayed context does not break provider message constraints.

## Error Handling

Missing files, corrupt cache entries, invalid memory summaries, and failed instruction reads must not panic. The system should either return explicit errors or rebuild with conservative diagnostics while avoiding stale cache usage.

## Non-Goals

- P1.3 subagent isolation and context inheritance
- P1.4 extension inventory
- full Claude Code cache economics
- UI-owned context inference
