# Second Version Kernel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `myclaw` from a runnable first-version kernel into a safer, more controllable, longer-running multi-agent core.

**Architecture:** Extend the current runtime by deepening four seams that already exist: permission policy evaluation, transcript compaction, subagent lifecycle management, and gateway control-plane queries. Keep channels out of scope and treat the WebSocket gateway as the single ingress for runtime control, status inspection, and task orchestration.

**Tech Stack:** Go, Gorilla WebSocket, standard library testing

---

### Task 1: Permission rules v2

**Files:**
- Create: `internal/permissions/rules_test.go`
- Modify: `internal/permissions/policy.go`
- Modify: `internal/config/config.go`
- Modify: `internal/app/daemon.go`
- Test: `internal/runtime/runner_test.go`

- [ ] **Step 1: Write the failing tests**
- [ ] Add tests for tool allow/deny lists, path-scoped allowlists, dangerous command blocking, and explicit subagent downgrade behavior in `internal/permissions/rules_test.go`.
- [ ] Extend `internal/runtime/runner_test.go` with one failing case proving a denied rule overrides a global mode allow.

- [ ] **Step 2: Run test to verify it fails**
- [ ] Run: `go test ./internal/permissions ./internal/runtime`
- [ ] Expected: FAIL because rule-level evaluation and rule-aware runtime checks do not exist yet.

- [ ] **Step 3: Write minimal implementation**
- [ ] Add rule-aware policy structures to `internal/permissions/policy.go`.
- [ ] Parse rule configuration from env-backed config in `internal/config/config.go`.
- [ ] Thread the richer policy through `internal/app/daemon.go` and the runtime entrypoints.

- [ ] **Step 4: Run test to verify it passes**
- [ ] Run: `go test ./internal/permissions ./internal/runtime`
- [ ] Expected: PASS

### Task 2: Gateway control-plane queries

**Files:**
- Modify: `internal/protocol/ws/message.go`
- Modify: `internal/gateway/server.go`
- Modify: `internal/gateway/server_test.go`
- Modify: `internal/agent/manager.go`
- Modify: `internal/session/manager.go`

- [ ] **Step 1: Write the failing tests**
- [ ] Add failing gateway tests for `session_status`, `tasks_list`, and `subagent_list` request methods in `internal/gateway/server_test.go`.
- [ ] Add a failing agent manager test for listing known runs and filtering active ones.

- [ ] **Step 2: Run test to verify it fails**
- [ ] Run: `go test ./internal/agent ./internal/gateway`
- [ ] Expected: FAIL because the protocol methods and query handlers do not exist.

- [ ] **Step 3: Write minimal implementation**
- [ ] Add request/response protocol constants in `internal/protocol/ws/message.go`.
- [ ] Add list/query methods in `internal/agent/manager.go` and session summary helpers in `internal/session/manager.go`.
- [ ] Implement request handlers in `internal/gateway/server.go`.

- [ ] **Step 4: Run test to verify it passes**
- [ ] Run: `go test ./internal/agent ./internal/gateway`
- [ ] Expected: PASS

### Task 3: Compaction v2 thresholds

**Files:**
- Modify: `internal/compaction/service.go`
- Modify: `internal/compaction/service_test.go`
- Modify: `internal/runtime/runner.go`
- Modify: `internal/runtime/runner_test.go`

- [ ] **Step 1: Write the failing tests**
- [ ] Add failing compaction tests for token-estimation thresholds, summary metadata, and preserving recent tool results.
- [ ] Add a runtime-level failing test that proves compaction can trigger before raw message-count overflow.

- [ ] **Step 2: Run test to verify it fails**
- [ ] Run: `go test ./internal/compaction ./internal/runtime`
- [ ] Expected: FAIL because compaction only uses raw message count today.

- [ ] **Step 3: Write minimal implementation**
- [ ] Add estimated-token accounting and configurable thresholds in `internal/compaction/service.go`.
- [ ] Preserve summary metadata needed by runtime and future memory extraction.
- [ ] Update runtime wiring to use the richer compaction result.

- [ ] **Step 4: Run test to verify it passes**
- [ ] Run: `go test ./internal/compaction ./internal/runtime`
- [ ] Expected: PASS

### Task 4: Subagent lifecycle v2

**Files:**
- Modify: `internal/agent/manager.go`
- Modify: `internal/agent/manager_test.go`
- Modify: `internal/runtime/runner.go`
- Modify: `internal/gateway/server.go`
- Modify: `internal/gateway/server_test.go`

- [ ] **Step 1: Write the failing tests**
- [ ] Add failing tests for stopping a subagent, resuming an existing run context, and exposing active/background status.
- [ ] Add a gateway test proving a stopped subagent is reflected in query results.

- [ ] **Step 2: Run test to verify it fails**
- [ ] Run: `go test ./internal/agent ./internal/gateway ./internal/runtime`
- [ ] Expected: FAIL because stop/list/resume lifecycle pieces do not exist yet.

- [ ] **Step 3: Write minimal implementation**
- [ ] Add lifecycle state transitions and cancellation handling in `internal/agent/manager.go`.
- [ ] Thread stop/status events through runtime and gateway.

- [ ] **Step 4: Run test to verify it passes**
- [ ] Run: `go test ./internal/agent ./internal/gateway ./internal/runtime`
- [ ] Expected: PASS

### Task 5: Verification and docs

**Files:**
- Modify: `README.md`
- Modify: `docs/plan/2026-04-04-second-version-kernel-plan.md`

- [ ] **Step 1: Update docs**
- [ ] Update `README.md` with the newly supported permission rules and gateway query methods.

- [ ] **Step 2: Run repository verification**
- [ ] Run: `go test ./...`
- [ ] Expected: PASS

- [ ] **Step 3: Run executable verification**
- [ ] Run: `go run ./cmd/myclaw version`
- [ ] Expected: prints `myclaw dev`
- [ ] Run: start `go run ./cmd/myclawd` and probe `http://127.0.0.1:18080/healthz`
- [ ] Expected: `200 ok`
