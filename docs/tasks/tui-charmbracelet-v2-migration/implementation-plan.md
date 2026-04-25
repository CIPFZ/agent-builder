# TUI Charmbracelet v2 Migration Implementation Plan

Date: 2026-04-25

## 1. Implementation Strategy

Do this as a controlled migration, not a rewrite.

The correct strategy is:

1. update dependencies
2. update imports
3. adapt compile errors
4. run focused TUI tests
5. verify production TUI still starts through `myclawd`

## 2. Work Package A: Dependency Update

Required outcome:

- `go.mod` includes `charm.land/bubbletea/v2`
- `go.mod` includes `charm.land/lipgloss/v2`
- legacy Charmbracelet imports are removed from direct TUI usage where possible

Do not manually guess versions if `go get` can resolve the current compatible versions.

## 3. Work Package B: Import Migration

Required outcome:

- replace `github.com/charmbracelet/bubbletea` with `charm.land/bubbletea/v2`
- replace `github.com/charmbracelet/lipgloss` with `charm.land/lipgloss/v2`
- update tests consistently

Search target:

```bash
Select-String -Path internal/tui/*.go -Pattern "github.com/charmbracelet"
```

## 4. Work Package C: API Compatibility

Required outcome:

- compile with Bubble Tea v2
- compile with Lip Gloss v2
- preserve current keyboard, mouse, resize, alt-screen, and cursor behavior

Pay special attention to:

- `tea.Model`
- `tea.Msg`
- `tea.Cmd`
- `tea.Program`
- `tea.NewProgram`
- mouse options
- key and mouse message types
- Lip Gloss width and style APIs

## 5. Work Package D: Boundary Verification

Required outcome:

- production TUI still starts through `NewMyclawdClient`
- no production runtime bridge is reintroduced
- test-only runtime helpers stay test-only

## 6. Work Package E: Validation

Required outcome:

- run focused TUI tests
- run app/config tests if startup wiring changed
- run `go test ./internal/tui ./internal/app ./internal/config`

If dependency migration creates broader compile impact, run the smallest additional package set needed to prove correctness.

## 7. Completion Standard

The migration is complete when:

- `internal/tui` no longer imports legacy Charmbracelet paths
- tests pass
- TUI backend boundary remains `myclawd`
- review can confirm the migration did not become a feature expansion
