# P2.2 LSP Runtime Capability Review Checklist

Date: 2026-04-30

## Scope

- [ ] LSP first-stage runtime capability is implemented.
- [ ] Full LSP client/protocol behavior remains deferred.
- [ ] React UI, marketplace, and remote trusted-device behavior remain out of scope.

## Ownership

- [ ] QueryEngine/runtime owns LSP state.
- [ ] Gateway and TUI only project runtime state.
- [ ] `lsp_boundaries` compatibility is preserved.

## Lifecycle

- [ ] LSP states cover discovered, configured, starting, active, degraded, failed, disabled, and stopped.
- [ ] Shared states map to P2.1 lifecycle constants where possible.
- [ ] Disable, enable, degraded, and failed operations work for LSP servers.
- [ ] Unsupported reload/start behavior is explicit.

## Persistence

- [ ] Disabled/degraded/failed LSP overlays persist through session metadata.
- [ ] Real session/store recovery tests cover restart behavior.
- [ ] Enable clears persisted disabled overlay.

## Tools And Permissions

- [ ] Read-only LSP tool contracts are registered.
- [ ] LSP tools return explicit unavailable/degraded results without a handler.
- [ ] Read-only LSP actions use read-only permission classification.
- [ ] Mutating actions are deferred or destructive if introduced.

## Validation

- [ ] Focused package tests pass.
- [ ] Full `go test ./...` passes.
- [ ] `git diff --check origin/main..HEAD` passes.
- [ ] QueryEngine production files remain under 1200 lines.
- [ ] Completion output includes git status and commit hash.
