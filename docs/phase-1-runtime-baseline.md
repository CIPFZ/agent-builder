# Phase 1 Runtime Baseline

This document is the active Phase 1 architecture baseline. Older documents may
describe Phase 1 as a mock UI prototype; that is historical context only.

## Active Boundary

```text
React UI -> Wails command adapter -> Go RuntimeBridge -> real Crush runtime
React UI -> loopback SSE event stream -> Go RuntimeBridge runtime events
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
- Runtime events are published through a local `127.0.0.1` SSE stream and are
  replayable through `RuntimeBridge.Events`.
- The shell tool uses Crush's embedded portable shell. It must not be treated
  as the user's system Bash, PowerShell, or cmd.exe; Windows commands must be
  selected with that boundary in mind.
- Wails remains an adapter behind `AgentRuntime`, not the long-term protocol.

## Shell Boundary

The current tool name remains `bash` for Crush compatibility, but Phase 1
behavior is a portable shell abstraction:

- Shell composition uses `mvdan.cc/sh` semantics.
- Windows does not require a real Bash installation.
- PowerShell and cmd.exe syntax are not implicitly supported by the shell tool.
- External commands such as `ping`, `go`, `npm`, or `rg` still execute from the
  host PATH and can have OS-specific arguments.
- Windows command output is normalized to UTF-8 before it reaches messages and
  the desktop UI.

Future plugin/tool work should add explicit native command runners when a task
requires PowerShell, cmd.exe, SSH clients, or product-specific executables.

## Long-Term Target

```text
React UI -> Client Transport -> HTTP/JSON-RPC + SSE/WebSocket -> Crush runtime
```

Phase 1 uses Wails bindings for command-style desktop calls and loopback SSE for
runtime events. Phase 2 should formalize the same boundary as a
transport-neutral runtime API for Web and remote clients.
