# P0 Runtime Parity Roadmap Task

Date: 2026-04-26

## 1. Task Purpose

This is the single execution document for planning P0 runtime parity work.

The purpose is to convert the Claude Code parity review into an actionable P0 roadmap using the existing `docs/tasks` task-document model.

This task does not implement runtime code.

## 2. Required Reading

Before creating or executing P0 implementation tasks, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p0-runtime-parity-roadmap/task.md`
5. `docs/tasks/p0-runtime-parity-roadmap/design.md`
6. `docs/tasks/p0-runtime-parity-roadmap/source-alignment.md`
7. `docs/tasks/p0-runtime-parity-roadmap/implementation-plan.md`
8. `docs/tasks/p0-runtime-parity-roadmap/test-validation-plan.md`
9. `docs/tasks/p0-runtime-parity-roadmap/review-checklist.md`

## 3. Objective

Define the P0 sequence that makes `myclaw` behaviorally credible as a Claude Code-style runtime core.

P0 is successful when:

- core tools have explicit parity contracts and tests
- slash commands have a registry and execution contract
- context, memory, transcript, and recovery semantics are reliable enough for long sessions
- runtime events are stable enough for TUI, daemon, and future SDK/control-plane clients

## 4. Current Reality

`myclaw` already has:

- QueryEngine
- tool registry
- file tools
- shell tools
- permissions and approvals
- session store and recovery primitives
- compaction and memory primitives
- subagent manager and Agent tool
- MCP and Skill partial capabilities
- TUI and `myclawd` gateway

But the parity review identified that P0 gaps remain in:

- concrete tool behavior
- command and slash-command visibility
- long-session context/memory/recovery
- structured runtime event semantics

## 5. P0 Workstreams

P0 is split into four ordered workstreams.

### P0.1 Tool Parity Core

Goal:

- make the most important model-callable tools behave predictably and closer to Claude Code semantics.

Primary tools:

- `Bash`
- `PowerShell`
- `system.run`
- `Read`
- `Write`
- `Edit`
- `MultiEdit`
- `Glob`
- `Grep`
- `LS`
- `TodoWrite`
- `Agent`
- `Skill`
- MCP dynamic tools

### P0.2 Command Registry

Goal:

- introduce a shared slash-command registry instead of scattered UI/client command handling.

Initial command set:

- `/help`
- `/permissions`
- `/model`
- `/memory`
- `/resume`
- `/compact`
- `/tasks`
- `/mcp`
- `/status`

### P0.3 Context, Memory, And Recovery

Goal:

- make long-session behavior reliable enough for real project work.

Required semantics:

- `CLAUDE.md` and workspace instruction loading
- memory injection and compaction memory save
- transcript-compatible message recovery
- pending approval recovery
- invoked skill recovery
- agent/task state recovery boundary
- read-file state and context cache design

### P0.4 Runtime Structured Events

Goal:

- make runtime events stable and client-neutral.

Required event families:

- session lifecycle
- message lifecycle
- tool lifecycle
- permission and approval lifecycle
- compaction lifecycle
- command lifecycle
- agent/task lifecycle
- MCP/skill inventory lifecycle

## 6. Explicit Non-Goals

Do not include these in P0:

- React operator UI
- visual parity with Claude Code React Ink UI
- full telemetry/GrowthBook clone
- enterprise managed settings
- complete bridge/remote/trusted-device stack
- plugin marketplace
- broad LSP implementation
- Docker/database execution surface unless required by a P0 tool contract

## 7. Required Implementation Order

Implement P0 in this order:

1. Tool Parity Core
2. Command Registry
3. Context, Memory, And Recovery
4. Runtime Structured Events

Reason:

- tools define the model's action surface
- commands define the user's direct control surface
- context/recovery defines long-session correctness
- structured events stabilize all clients after behavior is known

## 8. Required Downstream Task Folders

Before implementation starts, create these task folders:

- `docs/tasks/p0-tool-parity-core/`
- `docs/tasks/p0-command-registry/`
- `docs/tasks/p0-context-memory-recovery/`
- `docs/tasks/p0-runtime-structured-events/`

Each folder must follow `docs/tasks/task-doc-standard.md`.

## 9. Validation Requirements

P0 cannot be considered complete unless:

- each P0 workstream has focused tests
- `go test ./...` passes
- at least one end-to-end runtime scenario exercises tool calls, approval, memory/context, recovery, and event emission
- each implementation task updates its own review checklist
- each implementation task maps completed work back to Claude Code source semantics

## 10. Completion Output Requirements

When P0 planning is complete, output:

- created roadmap files
- the four downstream implementation task names
- the recommended first implementation task
- validation status
- unresolved planning risks

## 11. Start Prompt For Implementation Planning

Use this prompt to create the first downstream implementation task:

```text
You are creating the P0 Tool Parity Core implementation task.

Before writing implementation docs, read:
1. docs/execution/implementation-rules.md
2. docs/claude-code-go-parity-semantic-review.md
3. docs/tasks/p0-runtime-parity-roadmap/task.md
4. docs/tasks/p0-runtime-parity-roadmap/design.md
5. docs/tasks/p0-runtime-parity-roadmap/source-alignment.md
6. docs/tasks/task-doc-standard.md

Create docs/tasks/p0-tool-parity-core/ using the standard task folder shape.
Do not implement code yet.
Focus on Bash/PowerShell/system.run and Read/Write/Edit/MultiEdit first, then TodoWrite, Agent, Skill, and MCP dynamic tools.
```

