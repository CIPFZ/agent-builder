# P1.5 Runtime Maturation Closure And P2 Readiness Task

Date: 2026-04-29

## Objective

Close P1 runtime maturation by validating that P1.1-P1.4 work behaves consistently as one runtime system and by preparing a concrete P2 roadmap.

P1.5 is a quality gate. It does not redesign P0 or P1 modules and does not introduce broad P2/P3 features.

## Scope

Validate and document:

- shared runtime command registry consistency across TUI, gateway, SDK/app-style input, and QueryEngine default input processing
- session/appstate recovery consistency for approvals, agent tasks, extension inventory, and context cache
- context cache invalidation for build-affecting inputs
- subagent/background task recovery of allowed tools, permission mode, cwd/worktree isolation, output file, and background state
- extension inventory coverage for runtime commands, configured commands, dynamic tools, skills, MCP servers, and LSP boundary placeholder
- gateway payload parity with runtime projection fields
- TUI command and interaction stability after P1 changes
- P2 roadmap tasks and handoff criteria

## Non-Goals

Do not implement:

- React UI or operator console integration
- plugin marketplace
- full LSP capability
- remote bridge or trusted-device runtime
- enterprise managed settings or policy administration
- new execution surfaces beyond closure validation

Do not move runtime truth into gateway or TUI. Client layers remain projections.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p1-runtime-maturation-roadmap/task.md`
5. `docs/tasks/p1-runtime-maturation-roadmap/implementation-plan.md`
6. `docs/tasks/p1-runtime-maturation-roadmap/review-checklist.md`
7. `docs/tasks/p1-appstate-session-continuity/task.md`
8. `docs/tasks/p1-context-cache-memory-depth/task.md`
9. `docs/tasks/p1-subagent-task-isolation/task.md`
10. `docs/tasks/p1-extension-platform-foundation/task.md`
11. `docs/tasks/p1-runtime-maturation-closure/design.md`
12. `docs/tasks/p1-runtime-maturation-closure/source-alignment.md`
13. `docs/tasks/p1-runtime-maturation-closure/implementation-plan.md`
14. `docs/tasks/p1-runtime-maturation-closure/test-validation-plan.md`
15. `docs/tasks/p1-runtime-maturation-closure/review-checklist.md`

## Go Ownership

- `internal/commands`: shared runtime slash command registry
- `internal/queryengine`: default input processing, context rebuild, extension inventory assembly
- `internal/runtime`: runner projections, continuation snapshots, task spawning, inventory API
- `internal/session` and `internal/store`: durable session metadata and recovery
- `internal/approval`: pending approval state
- `internal/agent`: subagent lifecycle and recovery metadata
- `internal/tools`: tool contracts, skill inventory, MCP surfaces, Agent tool schema
- `internal/gateway`: protocol serialization only
- `internal/tui`: client display and local shell behavior only
- `internal/prompt`, `internal/workspace`, `internal/memory`, `internal/model`: context cache and prompt build inputs

## Integration Acceptance Items

- Runtime slash command metadata is shared by QueryEngine default input processing, TUI local command metadata, runtime extension inventory, and gateway `extension_inventory` payloads.
- Configured slash commands override same-name runtime commands in extension inventory without duplicates.
- Recovered pending approvals remain visible and do not mark the session ready for a new prompt.
- Recovered task/subagent state preserves allowed tools, permission mode, cwd, worktree isolation, output file, and background state.
- Context cache keys include all Build output inputs: session identity, current user message, projected history, workspace files/fingerprints, memories, system prompt variants, user/system context lines, and tool definitions.
- Read-file metadata changes are represented in system context so stale file state does not reuse cached context.
- Extension inventory rebuild is deterministic after restart/reconnect from runtime-owned inputs.
- Gateway payloads serialize runtime projection fields without inventing client-local truth.
- TUI command help and command handling do not regress because of P1 changes.

## P2 Handoff

Move these remaining capabilities to P2:

- plugin and extension lifecycle hardening
- LSP runtime capability
- remote/bridge/trusted-device foundation
- advanced execution surfaces if still relevant
- operator/UI integration readiness if still relevant

## Validation Requirements

Run:

```powershell
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions
go test ./...
```

## Completion Output Requirements

Report:

- changed docs and code files
- P1 closure gaps fixed or covered
- added tests
- P2 roadmap split
- verification commands and results
- whether P1 is ready to enter P2
- blockers or P2 deferred items
- final `git status`
- commit hash if committed
