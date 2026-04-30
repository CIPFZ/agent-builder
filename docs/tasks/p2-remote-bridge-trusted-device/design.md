# P2.3 Remote/Bridge/Trusted-Device Foundation Design

Date: 2026-04-30

## Architecture

P2.3 introduces a runtime-owned remote manager. The manager is attached to `runtime.Runner`, persists through `session.SessionMetadata`, and exposes snapshots to gateway. Gateway can report heartbeat, reconnect, disconnect, trust update, and approval correlation operations, but runtime normalizes and owns the final state.

## Remote Identity Model

Each remote identity contains:

- connection ID and session ID
- client identity and device ID
- user ID and agent ID
- transport kind
- trust state
- liveness state
- connected, disconnected, heartbeat, and reconnect deadline timestamps
- capabilities
- correlation metadata

Identity records are normalized by runtime so empty IDs, duplicate capabilities, or unknown states do not leak into projections.

## Trust State Model

Trust states:

- `unknown`: runtime has not classified the device
- `untrusted`: device is known but not trusted
- `trusted`: device is trusted for identity projection only
- `revoked`: device trust has been revoked
- `expired`: trust has expired

Trust state does not grant tool permissions and does not bypass approval.

## Liveness And Reconnect Model

Liveness states:

- `connected`: active connection with recent heartbeat
- `stale`: heartbeat is older than stale threshold but reconnect window remains open
- `disconnected`: explicit disconnect or no active connection
- `reconnecting`: reconnect attempt recorded
- `expired`: reconnect deadline elapsed

Runtime derives stale/expired state from stored timestamps and current time.

## Approval Correlation Model

Approval forwarding correlation records contain:

- local approval ID
- remote correlation ID
- remote connection, client, and device identity
- status
- created, updated, and expiry timestamps
- decision payload boundary as opaque structured metadata

The local approval manager remains authoritative. Correlations only preserve remote routing and reconciliation metadata.

## Gateway Projection

Gateway exposes:

- remote state snapshot
- remote heartbeat/update
- remote reconnect/update
- trust update
- approval correlation creation/listing

Payloads are additive and compatible with the existing WebSocket protocol.

## Persistence And Recovery

Remote identities and approval correlations are stored in session metadata. Rehydrating `session.Manager` from a store is enough to reconstruct the runtime manager.

## Deferred Behavior

P2.3 defers full remote transport, direct-connect negotiation, enterprise device policy, remote approval transport, and UI.
