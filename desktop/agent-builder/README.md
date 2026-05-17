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

## Frontend Sync

The Wails build runs `npm run build` in `../../client` through
`scripts/sync-client-dist.mjs`, then copies `../../client/dist` into
`frontend/dist` before embedding assets.

Manual sync:

```powershell
node scripts/sync-client-dist.mjs --build-client
```
