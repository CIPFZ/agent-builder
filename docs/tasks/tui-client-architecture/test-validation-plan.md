# TUI Client Architecture Test Validation Plan

Date: 2026-04-25

## 1. Validation Goal

Validate that the chosen TUI architecture is:

- compatible with `myclawd`
- not dependent on raw runtime internals
- viable across normal terminal environments

## 2. Required Validation Areas

### A. Transport Validation

Validate:

- TUI can connect to `myclawd`
- TUI consumes websocket/control-plane messages
- reconnect/session binding strategy is coherent

### B. Store Validation

Validate:

- message state
- tool progress state
- approval state
- subagent/task summary state
- runtime inventory state

### C. Terminal Behavior Validation

Validate:

- resize handling
- viewport/list behavior
- graceful behavior across typical terminal environments

### D. Scope Validation

Validate:

- TUI stays lightweight
- TUI does not absorb workflows meant for React UI

## 3. Failure Gates

Do not declare the architecture complete if:

- TUI still depends on private runtime paths
- TTY-specific behavior is still the core architecture
- `myclawd` is not the exclusive backend boundary
- scope is still broad enough to compete with React UI
