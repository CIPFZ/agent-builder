# P1 Extension Platform Foundation Source Alignment

Date: 2026-04-29

## Claude Code Semantic Alignment

Claude Code presents extensions as runtime capabilities rather than UI-owned state. This implementation follows that shape by making queryengine/runtime the source of extension inventory truth and gateway a serializer.

## MCP

Existing MCP connection, auth, reconnect, tools, resources, prompts, and skills state remains in queryengine/tools. P1.4 reuses `MCPServers` and `MCPInventory` semantics and adds them to a broader `ExtensionInventory` projection.

## Skills

Claude-style skill frontmatter can carry allowed tools, context, hooks, and agent metadata. P1.4 preserves those fields in a client-visible inventory while leaving invocation and permission enforcement in existing tool/runtime paths.

## Commands And Tools

Dynamic commands and tools are projected from their runtime contracts. Tool identity, source, schema, deferred loading flags, and classification fields remain the registry-owned contract surface.

## LSP And Marketplace

LSP is represented as a schema boundary only. Plugin marketplace and remote extension lifecycle are explicitly deferred to P2/P3.
