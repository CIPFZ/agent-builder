# Agent Builder Desktop

This is the Phase 1 Wails desktop shell for the agentic operations client.
It intentionally stays thin: the product UI remains in `../client`, and
the desktop build embeds the shared `client/dist` output.

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
`bin/config/model.json`. See `scripts/phase2-smoke.ps1` for the current
packaged desktop smoke flow. The historical Phase 1 acceptance flow is archived
at `../docs/archive/phase-1-acceptance-test.md`.

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
executable, and runs the desktop bridge contract test for new-chat handoff,
event cursor forwarding, `SessionActivity` interrupted recovery hydration, and
`MarkInterruptedDone` acknowledgement semantics.

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
