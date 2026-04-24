# myclaw Docs

## Current Goal

`myclaw` is no longer tracking "full Claude Code UI/TUI 1:1 parity" as the primary execution target.

The active target is:

- keep the Go runtime aligned with Claude Code core runtime semantics
- build `myclawd` into the control plane
- use a dedicated React operator UI for complex product interaction
- prioritize the capability set required to let `myclaw` fully control a real software project

## Read In This Order

1. `architecture/runtime-target-architecture.md`
2. `architecture/frontend-backend-boundary.md`
3. `architecture/claude-code-alignment.md`
4. `review/current-capability-gap-review.md`
5. `roadmap/implementation-phases.md`
6. `roadmap/priority-backlog.md`
7. `roadmap/next-phase-plan.md`
8. `execution/implementation-rules.md`
9. `tasks/execution-surface-program/design.md`
10. `tasks/ssh-runtime-capability/design.md`
11. `tasks/mcp-runtime-capability/design.md`
12. `tasks/subagent-runtime-capability/design.md`

## Current Program Status

- runtime core is already usable for local code inspection, file modification, tool calling, MCP, skills, and basic subagent delegation
- the main gap is no longer "basic agent loop"
- the main gap is the execution and control surface around the runtime:
  - ssh
  - docker
  - database operations
  - stronger subagent/task lifecycle
  - `myclawd` control-plane protocol
  - React operator UI

## What We Are Not Optimizing For Right Now

- Go TUI visual 1:1 parity with Claude Code
- React/Ink rendering parity
- preserving historical roadmap documents that no longer drive execution

## Documentation Rule

Only keep documents that still have direct execution value for the current architecture and plan.
