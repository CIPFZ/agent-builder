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

### [x] Phase 3: Single-scroll conversation

- Remove the process list max height and nested vertical overflow.
- Move large compact summaries and complete outputs to explicit detail surfaces.
- Preserve stable conversation scroll behavior while content streams or expands.

Review notes:

- `.chatContent` is the only vertical scroll owner for the conversation.
- Removed max-height and vertical overflow from process narration, Compact
  summaries, and tool output details.
- Added `conversation-scroll-container` and `process-stream` test identifiers.
- Markdown code blocks retain horizontal overflow without a height cap.
- Terminal and right-side tool panels retain their independent scroll ownership
  because they are separate application surfaces, not nested conversation flow.
- Verified with build, ESLint, phase 07 structural smoke, conversation output
  smoke, streaming smoke, and a residual overflow scan.

### [x] Phase 4: Process disclosure policy

- Expand only running and permission-waiting processes by default.
- Collapse completed, partially failed, failed, interrupted, and cancelled turns.
- Preserve explicit user expand/collapse choices during the turn lifecycle.

Review notes:

- Authoritative terminal Turn states always default to collapsed, even when
  exploration counters report failures or a stale child item remains running.
- Running, queued, streaming, and permission-waiting processes default to open.
- Failed, partially failed, interrupted, cancelled, denied, and completed
  processes default to collapsed.
- Explicit user choices remain latched for the current Turn and reset when the
  Turn id changes.
- Error-tone TraceRows no longer auto-expand; error state is communicated by
  icon, text, border, and the process summary color.
- Verified with policy smoke assertions, build, ESLint, phase 07 structural
  smoke, conversation output smoke, streaming smoke, and code review.

### [x] Phase 5: Tool disclosure hierarchy

- Keep tool groups collapsed by default.
- Keep individual tool details collapsed by default.
- Keep permission actions directly visible when user input is required.
- Show compact status, duration, target, count, and failure summaries.

Review notes:

- Timeline tool groups render through `tool-group-disclosure` and default to
  collapsed for completed, running, and failed states.
- Expanding a group reveals `tool-item-disclosure` rows; every individual tool
  detail starts collapsed and toggles independently.
- Legacy single/group tool entry points were aligned to the same default-closed
  policy, including the all-quiet group path.
- Permission actions remain in the ConversationDock outside process/tool
  disclosures. Active sessions no longer fall back to a permission request
  belonging to another session.
- Added stable `tool-detail`, tool id/kind/status, and group status markers for
  browser-level verification.
- Verified with build, ESLint, phase 07 structural smoke, conversation output
  and streaming smokes, and code review.

### [x] Phase 6: Large-output protection

- Truncate commands to a single-line summary when collapsed.
- Show bounded stdout/stderr excerpts.
- Summarize reads, searches, diffs, and artifacts.
- Load full output through a drawer or runtime output reference.

Review notes:

- Tool output/error previews are bounded to 24 lines and 6000 characters.
- Failure excerpts outside disclosures are bounded to two lines and 320
  characters.
- Long commands and details use bounded previews; copy actions retain the full
  original text.
- Truncated content exposes an explicit "view full content" action that opens a
  separate Drawer instead of expanding the conversation document.
- Target lists are capped at ten visible entries with a remaining-count summary.
- Output, artifact, and diff references continue to render as compact counts.
- Added pure preview-policy assertions and structural smoke coverage.
- Verified with build, ESLint, conversation output/streaming smokes, phase 07
  structural smoke, and code review.

### [x] Phase 7: Visual system alignment

- Normalize typography, spacing, colors, and responsive layout.
- Avoid large failure backgrounds and excessive borders.
- Keep process narration secondary and final responses primary.

Review notes:

- Final assistant responses now use the primary text token and the shared
  760px reading column; process narration uses the same column with secondary
  typography so the answer remains the visual focus.
- Removed process-panel borders and large error/warning fills. Failed process
  rows and tools now use compact icons, text, and a two-pixel side marker.
- Failed tools no longer expose error excerpts while collapsed; full error
  content remains available inside the individual tool disclosure and Drawer.
- Reduced tool-detail card nesting, normalized neutral surfaces to Ant Design
  theme tokens, and kept disclosure chevrons discoverable without hover.
- Added responsive spacing and indentation rules for 720px and 480px layouts,
  including single-column detail metadata on narrow screens.
- Added structural smoke assertions preventing large failure backgrounds and
  preserving primary/secondary text-token hierarchy.
- Verified with frontend build, ESLint, phase 07 structural smoke, conversation
  output/streaming smokes, diff validation, and independent UI/code review.

### [x] Phase 8: End-to-end review and verification

- Verify no-tool, multi-tool, failed-tool, permission, interrupted, Todo, reconnect,
  long-output, historical-turn, and responsive-layout scenarios.
- Run frontend build/lint/smokes and relevant Go tests.
- Complete a final code review and update this document.

Review notes:

- Consolidated React-to-Runtime transport on Wails 3. Session output and
  global runtime updates now use dedicated Wails application event streams;
  HTTP, SSE, polling fallbacks, endpoint/token APIs, and browser HTTP harnesses
  were removed.
- Removed the embedded Runtime HTTP/SSE servers and replaced the generic SSE
  broadcaster with an in-process event broker consumed by the Wails bridge.
- Hardened output convergence: foreign-session events are rejected, terminal
  tools/permissions/items cannot regress to active states, terminal turns
  reject late text deltas, and duplicate/out-of-order deltas remain separate
  for reducer-level idempotency checks.
- Pending permissions owned by terminal turns are no longer actionable. Todo
  state with an explicit unknown turn id no longer falls back to the newest
  turn and cannot restart its spinner.
- Assistant messages persist their owning turn id; final-message lookup is
  constrained to that turn so a failed later turn cannot reuse an earlier
  answer.
- Generalized bounded text previews and applied them to tool output, compact
  summaries, workflow notices, and context notices.
- Removed obsolete HTTP/browser scripts, route contracts, CLI `serve-http`,
  and HTTP profiling startup. Provider, MCP, and download networking remain
  external capabilities and are not React-to-Runtime transports.
- Verified with frontend build and ESLint; conversation output, streaming,
  Phase 07, and Markdown smokes; desktop/runtime focused tests; `go test ./...`;
  `go build ./...`; dependency tidy; diff validation; and independent reviews.
- Final hands-on validation should run the packaged Wails desktop app and focus
  on visual preference, wheel/expansion feel, and real provider/permission
  flows; browser/Vite mode is intentionally not a supported runtime path.

Post-completion review:

- Closed the Wails stream handshake window by registering listeners on stable
  event names before starting streams with client-generated stream ids.
- Removed desktop delta coalescing; batching now preserves every fragment for
  the reducer's `contentLen` duplicate/out-of-order checks.
- Added per-entity event sequence watermarks so stale item, message, turn,
  tool, permission, result, step, and agent-task events cannot overwrite newer
  state, including terminal-to-terminal races and stale deletes.
- Added regression coverage for listener-before-start ordering, Wails-only
  transport, preserved delta fragments, and entity sequence monotonicity.
- Replaced the draft/session inference and race workaround with one explicit
  conversation target. Starting a new conversation is now a local draft only;
  its first submit omits `sessionId`, then atomically records the runtime's
  returned id. Every later submit must use that id and can only create a Turn.
- Serialized consecutive draft submits across that atomic transition, so two
  immediate sends issue one session-creating request followed by one request
  addressed to the returned session. Removed adapter-owned assistant loading;
  Wails session-output events now exclusively create and settle assistant rows.
- Added regression coverage for `2 sends = 1 Session / 2 Turns`, preservation
  of the optimistic user request while adopting the returned session id, and
  Wails-driven assistant loading convergence.
- Redesigned the empty conversation surface as one centered starting workspace
  instead of separating the heading and bottom composer. The introduction,
  workflow cue, composer, model controls, and project context now read as one
  responsive visual group; active conversations retain the existing layout.
- Replaced the generic exploration/processing pair with phase-aware process
  copy: thinking, tool use, reply composition, and permission waiting. Empty
  active progress/narration placeholders are suppressed so one turn displays
  one status line until real process content is available.
- Simplified the empty-state introduction by removing the decorative mark and
  low-value workflow labels. The draft composer now uses one shared outer
  border and focus treatment, with the project selector attached as its footer
  instead of rendering a second framed Sender inside a card.
- Scoped the combined Sender/project-context treatment strictly to local draft
  conversations; active and historical sessions render the standard Sender
  alone. Opening or creating a project now atomically rebinds the local draft
  target to the hydrated project id instead of retaining standalone scope.
- Closed the first-submit output hydration gap by reading the new session's
  Wails `SessionOutput` snapshot immediately after the runtime returns its id,
  then subscribing from that cursor. Removed the synthetic assistant loading
  row so missed handshake events cannot leave "generating" stranded onscreen.
- Restored TodoWrite visibility on the Wails-only output path. Structured todo
  summaries now hydrate into `OutputStore`, update from live `todo.updated`
  events, and project directly into the Todo task bar instead of depending on
  a later full-workbench refresh that the session stream intentionally skips.
- Removed the duplicate active-session Todo state from `WorkbenchViewModel`,
  Shell event handling, and adapter `SessionTodos` hydration. Workspace now
  selects Todo state only from the active session's OutputStore; ownership
  checks reject stale stores during session switches while the standalone Go
  query remains available for diagnostics and non-UI consumers.
