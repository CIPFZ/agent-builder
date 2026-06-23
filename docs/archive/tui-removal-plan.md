# TUI Removal Plan

This document defines the target and execution rules for removing the legacy
Bubble Tea TUI from Agent Builder.

## Current Status

The legacy Agent Builder TUI has been removed from the active codebase. The current
product path is:

```text
React Client -> Wails Adapter -> Go Runtime / Runtime -> Agent / Tools
```

`internal/ui/` and `internal/commands/` are gone. `internal/cmd/` is reduced to
a minimal non-TUI compatibility stub for the root `main.go` entry point. App and
runtime event subscription now expose raw application/runtime payloads through
`pubsub.Event[any]`; `tea.Msg` and `*tea.Program` are no longer part of the
client runtime path.

## Goal

Remove the complete Agent Builder TUI surface from the product codebase and leave Agent
Builder with a client-first runtime path:

```text
React Client -> Wails Adapter -> Go Runtime / Runtime -> Agent / Tools
```

The final codebase should not require Bubble Tea UI models, terminal chat
renderers, terminal dialogs, or TUI workspace adapters for the desktop client
to build and run.

## Non-Goals

- Do not preserve the interactive terminal TUI.
- Do not keep terminal rendering as a hidden compatibility layer.
- Do not keep TUI code only because tests still reference it; migrate or delete
  those tests with the TUI package.
- Do not reintroduce upstream Agent Builder CLI installation docs, release automation,
  or TUI-focused configuration.
- Do not ask the user to decide each conflict during execution. Make pragmatic
  client-first decisions and complete the task end to end.

## Current Evidence

Existing docs already mark the TUI as legacy:

- `docs/legacy-agent-builder-inventory.md` classifies `internal/ui/`,
  `internal/cmd/`, and `internal/commands/` as legacy CLI/TUI surface.
- `docs/desktop-runtime-boundary.md` defines Wails as an adapter and Go runtime
  as the source of truth.
- `docs/archive/project-structure-refactor-plan.md` and
  `docs/client-first-runtime-refactor.md` identify `tea.Msg` as a runtime path
  leak that must be removed.

Code-level TUI leakage currently exists in:

- `internal/app/app.go`: `pubsub.Broker[tea.Msg]`,
  `Subscribe(program *tea.Program)`, and terminal spinner styling.
- `internal/workbench/events.go`: `SubscribeEvents(... tea.Msg)`.
- `internal/workspace/workspace.go`: `Subscribe(program *tea.Program)`.
- `internal/workspace/app_workspace.go` and
  `internal/workspace/client_workspace.go`: TUI/CLI workspace adapters.
- `internal/cmd/*`: Cobra CLI commands that directly start or render the TUI.
- `internal/format/spinner.go`: Bubble Tea spinner for non-interactive CLI.
- `internal/ui/*`: Bubble Tea chat, dialog, list, style, model, and renderer
  packages.

## Required Final State

The final repository state should satisfy:

- `internal/ui/` is deleted.
- TUI-specific tests and golden fixtures are deleted with it.
- `internal/cmd/` is deleted or reduced to a non-TUI, non-Bubble-Tea stub only
  if root `main.go` still needs a temporary command entry.
- `internal/commands/` is deleted unless a clearly runtime-owned subset is
  extracted first.
- `internal/app`, `internal/workbench`, `internal/workspace`, `desktop`, and
  `internal/runtime` do not import:
  - `charm.land/bubbletea/v2`
  - `charm.land/lipgloss/v2`
  - `charm.land/glamour/v2`
  - `github.com/CIPFZ/agent-builder/internal/ui/...`
- `go.mod` no longer has direct TUI-only dependencies unless another retained
  non-TUI package still requires them.
- The desktop client remains the primary build target.
- Daily lightweight CI still passes.

## Execution Strategy

This is intended as one complete cleanup task in a fresh session. Do not stop
after only documenting or isolating the first dependency. Continue through code
deletion, compile fixes, tests, commit, and push.

Recommended order:

1. Inspect current references:

   ```bash
   rg "internal/ui|bubbletea|lipgloss|glamour|tea\\.Msg|\\*tea\\.Program" internal desktop main.go go.mod
   ```

2. Replace app/workbench event transport with non-TUI types:

   - Change `internal/app` event broker away from `pubsub.Broker[tea.Msg]`.
   - Remove or replace `App.Subscribe(program *tea.Program)`.
   - Keep a runtime/raw event subscription suitable for runtime, server, Wails,
     and desktop runtime use.
   - Update `internal/workbench/events.go` so non-TUI consumers no longer see
     `tea.Msg`.

3. Remove TUI workspace adapter shape:

   - Remove `Subscribe(program *tea.Program)` from
     `internal/workspace.Workspace`.
   - Delete or rewrite `AppWorkspace.Subscribe` and `ClientWorkspace.Subscribe`.
   - If `internal/workspace` becomes unused after removing CLI/TUI, delete the
     package.

4. Remove CLI/TUI entry points:

   - Delete TUI-starting command code under `internal/cmd`.
   - Delete root CLI paths in `main.go` if they only exist for Agent Builder TUI.
   - If a temporary root command is needed, keep it minimal and desktop-safe,
     but do not import TUI packages.

5. Remove terminal presentation packages:

   - Delete `internal/ui/`.
   - Delete `internal/format/spinner.go` or replace it with a simple
     non-Bubble-Tea progress helper if still needed.
   - Remove dependencies from `internal/app` on `internal/ui/anim`,
     `internal/ui/styles`, `lipgloss`, and terminal styling.

6. Remove slash-command terminal UI if no runtime consumer remains:

   - Delete `internal/commands/` if it is only used by the TUI/CLI.
   - If any parser logic is worth retaining for future client command palette,
     extract only the pure parser into a runtime/client-owned package and delete
     terminal-specific behavior.

7. Clean dependencies:

   ```bash
   go mod tidy
   ```

   Then verify `go.mod` no longer directly requires removed TUI libraries
   unless there is a retained non-TUI reason.

8. Update docs that still describe the TUI as active:

   - `docs/legacy-agent-builder-inventory.md`
   - `docs/client-first-runtime-refactor.md`
   - `docs/archive/project-structure-refactor-plan.md`
   - `docs/desktop-runtime-boundary.md` if needed

   Move the wording from "legacy to remove later" to "removed; desktop client
   is the active surface".

## Verification

Run these checks before finishing:

```bash
rg "internal/ui|bubbletea|lipgloss|glamour|tea\\.Msg|\\*tea\\.Program" internal desktop main.go go.mod
gofmt -l $(git ls-files '*.go' ':!:desktop/build/**')
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -failfast -skip '^TestCoderAgent$' . ./internal/...
```

If the `rg` command still finds matches, each remaining match must be justified
as non-TUI and not on the client runtime path. Otherwise remove it.

For desktop validity, also run when feasible:

```bash
cd client
npm ci
npm run build
cd ../desktop
node scripts/sync-client-dist.mjs
go test ./...
```

Run a full Wails desktop build if the cleanup touches `desktop/`,
`client/`, Wails config, or embedded frontend assets.

## Commit Rules

- Use semantic commits.
- Prefer one cleanup commit if the full TUI removal is coherent and tests pass.
- Use additional commits only when there are clearly separable preparatory
  changes.
- Push to `origin/main` after successful verification when the user requested a
  complete end-to-end cleanup.

## Final Report

The final response should include:

- Deleted packages and major files.
- Replacement event/runtime model.
- Any retained CLI or command stub, with reason.
- Validation commands and results.
- Commit hash and push status.
