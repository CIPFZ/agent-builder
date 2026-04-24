# Subagent Runtime Capability Source Alignment

Date: 2026-04-24

## 1. Purpose

This document maps the intended subagent module behavior to the relevant Claude Code semantic owners and the current `myclaw` Go-side ownership points.

The goal is semantic alignment, not filename-copying.

## 2. Primary Claude Code References

### Core Runtime Ownership

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`

Why they matter:

- they define how delegated tools live inside the main query lifecycle
- they define how tool output and follow-up turns remain transcript-safe
- they define the shared runtime context available to delegated work

### Delegated Worker Ownership

- `claude-code/src/tools/AgentTool/AgentTool.tsx`
- `claude-code/src/tools/AgentTool/runAgent.ts`
- `claude-code/src/tools/AgentTool/resumeAgent.ts`
- `claude-code/src/tools/AgentTool/forkSubagent.ts`

Why they matter:

- they define task identity and worker execution ownership
- they define fork behavior, background behavior, and resume behavior
- they show that delegated work reuses the existing child context rather than being treated as a separate ad hoc tool

### Control And Prompting Ownership

- `claude-code/src/constants/prompts.ts`
- `claude-code/src/coordinator/coordinatorMode.ts`
- `claude-code/src/tools/SendMessageTool/SendMessageTool.ts`
- `claude-code/src/tools/TaskStopTool/TaskStopTool.ts`
- `claude-code/src/cli/print.ts`

Why they matter:

- they define how delegated tasks are framed to the model
- they define how background tasks are stopped, continued, and surfaced back to the user
- they show the importance of task notifications and stable task IDs

## 3. Claude Semantics We Intend To Carry Over

The `myclaw` subagent module should preserve these semantics:

1. delegated work is first-class task state, not anonymous helper execution
2. task IDs remain stable across lifecycle actions
3. background work is observable outside the main tool return path
4. continuing or resuming work reuses the task's existing child context
5. fork behavior is explicit and preserves parent context intentionally
6. stop and follow-up actions operate on task identity, not hidden session internals

## 4. Current Go Ownership Points

### Runtime Lifecycle Layer

- `internal/runtime/runner.go`
- `internal/agent/manager.go`
- `internal/session/*`
- `internal/model/session.go`

Current alignment:

- good alignment on spawn, resume, fork prompt shaping, permission derivation, and worktree reuse

Current gaps:

- the lifecycle ledger is still too thin
- running delegated tasks do not yet expose a normalized runtime control channel

### Query And Tool Layer

- `internal/queryengine/queryengine.go`
- `internal/queryengine/queryengine_test.go`

Current alignment:

- `agent.task` already routes through runtime ownership
- delegated runs already stay on the shared runtime path

Current gaps:

- delegated-task result and background contract are still ad hoc JSON strings
- the query-layer contract does not yet describe one normalized delegated-task shape

### Control Plane Layer

- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`
- `internal/orchestration/coordinator.go`

Current alignment:

- good initial method coverage for spawn, list, status, stop, steer, and resume

Current gaps:

- gateway spawn does not yet expose the runtime option surface already available internally
- event payloads still conflate lifecycle status and control actions
- there is no completed delegated-task lifecycle contract strong enough for future UI work

## 5. Explicit Non-Alignment Areas

The Go implementation should not attempt 1:1 parity on:

- React or Ink task UI visuals
- Claude Code team or swarm product surfaces not required for local runtime parity
- remote cloud worker infrastructure

Those are product and UI layers, not the backend parity target for this module.

## 6. Alignment Decision

The correct replication target is:

- Claude Code delegated-task runtime semantics
- not Claude Code task UI fidelity

If there is any tradeoff, preserve in this order:

1. stable task identity and lifecycle
2. runtime-owned control and resume behavior
3. explicit fork and worktree semantics
4. shared control-plane reachability
5. client and UI fidelity last
