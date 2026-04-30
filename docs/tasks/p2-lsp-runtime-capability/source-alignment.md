# P2.2 LSP Runtime Capability Source Alignment

Date: 2026-04-30

## Claude Code Semantic Shape

Claude Code source semantics treat extension-like runtime capability as:

- runtime-owned discovery and state
- conservative availability projection
- tool contracts with clear permission semantics
- recoverable operator intent
- client-neutral gateway/TUI projections

P2.2 aligns to that shape by adding an LSP runtime boundary without claiming complete IDE or protocol parity.

## Current Go Foundation

The Go codebase already has:

- P1 extension inventory for tools, commands, skills, MCP servers, and LSP placeholder
- P2.1 lifecycle records and operation APIs
- persisted lifecycle overlays in session metadata
- QueryEngine-owned inventory assembly
- runtime wrappers for inventory and lifecycle operations
- gateway extension inventory serialization
- permission-aware tool registry exposure and invocation

## Reused Contracts

P2.2 reuses:

- `tools.ExtensionLifecycleRecord`
- `SessionMetadata.ExtensionLifecycleOverlays`
- QueryEngine `ExtensionInventory`
- runtime `Options` pass-through
- gateway `lsp_boundaries`
- tool registry read-only/destructive classification
- permission policy blanket deny behavior

## Intentional Deviations

P2.2 does not implement a full protocol client, process manager, or editor UI. It introduces the runtime contract and mockable handler first so later P2/P3 work can attach a real client without changing the inventory and permission contracts.

## Deferred To Later P2/P3

- external LSP process start/stop/reload
- JSON-RPC protocol transport
- rename/edit/code action apply
- UI presentation for symbols, diagnostics, and navigation
- remote/trusted-device LSP bridging
