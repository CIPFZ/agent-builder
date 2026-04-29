# P1 Extension Platform Foundation Test Validation Plan

Date: 2026-04-29

## Focused Tests

- Runtime inventory includes dynamic tools, MCP tools, MCP server lifecycle state, commands, skills, LSP boundary, and deferred capabilities.
- Runtime inventory filters denied tools through the session permission policy.
- Runtime inventory sorting is deterministic.
- Runtime inventory rebuild is deterministic after runner reconstruction with the same runtime inputs.
- Skill frontmatter inventory projection includes allowed tools, context, agent, effort, and hooks.
- Gateway `extension_inventory` returns the runtime projection.

## Regression Tests

- P0 slash command registry and SubmitPrompt single-processing remain covered by existing suites.
- P1.1 continuation snapshot remains covered by existing runtime/session/gateway tests.
- P1.2 context cache and memory rebuild remain covered by existing workspace/prompt/memory/runtime/queryengine tests.
- P1.3 subagent isolation remains covered by existing agent/runtime/session/permissions tests.

## Required Commands

- `go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway`
- `go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway`
- `go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine`
- `go test ./internal/agent ./internal/tools ./internal/runtime ./internal/session ./internal/permissions`
- `go test ./...`
