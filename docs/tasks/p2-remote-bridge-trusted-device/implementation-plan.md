# P2.3 Remote/Bridge/Trusted-Device Implementation Plan

Date: 2026-04-30

## Phase 1 Documentation

- Create the P2.3 task document set.
- Record runtime ownership, gateway projection, persistence, and non-goals.

## Phase 2 Red Tests

- Add runtime tests for identity normalization, trust transition, liveness, reconnect, expiry, and approval correlation persistence.
- Add session/store recovery tests using real store-backed session managers.
- Add gateway tests for remote snapshot/update and approval correlation serialization.
- Add permission regression proving trusted remote state does not grant tool permission.

## Phase 3 Runtime Model

- Add `internal/runtime/remote.go`.
- Define trust, liveness, correlation statuses, identity records, operation inputs, and snapshots.
- Add a `RemoteManager` owned by `Runner`.
- Persist state through session metadata after every operation.

## Phase 4 Session Metadata

- Extend `model.SessionMetadata` with remote identity records and approval correlation records.
- Keep fields optional and backward compatible.
- Rehydrate from session metadata during runner construction.

## Phase 5 Gateway Projection

- Add protocol methods for remote snapshot, heartbeat/reconnect/update, trust update, and approval correlation.
- Keep payload parsing explicit and additive.
- Serialize runtime snapshots without gateway-local inference.

## Phase 6 Validation

- Run focused tests.
- Run full required validation.
- Check `git diff --check`.
- Check queryengine production file sizes.
- Commit if all checks pass.
