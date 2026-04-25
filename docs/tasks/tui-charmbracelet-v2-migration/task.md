# TUI Charmbracelet v2 Migration Task

Date: 2026-04-25

## 1. Task Purpose

This is the execution entrypoint for migrating the Go TUI to the Charmbracelet v2 package paths:

- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`

The goal is to modernize the terminal stack without changing the TUI's product role or backend boundary.

## 2. Required Reading

Before coding, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/tui-client-architecture/task.md`
3. `docs/tasks/tui-client-architecture/design.md`
4. `docs/tasks/tui-client-architecture/source-alignment.md`
5. `docs/tasks/tui-charmbracelet-v2-migration/task.md`
6. `docs/tasks/tui-charmbracelet-v2-migration/design.md`
7. `docs/tasks/tui-charmbracelet-v2-migration/source-alignment.md`
8. `docs/tasks/tui-charmbracelet-v2-migration/implementation-plan.md`
9. `docs/tasks/tui-charmbracelet-v2-migration/test-validation-plan.md`
10. `docs/tasks/tui-charmbracelet-v2-migration/review-checklist.md`
11. `docs/roadmap/tui/roadmap.md`

After reading, output a short execution summary before coding.

That summary must include:

- migration objective
- package paths to migrate to
- files likely to change
- behavior that must be preserved
- validation commands

## 3. Objective

Migrate TUI imports and dependencies from the older Charmbracelet module paths to:

- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`

## 4. In Scope

- Go dependency update
- import migration
- API compatibility fixes
- focused TUI tests
- startup boundary verification

## 5. Out Of Scope

- React UI work
- new TUI feature expansion
- full Claude Code UI parity
- backend/runtime semantic changes
- reintroducing a production direct runtime bridge

## 6. Architecture Constraints

- TUI remains a lightweight `myclawd` client.
- Production TUI must not call `runtime.Runner` directly.
- The migration should preserve current UI behavior.
- Any broader code change must be justified by a Charmbracelet v2 compatibility need.

## 7. Required Implementation Order

1. update dependencies
2. replace imports
3. fix compile issues
4. run focused tests
5. search for legacy imports
6. document any remaining limitations

## 8. Required Validation

Run:

```bash
go test ./internal/tui ./internal/app ./internal/config
```

Search:

```bash
Select-String -Path internal/tui/*.go -Pattern "github.com/charmbracelet"
```

Expected result:

- no production TUI imports from legacy Charmbracelet paths

## 9. Start Prompt For Claude Code

Use this prompt to start implementation:

```text
You are migrating the myclaw Go TUI to the Charmbracelet v2 package paths.

Before coding, read:
1. docs/execution/implementation-rules.md
2. docs/tasks/tui-client-architecture/task.md
3. docs/tasks/tui-client-architecture/design.md
4. docs/tasks/tui-client-architecture/source-alignment.md
5. docs/tasks/tui-charmbracelet-v2-migration/task.md
6. docs/tasks/tui-charmbracelet-v2-migration/design.md
7. docs/tasks/tui-charmbracelet-v2-migration/source-alignment.md
8. docs/tasks/tui-charmbracelet-v2-migration/implementation-plan.md
9. docs/tasks/tui-charmbracelet-v2-migration/test-validation-plan.md
10. docs/tasks/tui-charmbracelet-v2-migration/review-checklist.md
11. docs/roadmap/tui/roadmap.md

After reading, output a short execution summary covering:
- migration objective
- package paths to use
- expected file impact
- behavior to preserve
- validation commands

Then implement the migration.

Rules:
- Target `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`.
- Do not reintroduce production RuntimeBridge or direct runtime.Runner access.
- Keep TUI as a myclawd client.
- Keep scope limited to dependency/import/API migration and required tests.
- Do not add unrelated TUI features.
```
