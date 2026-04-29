# P1.5 Test Validation Plan

Date: 2026-04-29

## Focused Closure Scenarios

- Runtime slash commands are visible through QueryEngine default input processing, TUI command metadata, runtime extension inventory, and gateway `extension_inventory`.
- Gateway `extension_inventory` payload includes runtime command metadata: type, name, aliases, description, argument hint, category, visibility, behavior, source, and user-invocable flag.
- Restart/reconnect preserves pending approvals, recovered task/subagent projections, and extension inventory rebuildability.
- Context cache invalidates when system context, read-file state, user context, system prompt variants, workspace files, projected history, memory, or tool definitions change.
- Recovered subagent state preserves allowed tools, permission mode, cwd, worktree root, output file, and background state.
- Configured commands override same-name runtime commands without duplicates.
- Extension inventory rebuild is deterministic after runner restart/reconnect.
- TUI command handling and visible command text remain stable.

## Required Commands

```powershell
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions
go test ./...
```

## Pass Criteria

- all required commands exit 0
- no existing test is deleted or weakened
- added closure tests assert runtime-owned behavior, not client-only workarounds
- remaining unimplemented areas are listed as P2 deferred items, not P1 blockers

