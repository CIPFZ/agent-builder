# TUI Client Architecture Review Checklist

Date: 2026-04-25

Use this checklist when reviewing the TUI architecture and downstream implementation.

## 1. Scope Check

- Is TUI still clearly a lightweight client?
- Has the architecture avoided full Claude Code TUI parity as a requirement?

## 2. Backend Boundary Check

- Does TUI use `myclawd` as the only backend contract?
- Does TUI avoid private runtime bypasses?

## 3. Technology Check

- Is the Bubble Tea-based stack clearly justified?
- Has the raw TTY-heavy direction been rejected as the long-term path?

## 4. Store / State Check

- Is client-side state organized cleanly?
- Are transport and rendering concerns separated?

## 5. Product Boundary Check

- Are complex workflows left to React UI?
- Is TUI still sufficient for validation and terminal operations?

## 6. Merge Bar

Do not approve if:

- TUI is still architecturally dependent on raw terminal quirks
- TUI bypasses `myclawd`
- TUI scope is too broad and conflicts with the React direction
