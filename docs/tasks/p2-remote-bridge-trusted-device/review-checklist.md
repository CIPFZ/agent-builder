# P2.3 Remote/Bridge/Trusted-Device Review Checklist

Date: 2026-04-30

## Scope

- [ ] Remote identity foundation is implemented.
- [ ] Trust state foundation is implemented.
- [ ] Liveness/reconnect foundation is implemented.
- [ ] Approval forwarding correlation foundation is implemented.
- [ ] Full remote parity, enterprise device management, and UI are deferred.

## Ownership

- [ ] Runtime owns remote state.
- [ ] Session/store persist recovery metadata.
- [ ] Gateway calls explicit runtime APIs and only serializes projections.

## Persistence

- [ ] Remote identities persist through store-backed session recovery.
- [ ] Approval correlations persist through store-backed session recovery.
- [ ] Stale/expired liveness is derived conservatively after restart.

## Security

- [ ] Trusted remote state does not grant tool permission.
- [ ] Approval correlation does not bypass local approval manager.
- [ ] Permission policy remains execution authority.

## Gateway

- [ ] Snapshot payload includes remote identities and correlations.
- [ ] Heartbeat/reconnect/trust/correlation methods are additive and compatible.
- [ ] Existing gateway tests continue passing.

## Validation

- [ ] Focused package tests pass.
- [ ] Full `go test ./...` passes.
- [ ] `git diff --check origin/main..HEAD` passes.
- [ ] QueryEngine production files remain under 1200 lines.
- [ ] Completion output includes git status and commit hash.
