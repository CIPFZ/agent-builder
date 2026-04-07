# myclaw Complete Test Plan

**Goal:** Validate the currently implemented `myclaw` kernel, control plane, permission system, memory flow, compaction, and subagent lifecycle end-to-end instead of only checking daemon liveness.

**Scope:** This plan covers the behavior already implemented in `myclaw` as of `2026-04-04`. It is focused on verification of current functionality, not future roadmap items.

---

## 1. Current Automated Coverage

These areas already have automated tests and should remain in the default regression suite:

- `internal/permissions`
  - mode evaluation: `ask`, `workspace-write`, `danger-full-access`
  - allow/deny rules
  - dangerous command approval gating
  - subagent derived policy behavior
- `internal/compaction`
  - message-count compaction
  - token-estimate compaction
  - recent tool result preservation
- `internal/memory`
  - summary persistence
  - typed memory (`summary`, `task`, `instruction`)
- `internal/prompt`
  - memory injection
  - typed memory grouping
- `internal/runtime`
  - tool loop
  - denied execution
  - compaction + memory save
  - session override policy
  - subagent spawn + policy derivation
  - parent-to-child cascade update
- `internal/gateway`
  - websocket connect
  - send_message
  - tool events
  - run.error
  - spawn/list/stop/steer/resume subagent
  - session status
  - session_set_permission
  - memory_list
- `internal/config`
  - permission env parsing
- `internal/app`
  - daemon wiring

Baseline command:

```powershell
go test ./...
```

Expected:

- exit code `0`
- no failing packages

---

## 2. Test Matrix

### A. CLI And Daemon Boot

**Purpose:** Confirm binaries start and expose the documented entrypoints.

Commands:

```powershell
go run ./cmd/myclaw version
go run ./cmd/myclawd
```

Checks:

- `myclaw version` prints `myclaw dev`
- daemon starts without panic
- daemon binds to `127.0.0.1:18080`
- `/healthz` returns `200 ok`
- `/statusz` returns session summary JSON or status output

Manual HTTP checks:

```powershell
Invoke-WebRequest http://127.0.0.1:18080/healthz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:18080/statusz -UseBasicParsing
```

---

### B. Session Lifecycle

**Purpose:** Verify main session creation, lookup, message persistence, and session status introspection.

Scenarios:

1. Connect with `agent_id=main` and no `session_key`
2. Verify daemon returns a main session id and key
3. Reconnect using returned `session_key`
4. Verify same session is reused
5. Query `session_status`

Expected:

- first connect creates a main session
- reconnect resolves the same session
- `session_status` returns:
  - `session_id`
  - `session_key`
  - `agent_id`
  - `is_main`
  - `message_count`
  - `permission_mode`
  - `subagent_mode`

---

### C. Basic Conversation Turn

**Purpose:** Verify one normal non-tool turn flows through queue, runtime, LLM, and transcript.

Scenario:

1. Connect websocket client
2. Send `send_message` with plain text like `hello`
3. Observe event flow

Expected event order:

- response `accepted`
- `message.created` for user message
- `agent.lifecycle.start`
- one or more `assistant.delta`
- `message.created` for assistant reply
- `agent.lifecycle.end`

Expected stored behavior:

- session transcript contains the user message
- session transcript contains the assistant reply

---

### D. Tool Loop

**Purpose:** Verify the model can emit a tool call, runtime executes the tool, and the assistant continues with tool output.

Scenarios:

1. `tool upper hello world`
2. `tool run echo hello`

Expected for `text.upper`:

- `tool.called`
- `tool.result`
- final assistant message includes `text.upper: HELLO WORLD`

Expected for `system.run`:

- command output reaches `tool.result`
- final assistant message includes `system.run: hello`

Regression risks to watch:

- tool event order inversion
- tool result not appended into session transcript
- second model pass not happening after tool result

---

### E. Permission Modes

**Purpose:** Verify each permission mode behaves correctly for `system.run`.

#### `ask`

Scenario:

- send `tool run pwd`

Expected:

- no tool execution
- `run.error` emitted
- error explains approval is required

#### `workspace-write`

Scenario:

- workspace root configured to repository workspace
- run command inside workspace
- run command outside workspace

Expected:

- in-root command allowed
- out-of-root command blocked with approval-required path

#### `danger-full-access`

Scenario:

- run `tool run pwd`

Expected:

- tool executes successfully

---

### F. Dangerous Command Gating

**Purpose:** Verify dangerous commands are intercepted even when mode would otherwise allow execution.

Scenarios:

1. `tool run rm -rf ./build`
2. Windows-style destructive pattern such as `tool run del /f /s temp`

Expected:

- command does not run immediately
- result is `run.error`
- reason mentions dangerous command approval

Note:

- this must be tested under at least one permissive mode such as `workspace-write` or `danger-full-access`

---

### G. Rule-Based Overrides

**Purpose:** Verify explicit allow/deny rules take precedence over coarse permission modes.

Scenarios:

1. global mode `danger-full-access` + deny rule for `rm -rf`
2. global mode `ask` + allow rule for safe workspace prefix
3. explicit deny for non-system tool such as `subagent.spawn`

Expected:

- deny rule hard-blocks execution
- allow rule bypasses default approval
- rule precedence stays above mode evaluation

---

### H. Session-Level Permission Overrides

**Purpose:** Verify live session permissions can be changed without restarting the daemon.

Scenario:

1. connect session running under permissive mode
2. call `session_set_permission` with `mode=ask`
3. query `session_status`
4. send `tool run pwd`

Expected:

- `session_set_permission` response succeeds
- `session_status.permission_mode == ask`
- later `system.run` requests fail with `run.error`

Additional scenario:

1. update workspace roots via `session_set_permission`
2. verify in-root/out-of-root behavior changes immediately

---

### I. Subagent Lifecycle

**Purpose:** Verify child sessions and child runs behave correctly across the full control surface.

Scenarios:

1. `spawn_subagent`
2. `subagent_list`
3. `tasks_list`
4. `subagent_stop`
5. `subagent_steer`
6. `subagent_resume`

Expected:

- `spawn_subagent` returns:
  - `run_id`
  - `child_session_id`
  - `child_session_key`
- child run eventually emits `subagent.completed`
- `tasks_list` and `subagent_list` include child runs
- `subagent_stop` changes state to `stopped`
- `subagent_steer` records steer message without crashing run manager
- `subagent_resume` reuses the previous child session

Transcript checks:

- child transcript is separate from parent transcript
- resumed subagent appends to existing child transcript rather than creating a new session

---

### J. Parent-Child Permission Inheritance

**Purpose:** Verify subagent sessions inherit or derive permissions correctly, and that cascade semantics are correct.

Scenarios:

1. parent mode `danger-full-access`, no override
2. spawn subagent
3. query child `session_status`

Expected:

- child derived mode is `workspace-write`

Override scenario:

1. parent `SubagentMode=ask`
2. spawn child

Expected:

- child mode is `ask`

Cascade scenario:

1. parent already has existing child session
2. call `session_set_permission` with `cascade_subagents=true`
3. query child session

Expected:

- existing child permission updates to the new derived mode

Non-cascade scenario:

1. repeat above with `cascade_subagents=false`

Expected:

- existing child keeps its current policy

---

### K. Compaction

**Purpose:** Verify transcript compaction is stable and preserves the most relevant recent state.

Scenarios:

1. exceed `MaxMessages`
2. exceed `MaxEstimatedTokens`
3. include a recent tool result before compaction

Expected:

- transcript front becomes a `summary` message
- recent turns remain
- recent tool result remains visible
- compaction does not corrupt session transcript

Manual validation:

- inspect stored messages after many turns
- confirm assistant can still answer using recent context after compaction

---

### L. Memory Persistence And Prompt Reinjection

**Purpose:** Verify summary memory and typed memory survive long enough to influence later turns.

Scenarios:

1. trigger compaction
2. observe `memory.saved`
3. call `memory_list`
4. send another user message that relies on prior memory

Expected:

- `memory.saved` event is emitted
- `memory_list` returns typed items with `type`
- prompt builder includes memory lines
- typed memory is grouped as:
  - `summary`
  - `task`
  - `instruction`

Manual validation:

- seed `task` and `instruction` memory
- verify a later prompt uses those constraints in the model request path

---

### M. Queueing And Ordering

**Purpose:** Verify per-session message serialization so concurrent sends do not corrupt transcript order.

Scenarios:

1. send multiple `send_message` requests rapidly on one session
2. observe `queue.enqueued`
3. inspect final transcript order

Expected:

- session queue serializes runs
- transcript order matches enqueue order
- no cross-run interleaving corruption

Additional scenario:

1. run parallel requests in two different sessions

Expected:

- different sessions proceed independently

---

### N. Sandbox Routing

**Purpose:** Verify `system.run` uses the correct local shell path and works on the current OS path conventions.

Scenarios:

1. main session `system.run`
2. child session `system.run`
3. non-main session sandbox expectation

Expected:

- current platform shell is selected correctly
- command output returns successfully
- non-main session behavior matches current sandbox routing rules

---

### O. Negative Protocol Tests

**Purpose:** Verify gateway rejects malformed or unsupported websocket messages cleanly.

Scenarios:

1. first message is not `connect`
2. unsupported method
3. `spawn_subagent` without `prompt`
4. `subagent_resume` without `run_id`
5. `session_set_permission` without `mode`

Expected:

- structured error response
- connection remains stable where appropriate
- server does not panic

---

## 3. Recommended Execution Order

Run in this order:

1. `go test ./...`
2. daemon boot and HTTP smoke checks
3. websocket happy-path flow
4. permission matrix
5. dangerous command matrix
6. subagent lifecycle
7. session override and cascade semantics
8. compaction + memory flow
9. queueing and multi-session concurrency
10. malformed protocol cases

---

## 4. Commands For Full Current Verification

### Baseline

```powershell
go test ./...
go run ./cmd/myclaw version
```

### Daemon

```powershell
go run ./cmd/myclawd
Invoke-WebRequest http://127.0.0.1:18080/healthz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:18080/statusz -UseBasicParsing
```

### Targeted Packages

```powershell
go test ./internal/runtime ./internal/gateway ./internal/permissions ./internal/compaction ./internal/memory ./internal/prompt
```

---

## 5. Gaps Still Worth Adding

These are not necessarily failing today, but they are the highest-value missing verification areas:

- explicit websocket integration script that runs the whole gateway matrix automatically
- queue stress test with concurrent clients
- longer compaction + memory regression over many turns
- permission cascade across deeper parent -> child -> grandchild chains
- stronger `system.run` OS-specific integration assertions on Windows
- daemon-level end-to-end test that boots a real HTTP server and drives websocket flows in one test

---

## 6. Exit Criteria

The current implementation should be considered fully verified only when all of the following are true:

- all unit and package integration tests pass
- websocket happy paths pass
- permission matrix passes
- dangerous command interception passes
- session override and subagent cascade behavior pass
- compaction and memory reinjection pass
- queue ordering passes
- malformed protocol requests fail cleanly
- daemon boot plus `/healthz` and `/statusz` checks pass
