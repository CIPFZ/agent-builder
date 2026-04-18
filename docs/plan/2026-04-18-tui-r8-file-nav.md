# TUI R8 File-Level Quick Open

## Goal

Bring Claude Code style file-level quick open semantics into the existing Go TUI quick open overlay without breaking the already-merged command/session/task/MCP picker behavior.

## Scope

- Keep the existing unified `Quick Open` dialog.
- Add workspace file indexing from available workspace roots.
- Show matched file entries when the user types a query.
- Show a lightweight preview of the focused file inside the quick open dialog.
- Support:
  - `Enter`: open focused file in external editor
  - `Tab`: insert `@path `
  - `Shift+Tab`: insert `path `
- Preserve existing quick open routing for commands, sessions, tasks, and MCP servers.

## Test Plan

The implementation was driven by failing TUI tests first:

1. `TestQuickOpenIncludesMatchedWorkspaceFilesWithPreview`
   - proves file matches appear in quick open
   - proves preview content is rendered for the focused file

2. `TestQuickOpenEnterOpensFocusedWorkspaceFile`
   - proves `Enter` opens the selected file via the configured opener

3. `TestQuickOpenTabAndShiftTabInsertWorkspaceFileReference`
   - proves `Tab` inserts `@path `
   - proves `Shift+Tab` inserts `path `

Regression verification then re-ran:

- `go test ./internal/tui`
- `go test ./...`

This confirms the new file semantics did not regress existing TUI modules or broader runtime behavior.

## Implementation Notes

- Quick open now has a file index built from workspace roots.
- Workspace roots come from runtime/platform snapshot first, then model config fallback.
- File matches are added only when the query is non-empty, which avoids flooding the initial quick open list.
- File preview is intentionally lightweight and line-limited.
- Existing quick open items remain intact and still route to the original dialogs/actions.

## Follow-Up

This branch completes the `file-level quick open` slice of R8.
The next recommended worktree is `workspace global search`.
