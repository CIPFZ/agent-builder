# Conversation UI Redesign Plan

This document tracks the conversation UI redesign. Update the status and review
notes after every phase so implementation state remains recoverable outside the
current Codex task.

## Status legend

- `[ ]` Pending
- `[~]` In progress
- `[x]` Completed and reviewed
- `[!]` Blocked; see the phase notes

## Design decisions

- Runtime turns remain the authoritative user/process/final response boundary.
- The conversation uses one vertical scroll container.
- Process narration and final responses use the same Markdown reading style.
- The process has one outer disclosure; narration is not individually collapsed.
- Tool groups and individual tool details remain independently collapsible and
  default to collapsed.
- Completed, failed, interrupted, and cancelled processes default to collapsed.
- Running and permission-waiting processes default to expanded.
- Failures use compact color/icon indicators instead of large expanded panels.

## Phases

### [x] Phase 1: Todo and turn-state convergence

Goals:

- Derive visible Todo totals from Todo items instead of stale summary counters.
- Never show a running spinner after the owning turn reaches a terminal state.
- Hide the Todo bar when all items are complete and the turn is complete.
- Show a static stopped/stale state when a terminal turn still has incomplete
  Todo data.
- Prevent Todo state from a previous turn or session leaking into the current UI.

Acceptance cases:

- `12/12 + completed` hides the Todo bar.
- `summary 11/12 + items 12/12` is treated as complete.
- `in_progress + completed` does not spin.
- `in_progress + failed/interrupted/cancelled` is static.
- `in_progress + running` still spins.
- Switching sessions does not retain the previous turn's running Todo state.

Review notes:

- Todo totals and completed counts are derived from `items`; stale runtime
  summary counters no longer control UI progress.
- Todo display is resolved against its owning or latest runtime turn.
- Terminal turns never render Todo spinners. Fully completed plans are hidden;
  incomplete terminal plans render a static stopped state.
- Todo data without a matching turn is hidden as stale.
- Session hydration continues to retain Todo data only when its `sessionId`
  matches the active session.
- Verified with frontend build, ESLint, conversation output smoke, conversation
  streaming smoke, and phase 07 runtime-rendering smoke.

### [x] Phase 2: Continuous process reading flow

- Remove the process rail, dots, and connecting lines.
- Replace Thinking cards and intermediate assistant cards with inline process
  narration using the shared Markdown presentation.
- Keep the final response visually stronger without changing its reading width.

Review notes:

- Removed the process rail, dots, connecting lines, and the old Thinking
  Collapse component.
- Thinking and intermediate assistant messages now share `ProcessNarration`
  and render as continuous Markdown in runtime sequence order.
- The Turn-level process disclosure remains the only narration disclosure.
- Empty running thinking renders a lightweight status line; empty settled
  thinking is omitted. React-callchain title-only entries remain visible.
- The process stream no longer has its own max height or vertical overflow.
- Verified with frontend build, ESLint, conversation output/streaming smokes,
  phase 07 structural smoke, and a dedicated code review.

### [ ] Phase 3: Single-scroll conversation

- Remove the process list max height and nested vertical overflow.
- Move large compact summaries and complete outputs to explicit detail surfaces.
- Preserve stable conversation scroll behavior while content streams or expands.

Review notes: Pending.

### [ ] Phase 4: Process disclosure policy

- Expand only running and permission-waiting processes by default.
- Collapse completed, partially failed, failed, interrupted, and cancelled turns.
- Preserve explicit user expand/collapse choices during the turn lifecycle.

Review notes: Pending.

### [ ] Phase 5: Tool disclosure hierarchy

- Keep tool groups collapsed by default.
- Keep individual tool details collapsed by default.
- Keep permission actions directly visible when user input is required.
- Show compact status, duration, target, count, and failure summaries.

Review notes: Pending.

### [ ] Phase 6: Large-output protection

- Truncate commands to a single-line summary when collapsed.
- Show bounded stdout/stderr excerpts.
- Summarize reads, searches, diffs, and artifacts.
- Load full output through a drawer or runtime output reference.

Review notes: Pending.

### [ ] Phase 7: Visual system alignment

- Normalize typography, spacing, colors, and responsive layout.
- Avoid large failure backgrounds and excessive borders.
- Keep process narration secondary and final responses primary.

Review notes: Pending.

### [ ] Phase 8: End-to-end review and verification

- Verify no-tool, multi-tool, failed-tool, permission, interrupted, Todo, reconnect,
  long-output, historical-turn, and responsive-layout scenarios.
- Run frontend build/lint/smokes and relevant Go tests.
- Complete a final code review and update this document.

Review notes: Pending.
