# TUI Charmbracelet v2 Migration Test Plan

Date: 2026-04-25

## 1. Validation Goal

Validate that the TUI compiles and behaves the same after moving to:

- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`

## 2. Required Automated Tests

Run:

```bash
go test ./internal/tui ./internal/app ./internal/config
```

If dependency changes affect shared packages, also run:

```bash
go test ./internal/gateway ./internal/protocol/...
```

## 3. Required Search Checks

Verify legacy imports are gone from production TUI:

```bash
Select-String -Path internal/tui/*.go -Pattern "github.com/charmbracelet"
```

Expected result:

- no production TUI imports from `github.com/charmbracelet/bubbletea`
- no production TUI imports from `github.com/charmbracelet/lipgloss`

If indirect dependencies remain in `go.sum`, that is acceptable only if required transitively.

## 4. Manual Smoke Test

If local `myclawd` is available:

1. Start `myclawd`.
2. Start `myclaw tui`.
3. Confirm the TUI enters alt screen.
4. Confirm typing works.
5. Confirm resize does not break layout.
6. Confirm mouse wheel still scrolls transcript.
7. Confirm exit returns the terminal to a usable state.

## 5. Review Evidence

Claude Code should report:

- exact dependency versions selected
- import migration summary
- tests run
- whether manual smoke was run
- any API behavior changed by Charmbracelet v2
