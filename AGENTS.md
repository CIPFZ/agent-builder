# Agent Builder Development Guide

## Project Overview

Agent Builder is a desktop agent client built in Go, Wails 3, and React.
The runtime is being refactored around a client-first architecture with Agent Builder
as the runtime base and Claude Code as the interaction design reference.

The module path is `github.com/CIPFZ/agent-builder`.

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
internal/adapters/           Legacy CLI/TUI adapters outside the product path
```

## Key Principles

- React is the product UI, not the source of truth.
- Runtime state belongs in Go, not in browser memory.
- Wails is an adapter, not the business boundary.
- CLI/TUI compatibility is legacy and should stay out of the main product path.
- Tool calls, permissions, sessions, turns, and audit data must remain
  structured and recoverable.
- The frontend uses Ant Design and Ant Design X as the primary UI foundations.
  See `docs/frontend-and-desktop.md`.
- Claude Desktop and similar clients may guide information architecture, but
  the product must not copy proprietary branding, assets, exact styling, copy,
  or visual identity.

## Build / Test / Lint Commands

- Build runtime and desktop: `go build ./...`
- Test runtime and desktop: `go test ./...`
- Build client: `cd client && npm run build`
- Lint: `task lint`

## Working Notes

- Keep edits focused on the current refactor boundary.
- Preserve existing tests when moving packages.
- Do not introduce new module splits unless required by a cleanup phase.
- For new frontend work, map Go runtime DTOs into UI view models instead of
  making React or Ant Design X hooks the runtime state source.
- Use Ant Design theme tokens and scoped CSS Modules for new UI surfaces; avoid
  expanding global CSS as the main styling mechanism.
- Frontend/runtime integration is Wails-only. Use generated bindings for
  request/response operations and Wails events for streams. Do not add HTTP,
  SSE, polling, `fetch`, `XMLHttpRequest`, or axios fallbacks between React and
  the Go runtime. See `docs/frontend-and-desktop.md`.
