# P0 Runtime Parity Roadmap Test And Validation Plan

Date: 2026-04-26

## 1. Purpose

Define the validation bar for P0 runtime parity.

This file validates the roadmap and defines the minimum test categories each downstream workstream must satisfy.

## 2. Roadmap Validation

The roadmap is valid when:

- every P0 gap from `docs/claude-code-go-parity-semantic-review.md` is assigned to a P0 workstream or explicitly deferred
- every workstream has clear source alignment
- every workstream has target Go ownership
- every workstream has acceptance criteria
- no P0 item depends on React UI, telemetry, enterprise settings, or bridge/remote implementation

## 3. Required Test Categories By Workstream

## P0.1 Tool Parity Core

Required test categories:

- shell success and failure
- shell approval-required path
- file read/write/edit/multiedit success and failure
- path normalization and workspace boundary behavior
- TodoWrite structured result behavior
- Agent spawn/status/wait/resume/steer/stop behavior
- Skill discovery and invocation behavior
- MCP dynamic tool call behavior
- tool identity preservation from tool call to tool result

Suggested command:

```powershell
go test ./internal/tools ./internal/tools/system ./internal/queryengine ./internal/runtime ./internal/permissions
```

## P0.2 Command Registry

Required test categories:

- command registration
- alias handling
- command visibility
- command execution without model invocation
- command execution that should continue into model query
- TUI delegation to shared command registry

Suggested command:

```powershell
go test ./internal/app ./internal/tui ./internal/queryengine
```

## P0.3 Context, Memory, And Recovery

Required test categories:

- workspace instruction loading
- prompt context ordering
- memory injection
- compaction boundary persistence
- pending approval recovery
- tool-use/tool-result recovery
- invoked skill recovery
- recovered session continuation

Suggested command:

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/session ./internal/store/... ./internal/model ./internal/runtime ./internal/queryengine
```

## P0.4 Runtime Structured Events

Required test categories:

- QueryEngine to runtime event mapping
- runtime to gateway event serialization
- websocket payload stability
- TUI runtime bridge consumption
- approval event shape
- tool lifecycle event shape
- command lifecycle event shape
- compaction event shape
- agent/task lifecycle event shape

Suggested command:

```powershell
go test ./internal/runtime ./internal/queryengine ./internal/gateway ./internal/protocol/ws ./internal/tui
```

## 4. End-To-End P0 Scenario

P0 must end with one documented scenario:

1. start a session
2. load workspace context
3. ask the model to inspect a file
4. execute a read tool
5. request approval for a write or shell action
6. approve and execute the action
7. update todo state
8. trigger or simulate compaction
9. persist and recover the session
10. resume the session
11. verify stable runtime events through gateway or captured event sink

## 5. Final Validation Command

At P0 completion, run:

```powershell
go test ./...
```

Expected:

- exit code 0
- all packages pass or report `[no test files]`

## 6. Exit Criteria

P0 should not be marked complete unless:

- all workstream focused tests pass
- `go test ./...` passes
- the end-to-end scenario is documented and passes
- review checklist passes for every child workstream
- all deferred work is explicitly assigned to P1 or P2

