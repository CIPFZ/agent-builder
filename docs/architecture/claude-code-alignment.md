# Claude Code Alignment Strategy

## Purpose

The project is still Claude Code-aligned, but alignment now means:

- preserve Claude Code runtime semantics where they matter
- do not force a 1:1 copy of the UI implementation technology
- use Claude source modules as the reference when designing each backend capability

This is a semantic replication strategy, not a rendering replication strategy.

## What Must Stay Aligned

These areas remain primary Claude Code alignment targets:

- QueryEngine loop
- tool execution contract
- ToolUseContext-like runtime context
- permission and approval semantics
- session persistence and continuation
- subagent and task lifecycle
- worktree and isolation semantics
- MCP and skills integration
- host/control-plane protocol shape

## What Does Not Need 1:1 Implementation

These areas do not need 1:1 implementation technology:

- Ink component tree
- React terminal rendering details
- Go TUI visual structure
- front-end interaction widgets

For UI, we replicate operator workflows and state semantics, not exact rendering code.

## Source Mapping By Program Area

### Runtime Core

Claude source to study first:

- `src/QueryEngine.ts`
- `src/query.ts`
- `src/Tool.ts`
- `src/tools/*`
- `src/utils/permissions/*`
- `src/utils/processUserInput/*`
- `src/utils/systemPrompt.ts`
- `src/utils/claudemd.ts`
- `src/context.ts`

Go target areas:

- `internal/queryengine`
- `internal/runtime`
- `internal/tools`
- `internal/permissions`
- `internal/prompt`
- `internal/workspace`

### Control Plane

Claude source to study first:

- `src/cli/structuredIO.ts`
- `src/cli/remoteIO.ts`
- `src/cli/transports/*`
- `src/bridge/*`
- `src/remote/*`
- `src/server/*`

Go target areas:

- `internal/gateway`
- `internal/protocol/ws`
- `cmd/myclawd`

### Agent Control

Claude source to study first:

- `src/tools/AgentTool/*`
- `src/Task.ts`
- `src/tasks/*`
- `src/utils/forkedAgent.ts`
- `src/utils/worktree.ts`
- `src/utils/swarm/*`

Go target areas:

- `internal/agent`
- `internal/runtime`
- `internal/orchestration`
- future task and isolation layers

### Frontend Operator UI

Claude source to study first:

- `src/components/*`
- `src/screens/*`
- `src/commands/*`
- `src/keybindings/*`
- `src/ink/*`

Target interpretation:

- replicate operator workflows
- replicate information architecture
- replicate state transitions and control affordances
- do not force Go to reproduce React/Ink implementation details

## Alignment Rule For Each New Capability

Before implementing any major capability:

1. identify the Claude source module that owns the semantics
2. identify the minimum Go subsystem that should own the equivalent behavior
3. write the Go design so the abstraction boundary matches the Claude source as closely as practical
4. only then implement

## Current Alignment Priority

The current alignment priority is no longer "TUI parity first".

The priority is:

1. runtime core semantics
2. execution surface semantics
3. control-plane semantics
4. operator UI semantics

## Red Flags

If any of these happen, the project is drifting away from Claude-aligned architecture:

- frontend starts owning approval or task lifecycle rules
- runtime behavior is exposed differently to CLI and React
- new tool classes are added without checking corresponding Claude source
- Go TUI work blocks runtime and control-plane work

