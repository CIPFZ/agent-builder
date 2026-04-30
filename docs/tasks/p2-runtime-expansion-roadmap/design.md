# P2 Runtime Expansion Design

Date: 2026-04-29

## Design Principle

P2 expands runtime capabilities without changing the P0/P1 source-of-truth boundaries.

The P2 design must remain runtime-first:

- runtime owns state and contracts
- queryengine owns model-facing runtime assembly
- session/store own durable state
- gateway and TUI remain projections
- UI work waits for stable backend payloads

## P2 Architecture Threads

### Extension Lifecycle

Build on P1 extension inventory by adding lifecycle state:

- discovered
- installed/configured
- loaded
- disabled
- failed
- needs-auth
- reloaded

This hardens local skills, plugin-like commands, MCP-backed extensions, and future extension types without introducing a marketplace in P2.1.

### LSP Runtime

Turn the P1 LSP placeholder into a bounded service:

- discover configured language servers
- expose server status
- define read-only and mutating LSP tool contracts
- route permission checks through existing policy
- degrade visibly when unavailable

### Remote/Bridge Foundation

Add identity, trust, liveness, reconnect, and approval-forwarding foundations. P2.3 is not full trusted-device product parity; it is the minimum runtime substrate for later bridge/remote work.

### Advanced Execution Surfaces

Only proceed if still relevant after P1/P2.1-P2.3. Each surface must have an explicit runtime contract, permission semantics, progress/result events, and recovery story.

### Operator/UI Readiness

Prepare backend payload stability for a future React/operator UI. This is readiness, not UI implementation.

## Compatibility Requirements

P2 must not break:

- slash command registry semantics
- QueryEngine prompt/input processing
- extension inventory field meanings
- session continuation payloads
- task/subagent isolation metadata
- approval pending/recovered semantics
- context cache keys and invalidation rules
