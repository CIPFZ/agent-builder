# P1 Subagent Task Isolation Review Checklist

Date: 2026-04-29

## Entry Gate

- [x] P0 shared command registry remains stable.
- [x] SubmitPrompt/SubmitMessage single-processing remains stable.
- [x] P1.1 continuation snapshot remains stable.
- [x] P1.2 context cache and memory rebuild remain stable.

## Scope

- [x] P1.3 only; P1.4 not mixed in.
- [x] No placeholder-only implementation.
- [x] Gateway/TUI do not own task truth.

## Lifecycle And Recovery

- [x] Background task lifecycle is persisted.
- [x] Foreground/background transition state is persisted.
- [x] Retry/resume/continue controls are represented and tested.
- [x] Task/subagent state recovers after restart.

## Isolation

- [x] Worktree isolation is tested.
- [x] CWD override is tested.
- [x] Remote isolation boundary is explicit and documented.
- [x] Allowed-tools inheritance is persisted and tested.
- [x] Permission inheritance is conservative and tested.

## Output

- [x] Task output file contract is persisted and visible.
- [x] Completed task output remains recoverable.

## Client Projection

- [x] Gateway exposes recovered task isolation state.
- [x] TUI/client store consumes the same projection.

## Tests

- [x] Focused P1.3 command passes.
- [x] P0/P1.1 regression command passes.
- [x] P1.2 regression command passes.
- [x] `go test ./...` passes.

## Gaps

- [x] Remote execution is deferred with documented boundary semantics.

## Review Fixes

- [x] Recovered subagent resume rebuilds persisted allowed-tools, permission mode, and cwd/worktree isolation boundaries.
- [x] Agent task schema exposes `run_in_background`.
