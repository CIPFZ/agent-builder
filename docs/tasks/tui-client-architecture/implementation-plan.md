# TUI Client Architecture Implementation Plan

Date: 2026-04-25

## 1. Implementation Strategy

This module should be implemented as a client-architecture task, not as a rendering-only task.

The correct strategy is:

1. lock client role and scope
2. lock transport/store boundary
3. lock screen/component boundary
4. then implement incrementally

## 2. Work Package A: TUI Role Lock

Required outcome:

- TUI scope is explicitly bounded
- TUI is positioned as a lightweight client
- React remains the rich product UI

## 3. Work Package B: Technology Stack Lock

Required outcome:

- `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` are explicitly chosen
- legacy `github.com/charmbracelet/*` imports are treated as migration targets
- raw-TTY-heavy direction is explicitly rejected as the long-term architecture

## 4. Work Package C: Transport And Store Design

Required outcome:

- `myclawd` websocket/control-plane boundary is explicit
- client transport model is explicit
- store/state model is explicit

## 5. Work Package D: Screen Model

Required outcome:

- identify initial terminal views
- define focus/navigation model
- define what remains intentionally out of scope

## 6. Work Package E: Incremental Delivery Roadmap

Required outcome:

- phase 1 minimal TUI
- phase 2 validation-friendly TUI
- phase 3 richer but still bounded operator TUI

## 7. Completion Standard

The architecture work is complete when:

- TUI direction is explicit
- implementation can proceed without re-litigating the stack
- the roadmap is consistent with `myclawd + React UI`
