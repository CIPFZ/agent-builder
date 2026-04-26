# P0 Runtime Parity Roadmap Design

Date: 2026-04-26

## 1. Design Goal

P0 should make `myclaw` credible as a Claude Code-style runtime core, not as a full Claude Code product clone.

The design therefore prioritizes runtime behavior over UI fidelity, telemetry, enterprise policy, and remote bridge features.

## 2. Design Principle

P0 uses this ordering:

```text
model action surface -> user control surface -> long-session correctness -> client-neutral events
```

This means:

- tools first
- commands second
- context and recovery third
- structured events fourth

## 3. Workstream Boundaries

## P0.1 Tool Parity Core

Ownership:

- `internal/tools`
- `internal/tools/system`
- `internal/queryengine`
- `internal/permissions`
- `internal/approval`
- `internal/runtime`

The goal is not to implement every Claude Code tool. The goal is to make the existing high-value tools precise enough that the model can rely on them.

Minimum contracts:

- every core tool has stable input schema
- every core tool has read/write/destructive classification
- every core tool has permission behavior
- tool results preserve enough structure for transcript, UI, daemon, and future SDK clients
- tool progress and failures are observable through shared runtime paths

## P0.2 Command Registry

Ownership:

- `internal/app`
- `internal/tui`
- `internal/queryengine`
- new package if needed: `internal/commands`

Commands must become runtime-owned capabilities, not TUI-only shortcuts.

Minimum contracts:

- slash command metadata is registered in one place
- command visibility can depend on runtime state and permission mode
- commands can either produce immediate output or invoke the model loop
- command results are recorded consistently in session/message state

## P0.3 Context, Memory, And Recovery

Ownership:

- `internal/workspace`
- `internal/prompt`
- `internal/memory`
- `internal/session`
- `internal/store`
- `internal/model`
- `internal/runtime`
- `internal/queryengine`

Long sessions must preserve enough state to continue correctly after process restart or client reconnect.

Minimum contracts:

- workspace instructions load deterministically
- memory injection is explicit
- compaction boundaries are recoverable
- pending approvals are recoverable
- tool-use and tool-result blocks preserve identity
- invoked skills are recoverable
- agent/task state has a clear persistence boundary

## P0.4 Runtime Structured Events

Ownership:

- `internal/runtime`
- `internal/queryengine`
- `internal/gateway`
- `internal/protocol/ws`
- `internal/tui`

Events must become the shared contract between runtime and clients.

Minimum contracts:

- event names are stable
- event payloads have documented fields
- gateway does not invent separate semantics from runtime
- TUI consumes the same event concepts as daemon/control-plane clients

## 4. Dependency Model

P0.1 must happen before P0.4 because event schemas depend on real tool lifecycle behavior.

P0.2 can start after the first half of P0.1 if shell/file tool semantics are stable enough for command behavior.

P0.3 can run partly in parallel with P0.2, but recovery tests should wait until command and tool result shapes are stable.

P0.4 should be the final integration workstream.

## 5. Acceptance Model

P0 is accepted only when the system can execute this representative scenario:

1. load workspace context and memory
2. accept a user prompt
3. execute a file read
4. request approval for a write or shell action
5. execute the approved tool
6. update todo state
7. compact or preserve session state
8. restart or recover the session
9. resume with tool identities and prior context intact
10. emit stable events visible to both TUI and daemon clients

## 6. Documentation Model

This roadmap folder is a parent planning artifact.

Each child workstream must create:

- `README.md`
- `task.md`
- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

Implementation should not begin from this roadmap alone.

