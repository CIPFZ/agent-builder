# Desktop Runtime Root Cause Analysis

Date: 2026-05-18

This document records the current end-to-end analysis after the desktop UI
reported:

```text
failed to send message to Agent Builder agent: context deadline exceeded
```

## Current Call Chain

The desktop chat path is:

```text
React App
  -> client/src/api/chat.ts sendRuntimePrompt()
  -> client/src/runtime/wailsRuntime.ts bridge.Chat()
  -> Wails generated binding
  -> desktop/runtime_bridge.go RuntimeBridge.Chat()
  -> internal/workbench/agent.go Workbench service.SendMessage()
  -> internal/agent/coordinator.go Coordinator.Run()
  -> internal/agent/agent.go sessionAgent.Run()
  -> fantasy.Agent.Stream()
  -> provider API
  -> message/session SQLite services
  -> RuntimeBridge.Messages()
  -> React message list
```

## Confirmed Facts

1. The UI is connected to the Go desktop bridge.

   Evidence:

   - `client/src/App.tsx` calls `sendRuntimePrompt()` and then
     `requestRuntimeMessages()`.
   - `client/src/runtime/wailsRuntime.ts` calls the generated Wails
     `RuntimeBridge.Chat()` and `RuntimeBridge.Messages()` bindings.
   - Real UI tests showed successful short prompts returning through
     `local-model/deepseek-v4-flash`.

2. The Go bridge is connected to the real Agent Builder runtime.

   Evidence:

   - `desktop/runtime_bridge.go:275` calls
     `r.runtime.SendMessage(...)`.
   - `internal/workbench/agent.go:22` calls `ws.AgentCoordinator.Run(...)`.
   - Runtime logs show `Skill turn summary`, which is emitted by the Agent Builder
     agent coordinator, not by the React UI.

3. The failure happens inside the synchronous agent run, not before runtime.

   Evidence from `bin/logs/agent-builder.log`:

   ```text
   2026-05-18T16:06:22 Desktop chat started prompt_len=45
   2026-05-18T16:08:22 Skill turn summary prompt_len=45
   2026-05-18T16:08:22 Desktop chat failed duration=2m0.0018309s error="context deadline exceeded"
   ```

   This matches `desktop/runtime_bridge.go:270`, where desktop
   creates a hard `context.WithTimeout(ctx, chatTimeout)` with
   `chatTimeout = 2 * time.Minute`.

4. Agent Builder stores useful usage data, but desktop does not expose it.

   Evidence:

   - `internal/session/session.go` stores `PromptTokens`,
     `CompletionTokens`, and `Cost`.
   - `internal/agent/agent.go:438` updates session usage from
     `stepResult.Usage`.
   - `internal/apitypes/session.go` already has JSON fields for usage.
   - `RuntimeBridge.Chat()` and `RuntimeBridge.Messages()` do not return
     session usage.

5. Agent Builder has an event system, but desktop is not using it.

   Evidence:

   - `internal/app/app.go:473` subscribes sessions, messages, permissions,
     history, agent notifications, MCP, LSP, and skills into the app event
     broker.
   - `internal/workbench/events.go` exposes `SubscribeEvents`.
   - The desktop bridge does not subscribe to or stream those events to React.
   - React refreshes messages only after `Chat()` returns.

## Root Problems

### P0: Desktop chat is implemented as one blocking RPC

`RuntimeBridge.Chat()` waits for the whole Agent Builder agent turn to complete before
returning to React.

Current behavior:

```text
UI send -> Wails Chat() -> SendMessage() blocks -> UI waits
```

This is not aligned with how Agent Builder is designed. Agent Builder writes messages and usage
progressively through message/session services and event brokers. The desktop UI
should observe runtime state, not wait on one long RPC.

Impact:

- Any slow model response, long generation, retry, tool call, or provider stall
  blocks the UI request.
- The hard two-minute timeout cancels the real agent context.
- Partial output may already be in DB, but React may not refresh until the call
  fails.
- The user sees a generic Wails error instead of a runtime turn status.

### P0: The two-minute bridge timeout cancels the provider call

The timeout is applied to `SendMessage()`, which eventually reaches
`fantasy.Agent.Stream()`. When it expires, the provider stream is canceled.

This is exactly what happened in the failed case:

```text
duration="2m0.0018309s" error="context deadline exceeded"
```

For desktop chat, this timeout is too aggressive and in the wrong layer. The UI
needs a visible "running" state and a cancel action. The runtime call should not
be killed just because one Wails request waited too long.

### P0: Desktop does not have an audit log for turn-level observability

Current logs show:

- workspace id
- session id
- prompt length
- provider/model
- duration
- content length
- error

They do not show:

- request id / turn id
- created user message id
- assistant message id
- prompt preview
- response preview
- finish reason
- session usage before/after
- prompt/completion token delta
- queued/busy state
- provider retry/error details correlated to the turn

This makes it hard to prove that the real model was called or to debug
failures.

### P1: `waitForAssistant()` can return the wrong completion signal

`RuntimeBridge.waitForAssistant()` polls for the latest assistant message. It
does not require that the assistant message has a finish part.

In the current successful flow this is partially masked because
`SendMessage()` blocks until the run completes. Once the bridge becomes
asynchronous, this polling rule will be wrong: an assistant message is created
before content streams and before finish.

### P1: Frontend refreshes messages only after `Chat()` completes

`client/src/App.tsx:181` waits for `sendRuntimePrompt(content)` and only then
calls `refreshMessages()`.

This means React cannot show:

- user message immediately from DB
- streaming assistant content
- long-running "still working" status
- tool/progress events
- usage changes

### P1: Desktop status does not expose session usage

`RuntimeStatus` only includes ready/workspace/session/workingDir/model/provider
and busy. It does not include usage. The user cannot see token movement even
though Agent Builder stores it.

### P2: Local provider config is minimal and may not match provider behavior

The local model config currently sets a custom OpenAI-compatible provider with:

```text
ContextWindow: 64000
DefaultMaxTokens: 4096
```

This is acceptable for a first bridge, but it hides provider-specific behavior:

- exact token accounting depends on fantasy/provider usage support
- DeepSeek API compatibility may vary by endpoint/protocol
- long prompts can trigger long generations with no desktop-level progress

This is not the direct cause of the observed timeout, but it makes debugging
harder.

## What Was Not The Primary Cause

1. It is not a pure React mock problem.

   The old mock files were removed and the UI uses Wails runtime calls.

2. It is not a basic model config path problem.

   The runtime loaded `bin/config/model.json` and logged
   `has_api_key=true`.

3. It is not a complete Wails binding failure.

   Short prompts have completed through the real bridge.

4. It is not proof that Agent Builder cannot call the model.

   Logs show successful real calls before and after. The failing prompt hit the
   desktop bridge timeout.

## Required Fix Plan

### Step 1: Add temporary desktop audit log

Create:

```text
desktop/bin/logs/agent-builder-audit.jsonl
```

Each chat turn should write JSONL records:

```json
{"event":"chat_started","request_id":"...","session_id":"...","provider":"local-model","model":"deepseek-v4-flash","prompt_preview":"...","prompt_len":45,"started_at":"..."}
{"event":"chat_finished","request_id":"...","assistant_message_id":"...","duration_ms":1234,"response_preview":"...","response_len":508,"finish_reason":"end_turn","prompt_tokens_before":0,"prompt_tokens_after":123,"completion_tokens_before":0,"completion_tokens_after":45,"prompt_tokens_delta":123,"completion_tokens_delta":45}
{"event":"chat_failed","request_id":"...","duration_ms":120001,"error":"context deadline exceeded","messages_count":4,"last_assistant_preview":"..."}
```

Rules:

- Never log API keys.
- Prompt/response preview should be capped.
- Include full lengths and token deltas.
- Include exact log file path in docs/UI later.

### Step 2: Stop using one blocking RPC as the UI lifecycle

Short-term acceptable fix:

- Increase/remove the hard two-minute chat timeout.
- On failure, always refresh messages and audit the partial assistant message.
- Return a clearer error that includes audit log path.

Correct architecture fix:

```text
ChatStart(prompt) -> returns request/session ids quickly
Messages()/Status()/Audit() -> UI polls or subscribes
Events stream -> later replace polling
Cancel(sessionID) -> user can stop a long run
```

### Step 3: Use Agent Builder session usage in desktop response/status

After each turn, read `Workbench service.GetSession(...)` and expose:

- prompt tokens
- completion tokens
- cost
- deltas from before/after

This proves the model call updated Agent Builder runtime state.

### Step 4: Use finish state when reading assistant messages

`RuntimeBridge.Messages()` should expose finish reason and error details.
`waitForAssistant()` should wait for a finished assistant message if it remains
in use.

### Step 5: Move toward event-driven UI

The long-term desktop path should use `Workbench service.SubscribeEvents(...)` or a
desktop-specific event bridge so React observes message/session updates instead
of waiting for Wails `Chat()` to finish.

## Priority

Immediate next implementation should be:

1. Add audit JSONL.
2. Add session usage to runtime status/chat response.
3. Make `waitForAssistant()` wait for a finished assistant message.
4. Increase chat timeout enough for Phase 1 acceptance, or split start/poll.
5. Add UI audit panel or link after the log has useful data.

The architectural target remains:

```text
React UI = display/input only
Go desktop bridge = client adapter
Agent Builder runtime/session/message/event services = runtime source of truth
Provider/fantasy = model execution
```
