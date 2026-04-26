# P1 Runtime Maturation Roadmap Task

Date: 2026-04-26

## 1. Task Purpose

This is the single execution document for planning P1 runtime maturation work.

P1 builds on P0. It should not re-open P0 questions unless implementation proves a P0 contract is incorrect.

This task does not implement runtime code.

## 2. Required Reading

Before creating or executing P1 implementation tasks, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/claude-code-go-parity-semantic-review.md`
3. `docs/tasks/task-doc-standard.md`
4. `docs/tasks/p0-runtime-parity-roadmap/task.md`
5. `docs/tasks/p1-runtime-maturation-roadmap/task.md`
6. `docs/tasks/p1-runtime-maturation-roadmap/design.md`
7. `docs/tasks/p1-runtime-maturation-roadmap/source-alignment.md`
8. `docs/tasks/p1-runtime-maturation-roadmap/implementation-plan.md`
9. `docs/tasks/p1-runtime-maturation-roadmap/test-validation-plan.md`
10. `docs/tasks/p1-runtime-maturation-roadmap/review-checklist.md`

## 3. Objective

Define the P1 sequence that makes `myclaw` durable across long sessions, recoverable client state, background task execution, and extension lifecycle.

P1 is successful when:

- runtime state can survive client reconnects and process restarts
- long-session context can be cached, projected, compacted, and recovered with clear rules
- subagents and background tasks support stronger lifecycle and isolation semantics
- MCP, skills, plugin-like commands, and future LSP support share one extension foundation

## 4. P1 Entry Criteria

P1 should start only after these P0 contracts exist:

- core tool identity and result semantics are stable
- slash commands are registered through shared runtime-owned metadata
- session recovery has a baseline contract
- runtime events have stable names and payload expectations

If one of these is missing, create a blocking note in the relevant P1 child task and do not work around it with client-specific shortcuts.

## 5. P1 Workstreams

P1 is split into four ordered workstreams.

### P1.1 AppState And Session Continuity

Goal:

- make UI/client state, session state, approvals, tasks, and runtime state recoverable as one coherent continuation model.

Required semantics:

- persisted runtime state snapshot
- client reconnect behavior
- pending approval restoration
- visible task/subagent state restoration
- active command/tool state visibility
- compatibility with TUI and `myclawd`

### P1.2 Context Cache And Memory Depth

Goal:

- improve long-session context quality beyond basic `CLAUDE.md` loading and memory injection.

Required semantics:

- read-file state
- context cache
- projected history view
- history snip and replay rules
- memory taxonomy
- drift prevention rules
- deterministic context rebuild after recovery

### P1.3 Subagent Task Isolation

Goal:

- make background agents and delegated tasks reliable enough for multi-step project workflows.

Required semantics:

- background task lifecycle
- output file contract
- task foreground/background transitions
- retry/resume/continue controls
- worktree isolation
- cwd override
- remote isolation boundary, documented even if not fully implemented
- allowed-tools and permission inheritance

### P1.4 Extension Platform Foundation

Goal:

- turn MCP, skills, plugin-like commands, and future LSP into one coherent extension surface.

Required semantics:

- extension inventory
- extension discovery lifecycle
- dynamic command and tool registration
- allowed tools and permission rules
- skill frontmatter expansion
- MCP server lifecycle
- LSP boundary design
- plugin marketplace explicitly deferred to P2/P3

## 6. Explicit Non-Goals

Do not include these in P1:

- React operator UI implementation
- visual parity with Claude Code React Ink UI
- full telemetry/GrowthBook clone
- enterprise managed settings
- full bridge/remote/trusted-device implementation
- plugin marketplace implementation
- full LSP feature set
- Docker/database execution surface unless needed by subagent isolation validation

## 7. Required Implementation Order

Implement P1 in this order:

1. AppState And Session Continuity
2. Context Cache And Memory Depth
3. Subagent Task Isolation
4. Extension Platform Foundation

Reason:

- session continuity is the storage and recovery base
- advanced context depends on stable recovery and transcript semantics
- subagent task isolation depends on stable state and context inheritance
- extension lifecycle depends on stable runtime inventory and task/session ownership

## 8. Required Downstream Task Folders

Before implementation starts, create these task folders:

- `docs/tasks/p1-appstate-session-continuity/`
- `docs/tasks/p1-context-cache-memory-depth/`
- `docs/tasks/p1-subagent-task-isolation/`
- `docs/tasks/p1-extension-platform-foundation/`

Each folder must follow `docs/tasks/task-doc-standard.md`.

## 9. Validation Requirements

P1 cannot be considered complete unless:

- each P1 workstream has focused tests
- `go test ./...` passes
- restart and reconnect scenarios are tested
- a delegated background task can be recovered and inspected
- context rebuild after recovery is deterministic
- extension inventory can be rebuilt after reconnect or process restart

## 10. Completion Output Requirements

When P1 planning is complete, output:

- created roadmap files
- the four downstream implementation task names
- recommended first implementation task
- validation status
- unresolved planning risks

## 11. Start Prompt For Implementation Planning

Use this prompt to create the first downstream implementation task:

```text
You are creating the P1 AppState And Session Continuity implementation task.

Before writing implementation docs, read:
1. docs/execution/implementation-rules.md
2. docs/claude-code-go-parity-semantic-review.md
3. docs/tasks/p0-runtime-parity-roadmap/task.md
4. docs/tasks/p1-runtime-maturation-roadmap/task.md
5. docs/tasks/p1-runtime-maturation-roadmap/design.md
6. docs/tasks/p1-runtime-maturation-roadmap/source-alignment.md
7. docs/tasks/task-doc-standard.md

Create docs/tasks/p1-appstate-session-continuity/ using the standard task folder shape.
Do not implement code yet.
Focus on persistent runtime state, client reconnect, pending approval recovery, task/subagent visibility, and TUI/myclawd compatibility.
```

