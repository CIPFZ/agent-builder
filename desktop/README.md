# Agent Builder Desktop

This is the Wails desktop shell for the Agent Builder client.
It intentionally stays thin: the product UI remains in `../client`, and
the desktop build embeds the shared `client/dist` output.

Conversation transport is also intentionally thin. `RuntimeBridge` forwards
the Go canonical conversation snapshot and emits each canonical entity batch
as one Wails event without splitting its raw sequence. React applies those
batches to its normalized store, then derives Turn presentation once. The
desktop shell does not build conversation items, tool groups, Todo summaries,
or any competing conversation state.

## Build

Install Wails v3 CLI if needed:

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

Build the desktop executable:

```powershell
$env:PATH="$env:USERPROFILE\go\bin;$env:PATH"
wails3 task build
```

The Windows executable is written to:

```text
desktop/bin/AgentBuilder.exe
```

The executable owns sibling runtime directories:

```text
desktop/bin/config/
desktop/bin/data/
desktop/bin/logs/
```

Model settings are saved by the Go runtime bridge to
`bin/config/model.json`. See `scripts/phase2-smoke.ps1` for a packaged desktop
smoke flow and `../docs/frontend-and-desktop.md` for the current boundary.

Desktop-managed runtime settings live next to the executable:

```text
bin/config/model.json
bin/config/skills.json
bin/config/mcp.json
```

## Frontend Sync

The Wails build runs `npm run build` in `../client` through
`scripts/sync-client-dist.mjs`, then copies `../client/dist` into
`frontend/dist` before embedding assets.

Manual sync:

```powershell
node scripts/sync-client-dist.mjs --build-client
```

## Phase 2 Smoke

Run the repeatable packaged desktop smoke from the desktop project:

```powershell
.\scripts\phase2-smoke.ps1 -Build
```

The script builds `bin/AgentBuilder.exe`, uses a temporary
`AGENT_BUILDER_DESKTOP_ROOT`, checks runtime API coverage, starts the packaged
exe, and verifies runtime directories are created outside the repository
runtime data directory.

To include the live DeepSeek chat smoke, put the key in an environment variable
instead of a checked-in file:

```powershell
$env:DEEPSEEK_API_KEY="..."
.\scripts\phase2-smoke.ps1 -Build -Live
```

## Phase 6.2 Wails Packaged Handoff/Recovery Smoke

Run the focused packaged desktop/Wails bridge smoke from the desktop project:

```powershell
.\scripts\phase62-wails-packaged-smoke.ps1 -Build
```

The script builds `bin/AgentBuilder.exe` when requested, uses
`../tmp/runtime-dev` for its temporary runtime root, starts the packaged
executable, and runs desktop bridge contract coverage for new-chat handoff,
canonical conversation snapshot/cursor forwarding, atomic entity batches,
interrupted recovery hydration, and `MarkInterruptedDone` acknowledgement.

## Phase 31.1 Wails Packaged Scheduler Smoke

Run the packaged scheduler bridge smoke from the desktop project:

```powershell
.\scripts\phase311-wails-packaged-scheduler-smoke.ps1 -Build
```

The script builds `bin/AgentBuilder.exe` when requested, uses
`../tmp/runtime-dev` for its temporary runtime root, runs the Wails bridge
scheduler projection/plan/execute contract test, starts the packaged
executable, and verifies packaged runtime directories are created. It does not
automate a WebView2 button click.

## Phase 36.1 Wails WebView Test Channel Smoke

Run the packaged WebView test-channel smoke from the desktop project:

```powershell
.\scripts\phase361-wails-webview-test-channel-smoke.ps1 -Build
```

The script builds the packaged executable with `EXTRA_TAGS=webview_test`, uses
`../tmp/runtime-dev` for its runtime root and WebView user-data directory,
starts the app with a local WebView2 remote-debugging port, and verifies the
CDP endpoint becomes reachable. This is test-only automation infrastructure;
normal untagged builds do not read the WebView test environment variables or
open a remote-debugging port.

## Phase 36.2 Packaged WebView Scheduler Click Smoke

Run the packaged WebView scheduler click smoke from the client project:

```powershell
cd ..\client
npm run smoke:phase362
```

The smoke starts a non-secret loopback provider, builds the packaged executable
with `EXTRA_TAGS=webview_test`, starts the packaged app with its runtime root
and WebView user-data directory under `../tmp/runtime-dev`, connects to the
test-only CDP endpoint, clicks the visible scheduler Execute button, and
verifies completion by re-reading Wails runtime DTOs.
