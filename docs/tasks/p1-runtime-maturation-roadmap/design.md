# P1 Runtime Maturation Roadmap Design

Date: 2026-04-26

## 1. Design Goal

P1 should make `myclaw` durable and operationally reliable after P0 establishes behaviorally stable runtime contracts.

P1 does not chase full Claude Code product parity. It matures the runtime capabilities that make long-running engineering workflows viable.

## 2. Design Principle

P1 uses this ordering:

```text
recoverable state -> deeper context -> isolated work delegation -> extension lifecycle
```

This means:

- continuity first
- context depth second
- subagent isolation third
- extension platform fourth

## 3. Workstream Boundaries

## P1.1 AppState And Session Continuity

Ownership:

- `internal/session`
- `internal/store`
- `internal/runtime`
- `internal/queryengine`
- `internal/approval`
- `internal/agent`
- `internal/tui`
- `internal/gateway`

Minimum contracts:

- runtime state snapshots have stable fields
- pending approvals can be restored
- active and completed tasks are visible after recovery
- TUI and gateway read state through shared runtime paths
- no client owns persistence semantics alone

## P1.2 Context Cache And Memory Depth

Ownership:

- `internal/workspace`
- `internal/prompt`
- `internal/memory`
- `internal/model`
- `internal/session`
- `internal/runtime`
- `internal/queryengine`

Minimum contracts:

- read-file state is separate from memory
- context cache has explicit invalidation rules
- projected history and replay have stable boundaries
- compaction memory save is deterministic
- recovered context can be rebuilt without hidden UI state

## P1.3 Subagent Task Isolation

Ownership:

- `internal/agent`
- `internal/tools/agent_tool.go`
- `internal/runtime`
- `internal/session`
- `internal/permissions`
- `internal/tui`
- `internal/gateway`

Minimum contracts:

- background task state is persistent
- task output is inspectable
- task continuation and retry are explicit actions
- worktree isolation has deterministic lifecycle
- permission inheritance is safer than parent by default
- allowed tools are enforced at runtime

## P1.4 Extension Platform Foundation

Ownership:

- `internal/tools`
- `internal/tools/mcp_*`
- `internal/tools/skill_*`
- `internal/config`
- `internal/runtime`
- `internal/queryengine`
- `internal/gateway`
- possible new package: `internal/extensions`

Minimum contracts:

- extensions have inventory records
- dynamic tools and commands register through shared runtime paths
- MCP lifecycle is part of the extension lifecycle
- skill frontmatter can express allowed tools, hooks, context, and agent metadata
- LSP is designed as an extension boundary, not a one-off tool

## 4. Dependency Model

P1.1 must happen first because the other workstreams depend on stable persistence and recovery.

P1.2 depends on P1.1 for deterministic state reconstruction.

P1.3 depends on P1.1 for durable task state and P1.2 for child context inheritance.

P1.4 should happen after P1.1 because extension inventory must survive reconnects and restarts.

P1.3 and P1.4 can proceed in parallel only after P1.1 is complete and their write scopes are separated.

## 5. Acceptance Model

P1 is accepted only when the system can execute this representative scenario:

1. start a session with workspace context
2. load memory and read-file state
3. invoke a skill or MCP-derived capability
4. start a background subagent with constrained tools
5. persist state while the task is active
6. restart or reconnect
7. inspect the task and extension inventory
8. resume or continue the task
9. rebuild context deterministically
10. verify TUI and gateway observe the same state

## 6. Documentation Model

This roadmap folder is a parent planning artifact.

Each child workstream must create:

- `README.md`
- `task.md`
- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

Implementation should not begin from this roadmap alone.

