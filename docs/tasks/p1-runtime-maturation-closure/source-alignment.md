# P1.5 Source Alignment

Date: 2026-04-29

## Claude Code Semantic Inputs

P1.5 remains aligned to the semantic areas identified in `docs/claude-code-go-parity-semantic-review.md`:

- `src/commands.ts` and `src/commands/`: command metadata, visibility, aliases, and slash command behavior
- `src/QueryEngine.ts`: prompt processing, mutable messages, tool loop, context handling, and runtime command entry
- `src/history.ts`, `src/state/`, and assistant/session modules: recoverable continuation state
- `src/context.ts`, `src/memdir/`, and context cache/read-file mechanics: context invalidation and rebuild
- `src/tools/AgentTool/` and `src/tasks/`: task lifecycle, background execution, permissions, and isolation
- `src/services/mcp`, `src/plugins`, `src/skills`, and `src/services/lsp`: extension inventory and deferred LSP/plugin lifecycle boundaries
- `src/cli/structuredIO.ts`, `src/bridge`, and `src/remote`: P2 remote/bridge/SDK boundary, not P1.5 implementation scope

## Go Alignment

Current Go ownership intentionally remains narrower than Claude Code:

- `internal/commands` owns P0/P1 runtime slash command metadata.
- `internal/queryengine` owns default input processing, prompt context, MCP state, and extension inventory assembly.
- `internal/runtime` owns runner projection APIs and continuation snapshots.
- `internal/session`, `internal/store`, `internal/approval`, and `internal/agent` own durable recovery metadata.
- `internal/prompt`, `internal/workspace`, and `internal/memory` own deterministic context build inputs.
- `internal/tools` owns tool contracts, Agent tool metadata, skill metadata, and MCP abstractions.
- `internal/gateway` and `internal/tui` remain projections.

## P2 Semantic Carryover

These Claude Code semantic areas are intentionally moved into P2:

- plugin lifecycle install/reload/marketplace hardening
- LSP runtime service and tool capability
- remote, bridge, trusted-device, and structured host control foundation
- advanced execution surfaces if they still fit the runtime architecture
- operator/React UI integration once runtime contracts remain stable

