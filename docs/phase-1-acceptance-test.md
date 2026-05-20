# Phase 1 Acceptance Test

This document defines the local acceptance flow for the Phase 1 desktop build.

## Build Artifact

Build the desktop package from `desktop/agent-builder`:

```powershell
wails3 task build
```

The Windows executable is:

```text
desktop/agent-builder/bin/AgentBuilder.exe
```

The desktop runtime owns these sibling directories:

```text
desktop/agent-builder/bin/config/
desktop/agent-builder/bin/data/
desktop/agent-builder/bin/logs/
```

## Model Configuration

Open `AgentBuilder.exe`, then open **Model settings**.

Required fields:

- Protocol: `OpenAI compatible` or `Anthropic compatible`.
- URL: provider base URL, for example `https://api.deepseek.com`.
- API key: pasted into the settings form. Do not write keys into source files.
- Model: provider model id, for example `deepseek-v4-flash`.

Optional advanced field:

- Proxy: HTTP proxy URL, for example `http://127.0.0.1:7890`.

The backend saves the config to:

```text
desktop/agent-builder/bin/config/model.json
```

React must not persist model settings itself. The UI displays the configuration
returned by the Go runtime bridge.

## Chat Smoke Test

Use this prompt:

```text
Reply exactly: runtime backend ok
```

Expected result:

- The user message appears in the thread.
- The assistant response appears in the thread.
- The assistant message shows the runtime provider tag, for example
  `local-model`.
- If the model requests a tool, the UI shows a permission dialog with
  **Allow once**, **Allow session**, and **Deny** actions.
- Tool calls and tool results appear as runtime activity, not as empty
  assistant bubbles.
- The current run can be cancelled from the header while the runtime is busy.
- No command-line window is shown while chatting.
- The UI does not require restarting after saving model settings.

## Runtime Log Check

Open:

```text
desktop/agent-builder/bin/logs/agent-builder.log
```

Expected log entries:

- `Desktop model configured`
- `Desktop chat started`
- `Desktop chat finished`
- `provider":"local-model"`
- `model":"<configured model>"`
- `has_api_key":true`

The log must not contain the API key value.

## Data Check

The runtime database is stored under:

```text
desktop/agent-builder/bin/data/
```

Message display must come from the Go/Crush session database through
`RuntimeBridge.Messages`, not from frontend-generated mock messages.

## Pass Criteria

Phase 1 acceptance passes when:

- `AgentBuilder.exe` starts.
- Missing model config is shown as a clear UI warning.
- Model settings save to `bin/config/model.json`.
- Chat works through the real Crush runtime.
- Logs confirm a completed backend chat turn.
- Permission requests are not unconditionally auto-approved.
- Runtime event counts and recent event log come from the Go bridge.
- No API key is printed in logs or committed files.
- `npm run lint`, `npm run build`, Go tests, and `wails3 task build` pass.
