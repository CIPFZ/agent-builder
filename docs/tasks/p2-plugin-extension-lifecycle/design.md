# P2.1 Plugin/Extension Lifecycle Design

Date: 2026-04-29

## Architecture

P2.1 treats extension lifecycle as runtime-owned metadata layered over the P1 extension inventory. QueryEngine owns the mutable lifecycle overlay because it already assembles tools, commands, skills, MCP state, and model-facing runtime projections. Runtime exposes narrow methods that delegate to QueryEngine. Gateway serializes the resulting inventory and operation responses only.

## Lifecycle State Model

Every extension projection can carry:

- `type`: `tool`, `command`, `skill`, `mcp_server`, or `lsp_boundary`
- `source`: `runtime`, `dynamic`, `plugin`, `skills`, `bundled`, `mcp`, or `lsp`
- `name`
- `version`
- `capabilities`
- `state`: `discovered`, `loaded`, `active`, `degraded`, `disabled`, `failed`, `unloaded`, or `reloaded`
- `last_error`
- `last_updated`
- `recovery_behavior`

Default rebuild rules are conservative:

- tools and commands that are available under runtime policy are `active`
- skills discovered from local, bundled, plugin, or MCP sources are `active`
- configured MCP servers without loaded resources are `loaded`
- connected MCP servers are `active`
- MCP auth-required servers are `degraded`
- MCP failed servers are `failed`
- LSP placeholder remains `discovered` with deferred capability notes

## Runtime Source Of Truth

The source of truth is QueryEngine lifecycle state plus runtime discovery inputs:

- tool registry definitions
- runtime command registry metadata
- configured commands
- dynamic skill registry
- MCP clients, tools, prompts, resources, skills, auth, and failures
- LSP placeholder metadata
- persisted or restored lifecycle records passed into runtime options

Gateway and TUI must never compute lifecycle state independently.

## Lifecycle Operations

Minimal runtime APIs:

- rebuild inventory from current runtime discovery inputs
- reload an extension or all extensions for a source
- disable an extension
- enable an extension
- mark an extension degraded or failed with an error

Unsupported operations return explicit errors. They do not silently no-op.

## Persistence And Recovery

Persistable lifecycle data is limited to operator intent and error overlays:

- `disabled` must persist if it affects runtime behavior.
- manually marked `failed` or `degraded` must persist if it affects runtime behavior or conservative projection.
- `last_error`, `last_updated`, and recovery behavior should persist with those states.

Rebuildable data is derived from config or discovery:

- runtime commands
- configured commands
- tools in the current registry
- skill files and dynamic skill registry
- MCP lists and auth/failure maps
- LSP placeholder

When persisted state is unavailable after restart, QueryEngine rebuilds conservatively from discovery and config. A recovered disabled extension stays disabled and is filtered from exposure where the current source supports enforcement.

## Permission Boundary

Lifecycle metadata can expose allowed-tools and permission hints, especially on skills and MCP tools. Those hints are advisory. Execution remains governed by `internal/permissions` and tool registry policy checks. Disabling an extension can remove or hide that extension from runtime projection, but it does not grant new capabilities.

## Deferred Behavior

- Plugin marketplace install/update flows are deferred.
- Full LSP lifecycle is deferred to P2.2.
- Remote extension lifecycle is deferred.
- Some sources can only support projection-level reload/disable in P2.1. Unsupported source behavior must be explicit in operation results.
