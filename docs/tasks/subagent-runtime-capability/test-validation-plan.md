# Subagent Runtime Capability Test And Validation Plan

Date: 2026-04-24

## 1. Purpose

Define the minimum test and validation bar for the subagent runtime capability module.

## 2. Test Areas

## A. Lifecycle Ledger

Cover:

- run creation metadata
- lifecycle status transitions
- terminal result and error persistence
- stop and close behavior if introduced

Suggested targets:

- `internal/agent/manager_test.go`
- `internal/runtime/runner_test.go`

## B. Runtime Spawn, Control, And Resume

Cover:

- spawn with agent type and isolation options
- real control input delivery to runtime-owned delegated runs
- stop behavior on running delegated runs
- resume with child-session reuse
- invalid resume protection for unfinished continuation, pending approval, and already-running cases

Suggested targets:

- `internal/runtime/runner_test.go`

## C. Fork And Worktree Behavior

Cover:

- fork behavior when no subagent type is provided through the intended path
- fork restrictions for nested fork children
- worktree-isolated delegated runs
- worktree path reuse on resume

Suggested targets:

- `internal/runtime/runner_test.go`

## D. Tool Contract

Cover:

- `agent.task` launched-task payload shape
- fast-finish inline result behavior
- background output file metadata
- transcript-safe task result handling

Suggested targets:

- `internal/queryengine/queryengine_test.go`
- `internal/runtime/runner_test.go`

## E. Control Plane

Cover:

- websocket spawn payload decoding
- tasks and subagents list payloads
- status payload fields
- stop, steer, and resume actions
- orchestration hook alignment
- wait or close semantics if introduced

Suggested targets:

- `internal/gateway/server_test.go`

## 3. Functional Validation Scenarios

At minimum, validate these flows:

1. spawn a background delegated task and observe its completion through control-plane state
2. send control input to a running delegated task and confirm runtime behavior changes
3. stop a delegated task and confirm terminal state is visible through status and events
4. resume a completed or stopped delegated task and confirm child session reuse
5. resume a worktree-isolated delegated task and confirm workspace context survives

## 4. Suggested Test Commands

At minimum, run:

```bash
go test ./internal/agent ./internal/runtime ./internal/queryengine ./internal/gateway
```

If the test surface becomes too slow, also run the focused subsets used during implementation:

```bash
go test ./internal/agent -run Agent
go test ./internal/runtime -run Subagent
go test ./internal/queryengine -run agent.task
go test ./internal/gateway -run Subagent
```

## 5. Exit Criteria

The module should not be considered ready for review unless:

- lifecycle tests pass
- control and resume tests pass
- fork and worktree tests pass
- gateway and orchestration tests pass
- at least one real background flow and one real resume flow were functionally validated
