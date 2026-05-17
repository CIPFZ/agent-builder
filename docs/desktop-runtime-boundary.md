# Desktop Runtime Boundary

This document records the Phase 1.5 architecture boundary for the desktop
client.

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
