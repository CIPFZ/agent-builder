# P2.2 LSP Runtime Capability Design

Date: 2026-04-30

## Architecture

P2.2 models LSP as a runtime extension boundary owned by QueryEngine and exposed through existing runtime and gateway projections. Configured LSP servers are rebuilt from runtime config. Operator intent and failure overlays are persisted through the P2.1 extension lifecycle metadata path.

Gateway and TUI remain consumers. They do not infer lifecycle state, availability, language coverage, or permission classification.

## Lifecycle Model

P2.2 LSP states are:

- `discovered`: known placeholder or discovered but not configured
- `configured`: configured and enabled, but no live client has been started
- `starting`: reserved for future external client startup
- `active`: handler/client is available
- `degraded`: configured but unavailable or marked degraded
- `failed`: marked failed with an error
- `disabled`: disabled by config or lifecycle overlay
- `stopped`: reserved for future explicit stop

Mapping to P2.1 shared lifecycle constants:

- `discovered`, `active`, `degraded`, `failed`, and `disabled` use the existing shared constants.
- `configured`, `starting`, and `stopped` are LSP-specific runtime states normalized by the LSP model and projected through lifecycle fields.
- P2.1 `loaded` maps to LSP `configured` when a configured server has no active client.
- P2.1 `unloaded` maps to LSP `stopped` for future stop behavior.

## Config Model

An LSP server config contains:

- server name
- language IDs
- file patterns
- command, args, env, and cwd
- workspace root boundary
- enabled flag
- capability hints
- read-only capability names
- mutating capability names

The initial implementation does not start external processes. It creates a mockable runtime boundary and explicit unavailable/degraded tool responses when no handler is configured.

## Inventory Projection

`lsp_boundaries` remains the gateway-compatible payload list. Each configured server projects:

- lifecycle type, source, name, version
- status, phase, notes, lifecycle state
- capabilities and recovery behavior
- last error and last updated
- language IDs and file patterns
- workspace root, command summary, enabled flag
- read-only and mutating capability classification
- permission classification

If no server is configured, the legacy deferred placeholder remains visible.

## Tool Contracts

P2.2 registers read-only LSP tools:

- `lsp_symbol_search`
- `lsp_definition`
- `lsp_references`
- `lsp_diagnostics`

The tools share a mockable `LSPHandler` boundary. Without a handler or active server, they return explicit unavailable/degraded errors. They never silently report success.

Mutating LSP actions such as rename, edit, or code action apply are deferred.

## Permission Boundary

LSP read-only tools are registered as read-only and non-destructive. They still flow through the existing registry and permission policy. Blanket denies in `internal/permissions` remove them from exposure and block invocation.

Future mutating LSP tools must be destructive or mutating and require explicit permission classification.

## Persistence And Recovery

Configured servers rebuild from runtime config on restart. Lifecycle overlays for `disabled`, `degraded`, and `failed` use `SessionMetadata.ExtensionLifecycleOverlays` through the P2.1 recovery path.

`EnableExtension` removes the persisted overlay, so a configured server returns to its config-derived state after restart.

## Unsupported Behavior

Reload/start/stop of real external LSP processes are unsupported in P2.2 and must return explicit unsupported or degraded results. This avoids a silent no-op and keeps process execution out of scope.
