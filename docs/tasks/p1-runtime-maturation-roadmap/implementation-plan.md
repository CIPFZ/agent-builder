# P1 Runtime Maturation Roadmap Implementation Plan

Date: 2026-04-26

## 1. Planning Goal

Create an executable sequence for P1 runtime maturation work.

This plan defines the order and acceptance gates for downstream P1 implementation task folders.

## 2. Phase Breakdown

## Phase 0: Confirm P0 Readiness

### Objective

Verify P1 has stable foundations to build on.

### Required Work

- read `docs/tasks/p0-runtime-parity-roadmap/review-checklist.md`
- confirm tool identity and result semantics are stable
- confirm command registry exists or has an accepted contract
- confirm baseline recovery exists
- confirm runtime event names and payloads are stable enough to consume

### Acceptance

Any missing P0 dependency is recorded in the first affected P1 child task. Do not silently work around missing foundations.

## Phase 1: Create Child Task Folders

### Objective

Turn this roadmap into four executable task folders.

### Required Work

Create:

- `docs/tasks/p1-appstate-session-continuity/`
- `docs/tasks/p1-context-cache-memory-depth/`
- `docs/tasks/p1-subagent-task-isolation/`
- `docs/tasks/p1-extension-platform-foundation/`

Each folder must contain:

- `README.md`
- `task.md`
- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

### Acceptance

Each task folder can be handed to an implementation agent without using this roadmap as the only source of truth.

## Phase 2: P1 AppState And Session Continuity

### Objective

Make session, runtime, approval, task, and client-visible state recoverable as one continuation model.

### Target Files

- `internal/session/manager.go`
- `internal/session/recovery.go`
- `internal/store/file/session_store.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/approval/manager.go`
- `internal/agent/manager.go`
- `internal/tui/state.go`
- `internal/gateway/server.go`

### Required Work

- define runtime state snapshot schema
- persist and recover pending approvals
- persist and recover task/subagent state metadata
- define reconnect behavior for TUI and gateway clients
- expose recovered state through shared runtime paths

### Acceptance

- restart recovery tests pass
- reconnect state tests pass
- pending approvals survive recovery
- visible task/subagent state survives recovery
- TUI and gateway observe consistent state

## Phase 3: P1 Context Cache And Memory Depth

### Objective

Improve long-session context quality and deterministic rebuild behavior.

### Target Files

- `internal/workspace/loader.go`
- `internal/prompt/builder.go`
- `internal/memory/service.go`
- `internal/model/claude_transcript.go`
- `internal/session/recovery.go`
- `internal/runtime/session_compaction.go`
- `internal/queryengine/context_provider.go`
- `internal/queryengine/queryengine.go`

### Required Work

- define read-file state schema
- define context cache schema and invalidation rules
- define projected history and replay boundaries
- improve compaction memory save and recovery behavior
- test deterministic context rebuild after recovery

### Acceptance

- read-file state tests pass
- context cache invalidation tests pass
- projected history tests pass
- compaction recovery tests pass
- recovered context rebuild is deterministic

## Phase 4: P1 Subagent Task Isolation

### Objective

Make delegated and background work reliable, inspectable, and safely isolated.

### Target Files

- `internal/agent/manager.go`
- `internal/tools/agent_tool.go`
- `internal/agents/loader.go`
- `internal/runtime/worktree.go`
- `internal/runtime/runner.go`
- `internal/session/manager.go`
- `internal/permissions/policy.go`
- `internal/tui/tasks.go`
- `internal/gateway/server.go`

### Required Work

- persist background task lifecycle
- define task output file contract
- add foreground/background transition semantics
- implement retry/resume/continue control contracts
- harden worktree and cwd isolation
- enforce allowed-tools and safer permission inheritance

### Acceptance

- background task lifecycle tests pass
- task recovery tests pass
- worktree isolation tests pass
- allowed-tools tests pass
- gateway task control tests pass

## Phase 5: P1 Extension Platform Foundation

### Objective

Create a coherent extension foundation for MCP, skills, plugin-like commands, and future LSP work.

### Target Files

- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`
- `internal/tools/mcp_oauth.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/skill_frontmatter.go`
- `internal/tools/bundled_skills.go`
- `internal/tools/registry.go`
- `internal/config/config.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`
- create if needed: `internal/extensions/*`

### Required Work

- define extension inventory schema
- unify MCP and skill discovery records
- support dynamic command and tool registration contracts
- expand skill frontmatter contract for allowed tools, hooks, context, and agent metadata
- define LSP boundary without implementing full LSP feature set
- expose extension inventory through shared runtime and gateway paths

### Acceptance

- extension inventory tests pass
- MCP/skill discovery tests pass
- dynamic registration tests pass
- permission and allowed-tools tests pass
- gateway inventory tests pass

## 3. Recommended First Task

Start with `p1-appstate-session-continuity`.

Reason:

- all other P1 work depends on recoverable runtime state
- context cache, task isolation, and extension inventory all need persistence and reconnect semantics
- it reduces the risk of building P1 features that only work in a single live process

## 4. Commit Strategy

Use one branch per P1 implementation workstream.

Suggested branches:

- `codex/p1-appstate-session-continuity`
- `codex/p1-context-cache-memory-depth`
- `codex/p1-subagent-task-isolation`
- `codex/p1-extension-platform-foundation`

Each workstream should commit docs first, then tests, then implementation.

## 5. Definition Of Done For P1

P1 is complete when:

- all four child workstreams are implemented
- all child review checklists pass
- `go test ./...` passes
- restart, reconnect, context rebuild, background task, and extension inventory scenarios pass
- remaining gaps are explicitly moved to P2/P3

