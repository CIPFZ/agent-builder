# Shell Runtime Capability Implementation Plan

Date: 2026-04-25

## 1. Implementation Strategy

This module should be implemented as one coherent execution-surface hardening task.

Do not split it into micro-tasks that lose the architectural contract.

The correct implementation strategy is:

1. normalize shell contract
2. harden approval behavior
3. normalize lifecycle observability
4. verify gateway/control-plane behavior

## 2. Work Package A: Current State Audit

Target files:

- `internal/tools/system/run.go`
- any shell-related tool registration path
- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/gateway/server.go`

Required outcome:

- explicitly map current shell entrypoints
- identify existing lifecycle hooks already available
- identify gaps between current behavior and documented target

## 3. Work Package B: Shell Contract Normalization

Target files:

- `internal/tools/system/run.go`
- related shell tool definitions if more than one tool participates

Required outcome:

- shell tool schema and invocation semantics are explicit
- result contract is stabilized
- local execution context rules are explicit

## 4. Work Package C: Permission And Approval Hardening

Target files:

- `internal/queryengine/queryengine.go`
- `internal/permissions`
- any shared approval classification path involved in shell tool execution

Required outcome:

- approval path for shell execution is clearly centralized
- shell sensitivity classification is explicit
- policy-driven behavior is testable and reviewable

## 5. Work Package D: Progress And Result Lifecycle

Target files:

- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/gateway/server.go`

Required outcome:

- tool.called, tool.progress, and tool.result semantics for shell are explicit and shared
- structured result data is preserved where runtime supports it
- gateway forwards shell lifecycle without custom hacks

## 6. Work Package E: Session And Worktree Behavior

Target files:

- `internal/runtime/runner.go`
- shell execution context helpers
- any session metadata or path resolution helpers involved

Required outcome:

- shell working directory behavior is explicit
- subagent worktree execution remains coherent
- future execution modules can rely on the same context rules

## 7. Work Package F: Tests And Functional Validation

Required outcome:

- unit and integration coverage for shell lifecycle
- gateway visibility coverage
- representative local command functional validation

## 8. Sequencing Rules

- finish shell contract and approval foundations before touching gateway polish
- do not start Docker/DB-specific code in this module
- do not expand into TUI work
- if a design mismatch appears, update docs first, then code

## 9. Completion Standard

The implementation is only complete when:

- shell semantics are centralized and documented
- approval/progress/result behavior is visible on shared runtime paths
- gateway visibility is stable
- tests and representative functional validation pass
