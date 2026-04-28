# P1 AppState Session Continuity Review Checklist

Date: 2026-04-28

## P0 Gate

- [ ] Core tool identity and result semantics remained untouched.
- [ ] Shared runtime command registry remains the slash command source.
- [ ] Baseline session recovery contract remains intact.
- [ ] Runtime event names and payload expectations remain stable.
- [ ] QueryEngine default input path still uses the shared command registry.
- [ ] SubmitPrompt/SubmitMessage still avoid double input processing.
- [ ] TUI footer does not contain `????navigate`.

## Scope

- [ ] P1.1 only; P1.2/P1.3/P1.4 not mixed in.
- [ ] No P0 contract was redefined.
- [ ] No placeholder-only implementation.

## Runtime Continuity

- [ ] Runtime continuation snapshot is implemented.
- [ ] Snapshot can be created after session manager and runner rebuild.
- [ ] Recovery failure is explicit or conservative and does not panic.

## Approvals

- [ ] Pending approval recovery uses approval/session metadata as source of truth.
- [ ] Recovered pending approvals remain visible to clients.
- [ ] Pending approval recovery is not marked `ready_for_prompt`.

## Tasks/Subagents

- [ ] Task/subagent visible metadata is recoverable.
- [ ] Security boundaries for permissions, allowed tools, worktree isolation, and cwd behavior are not weakened.

## Clients

- [ ] Gateway `session_status` exposes recovered continuation state.
- [ ] TUI/client store consumes the same projection.
- [ ] Gateway and TUI visible state remain consistent.

## Tests

- [ ] Runtime snapshot creation/recovery test exists.
- [ ] Pending approval recovery test exists.
- [ ] Task/subagent recovery test exists.
- [ ] Gateway reconnect state test exists.
- [ ] TUI/client store projection test exists.
- [ ] Error path recovery test exists.
- [ ] P0 regression tests exist or remain covered.
- [ ] Focused test command passes.
- [ ] `go test ./...` passes.
