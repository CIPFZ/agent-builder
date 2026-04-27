# P0 Runtime Structured Events Task

Date: 2026-04-28

## Objective

Stabilize runtime event names and payload contracts so CLI, TUI, gateway, and future SDK/control-plane consumers read the same runtime semantics.

## Required Reading

`docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`, `claude-code/docs/22-cli-structured-io-and-transports.md`, `claude-code/docs/05-tool-system.md`, and `claude-code/docs/07-query-engine-and-context.md`.

## Go Ownership

`internal/runtime`, `internal/queryengine`, `internal/gateway`, `internal/protocol/ws`, and `internal/tui`.

## Validation

```powershell
go test ./internal/runtime ./internal/queryengine ./internal/gateway ./internal/protocol/ws ./internal/tui
```