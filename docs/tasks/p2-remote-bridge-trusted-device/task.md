# P2.3 Remote/Bridge/Trusted-Device Foundation Task

Date: 2026-04-30

## Objective

Implement the first runtime-owned remote/bridge/trusted-device foundation on top of P1 recovery, P2.1 lifecycle, and P2.2 LSP runtime capability.

P2.3 defines remote identity, trust state, liveness/reconnect, approval forwarding correlation, persistence, and gateway projection. It is not full Claude Code remote parity, enterprise device management, or UI implementation.

## Scope

- Define runtime-owned remote identity with connection/session ID, client identity, device ID, user/agent ID, transport kind, trust state, liveness state, timestamps, capabilities, and correlation metadata.
- Define trusted-device states: `unknown`, `untrusted`, `trusted`, `revoked`, and `expired`.
- Define liveness states: `connected`, `stale`, `disconnected`, `reconnecting`, and `expired`.
- Add heartbeat, reconnect, disconnect, expiry, and trust update operations through runtime-owned APIs.
- Add durable approval forwarding correlation records with local approval ID, remote correlation ID, remote client/device identity, status, timestamps, expiry, and decision payload boundary.
- Persist remote identity, trust, liveness, and approval correlation through session metadata/store recovery.
- Add gateway methods/payloads for remote state snapshot, heartbeat, reconnect/update state, trust update, and approval correlation projection.
- Preserve all existing connect, approval, permission, task, extension inventory, and LSP flows.

## Non-Goals

- Do not implement full remote transport parity.
- Do not implement enterprise device management or managed settings.
- Do not implement React/operator UI.
- Do not let trusted-device state grant tool permissions.
- Do not forward approval decisions to bypass the local approval manager.
- Do not make gateway a source of truth for final trust or liveness state.

## Required Reading

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p1-runtime-maturation-closure/task.md`
5. `docs/tasks/p2-runtime-expansion-roadmap/task.md`
6. `docs/tasks/p2-runtime-expansion-roadmap/design.md`
7. `docs/tasks/p2-runtime-expansion-roadmap/source-alignment.md`
8. `docs/tasks/p2-runtime-expansion-roadmap/implementation-plan.md`
9. `docs/tasks/p2-runtime-expansion-roadmap/test-validation-plan.md`
10. `docs/tasks/p2-runtime-expansion-roadmap/review-checklist.md`
11. `docs/tasks/p2-plugin-extension-lifecycle/task.md`
12. `docs/tasks/p2-lsp-runtime-capability/task.md`

## Claude Semantic Alignment

Claude Code bridge and remote semantics include external host identity, trusted-device state, liveness/reconnect, and permission/approval forwarding boundaries. P2.3 aligns to that foundation while keeping local runtime and local approval authority as the source of truth.

## Go Ownership Boundary

- `internal/runtime`: remote manager, state transition APIs, approval correlation APIs, snapshots.
- `internal/model` and `internal/session`: durable session metadata for recovery.
- `internal/gateway`: protocol parsing and serialization only; gateway calls runtime APIs.
- `internal/approval`: local approval authority remains unchanged.
- `internal/permissions`: trusted remote state does not change permission decisions.

## Implementation Order

1. Create this task document set.
2. Add failing unit tests for identity normalization, trust transitions, liveness, and approval correlations.
3. Add failing session/store recovery tests.
4. Add failing gateway serialization/API tests.
5. Implement runtime remote manager and metadata conversion.
6. Wire runtime runner APIs.
7. Wire gateway protocol methods and payload helpers.
8. Add permission boundary regression tests.
9. Run focused and full validation, line counts, status, and commit.

## Validation Requirements

Run:

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

## Completion Output Requirements

Report:

- created or modified docs files
- created or modified code files
- remote identity model
- trust state model
- liveness/reconnect model
- approval forwarding correlation model
- runtime source of truth location
- gateway projection/API
- persistence and recovery rules
- security/permission boundary
- unsupported or deferred behavior
- added or updated tests
- queryengine production file line counts
- validation command results
- whether P2.4 entry criteria are met
- final `git status --short --branch`
- commit hash if committed

## Starter Prompt

Implement P2.3 Remote/Bridge/Trusted-Device Foundation in the current branch. Read this task and companion docs first, then use TDD to add runtime-owned remote identity, trust/liveness state, approval correlation persistence, gateway projection, validation, and commit.
