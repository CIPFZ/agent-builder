# Implementation Phases

## Phase Model

The program is organized around execution value, not around exhaustive source tree coverage.

## Phase 1: Runtime Core Hardening

Goal:

- make the existing core runtime stable enough to serve as the permanent backend foundation

Scope:

- QueryEngine consistency
- tool lifecycle consistency
- permission/approval consistency
- session/recovery consistency
- MCP and skills integration cleanup
- control-plane event normalization support

Exit criteria:

- runtime behavior is stable enough that terminal and web clients can consume one shared backend contract

## Phase 2: Execution Surface

Goal:

- let `myclaw` actually control a real software project environment

Scope:

- shell execution hardening
- ssh tool
- docker tool or docker control layer
- database operation tool or db control layer
- safer approval boundaries for external execution

Exit criteria:

- a Go web project can be controlled through code edits, shell, docker, and db operations without ad hoc client-side logic

## Phase 3: Agent Control

Goal:

- make subagents and tasks reliable enough for larger project workflows

Scope:

- task lifecycle contract
- subagent output and status semantics
- background execution behavior
- stronger resume and continuation
- future isolation/worktree improvements

Exit criteria:

- operator can delegate, inspect, continue, and manage multi-step execution flows through stable runtime semantics

## Phase 4: Control Plane Maturation

Goal:

- turn `myclawd` into the stable contract boundary for all clients

Scope:

- websocket/event schemas
- approval APIs
- task/subagent APIs
- runtime inventory APIs
- client-neutral protocol cleanup

Exit criteria:

- terminal and web UI use the same control-plane semantics

## Phase 5: React Operator UI

Goal:

- move complex operator workflows out of the Go TUI and into a richer frontend

Scope:

- conversation surface
- approval center
- task/subagent dashboard
- tool progress and detail views
- runtime inventory pages
- future docker/db/ssh operation views

Exit criteria:

- the Go TUI is no longer the only practical control surface for complex workflows

## Ongoing Alignment Work

Every phase must stay aligned with the Claude Code source modules that define the equivalent semantics.

That alignment work is continuous and does not sit in its own isolated phase.

