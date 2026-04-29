# P1 AppState Session Continuity Source Alignment

Date: 2026-04-28

## Claude Code Source Areas

Relevant semantic sources:

- `claude-code/src/state/AppStateStore.ts`
- `claude-code/src/history.ts`
- `claude-code/src/assistant/sessionHistory.ts`
- `claude-code/src/screens/`
- `claude-code/src/components/`
- `claude-code/src/QueryEngine.ts`
- `claude-code/docs/06-ui-state-and-repl.md`
- `claude-code/docs/16-session-persistence-and-recovery.md`
- `claude-code/docs/40-agent-runtime-lifecycle-background-and-resume.md`

## Go Source Areas

Primary Go source areas:

- `internal/session/manager.go`
- `internal/session/recovery.go`
- `internal/store/file/session_store.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/approval/manager.go`
- `internal/agent/manager.go`
- `internal/tui/state.go`
- `internal/tui/client_store.go`
- `internal/gateway/server.go`

## Alignment Requirements

- Session recovery must preserve user-visible pending work.
- Pending approvals must not become transient UI state.
- Subagent/task visible metadata must not disappear across restart.
- Gateway and TUI must observe the same runtime-owned continuation projection.
- Clients must not rerun the model to discover current session state after reconnect.

## Known Gap After P1.1

This task does not replicate the full Claude Code AppStateStore or React Ink UI state graph. It creates the Go runtime continuity contract that later UI surfaces can consume.
