# Next Phase Plan

## Active Phase

Phase 2: Execution Surface

## Why This Phase Is Next

The project already has enough runtime core to justify moving outward.

The primary blocker is no longer:

- basic QueryEngine
- basic file editing
- MCP basics
- skills basics

The blocker is that `myclaw` still cannot reliably control the full environment around a real project.

## This Phase Outcome

At the end of this phase, `myclaw` should be able to act as a real operator for a Go web project:

- modify source code
- run project commands
- access remote hosts through ssh
- control Docker-based services
- perform database operations
- expose all of that through stable runtime and control-plane semantics

## Workstreams

### Workstream A: Shell Hardening

Deliverables:

- shell execution contract review against Claude source semantics
- improved permission classification and approval behavior
- clearer background execution behavior
- better progress and result payloads

### Workstream B: ssh Tool

Deliverables:

- first-class ssh tool contract
- secure input schema
- session-aware progress and output
- approval and policy integration

### Workstream C: Docker Tooling Strategy

Deliverables:

- choose first-class tool vs runtime facade design
- define minimum Docker operations for target workflows
- implement initial runtime support

### Workstream D: Database Tooling Strategy

Deliverables:

- choose direct SQL vs command-wrapper approach
- define minimum db operations for target workflows
- implement initial runtime support

### Workstream E: myclawd Protocol Support

Deliverables:

- event shapes for execution tools
- approval and progress support for new execution surfaces
- client-neutral request/response contracts

## Exit Criteria

- `myclaw` can control a local Go web project end-to-end
- `myclaw` can control a remote or containerized project environment through approved execution tools
- the same backend contracts are usable from light TUI and future React UI

## Not In This Phase

- React operator UI implementation
- Go TUI parity growth
- broad plugin platform work
- broad LSP work

