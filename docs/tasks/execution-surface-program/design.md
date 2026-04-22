# Execution Surface Program Design

Date: 2026-04-22

## 1. Objective

Turn the current `myclaw` runtime from a "local code-editing agent runtime" into a "project control runtime" that can reliably operate a real Go web project and its environment.

This program is aligned to three inputs:

1. user requirement:
   - subagent
   - file add/delete/update
   - tool calling
   - MCP
   - skills
   - ssh
   - full runtime chaining
   - control of a Go web project including Docker and database operations
2. current architecture:
   - `Go Runtime Core`
   - `myclawd Control Plane`
   - future `React Operator UI`
3. Claude Code semantic reference:
   - runtime loop and tool contracts from `src/QueryEngine.ts`, `src/Tool.ts`, `src/tools/*`
   - control-plane and remote semantics from `src/cli/*`, `src/bridge/*`, `src/remote/*`, `src/server/*`

## 2. Program-Level Design Decision

The execution-surface program should be split into **four large modules**, not a long list of tiny tools.

Those four modules are:

1. `Execution Command Substrate`
2. `Remote Execution Surface`
3. `Environment Control Surface`
4. `Operator Control Protocol`

This is the correct decomposition because it matches both the actual product goal and the Claude Code architecture pattern:

- Claude Code does not treat execution as isolated helper scripts
- execution is a runtime contract tied to permissions, tool lifecycle, and transport/control surfaces

## 3. Program Modules

## Module A: Execution Command Substrate

### Purpose

Strengthen the existing shell execution path so it can serve as the backend foundation for all higher-level execution features.

### Why It Comes First

The codebase already has:

- `internal/tools/system/run.go`
- `internal/sandbox/router.go`

But this is still a simplified command runner. Before adding ssh/docker/db as product features, the command substrate needs stable semantics for:

- structured input
- approval integration
- progress lifecycle
- background/long-running behavior
- session/worktree awareness
- result shaping

### Claude Code Reference

- `src/tools/BashTool/*`
- `src/tools/PowerShellTool/*`
- `src/Tool.ts`
- `src/utils/permissions/*`

### Deliverables

- hardened shell command contract
- normalized progress and result payload model
- better policy/approval integration
- stable execution event model for `myclawd`

## Module B: Remote Execution Surface

### Purpose

Introduce first-class remote host control through ssh.

### Why This Is The First New Tooling Module

The target use case explicitly requires project control beyond the local filesystem.

ssh is the minimum viable remote control capability because it unlocks:

- remote command execution
- Docker control on remote hosts
- remote log inspection
- remote deployment workflows
- remote DB maintenance commands

### Claude Code Reference

There is no single `SshTool` in Claude Code, so alignment must be semantic rather than by filename.

Reference areas:

- `src/tools/BashTool/*`
- `src/bridge/*`
- `src/remote/*`
- `src/cli/transports/*`

The alignment target is:

- first-class runtime execution semantics
- approval-safe remote command invocation
- stable client-neutral event flow

### Deliverables

- first-class ssh runtime capability
- approval and policy integration
- progress and result streaming
- `myclawd` protocol support

## Module C: Environment Control Surface

### Purpose

Expose higher-level project environment operations as runtime capabilities instead of leaving them as ad hoc shell usage.

### Scope

- Docker control
- database operations

### Why This Is A Single Module

Docker and DB are related because they both represent "environment control around the project" rather than "core code editing".

If split too early, the project risks building two independent and inconsistent execution models.

### Claude Code Reference

These are not one-to-one module copies from Claude Code. The alignment target is instead:

- execution tools behave like first-class runtime tools
- permission and approval semantics remain centralized
- transport and progress semantics stay shared with the rest of the runtime

Reference areas:

- `src/Tool.ts`
- `src/tools/*`
- `src/QueryEngine.ts`

### Deliverables

- Docker control strategy
- DB control strategy
- first runtime-grade implementation for both

## Module D: Operator Control Protocol

### Purpose

Normalize everything above into `myclawd` so the light TUI and future React UI consume the same control plane.

### Why It Is Its Own Module

Without this layer, each new tool capability will be implemented twice:

- once in runtime
- once again in client-specific logic

That would directly violate the new architecture.

### Claude Code Reference

- `src/cli/structuredIO.ts`
- `src/cli/transports/*`
- `src/bridge/*`
- `src/remote/*`
- `src/server/*`

### Deliverables

- unified execution event shapes
- approval request/response protocol for execution tools
- task/progress/result payload model
- client-neutral transport contracts

## 4. Recommended Execution Order

The correct order is:

1. `Execution Command Substrate`
2. `Remote Execution Surface`
3. `Operator Control Protocol` updates for remote execution
4. `Environment Control Surface`

Reasoning:

- docker and db should not be designed on top of a weak command substrate
- ssh should be built before docker/db because it unlocks both local and remote operator workflows
- `myclawd` protocol work should evolve alongside ssh, not after all tools are done

## 5. Program Backlog

### Stage 1

- harden shell command substrate
- design and implement ssh runtime capability
- extend `myclawd` to stream execution events for ssh

### Stage 2

- choose Docker control model
- choose DB control model
- implement first stable runtime support

### Stage 3

- improve subagent usage of execution tools
- improve approval ergonomics for environment-control tasks
- prepare protocol for richer frontend surfaces

## 6. What Not To Split Yet

Do not prematurely split the work into:

- separate local ssh and remote ssh epics
- separate docker-start, docker-logs, docker-exec, docker-build projects
- separate queryengine progress, gateway progress, websocket payload projects

Those are implementation subproblems, not the right task granularity at the planning level.

## 7. Acceptance Criteria

This program design is correct if:

- each module is large enough to be worth a design document
- each module maps cleanly to both user need and runtime architecture
- each module can later produce a precise implementation plan
- the system evolves toward `runtime + control plane + operator UI`, not back toward Go TUI product lock-in

