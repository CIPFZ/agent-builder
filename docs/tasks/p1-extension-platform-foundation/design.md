# P1 Extension Platform Foundation Design

Date: 2026-04-29

## Design Summary

The extension platform foundation is a read-only inventory surface owned by runtime/queryengine. It gathers existing extension-capable subsystems into one stable projection:

- Tool registry contracts for built-in, dynamic, and MCP tools.
- Runtime command configuration for plugin-like slash commands.
- Skill discovery/frontmatter metadata for bundled, plugin, dynamic, and MCP skills.
- MCP server lifecycle snapshots.
- Explicit future boundaries for LSP and deferred marketplace work.

Gateway serializes this projection but does not infer, mutate, or persist extension truth.

## Schema

`ExtensionInventory` contains:

- `Summary`: counts for tools, commands, skills, MCP servers, and LSP boundaries.
- `Tools`: stable sorted tool contracts filtered by effective permission policy.
- `Commands`: stable sorted command metadata.
- `Skills`: stable sorted skill metadata with allowed tools, context, hooks, and agent fields.
- `MCPServers`: existing MCP server snapshots.
- `LSPBoundaries`: schema-only deferred LSP boundary records.
- `DeferredCapabilities`: explicit names for P2/P3 capabilities.

## Permission Semantics

Inventory projection never grants capability. Tool projection is filtered by the current session permission policy, so denied tools do not appear as available extension tools. Skill `allowed_tools` are metadata only; execution paths must still apply existing tool policy and P1.3 inheritance rules.

## Recovery Semantics

Inventory rebuilds from runtime-owned inputs: tool registry, configured commands, discovered skills, and MCP state maps. On restart/rebuild, the same inputs produce deterministic inventory ordering and content. If remote extension lifecycle state cannot be rebuilt, the conservative projection is a deferred boundary rather than a connected capability.
