# P0 Context Memory Recovery Task

Date: 2026-04-28

## Objective

Make long-session continuation reliable enough to recover workspace context, memory, tool-use identity, pending approvals, compaction boundaries, invoked skills, and agent/task persistence boundaries after restart.

## Required Reading

`docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`, `claude-code/docs/07-query-engine-and-context.md`, `claude-code/docs/16-session-persistence-and-recovery.md`, `claude-code/docs/19-context-compression-and-history-management.md`, and `claude-code/docs/21-memory-and-claude-md.md`.

## Go Ownership

`internal/workspace`, `internal/prompt`, `internal/memory`, `internal/session`, `internal/store`, `internal/model`, `internal/runtime`, and `internal/queryengine`.

## Validation

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/session ./internal/store/... ./internal/model ./internal/runtime ./internal/queryengine
```