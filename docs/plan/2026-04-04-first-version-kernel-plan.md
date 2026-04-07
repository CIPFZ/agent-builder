# First Version Kernel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable `myclaw` version with a Claude-Code-style Go kernel, permission model, compaction entry point, subagent lifecycle, and a gateway that exposes the kernel cleanly.

**Architecture:** Introduce a dedicated engine layer that owns the conversation loop and delegates to focused permission, compaction, and subagent services. Keep the WebSocket gateway as a thin control plane and preserve seams for an always-on OpenClaw-style daemon and future remote channels.

**Tech Stack:** Go, Gorilla WebSocket, standard library testing

---

### Task 1: Lock the engine-oriented file layout

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/types.go`
- Create: `internal/permissions/policy.go`
- Create: `internal/compaction/service.go`
- Create: `internal/agent/manager.go`
- Modify: `internal/runtime/runner.go`
- Modify: `internal/gateway/server.go`
- Test: `internal/engine/engine_test.go`

- [ ] Define the engine-facing interfaces in `internal/engine/types.go`
- [ ] Add a failing compile-oriented test in `internal/engine/engine_test.go` that references the planned constructor and turn result
- [ ] Run `go test ./internal/engine`
- [ ] Add the minimal type definitions to make the test compile
- [ ] Run `go test ./internal/engine`

### Task 2: Add permission-gated tool execution

**Files:**
- Create: `internal/permissions/policy_test.go`
- Create: `internal/tools/system/run_test.go`
- Modify: `internal/tools/registry.go`
- Modify: `internal/tools/system/run.go`
- Modify: `internal/runtime/runner.go`

- [ ] Write a failing test for `ask`, `workspace`, and `danger-full-access` permission modes in `internal/permissions/policy_test.go`
- [ ] Run `go test ./internal/permissions ./internal/tools/system`
- [ ] Implement the minimal permission evaluator and tool invocation hook-up
- [ ] Run `go test ./internal/permissions ./internal/tools/system`
- [ ] Refactor names and error messages while keeping tests green

### Task 3: Move the conversation loop into the engine

**Files:**
- Create: `internal/engine/engine_test.go`
- Modify: `internal/engine/engine.go`
- Modify: `internal/runtime/runner.go`
- Modify: `internal/llm/client.go`

- [ ] Write a failing engine test that proves one turn can stream text, execute a tool, and continue with the tool result
- [ ] Run `go test ./internal/engine ./internal/runtime`
- [ ] Implement the minimal multi-pass engine loop and keep `runtime.Runner` as a wrapper
- [ ] Run `go test ./internal/engine ./internal/runtime`
- [ ] Refactor duplicated event assembly if needed and rerun the same tests

### Task 4: Add subagent lifecycle support

**Files:**
- Create: `internal/agent/manager_test.go`
- Modify: `internal/agent/manager.go`
- Modify: `internal/engine/engine.go`
- Modify: `internal/session/manager.go`
- Modify: `internal/gateway/server.go`

- [ ] Write a failing test for spawning a subagent, tracking parent-child session relationships, and collecting the result
- [ ] Run `go test ./internal/agent ./internal/engine ./internal/session`
- [ ] Implement the minimal in-process subagent manager and session linkage
- [ ] Run `go test ./internal/agent ./internal/engine ./internal/session`
- [ ] Add any missing gateway event assertions with the tests still green

### Task 5: Add compaction threshold and summary boundary messages

**Files:**
- Create: `internal/compaction/service_test.go`
- Modify: `internal/compaction/service.go`
- Modify: `internal/engine/engine.go`
- Modify: `internal/session/manager.go`

- [ ] Write a failing test that proves long transcripts trigger compaction and append a compacted summary boundary
- [ ] Run `go test ./internal/compaction ./internal/engine ./internal/session`
- [ ] Implement the minimal token estimator, threshold check, and summary injection
- [ ] Run `go test ./internal/compaction ./internal/engine ./internal/session`
- [ ] Refactor summary formatting only after the tests stay green

### Task 6: Extend the gateway into a thin control plane

**Files:**
- Create: `internal/gateway/server_test.go`
- Modify: `internal/gateway/server.go`
- Modify: `internal/protocol/ws/*.go`
- Modify: `cmd/myclawd/main.go`

- [ ] Write failing gateway tests for session connect, message send, subagent events, and permission-denied responses
- [ ] Run `go test ./internal/gateway ./internal/protocol/ws`
- [ ] Implement the minimal protocol and server wiring changes
- [ ] Run `go test ./internal/gateway ./internal/protocol/ws`
- [ ] Refactor repeated payload builders if the tests remain green

### Task 7: Run repository verification

**Files:**
- Modify: `README.md`
- Test: `./...`

- [ ] Update `README.md` to describe the first-version kernel capabilities that now exist
- [ ] Run `go test ./...`
- [ ] Run `go run ./cmd/myclaw version`
- [ ] Run `go run ./cmd/myclawd`
- [ ] Fix any regressions and rerun the same commands until they pass cleanly
