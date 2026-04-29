# P1.5 Implementation Plan

Date: 2026-04-29

## Phase 1: Create Closure Docs

Create the P1.5 task folder with the standard files:

- `README.md`
- `task.md`
- `design.md`
- `source-alignment.md`
- `implementation-plan.md`
- `test-validation-plan.md`
- `review-checklist.md`

Acceptance:

- P1.5 is clearly documented as P1 closure and P2 readiness gate.
- P2/P3 scope is explicitly excluded.

## Phase 2: Integration Audit

Audit current implementation across:

- commands
- queryengine
- runtime
- gateway
- TUI
- session/store/recovery
- approval
- agent
- prompt/context cache
- tools/extension inventory

Acceptance:

- every required P1.5 integration area is either covered by existing tests or receives a new focused test.
- any discovered behavior bug is fixed only after a failing test.

## Phase 3: Test Coverage Top-Up

Add focused tests for closure gaps. Priority:

- runtime command metadata through gateway `extension_inventory`
- configured command override dedupe
- extension inventory deterministic rebuild
- recovered subagent isolation metadata
- context cache invalidation for system/user/read-file/prompt override inputs

Acceptance:

- new tests run in focused package commands.
- no existing tests are removed or weakened.

## Phase 4: P2 Roadmap

Create `docs/tasks/p2-runtime-expansion-roadmap/` with the standard files.

Acceptance:

- P2.1-P2.5 are split with entry criteria, acceptance criteria, and test requirements.
- P2 explicitly preserves P0/P1 stable contracts.

## Phase 5: Roadmap Status Update

Update P1 roadmap companion docs.

Acceptance:

- P1.1-P1.4 completed capabilities are listed based on their review checklists.
- P1.5 closure gate and P2 handoff are recorded.
- no untested or unimplemented P2/P3 capability is claimed as complete.

## Phase 6: Verification And Commit

Run required validation commands and `git status`.

If all required validation passes, commit the work.

