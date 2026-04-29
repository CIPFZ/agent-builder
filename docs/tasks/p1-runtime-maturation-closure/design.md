# P1.5 Closure Design

Date: 2026-04-29

## Design Position

P1.5 treats P1 as a completed set of runtime foundations that must now be checked together. The closure gate validates cross-module behavior rather than adding large new capabilities.

The central design rule is one runtime source of truth:

- command metadata comes from `internal/commands`
- continuation state comes from runtime/session/approval/agent metadata
- context cache truth comes from prompt/queryengine/session inputs
- extension inventory comes from queryengine/runtime projections
- gateway and TUI serialize or display projections only

## Closure Gates

### Command Registry

Runtime commands must be consistent across:

- QueryEngine default input processor
- TUI command metadata and local immediate command handling
- runtime extension inventory
- gateway `extension_inventory` serialization

Configured commands may override runtime commands by name in inventory, but must not create duplicate slash command records.

### Continuation State

Session recovery must preserve:

- pending approvals and conservative prompt-readiness
- task/subagent visible state
- subagent allowed tools and permission mode
- cwd and worktree isolation metadata
- output file and background state
- extension inventory rebuildability
- context rebuild inputs

### Context Cache

The cache key must cover every input that affects `prompt.Build` output. Read-file changes are carried through system context lines, so stale file metadata changes must invalidate the cache.

### Extension Inventory

Inventory must include:

- runtime slash commands
- configured slash/plugin-like commands
- dynamic and MCP tools
- bundled, plugin, dynamic, local, and MCP skills
- MCP server lifecycle state
- LSP boundary placeholder
- explicit deferred capabilities

Inventory ordering must be stable and dedupe rules must be explicit.

### Client Projections

Gateway and TUI must remain clients:

- gateway serializes runtime projection fields
- TUI uses shared runtime command metadata and continuation projection
- neither layer owns command, session, task, context, or extension truth

## Deferred Design Boundaries

P1.5 confirms but does not implement:

- plugin marketplace and lifecycle hardening
- full LSP service/tooling
- remote bridge and trusted-device session runtime
- enterprise policy administration
- React/operator UI integration

