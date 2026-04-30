# P2 Runtime Expansion Implementation Plan

Date: 2026-04-29

## Phase 0: Preserve P1 Contracts

Before any P2 child task starts, run the P1.5 validation command relevant to the touched packages and record results.

Do not begin implementation if the task requires changing a P0/P1 stable contract without a written migration plan.

## Phase 1: P2.1 Plugin/Extension Lifecycle Hardening

Target ownership:

- `internal/tools`
- `internal/queryengine`
- `internal/runtime`
- `internal/session`
- `internal/gateway`

Work:

- define lifecycle state model
- persist or rebuild lifecycle state
- emit lifecycle events
- expose read-only gateway projection
- keep marketplace deferred

## Phase 2: P2.2 LSP Runtime Capability

Target ownership:

- `internal/tools`
- `internal/runtime`
- `internal/queryengine`
- `internal/permissions`
- `internal/gateway`

Work:

- define LSP server config and lifecycle state
- add LSP tool contracts
- classify permissions
- include LSP status in extension inventory

## Phase 3: P2.3 Remote/Bridge/Trusted-Device Foundation

Target ownership:

- `internal/gateway`
- `internal/runtime`
- `internal/session`
- `internal/approval`
- `internal/protocol/ws`

Work:

- define remote identity and trust state
- model liveness/reconnect
- preserve approval correlation across transport boundaries
- document trusted-device limits

## Phase 4: P2.4 Advanced Execution Surfaces

Target ownership depends on selected surface.

Work only if still relevant:

- write task-specific runtime contract
- wire permission/approval
- add progress/result/recovery tests
- expose control-plane projection

## Phase 5: P2.5 Operator/UI Integration Readiness

Target ownership:

- `internal/gateway`
- `internal/runtime`
- `internal/queryengine`
- `internal/tui` for regression coverage only

Work:

- stabilize payload shape
- add compatibility tests
- document UI consumption boundaries
- avoid implementing React UI in this roadmap task
