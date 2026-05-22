# Agent Builder Development Guide

## Project Overview

Agent Builder is a desktop agent client built in Go, Wails 3, and React.
The runtime is being refactored around a client-first architecture with Crush
as the backend base and Claude Code as the interaction design reference.

The module path is currently `github.com/charmbracelet/crush` and will be
normalized as the runtime/client structure stabilizes.

## Architecture

```
client/                      React client
desktop/                     Wails desktop shell
internal/runtime/            Runtime service, events, sessions, turns, audit
internal/agent/              Agent loop, prompts, providers, context
internal/tools/              Tool scheduler and tool execution
internal/permission/         Permission and policy primitives
internal/config/             Config loading and validation
internal/session/, message/   Core persistence models
internal/db/                 SQLite storage and migrations
internal/skills/, hooks/, lsp/  Capability and context support
internal/adapters/           Wails/HTTP/CLI/TUI adapters
```

## Key Principles

- React is the product UI, not the source of truth.
- Runtime state belongs in Go, not in browser memory.
- Wails is an adapter, not the business boundary.
- CLI/TUI compatibility is legacy and should stay out of the main product path.
- Tool calls, permissions, sessions, turns, and audit data must remain
  structured and recoverable.

## Build / Test / Lint Commands

- Build runtime and desktop: `go build ./...`
- Test runtime and desktop: `go test ./...`
- Build client: `cd client && npm run build`
- Lint: `task lint`

## Working Notes

- Keep edits focused on the current refactor boundary.
- Preserve existing tests when moving packages.
- Do not introduce new module splits unless required by a cleanup phase.

