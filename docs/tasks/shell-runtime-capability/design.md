# Shell Runtime Capability Design

Date: 2026-04-25

## 1. Design Goal

Design shell execution as a reusable execution substrate for `myclaw`, not as a one-off command runner.

The design target is:

- stable runtime semantics
- clear permission behavior
- explicit lifecycle events
- reusable contracts for later execution modules

## 2. Why This Module Matters Now

The project is currently in the execution-surface phase.

That means the next priority is not more UI or another parity sweep. The next priority is turning the execution layer into a stable base for real project control.

Shell is the base layer for:

- local project command execution
- ssh alignment boundaries
- Docker control bootstrap
- database operation bootstrap
- operational scripts and tool wrappers

If shell remains underspecified, every downstream execution module will drift.

## 3. Current Design Gap

Today the codebase already has shell tools, but they still behave more like callable utilities than a fully normalized runtime capability.

The main gaps are:

- insufficiently explicit approval semantics
- incomplete documentation of runtime event lifecycle
- weakly documented result contract
- no explicit module-level guarantee for worktree/session-aware execution
- no one canonical implementation target for Claude Code to follow

## 4. Target Runtime Shape

The shell runtime capability should behave like this:

1. a shell tool invocation enters the shared tool lifecycle
2. runtime determines approval behavior from permission policy
3. execution starts within the correct session/worktree context
4. progress is emitted through shared runtime events when available
5. final result is emitted through shared tool result semantics
6. gateway forwards the same lifecycle to any client without tool-specific hacks

## 5. Contract Layers

### Tool Layer

The shell tool layer owns:

- input schema
- command normalization rules
- local execution dispatch
- structured result production where needed

### Runtime Layer

The runtime layer owns:

- permission policy evaluation
- session-aware execution context
- tool lifecycle events
- approval and result persistence behavior

### Gateway Layer

The gateway layer owns:

- websocket/event serialization
- client-neutral visibility of shell lifecycle state

The gateway must not invent shell semantics that the runtime does not own.

## 6. Permission Model Expectations

Shell execution is a high-sensitivity capability.

The module must make these boundaries explicit:

- read-only file tools are not shell
- shell execution must remain approval-aware
- permission decisions must not be inferred from client behavior
- worktree-local execution can still require approval depending on policy
- future ssh/docker/db surfaces should inherit shell-grade sensitivity expectations unless intentionally stricter

## 7. Result Model Expectations

The result model should support:

- clear success/failure outcome
- exit status visibility
- stdout/stderr or equivalent textual output
- structured result fields when already supported by runtime event surfaces
- stable payloads for future frontend rendering

The module should avoid returning only loosely formatted text when structured result data already exists on the shared runtime path.

## 8. Progress Model Expectations

Progress is not mandatory for every shell command, but the capability must support it where execution can expose intermediate state.

The important requirement is architectural:

- progress must flow through shared runtime hooks
- the gateway must forward it without client-specific logic
- downstream clients should be able to render progress uniformly

## 9. Worktree and Session Context

Shell execution must respect runtime session context.

At minimum, the module should define:

- how working directory is chosen
- how subagent worktrees alter execution root
- how path-sensitive execution behaves when a child session runs in an isolated worktree

This is required for later subagent + shell combinations to remain coherent.

## 10. Control-Plane Requirements

`myclawd` should expose shell lifecycle state through shared event semantics, not through a special shell dashboard contract.

The control plane should be able to surface:

- tool called
- tool progress
- tool result
- approval required
- approval resolved
- run error where appropriate

## 11. Downstream Value

Once this module is complete:

- ssh can inherit the same lifecycle model where appropriate
- Docker and database modules can reuse approval/progress/result semantics
- React UI work can consume one stable execution contract
- the Go TUI can stay thin without blocking runtime maturity
