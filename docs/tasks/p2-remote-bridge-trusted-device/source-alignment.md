# P2.3 Remote/Bridge/Trusted-Device Source Alignment

Date: 2026-04-30

## Claude Code Semantic Shape

Claude Code remote and bridge areas include external host identity, trusted-device state, transport liveness, reconnect, upstream proxy boundaries, and approval forwarding. P2.3 aligns to the substrate rather than implementing full parity.

Relevant semantic source areas:

- `src/bridge`
- `src/remote`
- `src/upstreamproxy`
- `src/cli/structuredIO.ts`
- `src/cli/transports`
- permission prompt and approval forwarding paths

## Current Go Foundation

The Go codebase already has:

- WebSocket gateway control plane
- session/store recovery
- approval manager and pending approval metadata
- permission policy evaluation
- runtime runner as the shared backend API
- task/subagent remote isolation metadata fields
- gateway session status projection

## Reused Contracts

P2.3 reuses:

- `session.SessionMetadata` for durable recovery
- `runtime.Runner` as source-of-truth API owner
- gateway request/response protocol with additive methods
- local approval IDs and approval manager status
- permission policy as the execution authority

## Deferred Semantics

P2.3 does not implement complete structured IO, remote transport multiplexing, direct-connect negotiation, enterprise trust policy, or UI. It records and projects the runtime state needed for those later tasks.
