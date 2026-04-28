# P1 AppState Session Continuity Review Checklist

Date: 2026-04-28

## P0 Gate

- [x] Core tool identity and result semantics remained untouched.
- [x] Shared runtime command registry remains the slash command source.
- [x] Baseline session recovery contract remains intact.
- [x] Runtime event names and payload expectations remain stable.
- [x] QueryEngine default input path still uses the shared command registry.
- [x] SubmitPrompt/SubmitMessage still avoid double input processing.
- [x] TUI footer does not contain `????navigate`.

## Scope

- [x] P1.1 only; P1.2/P1.3/P1.4 not mixed in.
- [x] No P0 contract was redefined.
- [x] No placeholder-only implementation.

## Runtime Continuity

- [x] Runtime continuation snapshot is implemented.
- [x] Snapshot can be created after session manager and runner rebuild.
- [x] Recovery failure is explicit or conservative and does not panic.

## Approvals

- [x] Pending approval recovery uses approval/session metadata as source of truth.
- [x] Recovered pending approvals remain visible to clients.
- [x] Pending approval recovery is not marked `ready_for_prompt`.

## Tasks/Subagents

- [x] Task/subagent visible metadata is recoverable.
- [x] Security boundaries for permissions, allowed tools, worktree isolation, and cwd behavior are not weakened.

## Clients

- [x] Gateway `session_status` exposes recovered continuation state.
- [x] TUI/client store consumes the same projection.
- [x] Gateway and TUI visible state remain consistent.

## Tests

- [x] Runtime snapshot creation/recovery test exists.
- [x] Pending approval recovery test exists.
- [x] Task/subagent recovery test exists.
- [x] Gateway reconnect state test exists.
- [x] TUI/client store projection test exists.
- [x] Error path recovery test exists.
- [x] P0 regression tests exist or remain covered.
- [x] Focused test command passes.
- [x] `go test ./...` passes.
