# P1.3 Subagent Task Isolation

Date: 2026-04-29

## Objective

Implement reliable, recoverable, inspectable, and conservatively isolated delegated task behavior for background agents and subagents.

## Scope

- background task lifecycle
- task output file contract
- foreground/background transition semantics
- retry/resume/continue controls
- worktree isolation
- cwd override
- remote isolation boundary metadata
- allowed-tools inheritance
- conservative permission inheritance
- task/subagent recovery after restart
- gateway/TUI observable state sourced from runtime/session/agent truth

## Non-Goals

- P1.4 extension platform work
- complete remote execution support
- replacing P0 command registry, P1.1 continuation, or P1.2 context rebuild contracts

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p0-runtime-parity-roadmap/task.md`
5. `docs/tasks/p1-runtime-maturation-roadmap/task.md`
6. `docs/tasks/p1-runtime-maturation-roadmap/design.md`
7. `docs/tasks/p1-runtime-maturation-roadmap/source-alignment.md`
8. `docs/tasks/p1-runtime-maturation-roadmap/implementation-plan.md`
9. `docs/tasks/p1-runtime-maturation-roadmap/test-validation-plan.md`
10. `docs/tasks/p1-runtime-maturation-roadmap/review-checklist.md`
11. `docs/tasks/p1-appstate-session-continuity/task.md`
12. `docs/tasks/p1-appstate-session-continuity/review-checklist.md`
13. `docs/tasks/p1-context-cache-memory-depth/task.md`
14. `docs/tasks/p1-context-cache-memory-depth/review-checklist.md`

## Go Ownership

- `internal/agent`: lifecycle state and control metadata
- `internal/runtime`: subagent spawning, permission/workspace resolution, persistence, recovery
- `internal/session`: durable metadata schema
- `internal/tools`: Agent tool request parsing and runtime handoff
- `internal/permissions`: conservative inheritance
- `internal/gateway`: client-visible projection only
- `internal/tui`: client-visible projection only

## Implementation Order

1. Add focused failing tests.
2. Extend run/session metadata for isolation and control state.
3. Persist and recover the metadata through P1.1 continuation paths.
4. Wire Agent tool, gateway, and TUI projections to consume runtime truth.
5. Verify P0, P1.1, and P1.2 regression commands.

## Validation

Run:

```powershell
go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./...
```

## Starter Prompt

Implement P1.3 Subagent Task Isolation without changing P0, P1.1, or P1.2 contracts. Use tests first, keep gateway/TUI read-only projections, and commit with `feat: add subagent task isolation`.
