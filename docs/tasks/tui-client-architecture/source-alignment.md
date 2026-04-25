# TUI Client Architecture Source Alignment

Date: 2026-04-25

## 1. Alignment Purpose

This file explains how the `myclaw` TUI should stay aligned with Claude Code semantics without trying to copy Claude Code's exact frontend stack.

The goal is semantic alignment, not rendering parity.

## 2. What Must Align

These things must align with Claude Code direction:

- runtime semantics stay in the backend
- client surfaces consume shared control-plane behavior
- tool lifecycle and approval semantics are shared
- subagent/task state is client-observable through one contract

## 3. What Does Not Need To Align Literally

These do not need literal parity:

- React/Ink implementation details
- exact component tree
- exact TUI rendering behavior
- exact terminal library choice

## 4. Claude Code Semantic References

Use these source areas as semantic references:

- `claude-code/src/QueryEngine.ts`
- `claude-code/src/Tool.ts`
- client/control-plane interaction patterns around task, approval, and tool visibility
- general product split between runtime behavior and UI rendering concerns

## 5. Semantic Rules To Preserve

### A. Backend Owns Semantics

For `myclaw`, preserve:

- runtime semantics stay in Go backend layers
- TUI consumes them through `myclawd`

### B. Client-Neutral Contract

For `myclaw`, preserve:

- TUI and React UI should be able to consume the same protocol
- terminal-specific hacks must not become backend truth

### C. Terminal Is A Client, Not The Product Center

For `myclaw`, preserve:

- TUI remains useful and supported
- but it does not drive architecture away from the chosen control-plane model

## 6. Review Standard

Reject TUI changes that:

- bypass `myclawd`
- depend on runtime internals unavailable to React
- make raw TTY behavior the architectural center
- attempt full Claude Code TUI parity as a product requirement
