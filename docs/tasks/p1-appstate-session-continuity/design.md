# P1 AppState Session Continuity Design

Date: 2026-04-28

## Goal

Create a single recoverable continuation model for runtime state that can be read by gateway and TUI clients after reconnect or process restart.

## Design Principles

- `internal/session` owns persisted session metadata and transcript-derived recovery state.
- `internal/runtime` owns the runtime continuation projection that joins session, approval, task, and policy state.
- `internal/approval` remains the source of truth for live approval requests, restored from session metadata when the runner is rebuilt.
- `internal/agent` owns visible subagent/task run state, restored from persisted session metadata when the runner is rebuilt.
- `internal/gateway` exposes client-visible projections only.
- `internal/tui` consumes the same projection and does not infer recovery state on its own.

## Snapshot Shape

The minimum runtime continuation snapshot should include:

- session identity
- continuation status and `ready_for_prompt`
- resume anchor
- compaction indicator
- pending approval projection
- visible task/subagent projections
- runtime policy and model projection
- recovery error, when the snapshot must use a conservative fallback

## Recovery Behavior

When persistent session data is valid, rebuilding a session manager and runtime runner must reconstruct the same client-visible continuation state without running the model.

When recovery data is incomplete or inconsistent, snapshot construction must not panic. It must either return a clear error or return a conservative snapshot where `ready_for_prompt=false` and the reason is visible.

## Pending Approvals

Pending approvals are recovered from session metadata into the approval manager. The snapshot reads pending approvals from the approval manager after rehydration and uses session metadata only as the persistence source.

## Task/Subagent State

Visible subagent state is persisted in session metadata and restored into the agent manager. This does not expand P1.3 isolation behavior; it only preserves visible run metadata needed for reconnect and restart inspection.

## Gateway And TUI

`session_status` must include continuation state and visible pending work. TUI/client store must be able to consume the same projection shape and show the same pending approval/task state.

## Non-Goals

- context cache and read-file state
- richer task isolation or retry semantics
- extension inventory
- React UI parity
