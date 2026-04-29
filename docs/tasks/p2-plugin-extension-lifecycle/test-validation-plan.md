# P2.1 Test Validation Plan

Date: 2026-04-29

## Focused Tests

- lifecycle state model unit tests in `internal/tools`
- inventory lifecycle field projection tests in `internal/runtime` or `internal/queryengine`
- reload, disable, and enable behavior tests in `internal/runtime`
- degraded and failed error propagation tests in `internal/runtime`
- deterministic rebuild after restart tests in `internal/runtime`
- gateway serialization tests in `internal/gateway`
- permission boundary regression tests in `internal/queryengine` or `internal/permissions`
- command, tool, and skill dedupe regression tests in `internal/runtime`

## Required Commands

```powershell
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./internal/permissions ./internal/tools ./internal/runtime ./internal/queryengine ./internal/gateway
go test ./...
```

## Acceptance

All focused and full tests must pass before commit. If a source cannot support an operation in P2.1, tests must assert the explicit unsupported result.
