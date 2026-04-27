# TUI Client Architecture Task

Date: 2026-04-25

## 1. Task Purpose

This is the single execution document for Claude Code to define and implement the `myclaw` TUI client architecture.

Claude Code should use this file as the primary entrypoint.

This file consolidates:

- scope
- non-goals
- architecture target
- technology direction
- reference review outcome
- implementation order
- validation requirements

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/tui-client-architecture/task.md`
3. `docs/tasks/tui-client-architecture/design.md`
4. `docs/tasks/tui-client-architecture/source-alignment.md`
5. `docs/tasks/tui-client-architecture/implementation-plan.md`
6. `docs/tasks/tui-client-architecture/test-validation-plan.md`
7. `docs/tasks/tui-client-architecture/review-checklist.md`
8. `docs/roadmap/tui/roadmap.md`

After reading, output a short execution summary before coding.

That summary must include:

- TUI objective
- explicit non-goals
- chosen terminal UI stack
- `myclawd` protocol boundary
- planned implementation order
- planned validation steps

## 3. Objective

Define and implement a lightweight TUI client that uses `myclawd` as the backend control plane and provides terminal-based access to core operator workflows without trying to fully recreate Claude Code's TUI product layer.

## 4. Current Reality

The project has already chosen the long-term product direction:

- Go runtime core
- `myclawd` control plane
- React operator UI

At the same time, a terminal client is still required for:

- function validation
- local operator use
- debugging
- lightweight terminal workflows

The current TUI direction is problematic because it depends too much on raw TTY behavior and terminal environment assumptions.

## 5. In Scope

- choose the TUI technology stack
- define TUI/client architecture boundaries
- define transport and store boundaries
- define what the TUI should and should not implement
- review and document whether external reference code can be reused
- produce an implementation roadmap for the TUI

## 6. Out Of Scope

Do not implement any of the following in this task:

- full Claude Code TUI parity
- TTY-driven runtime internals
- React UI implementation
- terminal-first backend architecture
- replacing `myclawd` with direct runtime calls

## 7. Architecture Constraints

- TUI must be a client of `myclawd`, not a second runtime path.
- TUI must not depend on runtime internals differently from the future React UI.
- TUI must not rely on fragile raw TTY assumptions as its core interaction model.
- TUI must remain lightweight; complex product workflows belong in React UI.
- The backend contract must stay client-neutral.

## 8. External Reference Review Outcome

This module may reference:

- `https://github.com/kylesnowschwartz/tail-claude`
- `https://github.com/armatrix/claude-agent-sdk-go`

But the architecture must treat them correctly:

- `tail-claude` is useful mainly as a Bubble Tea terminal UI reference and interaction pattern source
- it is not a direct `myclawd` terminal client architecture
- `claude-agent-sdk-go` is more relevant for transport/client abstraction ideas than for TUI rendering

Direct copy is only acceptable for tightly bounded UI/component techniques after review.
Do not import either project's architecture wholesale.

## 9. Required Module Behaviors

The resulting TUI direction must provide:

1. a stable terminal UI technology choice
2. a `myclawd`-only backend boundary
3. a clear client transport/store/render split
4. a constrained TUI feature surface
5. a roadmap that keeps React as the rich UI target

## 10. Required TUI Scope

The TUI should support:

- session connect/bind
- prompt input
- message stream view
- tool progress visibility
- approval handling
- subagent/task summary visibility
- basic runtime inventory visibility

The TUI should not try to own:

- rich diff UX
- complex structured operation pages
- advanced admin dashboards
- heavy multi-panel product workflows better suited to React

## 11. Required Technology Decision

This task must explicitly decide the terminal UI stack.

Recommended direction:

- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`

This stack should be justified against:

- portability
- maintainability
- compatibility with `myclawd` event-driven architecture
- lower dependence on terminal-specific quirks than the current TTY-heavy path

The package paths are part of the decision. New TUI work should target the
Charmbracelet v2 module paths instead of the older `github.com/charmbracelet/*`
imports.

## 12. Required Implementation Order

Implement in this order:

1. lock TUI role and scope
2. lock terminal UI stack
3. define `myclawd` transport boundary
4. define client-side state/store model
5. define screen/component model
6. define roadmap for incremental delivery

## 13. Validation Requirements

At minimum, validate:

- TUI does not require direct runtime internals
- TUI state comes from `myclawd`
- architecture works across common terminal environments at a design level
- chosen stack supports required lightweight workflows

## 14. Start Prompt For Claude Code

Use the following prompt to start work:

```text
You are defining the myclaw TUI client architecture.

Before coding, read these files in order:
1. docs/execution/implementation-rules.md
2. docs/tasks/tui-client-architecture/task.md
3. docs/tasks/tui-client-architecture/design.md
4. docs/tasks/tui-client-architecture/source-alignment.md
5. docs/tasks/tui-client-architecture/implementation-plan.md
6. docs/tasks/tui-client-architecture/test-validation-plan.md
7. docs/tasks/tui-client-architecture/review-checklist.md
8. docs/roadmap/tui/roadmap.md

After reading, output a short execution summary covering:
- TUI objective
- non-goals
- chosen TUI stack
- myclawd protocol boundary
- implementation order
- validation plan

Then implement within the documented scope.

Do not turn the TUI into a second runtime path.
Do not build raw-TTY-dependent architecture as the long-term direction.
Keep React as the rich product UI and TUI as the lightweight terminal client.
```
