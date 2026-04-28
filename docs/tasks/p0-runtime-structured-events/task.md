# P0 Runtime Structured Events Task

Date: 2026-04-28

## Objective

Stabilize runtime event names and payload contracts so CLI, TUI, gateway, and future SDK/control-plane consumers read the same runtime semantics.

## Required Reading

`docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`, `claude-code/docs/22-cli-structured-io-and-transports.md`, `claude-code/docs/05-tool-system.md`, and `claude-code/docs/07-query-engine-and-context.md`.

## Go Ownership

`internal/runtime`, `internal/queryengine`, `internal/gateway`, `internal/protocol/ws`, and `internal/tui`.

## Validation

```powershell
go test ./internal/runtime ./internal/queryengine ./internal/gateway ./internal/protocol/ws ./internal/tui
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

## Payload Schema

See payload-schema.md for the stable runtime event payload fields and compatibility notes.

