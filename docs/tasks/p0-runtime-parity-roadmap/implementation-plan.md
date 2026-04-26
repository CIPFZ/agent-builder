# P0 Runtime Parity Roadmap Implementation Plan

Date: 2026-04-26

## 1. Planning Goal

Create an executable sequence for P0 runtime parity work.

This plan defines the order and acceptance gates for downstream implementation task folders.

## 2. Phase Breakdown

## Phase 0: Create Child Task Folders

### Objective

Turn this roadmap into four executable task folders.

### Required Work

Create:

- `docs/tasks/p0-tool-parity-core/`
- `docs/tasks/p0-command-registry/`
- `docs/tasks/p0-context-memory-recovery/`
- `docs/tasks/p0-runtime-structured-events/`

Each folder must contain:

- `README.md`
- `task.md`
- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

### Acceptance

Each task folder can be handed to an implementation agent without reading this roadmap as the only source of truth.

## Phase 1: P0 Tool Parity Core

### Objective

Normalize the highest-impact model-callable tools.

### Target Files

- `internal/tools/registry.go`
- `internal/tools/filesystem_tools.go`
- `internal/tools/extended_tools.go`
- `internal/tools/agent_tool.go`
- `internal/tools/system/run.go`
- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/permissions/policy.go`
- `internal/approval/manager.go`

### Required Work

- define per-tool parity contracts
- write focused tests for shell/file/todo/agent/skill/MCP behavior
- harden tool identity and tool result behavior
- normalize observable input behavior
- normalize read-only/destructive classification
- ensure failures return model-consumable tool result semantics

### Acceptance

- focused tool tests pass
- tool lifecycle tests pass
- representative shell, file edit, todo, agent, skill, and MCP flows work through QueryEngine

## Phase 2: P0 Command Registry

### Objective

Introduce runtime-owned slash command registration and execution.

### Target Files

- `internal/tui/commands.go`
- `internal/tui/model.go`
- `internal/app/cli.go`
- `internal/queryengine/queryengine.go`
- create if needed: `internal/commands/registry.go`
- create if needed: `internal/commands/*`

### Required Work

- define command metadata
- define command execution result shape
- move command availability logic out of TUI-only paths
- implement initial command set
- route command results into session/runtime consistently

### Acceptance

- commands can be listed
- command visibility is test-covered
- `/permissions`, `/model`, `/memory`, `/resume`, `/compact`, `/tasks`, `/mcp`, and `/status` have explicit behavior
- TUI command handling delegates to shared command registry

## Phase 3: P0 Context, Memory, And Recovery

### Objective

Make long-session continuation reliable.

### Target Files

- `internal/workspace/loader.go`
- `internal/prompt/builder.go`
- `internal/memory/service.go`
- `internal/session/manager.go`
- `internal/session/recovery.go`
- `internal/model/claude_transcript.go`
- `internal/runtime/session_compaction.go`
- `internal/queryengine/queryengine.go`
- `internal/queryengine/context_provider.go`

### Required Work

- document and implement deterministic workspace instruction loading
- persist/recover pending approval state
- persist/recover invoked skill state
- persist/recover tool-use/tool-result identity
- define read-file state and context cache boundaries
- improve compaction boundary recovery

### Acceptance

- session recovery tests cover user messages, assistant messages, tool use, tool result, pending approval, compaction summary, and invoked skills
- recovered sessions can continue through QueryEngine
- context injection is deterministic and test-covered

## Phase 4: P0 Runtime Structured Events

### Objective

Stabilize runtime events for all clients.

### Target Files

- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`
- `internal/tui/runtime_bridge.go`
- `internal/tui/model.go`

### Required Work

- define event taxonomy
- define event payload schemas
- map QueryEngine events to runtime events consistently
- expose events through gateway without duplicating business semantics
- ensure TUI consumes shared event shapes where practical

### Acceptance

- event schema tests pass
- gateway event tests pass
- TUI/runtime bridge tests pass
- representative scenario emits stable events for session, command, tool, approval, compaction, and agent/task lifecycle

## 3. Recommended First Task

Start with `p0-tool-parity-core`.

Reason:

- tool behavior controls model action reliability
- command, recovery, and events depend on stable tool identity and result semantics
- current code already has a strong tool foundation, so this is the highest-value P0 work

## 4. Commit Strategy

Use one branch per P0 implementation workstream.

Suggested branches:

- `codex/p0-tool-parity-core`
- `codex/p0-command-registry`
- `codex/p0-context-memory-recovery`
- `codex/p0-runtime-structured-events`

Each workstream should commit docs first, then tests, then implementation.

## 5. Definition Of Done For P0

P0 is complete when:

- all four child workstreams are implemented
- all child review checklists pass
- `go test ./...` passes
- an end-to-end runtime parity scenario is documented and passing
- remaining gaps are explicitly moved to P1/P2

