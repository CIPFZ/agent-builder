# Long Conversation Hardening Plan

## Goal

Make Agent Builder reliably handle long conversations and long-running agent
tasks without rushing into a large runtime rewrite.

The near-term goal is not a new product mode. It is to make the current
session/turn/tool timeline trustworthy enough for engineering-scale tasks:

- The user can see what the agent is doing while it runs.
- Refresh or desktop restart does not lose the visible process.
- Tool failures, denied permissions, and missing artifacts are obvious.
- The runtime, not React, remains the source of truth.
- Each improvement has focused tests and a real long-session validation case.

## Current Assessment

The current implementation is strong enough for medium and large local tasks,
based on stress runs against the runtime HTTP adapter.

Validated:

- Multi-turn continuity works.
- Long report generation can use `write` with `mode: "append"`.
- Tool calls are persisted and can be reconstructed into the timeline.
- Tool display metadata now covers file read, file search, file write,
  file edit, shell, and generic tools.
- The frontend can render interleaved assistant messages and tool calls.
- The frontend can recover the timeline from `SessionActivity` after refresh.
- Runtime event refresh exists, with polling fallback when `EventSource` is
  unavailable.

Known weak spots:

- Runtime events currently trigger hydration, but the UI still depends on
  periodic refresh for full state. This is acceptable now, but not ideal for
  very long tasks.
- Tool detail quality is uneven. Some cards show only generic summaries instead
  of clear command, target path, output excerpt, or artifact path.
- Phase 1 now detects requested artifacts that are not represented by produced
  tool artifacts, but it does not yet perform a final filesystem existence
  check.
- Produced artifact detection is still conservative. It handles `write` and
  file-write metadata, but shell-created files and custom MCP artifact
  references need first-class runtime extraction.
- There is no user-facing turn diagnostics panel that explains why a turn is
  running, waiting, failed, denied, or completed with missing expected output.
- Recovery is durable enough for turns and tool calls, but not yet a full
  checkpoint/resume model.
- A full Run state machine remains intentionally deferred until turns,
  diagnostics, artifacts, events, and interrupted-state UX are stable.

## Problems To Solve

### P0: Completed Turn With Missing User-Requested Artifact

Observed during stress validation:

- The agent completed a turn after command/tool issues.
- The requested `command-validation.md` was not produced.
- The UI had no explicit signal that the task objective was incomplete.

Impact:

- Users may trust a completed state even when the requested deliverable is
  missing.
- Long engineering tasks need artifact-level confirmation, not only turn-level
  completion.

Direction:

- Add a lightweight artifact expectation mechanism before building a full task
  state machine.
- Capture expected output paths when the user explicitly asks for a file path,
  or when the model uses `write`.
- Surface "expected artifact missing" in the turn summary and timeline.
- After the conservative DTO-based Phase 1 slice, add a filesystem-backed
  existence check for explicit local expected artifacts.

### P0: Artifact Production Is Too Narrow

Current behavior:

- Phase 1 treats completed `write`/file-write calls as produced artifacts.
- Shell commands can create files without structured artifact metadata.
- Custom MCP tools can return artifact refs or path-like outputs that are not
  normalized into runtime produced artifacts yet.

Impact:

- Successful shell/MCP file creation may still look missing.
- Diagnostics can become less trustworthy for non-`write` workflows.

Direction:

- Normalize produced artifact extraction in runtime, not React.
- Prefer structured refs and known tool metadata over parsing prose.
- Add conservative shell output detection only for explicit local paths in
  commands whose action clearly creates or writes files.
- Add MCP artifact/path extraction from structured outputs and refs when the
  tool result provides machine-readable targets.

### P1: Real-Time Progress Is Still Hydration-Based

Current behavior:

- Runtime emits events.
- Frontend receives events or falls back to polling.
- Event receipt triggers `adapter.refresh()`.
- The UI rebuilds from `SessionActivity`.

This is safe and preserves runtime ownership, but it can be coarse during heavy
tool bursts.

Direction:

- Keep `SessionActivity` as the source of truth.
- Add a small runtime event cursor to the client so refreshes can be throttled
  by event type.
- Refresh immediately for visible state changes:
  - `turn.started`
  - `tool.call.started`
  - `tool.call.completed`
  - `tool.call.failed`
  - `permission.requested`
  - `turn.completed`
  - `turn.failed`
- Coalesce high-frequency message delta events.

### P1: Tool Details Are Not Yet Actionable Enough

Current behavior:

- Tool cards are readable and compact.
- The title and status are clear.
- Expanded details are still inconsistent depending on the stored tool input
  and output.

Needed:

- File tools should show normalized target paths.
- Shell tools should show command, working directory, exit code, duration,
  stdout excerpt, stderr excerpt, and copy action.
- Failed tools should open by default and show the failure reason.
- Write/edit tools should expose artifact/diff refs when available.
- Grouped cards should make it clear which individual call failed.

### P1: Permission And Policy Outcomes Need Better Recovery UX

Current behavior:

- Pending permissions can render inline.
- Resolved permissions are hidden from the main timeline.

Risk:

- For long tasks, hidden denied/allowed decisions can make the final behavior
  hard to understand after refresh.

Direction:

- Keep resolved permission cards hidden by default.
- Add a compact "permission decisions" line in expanded tool details when a
  tool had a decision.
- Add a turn diagnostics summary that counts pending, allowed, denied, and
  expired permissions.

### P2: Recovery Is Not Yet Checkpoint/Resume

Current behavior:

- Unfinished persisted tool calls are cancelled on runtime startup.
- Active/interrupted turns are recoverable enough for display.

This is safer than stale running state, but it does not resume a long task.

Direction:

- First make interrupted state explicit and user-facing.
- Then add checkpoints for future runs:
  - last completed step
  - created artifacts
  - failed tool call
  - pending permission
  - compacted context boundary
- Resume should be a deliberate user action, not automatic replay.

### P2: Full Run State Machine Is Deferred

Current behavior:

- Turns and tool calls are now durable enough for the current timeline.
- `RuntimeAgentTask` persistence exists as a foundation, but there is no
  product-level Run object that spans multiple turns.

Direction:

- Do not introduce Run until Phase 1.1 through Phase 5 are stable.
- First define the minimum Run contract from observed long-task needs:
  objective, expected artifacts, turn ids, task ids, checkpoints, final
  verification state, and user-controlled resume/discard actions.
- Keep Run additive and transport-neutral. It must not move runtime state into
  React or recreate CLI/TUI assumptions.

## Implementation Plan

### Phase 1: Diagnostics And Artifact Expectations

Status: implemented for the conservative runtime slice.

Scope:

- Add runtime-level diagnostics for each turn:
  - expected artifacts
  - produced artifacts
  - missing artifacts
  - denied tool count
  - failed tool count
  - last tool status
- Initially derive expected artifacts conservatively:
  - explicit absolute paths in the user prompt ending in common document/code
    file extensions
- Initially derive produced artifacts conservatively from paths passed to
  completed `write`/file-write tools.
- Expose diagnostics through existing turn/session activity DTOs.
- Render a compact diagnostics row only when there is a warning.

Implemented:

- `RuntimeTurn` now exposes `diagnostics` with expected, produced, and missing
  artifacts, failed/denied tool counts, last tool status, and warning text.
- Expected artifacts are extracted from explicit local file paths in the turn
  prompt preview and persisted user message content.
- Produced artifacts are extracted conservatively from completed `write` and
  `download` style file-write tool calls and their display targets/artifact
  refs.
- Completed turns with missing expected artifacts surface a runtime warning
  through `Turn` and `SessionActivity`.
- The React timeline maps the runtime DTO into a lightweight warning row only
  when `diagnostics.warning` is present.

Acceptance:

- A completed turn that was asked to write a file but did not create it shows
  a visible warning.
- Existing successful stress runs do not show false warnings.
- Refresh reconstructs the same diagnostics.

Tests:

- Go tests for artifact expectation extraction.
- Runtime tests for completed turn with missing artifact.
- Frontend test or browser smoke for warning row rendering.

Validation:

- `go test ./internal/runtime`
- `go test ./...`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- Runtime HTTP/Vite stress validation:
  - Success case created
    `tmp/runtime-dev/phase1-success/diagnostics-success.md`; diagnostics
    included the path in expected and produced artifacts with no warning.
  - Missing case used
    `tmp/runtime-dev/phase1-missing-no-tool/diagnostics-missing-no-tool.md`;
    the file remained absent and diagnostics returned `missingArtifacts` plus
    warning `expected artifact was not produced`.
  - A first attempted missing case showed the model may create the file despite
    an instruction not to; this is a validation risk, not a diagnostics bug.
- In-app browser verification on `http://127.0.0.1:5175/`:
  - Missing warning row rendered as "期望文件未生成".
  - Warning survived refresh from `SessionActivity`.
  - Successful artifact session showed no warning.
  - Tool cards and assistant messages remained interleaved and left-aligned.

Remaining risks:

- Artifact matching is intentionally path/string based and checks produced tool
  artifacts, not filesystem existence after the turn.
- Expected artifact extraction only handles obvious local paths with common file
  extensions.
- Produced artifact extraction is conservative and may miss custom MCP tools or
  shell commands that create files without `write`/`download` metadata.

### Phase 1.1: Artifact Verification Hardening

Status: implemented for the conservative runtime slice.

Scope:

- Add a runtime-only final existence check for explicit local expected
  artifacts when a turn reaches a final status.
- Keep the check conservative:
  - only local absolute paths already accepted as expected artifacts
  - no glob expansion
  - no recursive directory scanning
  - no network or virtual URI checks
- Extend diagnostics with artifact verification details:
  - `verifiedArtifacts`
  - `unverifiedArtifacts`
  - `missingArtifacts`
  - `artifactVerificationAt`
  - warning reason/source
- Distinguish "not produced by a tool" from "not present on disk" so users can
  tell whether metadata or actual output is missing.
- Add conservative produced-artifact extraction for:
  - shell commands that clearly write a specific local file path
  - shell stdout/stderr/result refs that expose structured artifact refs
  - MCP/custom tool structured output fields that carry explicit artifact refs
    or local path targets

Implemented:

- `RuntimeTurnDiagnostics` now includes `verifiedArtifacts`,
  `unverifiedArtifacts`, `artifactVerificationAt`, `warningReason`, and
  `warningSource`.
- Final turn diagnostics perform a conservative `os.Stat` existence check only
  for explicit local absolute expected artifact paths already accepted by
  Phase 1 extraction.
- Missing expected artifacts now distinguish:
  - no produced tool metadata:
    `warningReason=expected_artifact_not_produced`,
    `warningSource=tool_metadata`
  - produced tool metadata exists but the file is absent:
    `warningReason=produced_artifact_missing_on_disk`,
    `warningSource=filesystem`
  - produced tool metadata exists and the file exists:
    the path is listed under `verifiedArtifacts` with no warning
- Produced artifact extraction remains conservative and now also covers:
  - explicit shell writes through redirection and clear PowerShell write/create
    commands such as `Set-Content`, `Out-File`, `Add-Content`, and `New-Item`
  - structured MCP/custom output fields and artifact refs that carry explicit
    local path targets
- React continues to render a diagnostics row only when runtime diagnostics
  include `warning`; warning copy is selected from runtime reason/source fields
  rather than frontend artifact inference.

Acceptance:

- A file created by shell redirection or a safe shell write command is counted
  as produced and verified when the file exists.
- A completed turn with an explicit local expected file that does not exist on
  disk still shows a missing artifact warning even if the assistant claims it
  was created.
- A completed turn with an expected file created by `write` but later missing
  on disk reports the filesystem absence separately from the tool production
  metadata.
- Custom MCP outputs with structured artifact refs can contribute produced
  artifacts without React parsing tool text.

Tests:

- `go test ./internal/runtime`
  - expected artifact present on disk enters `verifiedArtifacts`
  - expected artifact absent with no produced metadata warns from
    `tool_metadata`
  - produced metadata present but disk file absent warns from `filesystem`
  - shell-created explicit file paths count as produced artifacts
  - MCP/custom structured artifact refs count as produced artifacts
  - existing failed/denied tool counts remain covered
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`

Validation:

- Runtime server ran on `http://127.0.0.1:5186` with desktop root fixed to
  `tmp/runtime-dev`; Vite ran on `http://127.0.0.1:5176`.
- Test artifact files were kept under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- Validation session:
  `phase11-artifact-verification`.
- Cases verified through `SessionActivity` and the in-app browser:
  - `phase11-write-verified.md`: write metadata + file exists,
    `verifiedArtifacts` populated, no warning
  - `phase11-shell-verified.md`: shell-created explicit path + file exists,
    `verifiedArtifacts` populated, no warning
  - `phase11-produced-missing.md`: produced metadata exists but file is absent,
    warning says the tool reported the file but disk is missing
  - `phase11-no-produced-missing.md`: no produced metadata and file is absent,
    warning says the expected file was not produced by a tool
  - browser refresh restored the same timeline warnings from runtime activity
  - tool cards and assistant messages remained interleaved and left-aligned in
    the timeline

Remaining risks:

- Shell detection is intentionally narrow and will miss complex shell scripts,
  variable-derived paths, pipeline-heavy commands, and non-PowerShell write
  helpers beyond explicit redirection.
- Structured MCP/custom extraction only trusts JSON-like machine-readable
  fields and artifact refs. It does not infer produced files from prose.
- Filesystem verification reports regular files only; directories are not
  treated as verified artifacts in this phase.
- Verification time is computed when diagnostics are built, so repeated
  activity hydration can refresh `artifactVerificationAt` without changing the
  underlying turn state.

### Phase 2: Event Refresh Tuning

Status: implemented.

Scope:

- Keep the current event-triggered hydration path.
- Track the latest event sequence in the adapter.
- Coalesce message/token events.
- Refresh immediately for tool/permission/turn lifecycle events.
- Keep busy polling as fallback only.

Acceptance:

- During long tasks, tool cards appear without waiting for turn completion.
- EventSource unavailable still works through `/v1/events` polling.
- No duplicate timeline items after refresh.

Tests:

- Adapter unit tests for event coalescing where practical.
- Browser validation with a long task and forced reload.
- Runtime HTTP SSE test already exists; extend only if sequence behavior changes.

Implemented:

- Runtime events remain refresh triggers only; `SessionActivity` remains the
  timeline source of truth.
- `/v1/events` history and SSE both support `after=<sequence>` cursor replay.
- The desktop bridge now forwards the event cursor to `RuntimeService.Events`.
- Runtime HTTP SSE tests cover named `runtime-event` delivery, history replay
  after a cursor, monotonic sequence order, and lifecycle linkage fields.
- The frontend adapter now keeps the latest event sequence across SSE and
  polling subscriptions.
- EventSource uses the named `runtime-event` listener and reconnects with the
  latest cursor.
- Polling fallback requests `/v1/events?after=<sequence>` instead of fetching
  the whole event history each cycle.
- Runtime event DTO mapping accepts the backend snake_case fields
  `session_id`, `turn_id`, `tool_call_id`, and `created_at`.
- Frontend refresh strategy now refreshes immediately for turn/tool/permission
  lifecycle and artifact/diagnostic-like events, while coalescing message,
  token, usage, and progress events.
- Busy polling is still present as a fallback while active turns are running.
- Pending permission rendering is de-duplicated by permission id when the same
  runtime permission appears more than once in hydrated activity.

Validation:

- `go test ./internal/runtime`
- `go test ./desktop`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- Runtime HTTP/Vite validation used only files under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- In-app browser validation ran against `http://127.0.0.1:5177/` with runtime
  HTTP on `http://127.0.0.1:5187`.
- A local fake OpenAI-compatible server under `tmp/runtime-dev` was used for
  deterministic runtime turns without external API keys.
- Event refresh strategy was validated through the Vite-served module:
  lifecycle/permission/artifact events return immediate refresh, while
  message/token events return coalesced refresh.
- Fake unreachable provider turn:
  - `turn.started` refreshed the UI into active/running state.
  - `turn.failed` refreshed the final failed state.
  - `/v1/events?after=68` returned only sequence `69` (`turn.failed`).
- Missing artifact completed turn:
  - requested
    `tmp/runtime-dev/phase2-missing-artifact.md`
  - fake model completed without tool output
  - diagnostics warning rendered and survived page reload from
    `SessionActivity`
- Permission/tool validation:
  - fake model requested a `write` tool call for
    `tmp/runtime-dev/phase2-tool-card.md`
  - tool card appeared before turn completion
  - pending permission appeared immediately
  - approving once completed the tool, hid the permission, wrote the file, and
    completed the turn
- Burst validation:
  - fake model requested two `write` tool calls in one burst
  - UI rendered a single grouped completed tool card with both tool ids
  - no pending permission duplication or top-level timeline duplication was
    observed
  - `tmp/runtime-dev/phase2-burst-a.md` and
    `tmp/runtime-dev/phase2-burst-b.md` were created
- Remaining-risk closure validation:
  - `go build ./desktop` regenerated the Wails bridge surface and confirmed the
    desktop `Events(after)` binding forwards the cursor.
  - `desktop\scripts\phase2-smoke.ps1 -Build` passed against the rebuilt
    desktop/runtime smoke path.
  - A deterministic long fake-model run created four `write` tool calls and
    delayed the final assistant response for 10 seconds after tool completion.
  - `SessionActivity` for session
    `b9829920-7a97-4d49-80a9-1ab528c473f5` restored one turn, seven messages,
    and four completed tool calls:
    `call_phase2_long_a`, `call_phase2_long_b`, `call_phase2_long_c`, and
    `call_phase2_long_d`.
  - After selecting that session and reloading `http://127.0.0.1:5177/` in the
    in-app browser, the timeline restored the final assistant message and a
    single grouped completed tool card without duplicate top-level items.
  - A follow-up browser reload after the pending-permission de-duplication fix
    produced no new console errors and still restored the same single grouped
    tool card.
  - The deterministic long-run artifacts
    `tmp/runtime-dev/phase2-long-a.md` through
    `tmp/runtime-dev/phase2-long-d.md` were present on disk.

Remaining risks:

- The frontend still hydrates the whole active view model on visible lifecycle
  events. This is intentional for Phase 2, but very large sessions may still
  need narrower session-scoped refresh later.
- EventSource validation used the Vite/browser path, runtime SSE tests, and the
  regenerated Wails binding smoke. A packaged production installer smoke is
  still useful before release.
- Synthetic fake-model validation now covers deterministic long tool bursts,
  permission display, reload recovery, and diagnostics recovery. A real
  external model long-running reasoning session remains dependent on provider
  credentials and should be repeated before calling Phase 2 production-ready.

### Phase 3: Tool Detail Normalization

Status: implemented for the current runtime DTO and ToolCallCard surface.

Scope:

- Add richer runtime display fields without a database migration:
  - `workingDir`
  - `primaryTarget`
  - `targets`
  - `stdoutExcerpt`
  - `stderrExcerpt`
  - `failureReason`
  - `artifactCount`
  - `diffCount`
- Normalize artifact-capable tool metadata:
  - shell command working directory and explicit output targets
  - shell-created artifact refs from safe, structured detections
  - MCP/custom tool artifact refs and local path targets from structured output
  - primary target selection for grouped tool cards
- Update `ToolCallCard` expanded details to use these fields first.
- Keep raw summaries behind copy/expand affordances, not as the default text.

Acceptance:

- Shell cards explain command, cwd, exit, duration, stdout/stderr excerpt.
- File cards show the actual path first.
- Failed cards open by default and clearly show error output.
- Shell/MCP-produced artifacts are visible as structured runtime metadata when
  the runtime can identify them conservatively.
- Grouped tool cards show which individual call produced, failed, or denied a
  target artifact.

Tests:

- Runtime metadata tests for shell/read/write/search/edit cases.
- Runtime metadata tests for MCP/custom artifact refs.
- Browser check for failed command, long stdout, and grouped reads.
- Browser check for shell-produced and MCP-simulated artifact metadata.

Implemented:

- `RuntimeToolCallDisplay` now exposes additive display metadata without a
  database migration:
  - `workingDir`
  - `primaryTarget`
  - `targets`
  - `stdoutExcerpt`
  - `stderrExcerpt`
  - `inputExcerpt`
  - `outputExcerpt`
  - `failureReason`
  - `artifactCount`
  - `diffCount`
  - display-level artifact and diff refs/summaries
- Runtime tool kind normalization is now stable for:
  - `file_read`
  - `file_write`
  - `file_edit`
  - `file_search`
  - `shell`
  - `generic`
- Runtime classification no longer uses generic `read`/`write` input-summary
  keyword fallback to decide primary kind.
- Shell tools stay `shell` even when the command contains file-tool keywords.
- MCP/plugin/custom tools default to `generic`; structured artifact refs and
  local path targets are extracted conservatively from machine-readable output.
- File targets are normalized into `primaryTarget` and `targets`; multi-target
  JSON fields are supported for paths, files, targets, artifact refs, and diff
  refs.
- Shell display metadata includes command, cwd, exit code, duration,
  stdout/stderr excerpts, failure reason for failed or nonzero-exit shell
  calls, sandbox/policy refs already present on the call, and conservative
  shell-created artifact targets.
- File edit display includes real diff ref counts when available and a
  conservative `diffCount=1` for structured edit summaries that expose
  additions/removals/edits-applied but not raw diff refs.
- `ToolCallCard` now reads runtime display metadata first for title, kind,
  detail, target, targets, command, cwd, exit code, duration, stdout/stderr
  excerpt, failure reason, artifact count, and diff count.
- Legacy fallback remains for old hydrated tool calls.
- Nonzero shell exit codes render as failed visual status and open by default,
  even when the persisted tool status is `completed`.

Validation:

- `go test ./internal/runtime`
- `go test ./...`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- Runtime HTTP/Vite validation used only files under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- Runtime HTTP ran on `http://127.0.0.1:5189`; Vite ran on
  `http://127.0.0.1:5179`; a local fake OpenAI-compatible server ran on
  `http://127.0.0.1:5193`.
- Validation session:
  `0ba53bcb-1774-4196-a939-c0c791d6d95c`.
- Turn:
  `1780789254668-f0650ef246d730d8`.
- Cases verified through runtime `SessionActivity` and the in-app browser:
  - shell success card showed command, cwd, duration, stdout excerpt, and refs
  - shell nonzero exit card showed exit `7`, stderr excerpt, failed visual
    status, and was expanded by default
  - write card showed normalized target path and diff refs/count
  - view card for a path containing `write` stayed `file_read`
  - glob card stayed `file_search`
  - multiedit card showed target path, artifact refs, and `diffCount=1`
  - diagnostics warning for missing
    `tmp/runtime-dev/phase3-missing-warning.md` rendered and survived refresh
  - browser refresh restored cards from `SessionActivity`
  - no duplicate tool card ids were observed after refresh
- Follow-up validation after adding `failureReason` used runtime HTTP
  `http://127.0.0.1:5190`, Vite `http://127.0.0.1:5180`, and a fake
  OpenAI-compatible server on `http://127.0.0.1:5193`; all validation files,
  logs, and generated artifacts stayed under `tmp/runtime-dev`.
- Follow-up cases verified through runtime `SessionActivity` and the in-app
  browser:
  - session `b3d0e13f-5d0b-4138-9fea-576a85e54845` restored seven tool calls,
    including shell success, nonzero shell exit, write, read, glob,
    read-written-file, and multiedit calls
  - nonzero shell exit exposed `exitCode=7`, stderr excerpt, and
    `failureReason=phase3 shell stderr failed`
  - file/write/read/edit cards exposed concrete `tmp/runtime-dev` targets and
    artifact/diff counts
  - session `2a5cc1ed-81b4-420e-a416-060d1d9314a8` verified a long stdout
    excerpt capped around 2000 characters with a `truncated` marker and no
    horizontal page overflow in the expanded card
  - session `d672da86-addf-43fc-a6ca-e380a1c8a11f` verified the Phase 1/1.1
    missing artifact diagnostics warning and missing path survive browser
    refresh
  - browser-composer event-refresh smoke showed a completed tool card before
    the delayed final assistant response; after refresh, the expanded card
    restored command, cwd, output excerpt, refs, and had no duplicate card ids

Remaining risks:

- Shell-created artifact target extraction remains intentionally conservative
  and does not try to understand arbitrary scripts, variables, or pipelines.
- MCP/custom validation used runtime/API structured refs rather than a live
  external MCP server because no suitable enabled local MCP tool was available
  in this validation environment.
- Shell tools can persist `status=completed` with a nonzero exit code; the UI
  now treats that as failed visually, but a later runtime status refinement
  could make this explicit in the DTO.
- Very large historical sessions can take a short moment to hydrate after
  browser refresh; Phase 2's event refresh remains whole-activity hydration by
  design.

### Phase 4: Turn Diagnostics Panel

Status: implemented for the current `SessionActivity` diagnostics surface.

Scope:

- Add a lightweight right-side or inline diagnostics surface for the active
  turn.
- Show:
  - turn id
  - status
  - running duration
  - tool counts by status/kind
  - permission counts
  - expected/produced artifacts
  - last event time
- This should be read-only at first, except existing cancel action.
- Carry forward Phase 3 residual risk into diagnostics:
  - show shell nonzero exit as a first-class diagnostic even when the persisted
    tool status is still `completed`
  - distinguish produced output refs from user-relevant file artifacts so
    ordinary shell/read/search refs do not look like deliverables
  - summarize MCP/custom artifact confidence as structured refs only,
    unverified external MCP, or not detected
  - expose hydration cost and last refresh timing for very large sessions so
    whole-activity refresh latency is visible before optimizing it

Acceptance:

- User can answer "what is it doing now?" without reading every card.
- User can answer "why did it stop?" after failure or denial.
- Panel data comes from runtime DTOs only.
- Nonzero shell exits are visible in turn diagnostics independent of the
  persisted tool-call status value.
- Artifact counts in diagnostics separate runtime refs from local file
  deliverables.

Tests:

- Browser checks for active, completed, failed, and interrupted turns.
- Runtime/browser checks for nonzero shell exit diagnostics, ref-vs-file
  artifact summaries, and large-session refresh timing display.

Implemented:

- `RuntimeTurnDiagnostics` now exposes an additive read-only summary without a
  database migration:
  - turn/session/status identity
  - started, finished, completed duration, running duration, and computed time
  - tool counts by persisted status and normalized display kind
  - failed, denied, cancelled, and nonzero-exit shell signals
  - permission counts for pending, allowed, denied, expired, and cancelled
  - artifact counts separated into expected, produced, verified, missing,
    local deliverables, runtime refs, produced metadata refs, and structured
    refs
  - artifact confidence buckets for verified local files, produced tool
    metadata, runtime output refs, structured MCP/custom refs, and unknown/not
    detected
  - last tool id, status, and title
  - last runtime event time and sequence
  - existing warning, warning reason, and warning source fields
- `SessionActivity` computes diagnostics from persisted turns, tool calls,
  permission requests, and runtime event history. Runtime events remain refresh
  triggers only; React still rebuilds the timeline and diagnostics from
  hydrated activity.
- Nonzero shell exits are reported through
  `nonzeroExitShellCount` even when the persisted tool-call status is
  `completed`.
- The artifact summary distinguishes local file deliverables from runtime
  output refs so ordinary shell/read/search refs do not look like final files.
- MCP/custom confidence remains conservative and only counts structured
  machine-readable refs. The diagnostics path still does not parse assistant
  prose.
- The React workbench view model now carries `turnDiagnostics` selected from
  the active turn, or the latest turn when no turn is active.
- `TurnDiagnosticsPanel` renders a read-only panel next to the timeline on
  desktop and stacks above the timeline on narrow screens. It uses Ant Design,
  Ant Design icons, and a scoped CSS Module.

Validation:

- `go test ./internal/runtime`
- `go test ./...`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- Runtime HTTP/Vite validation used only files and logs under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- Runtime HTTP ran on `http://127.0.0.1:5194`; Vite ran on
  `http://127.0.0.1:5184`; the existing fake OpenAI-compatible server ran on
  `http://127.0.0.1:5193`.
- Browser validation session:
  `271394a4-f729-4788-a5cb-1a70c926c66a`.
- Browser and `SessionActivity` checks verified:
  - diagnostics panel rendered from the runtime DTO
  - completed turn status and duration displayed
  - seven grouped tool calls remained individually counted
  - tool counts showed `completed 7`
  - kind counts included shell, file write, file read, file search, and file
    edit
  - nonzero-exit shell was visible as `nonzero shell 1` while the persisted
    shell tool status remained `completed`
  - shell card still showed command output, exit `7`, and failure reason
    `phase3 shell stderr failed`
  - write/edit/read/search cards still showed concrete
    `tmp/runtime-dev` target paths, output refs, artifact counts, and diff
    counts
  - artifact summary separated one produced local file from 23 runtime refs
  - last tool and last runtime event sequence/time rendered
  - browser reload restored the timeline and diagnostics panel from
    `SessionActivity`
  - no duplicate tool ids were present after reload
- A delayed event-refresh browser smoke in session
  `20391e7a-5a4c-4244-841f-77687401840c` verified lifecycle refresh still
  updates tool cards and panel data for a shell turn, and restored last
  tool/event diagnostics from activity.
- Existing missing-artifact warning UI was rechecked through a recovered Phase
  3 session and remained visible alongside the diagnostics panel.

Remaining risks:

- The panel currently shows the active/latest turn, not an arbitrary selected
  historical turn inspector.
- Browser validation covered denied/missing warning display and backend
  permission-count recovery; an interactive browser run that deliberately
  leaves a permission pending should be repeated when the policy test fixture is
  separated from the full-access fake-provider smoke.
- `computedAt` is rebuilt during activity hydration, so it represents
  diagnostics build time rather than durable turn completion time.
- Large-session hydration remains whole-activity hydration by design from
  Phase 2.

### Phase 5: Explicit Interrupted And Resume Flow

Status: implemented for the explicit interrupted/recovery slice.

Scope:

- Do not resume automatically.
- When runtime starts and finds interrupted turns, show them as interrupted.
- Provide actions:
  - inspect
  - copy summary
  - start follow-up from this state
  - discard/interrupted done
- Later, define a true checkpoint resume protocol.
- Carry forward Phase 3 recovery risks:
  - interrupted/restarted long sessions should keep tool display metadata
    available from `SessionActivity`
  - large historical sessions should remain inspectable after restart even when
    hydration takes noticeable time
  - interrupted shell/custom tools should preserve structured refs and failure
    reasons without trying to infer from assistant prose

Acceptance:

- Restart with active turn does not show stale running tools.
- User sees what was interrupted and what artifacts already exist.
- Follow-up turn can be started with the interrupted summary.
- Restarted interrupted turns retain command, cwd, exit/failure reason,
  target paths, and artifact/diff counts in recovered tool details.

Tests:

- Runtime startup recovery tests.
- Browser restart/reload smoke.
- Restart smoke with shell failure, file edit, and MCP/custom structured refs.

Implemented:

- Runtime startup/recovery continues to avoid automatic resume.
- Active persisted turns are marked `interrupted` during recovery; unfinished
  persisted tool calls are cancelled so `SessionActivity` does not restore
  stale running tools.
- `RuntimeTurn` now exposes an additive `interrupted` summary DTO computed
  from persisted turn, tool call, permission, runtime event, and diagnostics
  data. No database migration was added.
- The interrupted summary includes:
  - turn/session/status identity
  - started/interrupted timestamps and duration
  - reason/source
  - last completed, failed, and pending-at-interruption tool summaries
  - expected, produced, verified, and missing artifact summaries
  - permission counts
  - failed, denied, cancelled, and nonzero shell signals
  - last runtime event time and sequence
  - Phase 3 tool display metadata, including target paths, command/cwd/exit,
    stdout/stderr/failure excerpts, artifact refs, diff refs, and display
    metadata when present
- The runtime only trusts structured tool fields, display metadata, refs, and
  machine-readable structured output for interrupted tool/custom/MCP artifact
  refs. It does not infer interrupted refs from assistant prose.
- A low-risk `MarkInterruptedDone` action was added. It only applies to an
  already interrupted turn, persists it as `cancelled`, emits a turn-cancelled
  refresh event, and does not replay or continue the original turn.
- HTTP/dev and Wails transports expose
  `POST /v1/turns/{turn_id}/interrupted/done`.
- The React workbench maps `SessionActivity.turns[].interrupted` into a
  view-model `interruptedTurn`; runtime events remain refresh triggers only.
- `TurnDiagnosticsPanel` now renders a compact interrupted recovery surface
  next to diagnostics with:
  - interrupted status/reason/source
  - last tool and pending tool
  - failure/denied/nonzero shell signals
  - artifact summary
  - permission summary
  - last event/hydration time
  - actions for Inspect, Copy, Follow-up, and Mark done
- Follow-up starts a new user turn from the recovery summary. It does not
  replay the original turn.

Validation:

- `go test ./internal/runtime`
- `go test ./internal/runtimeapi`
- `go test ./desktop`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- Runtime HTTP/Vite validation used only files and logs under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- A Phase 5 fake OpenAI-compatible server was added under
  `tmp/runtime-dev/phase5-fake-openai.mjs` and served
  `phase5-fake-model` on `http://127.0.0.1:5193`.
- Runtime HTTP ran on `http://127.0.0.1:5183`; Vite ran on
  `http://127.0.0.1:5185`.
- Browser validation in the Codex in-app browser:
  - opened `http://127.0.0.1:5185/`
  - submitted a Phase 5 long task through the browser first; this revealed an
    existing new-chat/active-session issue described under risks
  - started a clean Phase 5 turn through the runtime API after clearing active
    session state
  - fake model requested a `write` tool for
    `tmp/runtime-dev/phase5-browser-long.md`
  - file-write tool completed and produced/verified the artifact
  - runtime was killed and restarted before the delayed final response
  - recovery status reported the turn as interrupted and no active turns
  - browser reload restored the timeline from `SessionActivity`
  - interrupted turn rendered with an interrupted progress row and no stale
    running tool
  - tool card still displayed the Phase 3 target path, output refs, artifact
    count, diff count, and policy metadata
  - diagnostics panel showed interrupted status, reason/source, duration,
    tool counts, artifact confidence, last tool, and last event sequence/time
  - interrupted recovery Inspect expanded the last completed tool and target
  - Copy summary worked
  - Follow-up from interrupted state submitted a new turn and completed with
    the fake provider
  - Mark done persisted the interrupted turn as `cancelled` and removed the
    interrupted recovery surface after hydration

Remaining risks:

- Browser validation for denied/pending permissions and nonzero shell signals
  was covered by runtime tests and existing diagnostics surfaces, not by a
  live interactive permission-denial browser run in this slice.
- Live MCP/custom validation still uses structured DTO test coverage rather
  than an external MCP server that produces interrupted structured refs.
- The existing browser "new chat" path can still send the next prompt to the
  previously active session because the runtime clears `sessionID` but the
  current frontend view model may still carry an active session id. This was
  observed during Phase 5 validation and should be addressed separately; it is
  not part of the interrupted-state scope.
- Mark done uses the existing `cancelled` terminal status rather than adding a
  new persisted acknowledgement field. This avoids schema churn but means the
  original interrupted state is no longer the visible terminal status after
  acknowledgement.

Follow-up task ownership:

- Completed in Phase 5.1: explicit browser recovery validation for pending
  permission, denied permission, and nonzero shell interrupted recovery.
- Completed in Phase 5.1: frontend new-chat active-session handoff fix. The
  fix keeps `SessionActivity` as the source of truth and does not add a
  frontend-only session source.
- Partially completed in Phase 5.1 and carried to Phase 6: MCP/custom
  structured refs were validated through a close-live persisted
  `SessionActivity` fixture. A true external MCP server end-to-end fixture
  remains required before promoting generic tool artifact refs to a stronger
  product guarantee.
- Carried to Phase 6: decide whether interrupted acknowledgement needs its own
  persisted field or whether the current `cancelled` terminal status remains
  sufficient for product UX and audit recovery.

### Phase 5.1: Interrupted Recovery Hardening

Status: implemented for the focused hardening slice.

Scope:

- Fix new-chat active-session handoff so the first prompt after `NewChat` does
  not reuse a stale active session id.
- Add stronger interrupted recovery fixtures for:
  - pending permission at interruption
  - denied permission diagnostics
  - nonzero shell exit during an interrupted turn
  - MCP/custom structured artifact refs during interrupted recovery
- Keep `SessionActivity` as the source of truth for timeline, diagnostics, and
  interrupted recovery UI.
- Keep runtime events as refresh triggers only.
- Do not auto replay or auto resume interrupted work.
- Do not introduce a Run state machine.

Implemented:

- The workbench now immediately clears the visible draft chat surface when the
  user clicks new chat.
- The runtime adapter records a one-shot draft-submit guard after `NewChat`.
  The next `Chat` request omits `sessionId` even if an older hydrated view
  model still contains a stale active session. The guard is cleared when a new
  turn returns a runtime session id or when the user explicitly selects an
  existing session.
- The guard is adapter-level request hygiene only; it does not own sessions,
  timeline, diagnostics, or interrupted state.
- Interrupted permission diagnostics now preserve a pending-at-interruption
  signal for permissions that were expired/cancelled by runtime recovery. The
  persisted permission remains non-pending, so reload does not restore a stale
  actionable permission gate.
- Runtime interrupted summary tests now cover pending/expired permission
  recovery, denied permission signals, cancelled tool signals, nonzero shell
  exits, and structured MCP/custom artifact refs.
- Structured MCP/custom artifact validation continues to trust only structured
  refs, tool metadata, and display metadata. Assistant prose is not used to
  infer artifact refs.

Validation:

- `go test ./internal/runtime`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- Runtime HTTP/Vite validation used runtime HTTP
  `http://127.0.0.1:5197`, Vite `http://127.0.0.1:5198`, and fake provider
  `http://127.0.0.1:5196`.
- All validation scripts, logs, pid files, and generated artifacts were kept
  under `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev`.
- Browser validation in the Codex in-app browser verified:
  - opening an old session, clicking new chat, and submitting
    `phase51-new-chat` created session
    `6187b9f0-8d5f-459f-af5d-a1a3b3edb911`; the old session
    `cbb139fe-8007-4469-b79f-a0342dea263e` did not receive the prompt
  - pending permission interrupted recovery restored from
    `SessionActivity` after runtime restart with interrupted status, no stale
    running/waiting tool, and diagnostics showing pending-at-interruption plus
    expired recovery state
  - denied permission was exercised through the browser permission gate; reload
    restored diagnostics with `denied 1` and no stale permission gate
  - nonzero shell interrupted recovery restored command, cwd, exit `7`,
    stdout, stderr, failure reason, runtime artifact refs, and
    `nonzero shell 1`
  - close-live MCP/custom structured refs interrupted fixture restored
    structured artifact refs, target, and display metadata while excluding the
    assistant-prose-only path `phase51-prose-should-not-count.json`
  - reload recovery rebuilt timeline, diagnostics, and interrupted recovery
    surfaces from hydrated `SessionActivity`
  - no duplicate tool ids were observed in the browser DOM for the validated
    sessions

Remaining risks:

- The MCP/custom structured refs browser fixture is close-live: it seeds
  persisted runtime tables under the local dev database and validates the real
  `SessionActivity` hydration/UI path. It is not an external MCP server
  end-to-end run.
- The Wails bridge uses the same adapter `NewChat`/`Chat` path and the same
  runtime bridge methods, but this slice's click-through browser validation
  used the HTTP/Vite development transport. Desktop bridge coverage should be
  kept through `go test ./desktop` and future packaged smoke runs.
- Pending-at-interruption is represented by counting recovered expired or
  cancelled permissions as pending in diagnostics for interrupted turns while
  leaving their persisted lifecycle status terminal. This avoids stale approval
  actions but means the diagnostics count intentionally differs from the raw
  permission status.

### Phase 6: Run State Machine Design Gate

Status: completed as a design gate only. No runtime Run store, database
migration, frontend Run UI, or full Run state machine was implemented.

Scope:

- Define the minimum Run contract from Phase 1.1 through Phase 5.1 evidence.
- Keep Run additive. It must not replace `Turn`, `ToolCall`,
  `PermissionRequest`, `RuntimeAgentTask`, or `SessionActivity`.
- Keep `SessionActivity` as the current source of truth for timeline,
  diagnostics, and interrupted recovery UX.
- Keep runtime events as refresh triggers only. They must not become React
  timeline, diagnostics, interrupted, or Run state.
- Treat this phase as a design gate, not an implementation phase.

Minimum Run contract:

```text
Run
  id
  workspace_id
  session_ids
  objective
  status: planned | active | waiting_user | interrupted | verifying | completed | failed | discarded
  expected_artifacts[]
    id
    label
    uri_or_path
    source: user_request | plan | checkpoint | tool_metadata
    required: true | false
  produced_artifacts[]
    id
    uri_or_path
    source_turn_id
    source_task_id optional
    source_tool_call_id optional
    confidence
  verified_artifacts[]
    id
    uri_or_path
    confidence
    verified_at
    verifier: filesystem | runtime_ref | structured_ref | user
  turn_ids[]
  task_ids[]
  checkpoints[]
    id
    turn_id optional
    task_id optional
    label
    summary
    completed_tool_call_ids[]
    failed_tool_call_ids[]
    pending_permission_ids[]
    artifact_ids[]
    created_at
    checkpoint_state: open | resumable | acknowledged | discarded
  final_verification
    state: not_started | partial | passed | failed | unknown
    expected_count
    produced_count
    verified_count
    missing_count
    warning_reason optional
    verified_at optional
  user_actions
    resume
      triggered_by_user_id
      checkpoint_id
      created_turn_id
      created_at
    discard
      triggered_by_user_id
      checkpoint_id optional
      reason optional
      created_at
  created_at
  updated_at
  finished_at optional
```

Contract rules:

- Run summarizes cross-turn work; it never hides or rewrites per-turn evidence.
- Run references existing turn ids, task ids, tool call ids, permission ids,
  artifact refs, and diagnostics. It does not own those primitives.
- Run state is derived from, and links back to, persisted runtime evidence.
  React may render a Run view in the future, but it must hydrate from runtime
  DTOs and may not become the Run source of truth.
- Resume is always user-triggered and creates a new turn from an explicit
  checkpoint summary. It must not replay an interrupted turn automatically.
- Discard is an explicit user action. It must not delete the underlying turn,
  tool, permission, artifact, or audit evidence.
- The first implementation slice, when approved later, should be DTO/API
  additive and transport-neutral. It should not require a schema migration
  until durability requirements are proven.

Design decisions:

- Shell nonzero exit stays a display/diagnostics-derived signal for now.
  Phase 3 and Phase 4 proved the UI can show a failed visual status and
  diagnostics can count `nonzeroExitShellCount` while the persisted tool-call
  status remains `completed`. Future runtime work may add an additive status
  refinement such as `completed_with_nonzero_exit`, but Phase 6 does not
  promote nonzero exit into the canonical `ToolCall.status` lifecycle.
- Artifact confidence is ordered from strongest to weakest:
  - `local_verified_file`: an explicit local expected artifact was verified by
    runtime filesystem check.
  - `produced_tool_metadata`: a trusted tool produced metadata or artifact refs
    for a target, but no final local verification is available.
  - `runtime_output_ref`: a persisted runtime ref exists for output, artifact,
    or diff material, but it is not necessarily a final deliverable.
  - `structured_mcp_custom_ref`: a custom/MCP tool exposed a machine-readable
    structured ref or target. This is trusted as conservative metadata, but it
    needs external MCP end-to-end validation before becoming a stronger product
    guarantee.
  - `unknown_not_detected`: no structured artifact evidence was detected.
    Assistant prose does not upgrade confidence.
- Large session hydration should move toward narrower hydration before a Run
  implementation is attempted. The next design should support
  session-scoped and turn-scoped activity reads, plus optional artifact and
  diagnostics slices, while keeping whole `SessionActivity` hydration as the
  current safe baseline.
- Interrupted acknowledgement should be modeled as a future Run-level
  checkpoint state when Run exists. The current Phase 5/5.1 implementation may
  continue using the existing `cancelled` terminal turn status for
  `MarkInterruptedDone`; no turn-level acknowledgement field is required in
  this design gate.
- Pending-at-interruption remains a computed diagnostics signal over terminal
  permission statuses for now. It should not become a persisted recovery
  lifecycle state until the product needs audit-distinct recovery semantics
  beyond display and follow-up handoff.

Planned follow-up phases:

- Phase 6.1: External MCP interrupted structured refs fixture.
  - Run a real external MCP server end-to-end.
  - Produce structured artifact refs during a turn that is interrupted.
  - Restart runtime and verify hydrated `SessionActivity` preserves refs,
    target metadata, diagnostics confidence, and interrupted recovery summary.
  - Do not infer refs from assistant prose.
- Phase 6.2: Wails packaged handoff/recovery smoke.
  - Exercise the packaged desktop bridge, not only HTTP/Vite.
  - Verify new-chat handoff creates or targets the correct session after the
    one-shot draft-submit guard.
  - Verify interrupted recovery bridge methods, `MarkInterruptedDone`, and
    event-triggered hydration still rebuild from `SessionActivity`.
- Phase 6.3: Pending-at-interruption lifecycle semantics.
  - Decide whether computed diagnostics remain sufficient.
  - If not, design a persisted recovery lifecycle field without restoring stale
    actionable permission gates after restart.
  - Include audit implications and migration requirements before any code
    change.
- Phase 6.4: Narrow activity hydration design.
  - Specify session-scoped and turn-scoped hydration APIs for very large
    sessions.
  - Preserve `SessionActivity` as the canonical current aggregate until the
    narrower reads are implemented and validated.
  - Carry the Phase 6.3 hydration risk forward: make narrower reads preserve
    diagnostics and interrupted recovery semantics without making runtime
    events or React local state the source of truth.
- Phase 6.5: MCP transport and native structured-content hardening.
  - Extend the Phase 6.1 fixture beyond stdio MCP to streamable HTTP MCP and
    SSE MCP, including restart/interruption timing around structured refs.
  - Cover hosted/provider-specific MCP auth and elicitation flows before
    promoting external MCP artifact refs to a broader production guarantee.
  - Capture native MCP `structuredContent` directly in the Go client wrapper
    instead of relying on servers to mirror the same JSON payload as text
    content.
  - Preserve the Phase 6.1 boundary: no Run state machine, runtime Run store,
    database migration, frontend Run UI, automatic resume, or prose-derived
    artifact inference.
- Phase 6.6: Packaged WebView/live long-turn validation.
  - Add a repeatable packaged desktop WebView click-through smoke when a stable
    UI automation hook is available for the Wails WebView.
  - Exercise a real packaged long-running turn with a live or deterministic
    provider through the packaged frontend, including new-chat handoff,
    interrupted recovery hydration, event-triggered refresh, and
    `MarkInterruptedDone`.
  - Keep all validation artifacts under `tmp/runtime-dev`.
  - Preserve the current boundary: no Run state machine, runtime Run store,
    database migration, frontend Run UI, automatic resume, stale tool recovery,
    or assistant-prose artifact inference.
- Phase 6.7: Narrow activity hydration implementation candidate.
  - Implement the Phase 6.4 session-window and turn-activity design only after
    the Phase 6.5 MCP transport/native structured-content hardening and Phase
    6.6 packaged live validation risks are reviewed.
  - Keep full `SessionActivity` as the compatibility fallback and parity
    oracle while narrow reads are introduced.
  - Add bounded session activity window and turn activity DTOs assembled from
    the same persisted runtime evidence as full `SessionActivity`.
  - Add store/query helpers needed to avoid whole-session reads in turn-scoped
    diagnostics:
    - messages by turn or bounded session window
    - tool calls by session/turn without per-turn loops
    - permissions by turn
    - events by turn or bounded session window
  - Add parity tests proving narrow reads preserve diagnostics, artifact
    evidence, interrupted summaries, terminal permission semantics, and
    `MarkInterruptedDone` acknowledgement behavior.
  - Define stable cursor/window ordering across messages, turns, tool calls,
    permissions, and runtime events before frontend adoption.
  - Teach the frontend adapter to request narrow hydration from event-triggered
    refreshes only after runtime parity tests pass; runtime events still remain
    refresh triggers only.
  - Preserve the current boundary: no Run state machine, runtime Run store,
    database migration unless separately justified, frontend Run UI, automatic
    resume, stale tool/permission recovery, or assistant-prose artifact
    inference.
- Phase 7 candidate: Additive Run DTO/API prototype.
  - Only after Phase 6.1 through Phase 6.7 are reviewed.
  - Start with a read-only runtime DTO assembled from existing turns, tasks,
    diagnostics, permissions, tool calls, and refs.
  - Carry the Phase 6.3 lifecycle risk forward: if a durable recovery
    lifecycle field is needed, design it as additive Run/checkpoint metadata
    with explicit audit events, migration/backfill behavior, acknowledgement
    UX, and a rule that stale permissions/tools never regain actionability.
  - Include virtual URI and non-local artifact handles as metadata-only Run DTO
    inputs unless or until a transport-specific verifier exists.
  - Defer migrations, background Run scheduler, and frontend Run UI until the
    DTO proves useful.

Acceptance:

- The Run contract can summarize a long multi-turn operation without hiding
  individual turn/tool evidence.
- The contract can report expected, produced, and verified artifacts across
  turns with confidence levels.
- Resume/discard are explicit user-triggered actions tied to checkpoints.
- The design keeps `SessionActivity` as the current timeline, diagnostics, and
  interrupted recovery source of truth.
- Runtime events remain refresh triggers only.
- No Run state machine, store, migration, or frontend Run UI is introduced in
  this phase.

Validation:

- Design reviewed against Phase 1.1 through Phase 5.1 implementation and
  validation records in this document.
- Documentation-only gate; no full Go/frontend test run is required unless code
  changes are made.

### Phase 6.1: External MCP Interrupted Structured Refs Fixture

Status: implemented for the focused external MCP interrupted structured-ref
fixture. This is not a Run implementation.

Scope:

- Close the Phase 5.1 close-live fixture gap with a real external stdio MCP
  server path.
- Keep `SessionActivity` as the source of truth for timeline, diagnostics, and
  interrupted recovery.
- Keep runtime events as refresh triggers only.
- Preserve current interrupted semantics:
  - no automatic resume
  - no stale running/waiting tool after restart recovery
  - pending-at-interruption remains computed diagnostics
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration, or
  frontend Run UI.

Implemented:

- Added a runtime test fixture that writes an external stdio MCP server script
  under `tmp/runtime-dev/phase61-tests`.
- The fixture runs through the real runtime/backend/agent/scheduler/MCP tool
  path:
  - a fake OpenAI-compatible provider requests the discovered
    `mcp_phase61_structured_artifact` tool
  - the external MCP server creates a local artifact under `tmp/runtime-dev`
    and returns machine-readable JSON artifact refs and target metadata
  - the fake provider blocks the post-tool model request so the turn remains
    unfinished after structured refs have been produced
  - restart recovery interrupts the turn and cancels unfinished live tool state
  - hydrated `SessionActivity` rebuilds the interrupted turn, diagnostics, tool
    display metadata, structured artifact evidence, and recovery summary
- MCP text results that are themselves JSON objects or arrays are now copied
  into tool response metadata. This lets the existing runtime scheduler create
  structured artifact refs and diagnostics from machine-readable MCP output
  without parsing prose.

Validation:

- `go test ./internal/runtime -run TestRuntimeExternalMCPInterruptedStructuredRefsFixture -count=1`
- `go test ./internal/agent/tools -count=1`
- `go test ./internal/runtime`
- `go test ./...`

Guarantees now covered:

- External stdio MCP structured JSON output can contribute produced artifacts
  and runtime artifact refs through the end-to-end tool path.
- Hydrated `SessionActivity` restores:
  - interrupted turn status
  - produced and verified local artifact path
  - runtime artifact refs
  - target/display metadata for the MCP tool
  - diagnostics structured MCP/custom artifact confidence
  - interrupted recovery artifact summary
- Assistant/user prose-only paths are not counted as produced structured refs.
  Only machine-readable tool output, runtime refs, tool/display metadata, and
  filesystem verification contribute artifact evidence.
- Runtime restart recovery does not restore stale running or waiting tool
  calls.

Remaining risks:

- The fixture validates stdio MCP and JSON text output. It does not yet cover
  streamable HTTP MCP, SSE MCP, hosted/provider-specific MCP auth, or
  elicitation flows.
- At the end of Phase 6.1, the MCP Go client wrapper still primarily exposed
  text/media content to the scheduler. Phase 6.5 closes this specific native
  `structuredContent` capture gap.
- Runtime refs prove structured MCP output was captured, while local
  filesystem verification only applies to explicit local paths. Virtual URIs
  and non-local artifact handles remain metadata-only in this phase.

### Phase 6.2: Wails Packaged Handoff/Recovery Smoke

Status: implemented for the focused packaged desktop/Wails bridge smoke. This
is not a Run implementation.

Scope:

- Exercise the packaged desktop path and the Wails `RuntimeBridge` contract,
  not the HTTP/Vite development transport.
- Keep `SessionActivity` as the source of truth for timeline, diagnostics, and
  interrupted recovery UX.
- Keep runtime events as refresh triggers only.
- Preserve current interrupted semantics:
  - no automatic resume
  - no stale running/waiting tool after restart recovery
  - pending-at-interruption remains a computed diagnostics signal
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration, or
  frontend Run UI.

Implemented:

- Added `desktop/scripts/phase62-wails-packaged-smoke.ps1`.
- The smoke script builds the shared React assets and packaged
  `desktop/bin/AgentBuilder.exe` when requested, starts the packaged
  executable with `AGENT_BUILDER_DESKTOP_ROOT` under
  `tmp/runtime-dev`, and verifies the packaged runtime creates its `config`,
  `data`, and `logs` directories there.
- Added `TestRuntimeBridgePhase62PackagedHandoffRecoveryContract` in the
  desktop package. The test exercises the Wails bridge-facing methods used by
  the packaged client:
  - `NewChat`
  - draft `Chat` without a stale session id
  - `Events(after)` cursor forwarding
  - `SessionActivity`
  - `MarkInterruptedDone`
- The bridge contract fixture verifies interrupted recovery data is consumed
  from hydrated `SessionActivity`, stale running/waiting tools are not exposed,
  `MarkInterruptedDone` keeps cancelled terminal acknowledgement semantics, and
  prose-only artifact paths are not treated as produced artifact evidence.

Validation:

- `go test . -run TestRuntimeBridgePhase62PackagedHandoffRecoveryContract -count=1`
  from `desktop`.

Guarantees now covered:

- The packaged executable can start through the Wails desktop shell with its
  runtime root redirected into `tmp/runtime-dev`.
- The Wails bridge exposes the new-chat handoff and interrupted recovery method
  surface needed by the packaged frontend.
- Event cursors cross the Wails bridge and remain suitable only as hydration
  refresh triggers.
- Interrupted recovery display data still comes from `SessionActivity`; the
  bridge does not introduce a frontend-owned recovery source.

Remaining risks:

- The smoke is not a full WebView click-through automation. It validates
  packaged executable startup plus the Wails bridge contract used by the
  packaged frontend.
- It does not exercise a live external provider or a real long-running
  packaged UI turn. Those remain separate manual or live smoke risks.
- It does not add narrow activity hydration, Run DTOs, Run persistence, or Run
  UI. Very large session hydration remains a later Phase 6.4 design topic.

### Phase 6.3: Pending-at-Interruption Lifecycle Semantics

Status: reviewed and locked to the current computed diagnostics semantics. This
is not a Run implementation.

Scope:

- Review whether pending-at-interruption needs a persisted lifecycle field.
- Keep `SessionActivity` as the source of truth for timeline, diagnostics, and
  interrupted recovery UX.
- Keep runtime events as refresh triggers only.
- Preserve current interrupted semantics:
  - no automatic resume
  - no stale running/waiting tool after restart recovery
  - no restored actionable permission gate after restart recovery
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration,
  frontend Run UI, or persisted interrupted acknowledgement field.

Review conclusion:

- Pending-at-interruption should continue to be a computed diagnostics signal.
  The persisted permission lifecycle remains the audit record of what happened
  to the permission request (`pending`, `allowed_*`, `denied`, `expired`, or
  `cancelled`).
- Restart recovery expires or cancels stale pending permission rows when their
  live runtime gate is gone. Those terminal rows can still contribute to the
  interrupted recovery summary and diagnostics, but they must not become
  approvable permissions again.
- Counting terminal `expired` and `cancelled` permissions as
  pending-at-interruption only when the owning turn is `interrupted` is enough
  for the current UI: it explains that user input was pending when the process
  stopped without changing the durable permission status.
- A persisted recovery lifecycle field is not needed in this phase. Adding one
  now would duplicate existing terminal permission evidence and would require
  migration, audit semantics, API/DTO review, and UX copy for how it differs
  from the permission status itself.
- If a future Run/checkpoint implementation needs durable lifecycle state, it
  should be designed as an additive field tied to recovery checkpoints or Run
  summaries. That design must specify migration/backfill behavior, audit event
  names and payloads, how acknowledgement/discard appears in UX, and how React
  hydrates the field from runtime DTOs. It must not restore stale permission or
  tool actionability.

Implemented:

- Added `TestRuntimeInterruptedPermissionLifecycleDiagnosticsAreComputed`.
  The test locks the current diagnostic behavior:
  - interrupted turns count terminal `expired` and `cancelled` permissions as
    pending-at-interruption signals
  - completed turns keep the same terminal permission statuses without
    incrementing pending diagnostics

Validation:

- `go test ./internal/runtime -run TestRuntimeInterruptedPermissionLifecycleDiagnosticsAreComputed -count=1`
- `go test ./internal/runtime`

Remaining risks:

- This phase does not introduce durable Run/checkpoint lifecycle semantics.
  That remains a Phase 7-or-later design and implementation topic after Phase
  6 follow-up risks are closed.
- `SessionActivity` remains the safe aggregate for the current UI. Very large
  session hydration remains a Phase 6.4 design topic.

### Phase 6.4: Narrow Activity Hydration Design

Status: reviewed and documented as a narrow hydration design. This is not a
Run implementation.

Scope:

- Review the current `SessionActivity` hydration path and large-session risk.
- Specify session-scoped and turn-scoped read boundaries for future
  implementation.
- Preserve `SessionActivity` as the current source of truth for timeline,
  diagnostics, and interrupted recovery UX until the narrower reads are
  implemented and validated against it.
- Keep runtime events as refresh triggers only.
- Preserve current interrupted semantics:
  - no automatic resume
  - no stale running/waiting tool recovery
  - no restored actionable permission gate after restart recovery
  - pending-at-interruption remains computed diagnostics
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration,
  frontend Run UI, or persisted interrupted acknowledgement field.

Current hydration assessment:

- `SessionActivity(sessionID)` is still the correct safety baseline because it
  returns messages, turns, tool calls, permissions, policy, per-turn
  diagnostics, and interrupted summaries from runtime-owned data.
- The current implementation is intentionally aggregate-heavy:
  - reads all displayable session messages
  - reads all turns for the session
  - lists tool calls once per turn
  - reads all permissions for the session
  - reads all runtime events for the session and groups them by turn
  - recomputes diagnostics and interrupted summaries for every hydrated turn
- That shape is safe but will scale poorly for very large sessions, especially
  when lifecycle events trigger frequent refreshes during long tool bursts.
- `Turn(turnID)` and `TurnToolCalls(turnID)` already provide useful
  turn-scoped foundations, but `Turn(turnID)` still reads whole-session
  messages and permissions before filtering enough data to compute
  diagnostics. It should not be treated as the final narrow hydration boundary.

Design conclusion:

- Keep full `GET /v1/sessions/{session_id}/activity` and Wails
  `SessionActivity(sessionID)` as the canonical aggregate and compatibility
  fallback.
- Add future narrow reads as additive DTO/API surfaces assembled from the same
  persisted runtime evidence. They must not create a parallel source of truth.
- The first narrow API should be session-scoped and timeline-oriented:

```text
GET /v1/sessions/{session_id}/activity-window
  query:
    cursor optional
    limit optional
    before optional
    after optional
    include=diagnostics,artifacts,interrupted,permissions optional
  response:
    session_id
    messages[]
    turns[]
    tool_calls[]
    permissions[] optional
    diagnostics[] optional
    interrupted_turns[] optional
    policy optional
    window
      cursor
      has_more_before
      has_more_after
      first_event_sequence optional
      last_event_sequence optional
```

- The second narrow API should be turn-scoped and evidence-oriented:

```text
GET /v1/turns/{turn_id}/activity
  query:
    include=messages,tool_calls,permissions,diagnostics,artifacts,events,interrupted optional
  response:
    turn
    messages[] optional
    tool_calls[] optional
    permissions[] optional
    diagnostics optional
    interrupted optional
    events[] optional
```

- Optional slices must be runtime-computed DTOs, not React-derived state:
  - `diagnostics`: same semantics as `RuntimeTurnDiagnostics`
  - `artifacts`: expected, produced, verified, missing, confidence, and refs
    derived from tool metadata, refs, structured output, and conservative local
    filesystem verification
  - `interrupted`: same semantics as `RuntimeInterruptedSummary`
  - `permissions`: pending and historical permission rows relevant to the
    session window or turn, with terminal expired/cancelled rows preserved as
    evidence but not actionability
- The future implementation should factor shared runtime helpers so
  `SessionActivity`, session-window activity, and turn activity compute
  diagnostics/interrupted summaries from the same code path. Full
  `SessionActivity` should remain the comparison oracle during rollout.

Event-triggered refresh design:

- Runtime event receipt continues to trigger hydration only.
- The event envelope may guide which narrow read is requested:
  - `turn.*` with `turn_id`: refresh `GET /v1/turns/{turn_id}/activity`
    including diagnostics and interrupted slices.
  - `tool.call.*` with `turn_id`: refresh the owning turn activity and tool
    slice.
  - `permission.*` with `turn_id`: refresh the owning turn activity including
    permissions and diagnostics.
  - `message.*` without a useful turn id: refresh a small session activity
    window around the active session tail.
  - session-level events: refresh session metadata and the active session
    window.
- Event payloads must not be merged directly into React timeline,
  diagnostics, artifacts, or interrupted recovery state.
- High-frequency token/progress/message delta events should stay coalesced.
  Lifecycle, permission, artifact/ref, and terminal turn events may trigger
  immediate narrow hydration.

Implementation order for a later phase:

1. Add store-level turn/session query helpers where needed:
   - list messages by turn or bounded session window
   - list tool calls by session or turn without per-turn loops
   - list permissions by turn
   - list events by turn or bounded session window
2. Add runtime service methods and DTOs for turn activity and session activity
   windows.
3. Add HTTP and Wails bridge methods.
4. Add tests proving narrow activity equals the corresponding subset of full
   `SessionActivity` for diagnostics, artifact evidence, interrupted summaries,
   and terminal permission semantics.
5. Teach the frontend adapter to use narrow hydration after event triggers
   while keeping full `SessionActivity` as fallback.

Validation:

- Documentation-only gate; no Go or frontend tests were required because no
  code changed.
- Reviewed current implementation paths:
  - `runtimeService.SessionActivity`
  - `runtimeService.Turn`
  - `runtimeService.TurnToolCalls`
  - runtime turn diagnostics and interrupted summary builders
  - HTTP/Wails bridge routes for session activity, turns, tool calls, events,
    and `MarkInterruptedDone`
  - frontend `wailsWorkbenchAdapter` activity mapping and
    `runtimeEventRefresh` trigger policy
- Performed git diff review for this documentation update.

Remaining risks:

- Very large sessions still hydrate through full `SessionActivity` until a
  later implementation phase adds narrow reads.
- `Turn(turnID)` is not yet a fully narrow diagnostic read because it can still
  consult whole-session messages and permissions.
- The future session-window cursor needs careful ordering semantics across
  messages, turns, tool calls, permissions, and runtime events.
- Narrow hydration must prove parity with `SessionActivity` before the
  frontend relies on it for diagnostics or interrupted recovery.
- MCP streamable HTTP/SSE and native `structuredContent` hardening were
  carried into Phase 6.5.

### Phase 6.5: MCP Transport And Native Structured-Content Hardening

Status: implemented for the focused native `structuredContent` capture path,
with transport/live-provider risks reviewed. This is not a Run implementation.

Scope:

- Review MCP stdio, streamable HTTP, and SSE result capture as it affects
  runtime artifact refs and interrupted recovery.
- Keep `SessionActivity` as the source of truth for timeline, diagnostics, and
  interrupted recovery UX.
- Keep runtime events as refresh triggers only.
- Preserve current interrupted semantics:
  - no automatic resume
  - no stale running/waiting tool after restart recovery
  - no restored actionable permission gate after restart recovery
  - pending-at-interruption remains computed diagnostics
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration,
  frontend Run UI, narrow hydration API, or persisted interrupted
  acknowledgement field.

Review conclusion:

- Streamable HTTP and SSE MCP use the same Go SDK `ClientSession.CallTool`
  result type as stdio after transport normalization. The highest-impact
  hardening point is therefore the shared MCP Go client wrapper, not a
  transport-specific runtime timeline path.
- The existing streamable HTTP and SSE configuration paths already construct
  SDK transports with resolved URLs and resolved headers. Hosted or
  provider-specific MCP auth that is expressible as headers is preserved by
  transport setup, while OAuth/browser-driven hosted flows and elicitation
  recovery remain live-provider risks outside this phase.
- Phase 6.1 proved end-to-end stdio MCP structured refs only when the MCP
  server also mirrored JSON as text content. That was too narrow: native MCP
  `structuredContent` could be dropped before scheduler/runtime diagnostics saw
  it.
- Restart/interruption timing around structured refs remains safe only when the
  scheduler has already recorded completed tool metadata or refs before
  recovery. Runtime restart still cancels unfinished live tool calls and does
  not make stale tools or permissions actionable again.

Implemented:

- The MCP Go client wrapper now serializes native SDK
  `CallToolResult.StructuredContent` into tool response metadata.
- The MCP agent tool wrapper now passes that metadata through to the scheduler
  for text, image, and media MCP responses.
- Existing scheduler/runtime paths then persist it as structured tool output,
  create runtime artifact refs, and let diagnostics/interrupted summaries
  extract artifact paths without relying on assistant prose or JSON mirrored as
  text content.
- Added focused unit coverage for MCP results that contain native
  `structuredContent` with and without any text-content JSON mirror.

Validation:

- `go test ./internal/agent/tools/mcp -count=1`
- `go test ./internal/agent/tools -count=1`
- `go test ./internal/agent -run "TestSchedulerTool|TestConvertToToolResult" -count=1`
- `go test ./internal/runtime -run "TestRuntimeExternalMCPInterruptedStructuredRefsFixture|TestRuntimeTurnDiagnosticsMCPStructuredArtifactRefsCountAsProduced|TestRuntimeInterruptedSummaryPhase51StructuredRefsOnly" -count=1`

Guarantees now covered:

- Native MCP `structuredContent` can contribute scheduler structured metadata
  even when `content` is empty or only contains non-JSON text.
- MCP servers no longer need to mirror JSON structured refs as text content for
  runtime diagnostics to see machine-readable artifact refs.
- Existing stdio interrupted structured-ref coverage still passes after the
  wrapper change.
- No frontend-owned inference, Run state, database migration, automatic resume,
  stale tool recovery, stale permission actionability, or narrow hydration API
  was introduced.

Remaining risks:

- This phase did not add a live streamable HTTP or live SSE MCP interrupted
  fixture. The shared SDK result path is covered, but transport disconnect,
  replay, and provider-hosted auth timing still need broader live validation.
- Hosted/provider-specific MCP OAuth and elicitation flows are still dependent
  on the MCP SDK/session behavior and existing runtime MCP request handling;
  they were reviewed but not expanded here.
- If an MCP tool is interrupted before the scheduler records completed output,
  recovery still cancels the unfinished tool and does not infer produced
  artifacts from partial transport state.
- Virtual artifact URIs and non-local handles remain metadata/runtime-ref
  evidence only; filesystem verification still applies only to explicit local
  paths.

### Phase 6.6: Live HTTP/SSE MCP Restart And Hosted Flow Validation

Status: implemented for deterministic local streamable HTTP and SSE MCP
structured-content restart fixtures, with hosted-provider flow risks reviewed.
This is not a Run implementation.

Scope:

- Add live or deterministic local fixtures for streamable HTTP MCP and SSE MCP
  restart/interruption timing around structured refs.
- Validate that native `structuredContent` captured in Phase 6.5 survives the
  same scheduler/runtime/ref/diagnostics path over HTTP and SSE transports.
- Exercise transport disconnect/replay timing where the MCP SDK can reconnect
  or replay requests.
- Review hosted/provider-specific MCP auth and elicitation behavior against
  existing runtime MCP request handling.
- Preserve current recovery boundaries:
  - no automatic resume
  - no stale running/waiting tool recovery
  - no restored actionable permission gate after restart recovery
  - pending-at-interruption remains computed diagnostics
  - `MarkInterruptedDone` / cancelled terminal status remains the
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration,
  frontend Run UI, narrow hydration API, or persisted interrupted
  acknowledgement field.

Implemented:

- Added deterministic local MCP fixtures for both streamable HTTP MCP and SSE
  MCP using the Go SDK HTTP handlers.
- The fixtures run through the real runtime path:
  - fake OpenAI-compatible provider requests a real MCP tool
  - the MCP tool returns native `structuredContent` with artifact refs
  - the scheduler records completed MCP output
  - the provider is held open before final assistant completion
  - runtime restart recovery interrupts the turn
  - `SessionActivity` hydrates diagnostics and interrupted recovery from
    persisted runtime evidence
- Both HTTP and SSE fixtures verify:
  - configured MCP transport headers are observed by the MCP server
  - native structured refs survive as produced artifacts after restart
  - local artifact paths are verified on disk
  - assistant prose-only paths are not counted as produced artifacts
  - interrupted summaries restore last completed MCP tool target/display/ref
    metadata
  - stale running/pending/waiting tool states are not restored
- Added a focused recovery fixture for the opposite timing edge:
  - a running MCP tool has partial structured output persisted before restart
  - restart cancels the unfinished tool
  - diagnostics and interrupted summary do not count the partial structured
    metadata as produced artifacts or structured artifact refs
- Hardened runtime diagnostics so artifact confidence/ref counts only consider
  completed tool calls, matching produced-artifact extraction. Cancelled,
  running, pending, and waiting tool rows remain timeline evidence but do not
  become artifact production evidence.

Validation:

- `go test ./internal/agent/tools/mcp -count=1`
- `go test ./internal/agent/tools -count=1`
- `go test ./internal/agent -run "TestSchedulerTool|TestConvertToToolResult" -count=1`
- `go test ./internal/runtime -run "TestRuntimeExternalMCPInterruptedStructuredRefsFixture|TestRuntimeHTTPAndSSEMCPInterruptedStructuredRefsFixture|TestRuntimeMCPPartialStructuredOutputCancelledOnRestartDoesNotProduceArtifact|TestRuntimeTurnDiagnosticsMCPStructuredArtifactRefsCountAsProduced|TestRuntimeInterruptedSummaryPhase51StructuredRefsOnly" -count=1`
- `go test ./internal/runtime -count=1`

Hosted auth / elicitation review:

- Header-based hosted auth remains covered by transport setup and by the new
  HTTP/SSE fixture observing the configured Authorization header.
- OAuth/browser-mediated hosted auth remains SDK/provider dependent. This phase
  does not store OAuth state in React, does not add an auth Run/checkpoint
  object, and does not restore stale auth actionability after restart.
- Existing runtime MCP request handling for auth and elicitation remains
  request-store based with pending/completed/denied/cancelled/failed terminal
  statuses. Phase 6.6 does not add a persisted interrupted acknowledgement
  field or re-open terminal MCP requests as actionable after restart.
- Elicitation flows are not expanded into live provider automation here because
  provider credentials and browser-mediated interaction cannot be safely
  automated as a deterministic repo test without introducing secrets or moving
  actionability into frontend state.

Remaining risks:

- Streamable HTTP disconnect/replay was probed with the SDK SSE stream close
  hook. In this runtime path the MCP call failed with `request terminated
  without response` before completed scheduler output was recorded. The safe
  behavior is covered by the partial-output recovery fixture: no artifact is
  inferred without completed output. A successful SDK replay case remains a
  live-provider/SDK behavior risk.
- Legacy SSE MCP completed-output restart is covered, but a deterministic SSE
  disconnect/replay fixture is not available through the same SDK close hook.
- Real third-party hosted MCP providers may require credentials, dynamic OAuth,
  browser redirects, or provider-specific elicitation UI. Those should be
  validated with a manual smoke checklist that records provider name, auth
  setup, actionability state before restart, restart timing, and
  `SessionActivity` output after restart. Secrets must not be stored in repo
  fixtures or React state.
- This phase still does not add automatic resume, stale tool recovery, stale
  permission or MCP request actionability, Run persistence, Run UI, database
  migrations, or narrow hydration APIs.

### Phase 6.7: Narrow Activity Hydration Implementation

Status: planned after Phase 6.6 validation risk is closed or explicitly
accepted.

Scope:

- Implement the Phase 6.4 session-scoped and turn-scoped narrow hydration
  design as additive runtime DTO/API/Wails surfaces.
- Factor shared runtime helpers so full `SessionActivity`, session activity
  windows, and turn activity compute diagnostics, artifact evidence,
  interrupted summaries, and terminal permission semantics from the same code
  path.
- Keep full `SessionActivity` as the compatibility fallback and parity oracle
  during rollout.
- Keep runtime events as refresh triggers only. Event payloads may choose which
  narrow read to request, but they must not become React timeline, diagnostic,
  artifact, or interrupted recovery state.
- Do not add a runtime Run store, Run state machine, Run database migration for
  Run, frontend Run UI, automatic resume, stale tool recovery, stale permission
  actionability, or prose-derived artifact/checkpoint inference.

Acceptance:

- Narrow turn activity matches the corresponding subset of full
  `SessionActivity` for messages, tool calls, permissions, diagnostics,
  artifacts, interrupted summaries, and terminal permission semantics.
- Narrow session window activity can hydrate the active session tail without
  losing warnings, refs, permission evidence, or interrupted recovery data.
- Frontend adapter can use narrow hydration after lifecycle events while
  falling back to full `SessionActivity`.

### Phase 6.8: Hosted MCP Replay/Auth/Elicitation Follow-up

Status: planned after Phase 6.7 narrow hydration parity is implemented or
explicitly deferred. This remains a validation/hardening phase, not a Run
implementation.

Scope:

- Revisit the Phase 6.6 live-provider risks that could not be closed with
  deterministic repo fixtures:
  - successful streamable HTTP disconnect/replay after the SDK or provider path
    records completed output
  - legacy SSE disconnect/replay behavior where a deterministic SDK close hook
    is unavailable
  - hosted/provider-specific OAuth flows
  - hosted/provider-specific elicitation flows
- Prefer deterministic local fixtures when the SDK exposes stable hooks. If a
  provider requires credentials or browser-mediated OAuth, use a manual smoke
  checklist instead of storing secrets or auth state in repo fixtures.
- Preserve current recovery boundaries:
  - no automatic resume
  - no stale running/waiting tool recovery
  - no restored actionable permission gate after restart recovery
  - no restored actionable MCP auth or elicitation request after restart
  - pending-at-interruption remains computed diagnostics
  - `MarkInterruptedDone` / cancelled terminal status remains the interrupted
    acknowledgement mechanism
- Do not add a runtime Run store, Run state machine, Run database migration,
  frontend Run UI, persisted interrupted acknowledgement field, or
  prose-derived artifact/checkpoint inference.

Acceptance:

- A successful replay case, when available, proves that only completed
  scheduler output contributes produced refs after restart.
- A failed/disconnected replay case proves the unfinished tool is cancelled and
  no artifact is inferred from partial transport state.
- Hosted auth and elicitation smoke results document:
  - provider and transport
  - auth or elicitation setup without secrets
  - runtime MCP request status before restart
  - restart timing
  - `SessionActivity` diagnostics/interrupted state after restart
  - confirmation that no stale MCP request or permission is actionable after
    restart
- If provider automation is not safe, the phase records an explicit manual
  validation gap and keeps the runtime/frontend boundaries unchanged.

## Validation Scenarios

Use these as recurring gates after each phase:

1. Short command task:
   - run a harmless command
   - verify shell card, output, exit code, and final assistant response

2. Multi-turn document task:
   - create a report with overwrite + append
   - follow up in the same session
   - verify continuity and refresh recovery

3. Missing artifact task:
   - ask for a specific output file
   - force or simulate no file created
   - verify diagnostics warning

4. Permission/denial task:
   - trigger a permission request
   - allow once, allow session, and deny in separate runs
   - verify timeline and diagnostics after refresh

5. Large module scan:
   - search/read at least 8 files
   - write at least 2 outputs
   - verify tool grouping and browser performance

6. Restart recovery:
   - start a long task
   - restart runtime during execution
   - verify interrupted state and no stale running tools

## Guardrails

- Do not move runtime state into React.
- Do not parse assistant prose as the primary source of tool state.
- Do not introduce a full Run state machine merely because the minimal Run
  contract is documented. Any Run implementation requires a separately approved
  phase after the Phase 6 follow-up risks are reviewed.
- Do not auto-resume interrupted tasks.
- Keep every phase independently testable.
- Prefer additive DTO fields over schema churn unless durability requires a
  migration.

## Immediate Next Step

Proceed to Phase 6.7 narrow activity hydration implementation.

Do not start Run persistence or Run UI before Phase 6.7 parity is validated and
the Phase 6.8 hosted MCP replay/auth/elicitation follow-up risks are explicitly
accepted or closed.
