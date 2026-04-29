# P2 Runtime Expansion Test Validation Plan

Date: 2026-04-29

## Baseline Regression

Every P2 child task must run the relevant P1.5 package command plus `go test ./...` before completion.

## P2.1 Tests

- extension lifecycle state transitions
- lifecycle recovery after restart
- dynamic command/tool/skill dedupe
- MCP auth/reconnect lifecycle projection
- gateway serialization from runtime projection

## P2.2 Tests

- LSP server discovery and status projection
- LSP tool contract generation
- read-only versus mutating permission classification
- degraded unavailable server behavior
- extension inventory compatibility

## P2.3 Tests

- remote identity and trust state persistence
- reconnect and liveness behavior
- approval forwarding correlation
- gateway protocol compatibility
- conservative failure behavior after restart

## P2.4 Tests

For each selected execution surface:

- runtime contract tests
- permission and approval tests
- progress and result event tests
- recovery tests for long-running work
- gateway projection tests

## P2.5 Tests

- gateway payload shape compatibility
- reconnect projection stability
- session status and extension inventory contract tests
- TUI regression tests for existing behavior

## Final Validation

P2 child task final validation must include:

```powershell
go test ./...
```

and any P1.5 focused command covering touched packages.
