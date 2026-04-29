# P1 Extension Platform Foundation Task

Date: 2026-04-29

## Objective

Implement the P1.4 runtime-owned extension inventory foundation. MCP servers, MCP tools/resources/prompts/skills, local skills, plugin-like commands, dynamic tools, and future LSP boundaries must be visible through one stable runtime projection that gateway clients can query without becoming the source of truth.

## Scope

- Define a stable extension inventory schema.
- Project MCP lifecycle state, including connected, error, needs-auth, reconnect, and auth states already tracked by runtime/queryengine.
- Project dynamic tool and command registration contracts.
- Project skill frontmatter metadata including allowed tools, context, hooks, and agent metadata.
- Expose the inventory through runtime and gateway.
- Rebuild inventory deterministically after runner/session rebuild from configured runtime sources.
- Document LSP as a boundary only.

## Non-Goals

- Full LSP implementation.
- Plugin marketplace implementation.
- Remote extension execution lifecycle beyond explicit deferred boundary records.
- Any P1.3 task isolation changes.

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
12. `docs/tasks/p1-context-cache-memory-depth/task.md`
13. `docs/tasks/p1-subagent-task-isolation/task.md`
14. `docs/tasks/p1-subagent-task-isolation/review-checklist.md`

## Ownership Boundary

- `internal/tools`: tool, skill, and frontmatter inventory models.
- `internal/queryengine`: runtime-owned inventory assembly from registry, commands, skills, and MCP state.
- `internal/runtime`: read-only inventory API.
- `internal/gateway`: client-visible projection only.
- `internal/protocol/ws`: gateway method constant.

## Validation Requirements

- `go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway`
- `go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway`
- `go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine`
- `go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions`
- `go test ./...`

## Starter Prompt

Implement P1.4 only. Add failing tests first for unified extension inventory, gateway query, skill frontmatter projection, conservative permission filtering, and deterministic rebuild. Then implement the smallest runtime-owned projection without moving truth into gateway/TUI.
