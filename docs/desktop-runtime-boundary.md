# Desktop Runtime Boundary

This document records the Phase 1.5 architecture boundary for the desktop
client. The active Phase 1 baseline is also summarized in
`docs/phase-1-runtime-baseline.md`.

## Decision

The desktop client must keep React as a thin presentation layer. The source of
truth for model config, session state, messages, provider selection, agent
execution, tool calls, and future permissions is the Go/Crush runtime.

Current implementation follows this boundary:

- `client/src/runtime/*` defines a narrow `AgentRuntime` facade.
- `client/src/App.tsx` collects user input, displays runtime messages, and
  opens settings.
- Model settings are saved by Go to
  `desktop/agent-builder/bin/config/model.local.json`.
- Chat execution goes through `RuntimeBridge.Chat`, which calls the real Crush
  backend and waits for the assistant message.
- Message display is refreshed from `RuntimeBridge.Messages`, backed by the
  Crush session database, instead of constructing user/assistant messages in
  React.
- Permission requests are owned by Crush and exposed through
  `RuntimeBridge.Permissions`; React can only allow once, allow for session, or
  deny a pending runtime request.
- Runtime cancellation is exposed through `RuntimeBridge.Cancel`, backed by
  `Backend.CancelSession`.
- Runtime events are exposed as a small queryable event log through
  `RuntimeBridge.Events`. This is the Phase 1 bridge toward the later
  SSE/WebSocket event stream.
- Wails is only the desktop bridge and packaging layer. It is not the runtime
  architecture.

## Consequences

- The first desktop package can be tested as an `.exe` without inventing a
  second frontend runtime.
- Later HTTP/SSE or JSON-RPC APIs can replace the in-process Wails binding
  without changing the React application model.
- The TUI, desktop client, future Web console, and automation clients can all
  converge on the same Crush runtime state model.
- UI-only mocks must not be reintroduced for model, message, provider, or agent
  execution paths unless they are isolated test fixtures.
- Phase 1 must not use unconditional permission bypass for normal desktop
  operation. Bypass can exist later as an explicit policy mode, but the default
  Phase 1 behavior is ask/allow/deny.

## Phase 1 Delta

Earlier planning documents described Phase 1 as a pure UI/mock prototype. That
is no longer the active baseline. The accepted Phase 1 foundation is:

```text
React UI -> Wails adapter -> Go RuntimeBridge -> real Crush backend/session/message/permission services
```

The long-term target is still a transport-neutral runtime API plus event stream:

```text
React UI -> Client Transport -> HTTP/JSON-RPC + SSE/WebSocket -> Crush runtime
```

The Wails binding is acceptable for the first executable, but it must remain an
adapter behind `AgentRuntime`, not the core runtime architecture.
