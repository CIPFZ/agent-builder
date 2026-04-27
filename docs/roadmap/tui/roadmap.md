# TUI Roadmap

Date: 2026-04-25

## Objective

Build a lightweight terminal client for `myclaw` that remains useful for validation and terminal operation while keeping `myclawd + React UI` as the long-term product architecture.

## Principles

- TUI is a client of `myclawd`
- TUI is lightweight
- TUI is not the source of truth for runtime semantics
- React remains the rich operator UI

## Phase 1: Transported Terminal Prototype

Goal:

- replace direct TTY-heavy assumptions with a `myclawd`-driven terminal client skeleton

Deliverables:

- Charmbracelet v2 application shell using `charm.land/bubbletea/v2`
- Lip Gloss v2 styling/layout using `charm.land/lipgloss/v2`
- `myclawd` websocket client
- basic session connect/bind
- simple message stream and prompt input

## Phase 2: Validation Console

Goal:

- make the TUI useful for everyday runtime validation

Deliverables:

- tool progress view
- approval handling
- subagent/task summary panel
- runtime inventory summary

## Phase 3: Stable Operator Terminal

Goal:

- provide a durable lightweight terminal experience without competing with React UI

Deliverables:

- better layout/focus handling
- reconnect/session recovery behavior
- keyboard navigation polish
- error and disconnected-state handling

## Explicitly Out Of Scope

- full Claude Code TUI parity
- rich multi-surface dashboards
- heavy diff UX
- advanced environment operation consoles
- UI behavior that should belong to React
