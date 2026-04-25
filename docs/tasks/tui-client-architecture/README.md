# TUI Client Architecture

Date: 2026-04-25

## Purpose

This folder defines the terminal client architecture module for `myclaw`.

The module goal is not to recreate Claude Code's TUI rendering stack.

The goal is to build a lightweight, reliable terminal client that:

- uses `myclawd` as the only backend control plane
- keeps terminal usability for validation and operator workflows
- avoids hard dependence on fragile TTY-specific behavior
- stays aligned with the long-term `myclawd + React UI` architecture

## Read Order

1. `task.md`
2. `design.md`
3. `source-alignment.md`
4. `implementation-plan.md`
5. `test-validation-plan.md`
6. `review-checklist.md`

## Delivery Standard

This module is only ready for implementation when all of the following are true:

- the TUI role is explicitly constrained
- the technology stack is explicitly chosen
- the `myclawd` protocol boundary is explicit
- TTY-specific anti-patterns are explicitly rejected
- the roadmap is clear enough for downstream implementation

## Module Outcome

After this module lands, `myclaw` should have a clear TUI direction:

- TUI is a terminal client on top of `myclawd`
- React remains the rich product UI
- runtime semantics stay in Go backend layers
- TUI is stable enough for validation, debugging, and light operator control
