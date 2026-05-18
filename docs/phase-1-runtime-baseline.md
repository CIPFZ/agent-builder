# Phase 1 Runtime Baseline

This document is the active Phase 1 architecture baseline. Older documents may
describe Phase 1 as a mock UI prototype; that is historical context only.

## Active Boundary

```text
React UI -> Wails adapter -> Go RuntimeBridge -> real Crush runtime
```

React is a thin presentation layer. These runtime concerns must come from
Go/Crush:

- Model configuration.
- Session state.
- Messages and message parts.
- Tool calls and tool results.
- Permission requests and decisions.
- Runtime cancellation.
- Usage and audit data.
- Runtime events.

## Required Phase 1 Behavior

- The desktop app uses the real Crush backend for chat.
- The UI does not synthesize chat messages for production paths.
- Tool activity is displayed from runtime message parts.
- Permission requests are shown to the user and require allow once, allow for
  session, or deny.
- Unconditional permission bypass is not allowed as the default desktop mode.
- Cancellation is exposed for the current runtime session.
- Wails remains an adapter behind `AgentRuntime`, not the long-term protocol.

## Long-Term Target

```text
React UI -> Client Transport -> HTTP/JSON-RPC + SSE/WebSocket -> Crush runtime
```

Phase 1 can use Wails bindings for the local executable, but Phase 2 should
formalize a transport-neutral runtime API and event stream.
