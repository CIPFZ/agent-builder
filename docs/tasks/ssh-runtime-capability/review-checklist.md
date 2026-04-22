# SSH Runtime Capability Review Checklist

Date: 2026-04-22

## Purpose

Use this checklist when reviewing the downstream Claude Code implementation of the SSH module.

The review goal is not style. The review goal is semantic correctness, architectural alignment, and readiness for later Docker and DB modules.

## 1. Scope Control

- Confirm the implementation only covers remote command execution.
- Confirm file transfer, PTY, port forwarding, and jump hosts were not silently added.
- Confirm no UI-specific SSH behavior was mixed into the runtime implementation.

## 2. Tool Identity

- Confirm there is a distinct first-class `SSH` tool.
- Confirm SSH was not implemented as a preset wrapper around `system.run`.
- Confirm the tool schema is structured and not plain-string-only.
- Confirm `host` and `command` are required.

## 3. Go Ownership Boundary

- Confirm primary ownership lives under `internal/tools/system`.
- Confirm registry and runtime assembly register `SSH` by default.
- Confirm local shell execution files were not heavily distorted to accommodate SSH.
- Confirm any `sandbox/router.go` changes are minimal and clearly justified.

## 4. Claude Alignment

- Confirm the implementation follows Claude-style execution semantics:
- structured input
- centralized permission check
- progress lifecycle
- transcript-safe tool result
- transport-neutral event forwarding

- Confirm no frontend-owned SSH semantics were introduced.

## 5. Permission And Approval

- Confirm `IsReadOnly` is conservative in v1.
- Confirm SSH is treated as destructive / approval-sensitive by default.
- Confirm approval payloads preserve host-aware structured input.
- Confirm rejection and acceptance flows both behave correctly.

## 6. Execution Backend

- Confirm v1 uses the system `ssh` binary behind an internal abstraction.
- Confirm the command builder does not rely on unsafe string concatenation where argument separation is possible.
- Confirm timeout cancels the whole SSH process.
- Confirm workdir wrapping is explicit and documented as POSIX-targeted for v1.

## 7. Progress And Result Semantics

- Confirm progress events exist for start, execution, and terminal states.
- Confirm progress payloads include host and command context.
- Confirm final result separates stdout, stderr, exit code, duration, and timeout internally.
- Confirm final `tools.ToolResult` includes both current-compatible output text and structured data for future consumers.

## 8. Query And Transcript Integration

- Confirm the query engine path did not require SSH-specific transcript hacks.
- Confirm `tool.called` and `tool.result` are emitted normally.
- Confirm SSH input object survives transcript and approval round-trips.

## 9. Gateway / myclawd Integration

- Confirm gateway forwarding reuses generic event families.
- Confirm no separate SSH-only protocol was added.
- Confirm `tool_input_object` and relevant result/progress metadata are forwarded to clients.
- Confirm current lightweight TUI can observe the execution without custom SSH logic.

## 10. Testing

- Confirm parser, command-construction, timeout, and result-shaping unit tests exist.
- Confirm approval/integration tests exist.
- Confirm gateway event propagation is tested.
- Confirm at least one controlled functional SSH flow was executed and documented.

## 11. Future-Readiness

- Confirm the implementation leaves a clean base for later Docker and DB modules.
- Confirm the code exposes reusable remote execution semantics instead of an SSH-only dead end.
- Confirm the result and progress contracts are generic enough for later environment-control tools.

## 12. Review Failure Conditions

Reject the implementation if any of the following are true:

- SSH is hidden behind shell templating
- approval ignores host context
- runtime and gateway carry only raw text and drop structured data
- local sandbox routing becomes harder to reason about
- tests only cover the happy path
