# Agent Builder

Agent Builder is a desktop agent client built with Go, Wails 3, and React.
It uses Crush as the backend runtime base while moving the product experience
from CLI/TUI toward a Codex-like desktop client.

## Goals

- Make the desktop client the primary product surface.
- Keep Go runtime state authoritative.
- Use React for input, display, permissions, settings, audit, and diagnostics.
- Treat Wails as a desktop adapter, not as the business boundary.
- Isolate legacy CLI/TUI code from the client runtime path.

## Repository Layout

```text
client/              React client
desktop/             Wails desktop shell
internal/runtime/    Client-first runtime service
internal/agent/      Agent loop, provider, prompt, and context logic
internal/tools/      Tool scheduling and execution
internal/permission/ Permission primitives
internal/db/         SQLite storage and migrations
docs/                Architecture and migration notes
scripts/             Development helpers
```

## Development

Run the Go test suite from the repository root:

```bash
go test ./...
```

Build the React client:

```bash
cd client
npm run build
```

Run the root task helpers when needed:

```bash
task --list
```

## Architecture Documents

Start here:

- `docs/client-runtime-architecture-review.md`
- `docs/client-architecture-and-core-flow.md`
- `docs/desktop-runtime-boundary.md`
- `docs/phase-2-runtime-api-boundary.md`
- `docs/tool-scheduler-design.md`
- `docs/permission-policy-model.md`
- `docs/turn-task-run-model.md`
- `docs/client-state-recovery.md`

## Current Status

The repository is mid-refactor. Some Go module paths, runtime package names,
and legacy CLI/TUI packages still carry Crush naming until the runtime boundary
is fully normalized.

The current architecture target is:

```text
React Client -> Runtime API + Event Stream -> Go Runtime -> Agent/Tools
```
