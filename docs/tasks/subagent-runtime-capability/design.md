# Subagent Runtime Capability Design

Date: 2026-04-24

## 1. Objective

Convert the current partial delegated-task implementation into a stable module that supports:

- first-class delegated task identity
- runtime-owned background execution
- explicit control and resume semantics
- worktree and fork behavior that remains intentional
- `myclawd` control-plane visibility

This module is aligned to three inputs together:

1. user requirement:
   - subagent is one of the core required abilities
   - later Docker and database control work should be composable through delegated execution
2. current architecture:
   - runtime first
   - `myclawd` as shared control plane
   - React UI later
3. Claude Code semantic reference:
   - worker and fork task identity
   - background notification model
   - continue and stop semantics by task identity
   - child session reuse for follow-up work

## 2. Current Assessment

`myclaw` already has more delegated-task runtime than the rest of the docs previously made explicit.

Existing strengths:

- `internal/runtime/runner.go` already supports spawn, resume, fork prompts, worktree isolation, permission derivation, and background output files
- `internal/agent/manager.go` already tracks active runs and supports stop, list, get, and control-message storage
- `internal/gateway/server.go` already exposes basic subagent control-plane methods
- `internal/runtime/runner_test.go` and `internal/gateway/server_test.go` already cover meaningful parts of spawn, background, worktree, stop, steer, and resume behavior

Current module-level gaps:

1. `agent.Manager` still models only a thin run record and cannot serve as a real lifecycle ledger
2. runtime-owned delegated runs do not yet consume control messages in a normalized way, so `subagent_steer` is not a trustworthy control surface for actual spawned runs
3. gateway spawn currently exposes only label and prompt even though runtime already supports agent type, model, effort, isolation, allowed tools, and fork selection
4. background delegated-task completion exists, but its result, notification, and control-plane contract are still ad hoc
5. lifecycle state is conflated with control actions in event payloads, which will make future UI and review logic fragile

The design therefore should not "rebuild subagents from scratch". It should complete and normalize the existing partial implementation.

## 3. Design Decision

The subagent module should be implemented as one runtime and control-plane module, not split into separate "agent.task", "background tasks", "resume", and "gateway" projects.

This is the correct cut because:

- all four behaviors share the same child session and lifecycle state
- all four behaviors share permission inheritance and worktree rules
- all four behaviors must stay synchronized in the control plane
- Claude Code models delegated work as a task lifecycle, not as isolated helper features

## 4. Target Module Shape

The completed module should have four internal layers.

### Layer A: Lifecycle Ledger

Purpose:

- define one authoritative runtime view of delegated-task state

Responsibilities:

- stable task ID and metadata
- start and finish timestamps
- lifecycle status
- latest output and latest error summary
- background output file metadata
- pending or recent control actions

This is the biggest immediate product gap because `myclawd` cannot become a stable operator surface if delegated runs are represented only as thin in-memory goroutine wrappers.

### Layer B: Runtime Task Controller

Purpose:

- make spawn, control, stop, and resume behavior real for runtime-owned delegated runs

Responsibilities:

- runtime-owned control channel for running tasks
- explicit stop semantics
- resume using existing child session and continuation state
- preserved worktree and permission semantics
- fork behavior as an intentional runtime path

This layer should absorb today's gap where control methods exist but are only partially meaningful for actual subagent execution.

### Layer C: Tool Contract

Purpose:

- normalize how delegated tasks appear to the model and the transcript

Responsibilities:

- `agent.task` return contract
- background launch contract
- inline fast-finish contract
- transcript-safe result content
- task notification metadata where appropriate

This layer must stay pragmatic. `myclaw` does not need to force every delegated run into one exact Claude CLI UX, but it does need one stable backend contract.

### Layer D: Control Plane Exposure

Purpose:

- expose delegated-task state and actions through `myclawd`

Responsibilities:

- spawn payload normalization
- task inventory payloads
- status payloads
- stop, steer, resume, and wait or close control actions
- orchestration hook compatibility

This is required by the runtime-first architecture. If delegated-task state exists only in runtime internals, the future React operator UI cannot consume it cleanly.

## 5. Key Behaviors

### 5.1 Lifecycle Model

The module should explicitly separate:

- lifecycle status
- last control action

Recommended lifecycle statuses:

- `running`
- `completed`
- `failed`
- `stopped`
- `closed`

Recommended control actions:

- `spawned`
- `steered`
- `resumed`
- `stopped`
- `closed`

This separation matters because current payloads sometimes overload `status` with action words such as `steered` and `resumed`, which makes list, UI, and review behavior ambiguous.

### 5.2 Runtime Control Channel

The module should make control input work for actual runtime-owned delegated runs.

That means:

- a running delegated task can receive control input through runtime-owned state
- control input is not only stored for tests or custom direct spawns
- a delegated run can choose whether to consume control input immediately or at the next safe boundary
- stop remains a separate hard control path

The exact mechanism can be mailbox, queue, or session-scoped control state, but it must not remain a fake capability.

### 5.3 Resume

Resume should remain child-session-based, not replacement-task-based.

That means:

- resume reuses the original child session when continuation state is valid
- resume preserves task lineage and worktree metadata
- resume does not silently spawn an unrelated child session
- pending approval or unfinished continuation state must remain a hard gate

This is already the right direction in `ResumeSubagent`; the module should normalize and expose it more completely.

### 5.4 Background Contract

Background delegated tasks should have a stable output and notification contract.

Minimum required fields:

- task ID
- child session ID and key
- label or description
- lifecycle status
- output file path when persisted
- whether the caller can read that output

Minimum required control-plane behavior:

- launched tasks appear in inventory immediately
- terminal completion is observable without polling transcript internals
- failure preserves an error summary that is visible through status and events

### 5.5 Fork And Isolation Semantics

Fork behavior should stay explicit rather than incidental.

The backend contract should preserve:

- inherited parent context for forked delegated runs
- inherited or overridden tool pool semantics
- prohibition on recursive fork misuse where needed
- worktree notice and worktree path persistence for isolated runs

The goal is semantic alignment with Claude Code fork behavior, not byte-for-byte CLI parity.

### 5.6 Control Plane

`myclawd` should expose enough delegated-task state to make a future UI possible without backend redesign.

At minimum, that means:

- normalized `spawn_subagent` input that reaches runtime options already supported internally
- list and status payloads that include lifecycle metadata, not only IDs
- real control operations for stop, steer, resume, and wait or close
- stable events for task updates and task completion

Do not keep delegated-task truth split between gateway-only state and runtime-only state.

## 6. Explicit Design Choice For `agent.task`

The module should preserve a pragmatic hybrid backend contract.

Recommended behavior:

- control-plane task spawning is always async and task-ID-based
- `agent.task` may return an inline completed result for fast-finish cases
- `agent.task` should otherwise return a launched delegated-task payload and let the task continue in the background

This keeps the backend useful for current runtime flows while still moving toward Claude-style delegated-task semantics.

## 7. Non-Goals

This module should not:

- implement the React task UI
- fully replicate Claude Code team or swarm product features
- redesign every prompt and session concept in the runtime
- absorb Docker or DB domain work
- turn `myclaw` into a full remote bridge product

## 8. Acceptance Criteria

This design is correct if:

- delegated tasks become a stable runtime-owned lifecycle
- control actions work for real delegated runs
- background output and notification behavior is explicit
- child-session resume remains intentional and testable
- `myclawd` gains a real delegated-task control surface
- the module can be implemented by Claude Code without inventing architecture
