# P2 Source Alignment

Date: 2026-04-29

## Claude Code Source Semantics

P2 draws from these Claude Code areas:

- `src/plugins` and plugin utilities: extension lifecycle, plugin commands, install/reload boundaries
- `src/services/mcp`: MCP lifecycle, auth, reconnect, resource/tool/prompt status
- `src/skills`: skill discovery, frontmatter, allowed tools, context, agent metadata
- `src/services/lsp`: LSP server lifecycle and model-facing capability boundary
- `src/bridge`, `src/remote`, and `src/upstreamproxy`: bridge, remote, trusted-device, and liveness foundations
- `src/cli/structuredIO.ts` and `src/cli/transports`: external host control and transport semantics
- `src/components` and `src/screens`: operator/UI payload needs, not direct UI parity target

## Current Go Foundation

Go already has:

- shared runtime command registry in `internal/commands`
- runtime extension inventory in `internal/queryengine` and `internal/runtime`
- gateway serialization in `internal/gateway`
- MCP abstractions in `internal/tools`
- skill discovery/frontmatter in `internal/tools`
- session recovery in `internal/session` and stores
- task/subagent lifecycle in `internal/agent` and `internal/runtime`
- permission policy in `internal/permissions`

## Gap Mapping

- Plugin lifecycle: inventory exists; install/reload/failure lifecycle is P2.1.
- LSP: boundary placeholder exists; runtime capability is P2.2.
- Remote/bridge/trusted-device: gateway exists; remote identity/trust/liveness is P2.3.
- Advanced execution surfaces: shell/tool patterns exist; first-class contracts are P2.4 if still relevant.
- Operator/UI: TUI and gateway projections exist; React/operator integration readiness is P2.5.

