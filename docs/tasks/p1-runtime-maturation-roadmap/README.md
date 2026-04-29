# P1 Runtime Maturation Roadmap

Date: 2026-04-26

## Purpose

This folder defines the P1 roadmap after P0 runtime parity foundations are in place.

P1 turns the runtime from a stable core into a durable multi-session, multi-agent, extensible runtime that can support serious project workflows.

This is a planning and sequencing artifact, not a direct implementation module.

## Read Order

1. `task.md`
2. `design.md`
3. `source-alignment.md`
4. `implementation-plan.md`
5. `test-validation-plan.md`
6. `review-checklist.md`

## Roadmap Outcome

P1 implementation proceeded through separate task folders for:

1. `p1-appstate-session-continuity`
2. `p1-context-cache-memory-depth`
3. `p1-subagent-task-isolation`
4. `p1-extension-platform-foundation`

The companion review checklists for P1.1-P1.4 record the implemented capabilities and their validation gates.

## Closure And Handoff

P1.5 is tracked in:

- `docs/tasks/p1-runtime-maturation-closure/`

P1.5 is the closure and P2 readiness gate. It verifies that P1.1-P1.4 remain coherent across command registry, session/appstate recovery, context cache, subagent isolation, extension inventory, gateway projection, and TUI command behavior.

P2 planning is tracked in:

- `docs/tasks/p2-runtime-expansion-roadmap/`

P2 inherits P0/P1 stable contracts and plans the next expansion work without claiming React UI, plugin marketplace, full LSP, remote trusted-device, or enterprise policy features as P1-complete.

