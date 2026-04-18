# TUI R8 Workspace Global Search

## Goal

Replicate the Claude Code style workspace global search flow inside the existing Go TUI:

- type a workspace text query
- search file contents across workspace roots
- show `file:line:text` matches
- show focused match preview context
- support open and prompt insertion actions

## Scope

- Add a dedicated `Global Search` dialog.
- Trigger it via `/search`.
- Execute workspace content search through an injected search function.
- Default runtime behavior:
  - prefer `rg`
  - fall back to in-process file scanning when `rg` is unavailable
- Support:
  - `Enter`: open file at line
  - `Tab`: insert `@file#Lline `
  - `Shift+Tab`: insert `file:line `

## Test Plan

The implementation was driven by failing TUI tests first:

1. `TestSlashSearchOpensGlobalSearchDialog`
2. `TestGlobalSearchShowsWorkspaceMatchesAndPreview`
3. `TestGlobalSearchEnterOpensFocusedMatchAtLine`
4. `TestGlobalSearchTabAndShiftTabInsertMatchReference`

Regression verification:

- `go test ./internal/tui`
- `go test ./...`

## Implementation Notes

- The dialog is state-driven and reuses the existing picker/dialog framework.
- Search roots come from runtime platform snapshot first, then model config fallback.
- Search results carry both display path and absolute path so the TUI can render relative paths while still opening the real file.
- Preview is line-context based rather than whole-file based.
- Search execution is injectable for tests and defaults to `rg` in real runs.

## Follow-Up

This completes the second R8 slice.
The next R8 branch should focus on attachment semantics and richer message/product fidelity.
