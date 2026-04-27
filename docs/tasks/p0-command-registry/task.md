# P0 Command Registry Task

Date: 2026-04-28

## Objective

Move slash-command metadata, visibility, and execution semantics into a shared runtime-owned command registry instead of leaving them as TUI-only shortcuts.

## Initial Command Set

`/help`, `/permissions`, `/model`, `/memory`, `/resume`, `/compact`, `/tasks`, `/mcp`, and `/status`.

## Required Reading

`docs/execution/implementation-rules.md`, `docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`, and `claude-code/docs/04-command-system.md`.

## Go Ownership

`internal/commands`, `internal/tui`, `internal/app`, and `internal/queryengine`.

## Validation

```powershell
go test ./internal/app ./internal/tui ./internal/queryengine
```