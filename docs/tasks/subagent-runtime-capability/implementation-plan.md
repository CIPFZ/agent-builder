# Subagent Runtime Capability Implementation Plan

Date: 2026-04-24

## 1. Planning Goal

Provide a concrete implementation sequence for turning the current partial delegated-task core into a complete `myclaw` runtime module.

## 2. Current Baseline

Already present:

- `agent.task` executor
- `SpawnSubagentWithOptions`
- fork prompt shaping
- worktree-isolated child sessions
- derived subagent permission policy
- background output file writing
- `ResumeSubagent`
- basic gateway lifecycle methods

Still missing as a module:

- one authoritative lifecycle ledger
- runtime-real control input for active delegated runs
- normalized task result and notification contract
- richer control-plane payloads
- module-level implementation contract

## 3. Phase Breakdown

## Phase 1: Lifecycle Ledger Normalization

### Objective

Make delegated-task state explicit and stable enough for runtime and control-plane consumption.

### Target Files

- `internal/agent/manager.go`
- `internal/agent/manager_test.go`
- `internal/runtime/runner.go`
- `internal/model/session.go` or session metadata files as needed

### Required Work

- extend task records to preserve lifecycle metadata rather than only current output
- separate lifecycle status from recent control action
- add timestamps and stable summary fields where needed
- preserve task lineage fields needed by resume and orchestration

### Acceptance

- delegated-task records are rich enough to drive list, status, review, and future UI work
- lifecycle state does not require re-deriving meaning from multiple unrelated fields

## Phase 2: Runtime Control And Resume Hardening

### Objective

Make runtime-owned control actions real for actual delegated runs.

### Target Files

- `internal/runtime/runner.go`
- `internal/runtime/runner_test.go`
- `internal/agent/manager.go`

### Required Work

- introduce a runtime-real control channel for active delegated runs
- ensure stop remains a hard cancellation path
- ensure steer or continue behavior reaches runtime-owned delegated execution
- preserve child-session reuse on resume
- preserve worktree path reuse and child permission policy reuse

### Acceptance

- control actions work on real spawned delegated runs
- resume reuses the original child session when valid
- invalid resume states still fail safely

## Phase 3: Tool Contract Normalization

### Objective

Normalize how delegated tasks appear through `agent.task` and runtime results.

### Target Files

- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/queryengine/queryengine_test.go`

### Required Work

- define one stable launched-task result payload
- preserve fast-finish inline result behavior where intentionally supported
- make background launch metadata explicit
- keep tool result and transcript behavior transport-neutral

### Acceptance

- `agent.task` no longer relies on ad hoc or ambiguous payloads
- foreground and background delegated-task outcomes are clearly distinguishable

## Phase 4: myclawd Control-Plane Exposure

### Objective

Expose delegated-task state and actions in a stable shared protocol.

### Target Files

- `internal/gateway/server.go`
- `internal/gateway/server_test.go`
- `internal/protocol/ws/message.go`
- `internal/orchestration/coordinator.go`

### Required Work

- extend `spawn_subagent` payload to reach runtime options already supported internally
- normalize list and status payload fields
- preserve stop, steer, and resume actions on shared runtime ownership
- add explicit wait or close semantics if needed to complete the lifecycle contract
- keep orchestration hooks aligned to the normalized payloads

### Acceptance

- future React UI can manage delegated tasks from `myclawd` without backend redesign
- no client-specific delegated-task lifecycle logic is required

## Phase 5: Tests And Validation

### Objective

Close the module with focused tests and representative runtime validation.

### Target Files

- `internal/agent/manager_test.go`
- `internal/runtime/runner_test.go`
- `internal/queryengine/queryengine_test.go`
- `internal/gateway/server_test.go`

### Acceptance

- relevant focused suites pass
- representative background, control, resume, and worktree scenarios are validated

## 4. Risks

### Risk A: Fake Control Semantics Survive

Mitigation:

- explicitly test steer or continue against real runtime-owned delegated runs

### Risk B: Lifecycle State Remains Ambiguous

Mitigation:

- normalize status versus action fields before expanding the gateway payload surface

### Risk C: Resume Regresses Existing Child Session Behavior

Mitigation:

- keep resume child-session reuse as a protected invariant with targeted tests

## 5. Non-Goals

- React task UI work
- teammate or swarm parity beyond local delegated-task needs
- Docker or database runtime work
- broad remote bridge work

## 6. Definition Of Done

This module is complete when:

- delegated tasks have a stable runtime lifecycle model
- real delegated runs can be controlled and resumed through runtime-owned paths
- `agent.task` has a normalized task result contract
- `myclawd` exposes delegated task state and actions through stable payloads
- the implementation passes the review checklist without a blocking gap
