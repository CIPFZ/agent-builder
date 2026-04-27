# P0 Tool Parity Core Task

Date: 2026-04-28

## Objective

Normalize the core model-callable tools so tool identity, permission checks, classifications, observable input, and result semantics are stable across QueryEngine, runtime, TUI, and gateway consumers.

## Scope

Primary tools: `Bash`, `PowerShell`, `system.run`, `Read`, `Write`, `Edit`, `MultiEdit`, `Glob`, `Grep`, `LS`, `TodoWrite`, `Agent`, `Skill`, and MCP dynamic tools.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/p0-runtime-parity-roadmap/task.md`
4. `docs/tasks/p0-runtime-parity-roadmap/design.md`
5. `docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`
6. `claude-code/docs/05-tool-system.md`
7. `claude-code/docs/08-permissions-and-safety.md`

## Go Ownership

`internal/tools`, `internal/tools/system`, `internal/queryengine`, `internal/runtime`, `internal/permissions`, and `internal/approval`.

## Validation

```powershell
go test ./internal/tools ./internal/tools/system ./internal/queryengine ./internal/runtime ./internal/permissions
```