# TUI Client Architecture Design

Date: 2026-04-25

## 1. Design Goal

Design the TUI as a lightweight terminal client on top of `myclawd`.

The TUI is not the source of truth for runtime behavior.

The design target is:

- stable terminal usability
- low environment fragility
- shared backend contract with React UI
- bounded feature scope

## 2. Core Principle

`myclawd` is the control plane.

The TUI and the future React UI are both clients of that control plane.

That means:

- TUI must not call runtime internals through a private path
- TUI must not embed unique execution semantics
- TUI must not define its own event model

## 3. Why The Current Direction Is Weak

The current TTY-heavy approach is risky because it:

- depends on terminal-specific behavior
- creates portability problems across different terminal environments
- makes the TUI too close to runtime internals
- works against the chosen `myclawd + React UI` architecture

## 4. Recommended Stack

Recommended stack:

- `charm.land/bubbletea/v2` for terminal UI state/update loop
- `charm.land/lipgloss/v2` for styling, layout, text width, borders, padding, and color
- optional Charmbracelet component packages only where they reduce local complexity

Why:

- mature Go-native terminal stack
- good fit for message/event-driven UI
- far better architectural direction than raw terminal control logic
- easier to keep client concerns separate from backend concerns

The v2 package paths are the target architecture. The TUI should not keep
long-term dependencies on the older `github.com/charmbracelet/bubbletea` or
`github.com/charmbracelet/lipgloss` import paths.

## 5. External Reference Review

### `tail-claude`

What is useful:

- Bubble Tea terminal architecture patterns
- transcript/event rendering ideas
- list/detail view composition
- keyboard navigation patterns

What is not directly reusable as architecture:

- it is not built around `myclawd`
- it is closer to a Claude session viewer than a control-plane client

Conclusion:

- reuse techniques selectively
- do not copy its product architecture wholesale

### `claude-agent-sdk-go`

What is useful:

- client/transport abstraction ideas
- event stream handling patterns

What is not directly reusable:

- it is not primarily a terminal UI architecture

Conclusion:

- reference transport patterns
- do not treat it as a TUI base

## 6. Target TUI Architecture

The TUI should be split into four layers:

1. transport layer
2. store/state layer
3. screen/component layer
4. terminal rendering layer

### Transport Layer

Responsibilities:

- connect to `myclawd`
- send requests
- receive websocket/control-plane events
- handle reconnect/session binding behavior

### Store / State Layer

Responsibilities:

- session state
- message timeline
- tool progress
- approvals
- subagent/task summaries
- runtime inventory snapshot

This is the client-side source of truth for the TUI.

### Screen / Component Layer

Responsibilities:

- compose the terminal views
- map store state into panels and lists
- manage focus and key actions

### Rendering Layer

Responsibilities:

- terminal drawing
- resizing
- styling
- viewport management

## 7. TUI Feature Boundary

The TUI should own:

- prompt input
- message stream
- basic tool progress
- approval interaction
- session/task summary views
- runtime inventory summary

The TUI should not own:

- rich visual dashboards
- complex diff tooling
- advanced multi-step product workflows
- high-density structured operation UIs

Those belong to React UI.

## 8. Long-Term Relationship To React

The TUI is not a failed React replacement.

It is the lightweight terminal client for:

- debugging
- validation
- low-friction terminal usage

React remains the rich operator console.

## 9. Design Conclusion

The correct TUI direction is:

- client of `myclawd`
- Charmbracelet v2-based
- state/store-centered
- lightweight in scope
- not strongly coupled to raw TTY behavior
