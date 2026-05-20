# Agent Builder Desktop

This is the Phase 1 Wails desktop shell for the agentic operations client.
It intentionally stays thin: the product UI remains in `../../client`, and
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
desktop/agent-builder/bin/AgentBuilder.exe
```

The executable owns sibling runtime directories:

```text
desktop/agent-builder/bin/config/
desktop/agent-builder/bin/data/
desktop/agent-builder/bin/logs/
```

Model settings are saved by the Go runtime bridge to
`bin/config/model.json`. See
`../../docs/phase-1-acceptance-test.md` for the local validation flow.

## Frontend Sync

The Wails build runs `npm run build` in `../../client` through
`scripts/sync-client-dist.mjs`, then copies `../../client/dist` into
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
