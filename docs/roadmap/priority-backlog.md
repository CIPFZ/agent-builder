# Priority Backlog

## P0

### 1. ssh Runtime Capability

Why:

- required for real project control
- foundational for remote Docker and remote database workflows

Reference areas:

- Claude remote/control semantics in `src/bridge/*`, `src/remote/*`, and execution tool patterns
- Go targets in `internal/tools`, `internal/runtime`, `internal/gateway`

### 2. Shell Execution Hardening

Why:

- shell already exists but is not strong enough to be the default execution surface for project control

Needed work:

- stronger permission classification
- background task handling
- session/worktree awareness
- better runtime progress and result semantics

### 3. Docker Control Surface

Why:

- one of the primary target use cases

Decision path:

- decide whether Docker lands as:
  - first-class tool
  - shell-backed runtime facade
  - MCP-backed runtime facade

### 4. Database Control Surface

Why:

- needed for realistic project operation and maintenance tasks

Decision path:

- define whether to support:
  - SQL execution
  - migration commands
  - ORM-driven project commands
  - env-aware DB connection resolution

### 5. myclawd Contract Normalization

Why:

- React UI should not be built on unstable ad hoc protocol behavior

## P1

### 6. Task And Subagent Lifecycle Strengthening

- richer statuses
- output contract
- background semantics
- continue/retry control

### 7. Approval Contract Cleanup

- normalize approval event shapes
- normalize continue/reject APIs
- preserve rich decision context

### 8. Runtime Inventory APIs

- tools
- MCP servers
- skills
- active sessions
- tasks/subagents

### 9. Worktree / Isolation Improvement

- strengthen current child-session execution boundaries

## P2

### 10. React Operator UI Bootstrap

- conversation page
- approval page
- task/subagent page
- runtime inventory page

### 11. Rich Tool Visualization

- tool detail cards
- progress timeline
- expandable structured results

### 12. Extended Claude Alignment Work

- continue closing source-semantic gaps in task, runtime, and control-plane areas

## Explicitly Deprioritized

- Go TUI visual parity
- Go TUI feature expansion beyond light operator needs
- React/Ink implementation parity
- preserving unused historical roadmap work

