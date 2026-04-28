# P0 Tool Parity Core Task

Date: 2026-04-28

## Objective

Normalize the core model-callable tools so tool identity, permission checks, classifications, observable input, and result semantics are stable across QueryEngine, runtime, TUI, and gateway consumers.

## Scope

Primary tools: `Bash`, `PowerShell`, `system.run`, `Read`, `Write`, `Edit`, `MultiEdit`, `Glob`, `Grep`, `LS`, `TodoWrite`, `Agent`, `Skill`, and MCP dynamic tools.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/p0-runtime-parity-roadmap/task.md`
4. `docs/tasks/p0-runtime-parity-roadmap/design.md`
5. `docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`
6. `claude-code/docs/05-tool-system.md`
7. `claude-code/docs/08-permissions-and-safety.md`

## Go Ownership

`internal/tools`, `internal/tools/system`, `internal/queryengine`, `internal/runtime`, `internal/permissions`, and `internal/approval`.

## Validation

```powershell
go test ./internal/tools ./internal/tools/system ./internal/queryengine ./internal/runtime ./internal/permissions
```
## Non-Goals

This child task does not implement React Ink visual parity, telemetry/GrowthBook, enterprise managed settings, bridge/remote, plugin marketplace, broad LSP, or unrelated UI/style/config changes.

## Claude Semantic Alignment

The implementation must follow the Claude Code source references listed in `source-alignment.md` and preserve the roadmap rule that runtime contracts are owned by Go runtime packages before client-specific consumption.

## Implementation Order

1. Write focused failing tests for this workstream.
2. Confirm the tests expose the current gap.
3. Implement the smallest production change in the owning packages.
4. Run the focused validation command from this task.
5. Update the review checklist with the verified result.
6. Commit this workstream separately.

## Completion Output Requirements

Report changed files, added tests, validation commands and results, unresolved gaps, checklist status, commit hash, and current git status.

## Starter Prompt

Implement this workstream in `C:\Users\ytq\work\ai\agent-builder\.worktrees\claude-code-semantic-review` on branch `codex/claude-code-semantic-review`. Follow TDD, preserve permission boundaries, do not change unrelated files, and run the validation command before committing.
