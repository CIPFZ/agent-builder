# Module Task Document Standard

Date: 2026-04-23

## Purpose

Each implementation module should have one primary execution document for Claude Code.

Standard filename:

- `docs/tasks/<module-name>/task.md`

This file is the single entrypoint that Claude Code reads before implementation.

## Rule

Do not require Claude Code to infer execution flow from many scattered docs alone.

Each module must have a `task.md` that consolidates:

- objective
- scope
- non-goals
- required reading
- Claude semantic alignment
- Go ownership boundary
- implementation order
- validation requirements
- completion output requirements
- starter prompt

Companion documents can still exist:

- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

But Claude Code should be able to start from `task.md` alone.

## Recommended Module Folder Shape

- `docs/tasks/<module-name>/task.md`
- `docs/tasks/<module-name>/design.md`
- `docs/tasks/<module-name>/source-alignment.md`
- `docs/tasks/<module-name>/implementation-plan.md`
- `docs/tasks/<module-name>/test-validation-plan.md`
- `docs/tasks/<module-name>/review-checklist.md`

## Operating Workflow

1. planning agent writes the module design documents
2. planning agent writes `task.md`
3. Claude Code reads `task.md` and companion docs
4. Claude Code implements the module
5. planning agent reviews the implementation

This is the standard workflow going forward.
