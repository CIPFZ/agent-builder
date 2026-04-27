# TUI Charmbracelet v2 Migration Design

Date: 2026-04-25

## 1. Design Goal

Move the Go TUI to the Charmbracelet v2 ecosystem while preserving the architectural decision that the TUI is a lightweight `myclawd` client.

This is a technical foundation task.

It is not a request to expand TUI product scope.

## 2. Target Stack

Required packages:

- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`

Responsibilities:

- `charm.land/bubbletea/v2`: event loop, keyboard events, mouse events, resize messages, alt screen, terminal cursor behavior, and program lifecycle
- `charm.land/lipgloss/v2`: terminal styling, text width, color, borders, padding, alignment, and layout rendering

Do not keep production TUI on the older `github.com/charmbracelet/bubbletea` or `github.com/charmbracelet/lipgloss` imports.

## 3. Architecture Boundary

The migration must not change the backend boundary.

The TUI still uses:

- `internal/tui/myclawd_client.go` as the production backend client
- `myclawd` websocket protocol as the control-plane contract
- local store/state/render layers as client-side UI structure

The migration must not reintroduce a production `RuntimeBridge` or any direct production dependency on `runtime.Runner` / `session.Manager`.

## 4. Migration Scope

In scope:

- update Go module dependencies
- update imports in production TUI code and tests
- adapt Bubble Tea v2 API differences
- adapt Lip Gloss v2 API differences
- preserve current behavior with focused tests
- document any v2 API compatibility decisions

Out of scope:

- React UI work
- full Claude Code TUI visual parity
- new TUI product panels unrelated to migration
- backend protocol changes unless tests reveal an existing bug

## 5. Expected Impact Areas

Primary files:

- `go.mod`
- `go.sum`
- `internal/tui/*.go`
- `internal/tui/*_test.go`

Review import usage before editing. The current codebase has many direct references to the legacy Charmbracelet import paths.

## 6. Design Conclusion

The desired end state is:

- production and test TUI compile on Charmbracelet v2 package paths
- current TUI behavior remains intact
- TUI remains a `myclawd` client
- docs and dependency graph no longer imply the older Charmbracelet stack is the long-term target
