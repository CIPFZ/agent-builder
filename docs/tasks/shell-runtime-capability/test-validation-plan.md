# Shell Runtime Capability Test Validation Plan

Date: 2026-04-25

## 1. Validation Goal

Validate that shell execution is now a stable runtime capability rather than only a callable local tool.

Validation must prove:

- correctness
- approval behavior
- lifecycle visibility
- session/worktree coherence

## 2. Required Automated Tests

### A. Tool-Level Tests

Cover:

- successful local command execution
- command failure / non-zero exit handling
- schema/input validation behavior

Likely files:

- `internal/tools/system/run_test.go`

### B. Runtime / QueryEngine Tests

Cover:

- shell tool lifecycle through shared runtime path
- approval-required shell flow
- progress forwarding where supported
- structured result forwarding where supported

Likely files:

- new shell-focused queryengine tests
- `internal/runtime/runner_test.go`

### C. Gateway Tests

Cover:

- websocket `tool.called`
- websocket `tool.progress`
- websocket `tool.result`
- shell execution visibility from external client path

Likely files:

- `internal/gateway/server_test.go`

### D. Session / Worktree Tests

Cover:

- shell execution under main session
- shell execution under child worktree session
- working directory resolution behavior

## 3. Required Functional Validation

At minimum run:

1. a successful local project command
2. a failing local command
3. an approval-gated command scenario
4. a websocket-observable shell progress/result scenario

## 4. Recommended Commands

Exact commands depend on implementation details, but validation should be of this class:

- `go test ./internal/tools/system`
- `go test ./internal/queryengine ./internal/runtime ./internal/gateway`
- representative targeted test names for newly added shell lifecycle cases

## 5. Failure Gates

Do not declare the module complete if any of the following remain true:

- shell approval behavior is only manually asserted and not tested
- progress is generated but not forwarded through gateway
- shell results are only observable via loose text formatting
- worktree/session behavior is ambiguous or untested
- gateway behavior depends on client-specific shell branches

## 6. Review-Driven Revalidation

If review findings touch:

- permission behavior
- runtime event behavior
- gateway event serialization
- session/worktree behavior

then rerun the relevant runtime and gateway tests plus one representative functional validation path before declaring the module complete.
