# P2.1 Source Alignment

Date: 2026-04-29

## Claude Code Semantics

P2.1 aligns with these Claude Code areas at a semantic level:

- `src/plugins`: plugin command and extension lifecycle ownership
- `src/skills`: skill discovery, frontmatter metadata, allowed-tools hints, and model-facing visibility
- `src/services/mcp`: server state, auth, reconnect, resource/tool/prompt visibility, and failure projection
- `src/services/lsp`: lifecycle boundary only for P2.1
- `src/commands`: command metadata and user-invocable projection

## Current Go Foundation

Current Go ownership:

- `internal/queryengine/queryengine.go` assembles extension inventory and MCP state.
- `internal/runtime/runner.go` exposes QueryEngine projections.
- `internal/gateway/server.go` serializes extension inventory payloads.
- `internal/tools/registry.go` owns tool definitions and permission-aware exposure.
- `internal/tools/skill_discovery.go` owns skill discovery and dynamic skill registry.
- `internal/tools/mcp_dynamic.go` and `internal/tools/mcp_client.go` own MCP dynamic tools and discovery records.
- `internal/permissions` remains the execution permission authority.

## Alignment Decision

Go will not clone Claude Code's full plugin product surface in P2.1. The aligned unit is a runtime lifecycle model for already-discovered extension-like capabilities. Lifecycle state is runtime-owned, visible through inventory, and conservative across restart.
