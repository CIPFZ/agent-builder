# P2.3 Remote/Bridge/Trusted-Device Test Validation Plan

Date: 2026-04-30

## Runtime Tests

- Remote identity normalization trims identity fields and dedupes capabilities.
- Trust state transitions preserve unknown, untrusted, trusted, revoked, and expired.
- Heartbeat keeps a connection connected and updates timestamps.
- Liveness derives stale and expired from time thresholds.
- Reconnect records reconnecting state and deadline.
- Disconnect records disconnected state.
- Approval correlation records preserve local approval ID, remote correlation ID, remote identity, status, expiry, and decision payload boundary.

## Recovery Tests

- Remote identity/trust/liveness state recovers through session/store metadata.
- Approval forwarding correlations recover through session/store metadata.
- Recovery does not require manually injecting in-memory state into runtime options.

## Gateway Tests

- Remote snapshot serializes identities and approval correlations.
- Heartbeat/reconnect/trust update methods call runtime-owned APIs and return snapshots.
- Approval correlation method returns durable correlation projection.
- Existing connect, session status, approval, extension inventory, and task methods remain compatible.

## Permission Tests

- Trusted remote state does not bypass local permission policy.
- Approval forwarding correlation does not approve or reject the local approval by itself.

## Required Validation

```powershell
git diff --check origin/main..HEAD
go test ./internal/runtime
go test ./internal/gateway
go test ./internal/session ./internal/store/...
go test ./internal/approval ./internal/permissions
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/permissions ./internal/tools ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./...
```
