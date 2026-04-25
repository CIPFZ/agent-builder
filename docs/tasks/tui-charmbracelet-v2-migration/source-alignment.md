# TUI Charmbracelet v2 Migration Source Alignment

Date: 2026-04-25

## 1. Alignment Purpose

This migration is about terminal frontend infrastructure, not runtime semantics.

Claude Code source alignment remains semantic:

- runtime behavior stays backend-owned
- client surfaces consume shared events and requests
- UI implementation details do not redefine agent behavior

## 2. Claude Code Semantic References

Use these areas as semantic references:

- `claude-code/src/QueryEngine.ts`
- `claude-code/src/Tool.ts`
- approval and tool lifecycle behavior
- task/subagent lifecycle visibility

Do not try to copy Claude Code's React/Ink/TUI implementation literally.

## 3. Go Source References

Use these Go areas as the migration boundary:

- `internal/tui/program.go`
- `internal/tui/model.go`
- `internal/tui/myclawd_client.go`
- `internal/tui/state.go`
- `internal/tui/render.go`
- `internal/tui/theme.go`
- `internal/tui/types.go`
- `internal/protocol/ws/message.go`
- `internal/gateway/server.go`

## 4. Non-Negotiable Rule

Changing the terminal framework must not change runtime ownership.

If migration requires changes outside `internal/tui`, `go.mod`, or direct test support, document why before making the change.
