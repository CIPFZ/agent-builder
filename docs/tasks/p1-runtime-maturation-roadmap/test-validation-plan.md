# P1 Runtime Maturation Roadmap Test And Validation Plan

Date: 2026-04-26

## 1. Purpose

Define the validation bar for P1 runtime maturation.

This file validates the roadmap and defines the minimum test categories each downstream workstream must satisfy.

## 2. Roadmap Validation

The roadmap is valid when:

- every P1 gap from `docs/claude-code-go-parity-semantic-review.md` is assigned to a P1 workstream or explicitly deferred
- every workstream has clear source alignment
- every workstream has target Go ownership
- every workstream has acceptance criteria
- no P1 item depends on React UI, telemetry, enterprise settings, or full bridge/remote implementation

## 3. Required Test Categories By Workstream

## P1.1 AppState And Session Continuity

Required test categories:

- runtime state snapshot creation
- runtime state recovery
- pending approval recovery
- task/subagent state recovery
- client reconnect state read
- TUI and gateway state consistency

Suggested command:

```powershell
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
```

## P1.2 Context Cache And Memory Depth

Required test categories:

- deterministic `CLAUDE.md` and workspace instruction loading
- read-file state tracking
- context cache hit and invalidation
- projected history view
- history snip and replay
- compaction memory save and recovery
- deterministic context rebuild after restart

Suggested command:

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
```

## P1.3 Subagent Task Isolation

Required test categories:

- background task spawn and completion
- task output file behavior
- task foreground/background transitions
- retry/resume/continue controls
- worktree isolation lifecycle
- cwd override behavior
- allowed-tools enforcement
- safer permission inheritance
- gateway task control behavior

Suggested command:

```powershell
go test ./internal/agent ./internal/agents ./internal/tools ./internal/runtime ./internal/session ./internal/permissions ./internal/tui ./internal/gateway
```

## P1.4 Extension Platform Foundation

Required test categories:

- extension inventory creation
- MCP discovery lifecycle
- skill discovery lifecycle
- skill frontmatter allowed-tools behavior
- dynamic command registration
- dynamic tool registration
- permission rules across extension types
- gateway extension inventory behavior

Suggested command:

```powershell
go test ./internal/tools ./internal/config ./internal/runtime ./internal/queryengine ./internal/gateway
```

## 4. End-To-End P1 Scenario

P1 must end with one documented scenario:

1. start a session
2. load workspace context and memory
3. read a file and record read-file state
4. discover a skill or MCP capability
5. start a background subagent with constrained tools
6. persist state while the task is active
7. restart or reconnect
8. inspect session, task, approval, context, and extension state
9. resume or continue the task
10. rebuild context deterministically
11. verify TUI and gateway report the same recovered state

## 5. Final Validation Command

At P1 completion, run:

```powershell
go test ./...
```

Expected:

- exit code 0
- all packages pass or report `[no test files]`

## 6. Exit Criteria

P1 should not be marked complete unless:

- all workstream focused tests pass
- `go test ./...` passes
- the end-to-end P1 scenario is documented and passes
- review checklist passes for every child workstream
- all deferred work is explicitly assigned to P2 or P3

