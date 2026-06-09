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
- Phase 7: Claude Code runtime mapping and read-only Run DTO design gate.
  - Only after Phase 6.1 through Phase 6.9 are reviewed.
  - Start with a read-only runtime DTO assembled from existing turns, tasks,
    diagnostics, permissions, tool calls, and refs.
  - Carry the Phase 6.3 lifecycle risk forward: if a durable recovery
    lifecycle field is needed, design it as additive Run/checkpoint metadata
    with explicit audit events, migration/backfill behavior, acknowledgement
    UX, and a rule that stale permissions/tools never regain actionability.
  - Include virtual URI and non-local artifact handles as metadata-only Run DTO
    inputs unless or until a transport-specific verifier exists.
  - Defer migrations, background Run scheduler, frontend Run UI, and public
    APIs until a Phase 7.1 read-only projection proves parity with
    `SessionActivity`.

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

Status: implemented for additive turn-scoped and session-window activity
hydration. This is not a Run implementation.

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

Implemented:

- Added additive runtime DTOs and service methods:
  - `RuntimeTurnActivityResponse`
  - `RuntimeSessionActivityWindowResponse`
  - `RuntimeActivityWindow`
- Added HTTP/dev-module routes:
  - `GET /v1/turns/{turn_id}/activity`
  - `GET /v1/sessions/{session_id}/activity-window?limit=N`
- Added Wails bridge methods:
  - `TurnActivity(turnID)`
  - `SessionActivityWindow(sessionID, limit)`
- Factored shared runtime activity hydration so full `SessionActivity`, turn
  activity, and session-window activity compute diagnostics, artifact evidence,
  interrupted summaries, tool calls, permission evidence, and runtime events
  from the same Go-side evidence path.
- Kept full `SessionActivity` unchanged as the compatibility fallback and
  parity oracle. Full activity still returns the full message list; narrow
  activity returns only messages tied to the hydrated turn window.
- Regenerated Wails bindings and synced desktop frontend dist assets so the
  packaged Wails path exposes the new methods.
- Updated the frontend adapter so runtime events remain refresh triggers only:
  the event envelope records a short-lived hint for which runtime DTO to read,
  then the UI hydrates from `TurnActivity`, `SessionActivityWindow`, or falls
  back to full `SessionActivity`. Event payloads are not merged into timeline,
  diagnostics, artifact, permission, or interrupted state.
- Narrow frontend hydration merges only runtime-returned DTO items into the
  existing runtime-hydrated view model. It does not parse assistant prose or
  infer artifact/checkpoint/recovery state from React local state.

Validation:

- `go test ./internal/runtime -run "TestRuntimeHTTPServerRoutesNarrowActivityToRuntimeService|TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy|TestRuntimeSessionActivityExposesTurnDiagnosticsWarning" -count=1`
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgePhase62PackagedHandoffRecoveryContract" -count=1`
- `go test ./internal/runtimeapi -count=1`
- `go test ./internal/runtime -count=1`
- `go test ./desktop -count=1`
- `go test ./...`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- `cd desktop && node scripts/sync-client-dist.mjs --build-client`

Parity coverage added:

- Narrow turn activity and session-window activity are compared against full
  `SessionActivity` for the same turn's:
  - messages
  - diagnostics warning and missing artifact evidence
  - terminal denied permission evidence and permission counts
  - runtime event sequence evidence
  - tool call subset
- HTTP and dev-module route tests prove both narrow reads reach the runtime
  service and preserve session/turn ids plus window limit.
- Wails bridge tests prove both narrow methods delegate to the transport-neutral
  runtime service.

Remaining risks:

- Session-window ordering is intentionally simple tail-by-turn ordering. It
  does not yet expose a durable cursor across mixed message/tool/permission
  streams.
- The frontend currently uses narrow hydration opportunistically after runtime
  events. Busy polling and failed narrow reads still fall back to full
  `SessionActivity`.
- Narrow activity does not introduce an MCP request UI or restore stale MCP
  auth/elicitation actionability. Terminal MCP request semantics remain covered
  by existing request/recovery/replay paths and Phase 6.8 follow-up risks.
- No automatic resume, stale running/waiting tool recovery, stale permission
  actionability, Run store, Run database migration, frontend Run UI, persisted
  interrupted acknowledgement field, or prose-derived artifact/checkpoint
  inference was added.

### Phase 6.8: Hosted MCP Replay/Auth/Elicitation Follow-up

Status: implemented for deterministic local replay/restart hardening and
manual hosted-provider smoke criteria. This remains a validation/hardening
phase, not a Run implementation.

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
- Narrow activity reads, when used during hosted MCP validation, preserve the
  same terminal MCP request/actionability semantics as full `SessionActivity`
  and recovery/replay evidence. Event payloads may select the narrow read, but
  must not recreate an actionable auth or elicitation request.
- If provider automation is not safe, the phase records an explicit manual
  validation gap and keeps the runtime/frontend boundaries unchanged.

Implemented:

- Hardened startup recovery for runtime MCP requests:
  - pending/required MCP auth and elicitation requests from a previous runtime
    are marked `cancelled` during startup recovery
  - cancellation uses the existing terminal MCP request status path and records
    runtime event/audit/replay evidence
  - no new persisted acknowledgement field was added
  - normal same-process recovery/listing still preserves currently pending MCP
    requests; only startup recovery prevents stale actionability after restart
- Extended the deterministic streamable HTTP and legacy SSE MCP interrupted
  structured-ref fixture:
  - after completed MCP scheduler output is recorded, the fixture closes MCP
    client connections before runtime interruption/restart
  - restart hydration still derives produced/verified artifacts from completed
    scheduler output and persisted structured refs
  - replay evidence includes the completed MCP tool call and structured output
    artifact ref
  - no running/pending/waiting MCP tool is restored after restart
- Added focused terminal MCP request coverage for hosted-style auth and
  elicitation:
  - stale auth and elicitation requests become terminal `cancelled` on startup
    recovery
  - `RecoveryStatus` no longer returns them as pending/actionable
  - `DecideMCPRequest` cannot re-open the terminal cancelled request
  - `ReplayExport` preserves redacted cancelled evidence and reports zero
    pending MCP request actionability
  - `TurnActivity` exposes terminal MCP request events only as hydrated runtime
    evidence; event payloads are not an actionability source

Validation:

- `go test ./internal/runtime -run "TestRuntimeMCPStartupCancelsStaleActionableAuthAndElicitationRequests|TestRuntimeHTTPAndSSEMCPInterruptedStructuredRefsFixture|TestRuntimeMCPPartialStructuredOutputCancelledOnRestartDoesNotProduceArtifact" -count=1`

Hosted provider manual smoke checklist:

- Record provider name and MCP transport.
- Record auth/elicitation setup without secrets, tokens, redirect cookies, or
  browser state in repo fixtures, docs, logs, React state, or screenshots.
- Start a turn that reaches an MCP auth or elicitation request.
- Record request kind/status before restart using runtime APIs with redacted
  output only.
- Restart the desktop/runtime before answering the request.
- Verify `RecoveryStatus` has no stale pending MCP auth/elicitation request.
- Verify `SessionActivity` restores the interrupted turn/tool evidence without
  any running/waiting stale MCP tool.
- Verify `ReplayExport` contains terminal redacted MCP request evidence and no
  pending MCP request recovery count.
- If narrow reads are used, verify `TurnActivity`/`SessionActivityWindow`
  return only hydrated runtime evidence and do not recreate actionable auth or
  elicitation state from event payloads.

Remaining risks:

- Real hosted OAuth/browser-mediated flows remain provider- and SDK-dependent.
  They require manual smoke because deterministic automation would need
  credentials or browser auth state that must not be stored in repo fixtures or
  React state.
- Legacy SSE still lacks a deterministic SDK hook for a true provider replay
  after transport disconnect. The local fixture now covers completed-output
  disconnect before restart; unfinished/partial transport state remains covered
  by cancellation/no-artifact evidence.
- This phase does not add automatic resume, stale tool recovery, stale
  permission or MCP request actionability, Run persistence, Run UI, database
  migrations, durable narrow cursors, or prose-derived artifact/checkpoint
  inference.

### Phase 6.9: Narrow Activity Cursor, Rollout, And Hosted MCP Smoke

Status: accepted after review for the durable narrow activity cursor contract,
frontend rollout hardening, bridge/browser contract coverage, and
deterministic hosted MCP recovery guarantees. Real hosted-provider
OAuth/elicitation smoke remains a redacted manual gap because no safe
credentials/browser auth state were available in this workspace. This is not a
Run implementation.

Scope:

- Replace the simple tail-by-turn session activity window with a durable cursor
  contract across mixed runtime evidence:
  - messages
  - turns
  - tool calls
  - permissions
  - runtime events
- Define stable ordering semantics for session windows so long sessions can
  hydrate around the active tail without losing adjacent user/assistant
  messages, terminal permissions, diagnostics warnings, artifact refs, or
  interrupted summaries.
- Add browser/Vite and packaged Wails smoke coverage for the frontend narrow
  hydration path:
  - event-triggered `TurnActivity`
  - event-triggered `SessionActivityWindow`
  - fallback to full `SessionActivity`
  - no duplicate timeline items after multiple lifecycle events
- Consider a runtime feature flag or adapter capability check for staged
  rollout if large-session validation finds ordering or merge regressions.
- Execute the Phase 6.8 hosted-provider manual smoke checklist against one or
  more real hosted MCP providers when credentials and browser OAuth can be
  handled outside repo fixtures.
- Cover provider-specific timing that deterministic local fixtures cannot
  safely automate:
  - browser-mediated OAuth redirects and token refresh
  - provider-specific elicitation prompts and cancellation paths
  - successful streamable HTTP provider replay after a transport disconnect
  - legacy SSE provider behavior where SDK replay hooks are limited
- Store only redacted observations under
  `C:\Users\ytq\work\ai\agent-builder\tmp\runtime-dev` during validation.
  Do not write secrets, OAuth tokens, cookies, browser profiles, or provider
  auth state into repo fixtures, committed docs, logs, screenshots, or React
  state.
- Preserve current boundaries:
  - full `SessionActivity` remains the parity oracle and fallback
  - runtime events remain refresh triggers only
  - no automatic resume
  - no stale running/waiting tool recovery
  - no restored actionable permission gate
  - no restored actionable MCP auth or elicitation request
  - no Run store, Run state machine, Run database migration, frontend Run UI,
    persisted interrupted acknowledgement field, or prose-derived
    artifact/checkpoint inference

Acceptance:

- A cursor-based session window can hydrate the active tail of a large session
  and prove parity with the corresponding full `SessionActivity` subset for
  messages, tool calls, permissions, diagnostics, artifact evidence,
  interrupted summaries, and terminal permission/MCP semantics.
- Browser/Vite validation proves event-triggered narrow hydration updates the
  visible timeline without using event payloads as state.
- Packaged Wails validation proves regenerated bridge bindings expose narrow
  methods and fallback safely to full `SessionActivity`.
- Repeated lifecycle, permission, artifact/ref, and terminal events do not
  duplicate or resurrect stale timeline/actionability state.
- For each provider smoke, record provider name, transport, redacted setup,
  request kind/status before restart, restart timing, and post-restart
  `RecoveryStatus`, `SessionActivity`, and `ReplayExport` evidence.
- Verify no stale MCP auth or elicitation request remains actionable after
  restart.
- Verify completed scheduler output is the only source of produced refs.
- Verify unfinished/partial/disconnected MCP tools are terminal cancelled and
  do not produce artifact evidence.
- If narrow reads are used, verify `TurnActivity`/`SessionActivityWindow`
  preserve the corresponding full `SessionActivity` subset semantics and do
  not use event payloads to recreate actionability.

Implemented:

- Replaced the simple tail-by-turn `SessionActivityWindow` selection with a
  cursor-based mixed-evidence window. The cursor order is stable across:
  - messages
  - turns
  - tool calls
  - permissions
  - runtime events
- Added `RuntimeActivityWindow` cursor metadata:
  - `cursor`
  - `firstCursor`
  - `lastCursor`
  - `hasMoreBefore`
  - `hasMoreAfter`
  - `evidenceCount`
- Added transport-neutral `SessionActivityCursorWindow(sessionID, cursor,
  limit)` while preserving the existing `SessionActivityWindow(sessionID,
  limit)` bridge/API fallback.
- Updated HTTP/dev-module `/v1/sessions/{session_id}/activity-window` to accept
  `cursor` and `limit`.
- Updated the Wails bridge with `SessionActivityCursorWindow`; packaged bridge
  callers can use the cursor method when available and fall back to the old
  method or full `SessionActivity`.
- Updated the frontend adapter to prefer the cursor-capable activity window
  after session-level event hints while keeping `TurnActivity` for turn-level
  hints and full `SessionActivity` as fallback.
- Kept runtime events as refresh triggers only. Event payloads still only
  choose which runtime DTO to read; they are not merged into timeline,
  diagnostics, artifact, interrupted, permission, or MCP actionability state.
- Kept full `SessionActivity` as the fallback and parity oracle. Cursor windows
  hydrate selected evidence through the same Go diagnostics/interrupted helper
  path as full activity.
- Recorded the hosted-provider smoke result as a redacted manual gap in
  `tmp/runtime-dev/phase-6.9-hosted-mcp-smoke-redacted.md`; no secrets,
  cookies, OAuth tokens, screenshots, browser profiles, provider auth state, or
  React state were written to repo fixtures or docs.

Validation:

- `go test ./internal/runtime -run "TestRuntimeSessionActivityCursorWindowPreservesMixedEvidenceParity|TestRuntimeHTTPServerRoutesNarrowActivityToRuntimeService|TestRuntimeMCPStartupCancelsStaleActionableAuthAndElicitationRequests|TestRuntimeHTTPAndSSEMCPInterruptedStructuredRefsFixture|TestRuntimeMCPPartialStructuredOutputCancelledOnRestartDoesNotProduceArtifact" -count=1`
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgePhase62PackagedHandoffRecoveryContract" -count=1`

Parity coverage added:

- Cursor windows prove parity with the corresponding full `SessionActivity`
  subset for mixed message/turn/tool/permission/event evidence.
- The cursor window test verifies diagnostics, produced artifact evidence,
  terminal permission evidence, interrupted turn state, and runtime event
  sequence evidence for the selected turn.
- HTTP/dev-module route coverage verifies browser/Vite fallback paths forward
  cursor and limit to the runtime service.
- Wails bridge coverage verifies packaged bridge exposure for
  `SessionActivityCursorWindow`, existing `SessionActivityWindow`, and
  `TurnActivity`.

Hosted MCP validation result:

- Deterministic Phase 6.8/6.9 tests continue to prove:
  - restart does not restore stale actionable MCP auth or elicitation requests
  - completed scheduler output is the only source of produced refs
  - unfinished/partial/disconnected MCP tools are cancelled and do not produce
    artifact evidence
  - narrow reads expose hydrated runtime evidence only and do not use event
    payloads to recreate actionability
- Real hosted OAuth/provider-specific elicitation smoke was not run because it
  requires credentials or browser auth state that must not be automated or
  persisted in this repo. The manual checklist remains in `tmp/runtime-dev` as
  a redacted validation artifact.

Remaining risks:

- Cursor windows are additive rollout infrastructure; full `SessionActivity`
  remains the safe fallback and parity oracle.
- Real hosted OAuth/browser-mediated MCP flows still require an operator-run
  manual smoke with redacted notes when credentials are available.
- Legacy SSE provider replay remains provider/SDK dependent beyond the local
  completed-output disconnect and partial-output cancellation fixtures.
- This phase did not add automatic resume, stale tool recovery, stale
  permission or MCP request actionability, Run persistence, Run UI, database
  migrations, persisted interrupted acknowledgement, or prose-derived
  artifact/checkpoint inference.

Acceptance review:

- Accepted commit: `4fe4beb12 Implement Phase 6.9 narrow activity cursor`.
- Review confirmed the implementation stayed within the Phase 6.9 boundary:
  - no Run state machine, runtime Run store, Run migration, or frontend Run UI
  - no automatic resume
  - no stale running/waiting tool recovery
  - no restored stale actionable permission gate
  - no restored stale actionable MCP auth or elicitation request
  - no event payload merge into timeline, diagnostics, artifact, interrupted,
    permission, or MCP actionability state
  - no assistant-prose artifact/checkpoint inference
- Full `SessionActivity` remains the fallback and parity oracle.
- Cursor-window activity remains an additive runtime DTO/API surface.
- Hosted-provider OAuth/elicitation is accepted as a manual redacted validation
  gap, not a reason to keep Phase 6.9 open, because closing it safely requires
  operator-held credentials or browser auth state outside repo fixtures.

### Phase 7: Claude Code Runtime Mapping And Read-only Run DTO Design Gate

Status: completed as a documentation/design gate only. No runtime Run store,
database migration, automatic resume, background Run scheduler, or frontend Run
UI was implemented.

Purpose:

- Ground Agent Builder's future Run design in the Claude Code runtime model
  under `C:\Users\ytq\work\ai\myclaw\claude-code`, not in a new invented
  abstraction.
- Define a read-only Run DTO candidate that can be derived from current Agent
  Builder evidence before any durable Run persistence exists.
- Preserve the current Phase 6.x safety boundary while deciding what a future
  Run implementation must prove.

Claude Code findings:

- `QueryEngine` owns conversation lifecycle. One engine represents one
  conversation, and each `submitMessage()` starts a new turn while mutable
  messages, permission denials, usage, read-file state, skill discovery, and
  memory state persist across turns.
- `QueryEngine.submitMessage()` records accepted user messages to transcript
  before the query loop returns assistant output, making a killed or stopped
  request resumable from the accepted user message.
- The query loop has an internal `State`, drains notifications by `agentId`,
  and separates main-thread progress from subagent/background notifications.
- Session resume reconstructs engineering context from transcript and metadata,
  not by pasting chat history into memory. Claude Code restores or accounts for
  transcript messages, content replacements, file history, attribution, todos,
  agent/mode/model metadata, and worktree state.
- Agent work is modeled as a task object. `TaskState` includes local agent,
  remote agent, shell, workflow, teammate, monitor MCP, and other task forms.
  `AppState` tracks `tasks`, task selection, foreground/background identity,
  and remote task counts.
- Local/remote agent tasks have durable identity, status, summaries, output
  files, and sidecar metadata. Backgrounding changes lifecycle visibility; it
  does not turn the agent into a stateless tool result.
- `resumeAgentBackground()` reads the subagent transcript and metadata,
  filters unresolved tool uses and orphan/empty assistant messages, restores
  replacement state and worktree cwd when possible, then continues with a new
  user prompt through the async agent lifecycle. It is continuation, not a
  fresh spawn.
- Background agent output reaches the main thread through task notification
  and output-file evidence, not by continuously merging every worker stream
  into the main transcript.

Mapping to Agent Builder:

| Claude Code concept | Agent Builder counterpart | Phase 7 rule |
| --- | --- | --- |
| `QueryEngine` conversation | Runtime service + `Session` | Keep session/runtime evidence authoritative. |
| `submitMessage()` turn | `RuntimeTurn` | A Run can reference turns but must not replace them. |
| transcript messages | persisted messages + `SessionActivity` | Run DTO reads message evidence; it does not infer from prose. |
| query loop tool stream/result | `RuntimeToolCall` + refs | Produced artifacts come only from completed structured tool evidence. |
| permission denials/callbacks | `RuntimePermissionRequest` | Terminal permission evidence is read-only; stale gates are not restored. |
| session metadata/recovery | runtime events, audit, replay, recovery status | Events trigger reads only; they are not Run state. |
| local/remote/background task state | `RuntimeAgentTask` | Run DTO can reference task ids and summaries, not own task lifecycle. |
| subagent transcript/output file | task/ref/artifact evidence | Future artifact handles must be structured refs, not assistant prose. |
| `resumeAgentBackground()` | future explicit checkpoint resume | Resume must be user-triggered continuation from a checkpoint, never auto replay. |
| worktree/session sidecar metadata | workspace/session metadata | Future persistence must carry workspace identity before enabling resume. |

Read-only Run DTO candidate:

```text
RuntimeRunSummary
  id
  workspace_id
  session_ids[]
  primary_session_id
  objective
  status: active | waiting_user | interrupted | completed | failed | cancelled
  turn_ids[]
  task_ids[]
  tool_call_ids[]
  permission_request_ids[]
  expected_artifacts[]
  produced_artifacts[]
  verified_artifacts[]
  checkpoints[]
  diagnostics
  interrupted
  user_actions
    resume[]
    discard[]
  evidence_cursor
  created_at
  updated_at
  finished_at optional
```

Phase 7 stability decision:

- The DTO shape is stable enough as a read-only projection vocabulary because
  every field maps to an existing Agent Builder primitive or to a Claude Code
  runtime concept.
- The DTO is not stable enough for a database schema yet. `id`, objective,
  checkpoint identity, cross-session grouping, and workspace/worktree metadata
  need product semantics before migration.
- A future API may expose `GET /v1/runs/{run_id}` and `GET /v1/runs`, but the
  first implementation must derive data from sessions, turns, tool calls,
  permissions, agent tasks, runtime events, replay, and `SessionActivity`.
- `SessionActivity` remains the full fallback and parity oracle. Any Run DTO
  subset must prove parity for messages, tool calls, permissions, diagnostics,
  artifact evidence, interrupted summaries, terminal permission semantics, and
  terminal MCP auth/elicitation semantics.

Required future implementation gates:

- Define how a Run id is assigned without hiding turn/session identity.
- Define when a Run crosses session boundaries and how workspace/worktree
  metadata participates.
- Define checkpoint evidence as structured runtime data. Do not infer
  checkpoint or artifact state from assistant prose.
- Define user-triggered resume as a new turn that references a checkpoint
  summary and current workspace state.
- Prove restart behavior does not restore stale running/waiting tools,
  actionable permission gates, or actionable MCP auth/elicitation requests.
- Prove runtime events only select DTO refreshes and never hydrate Run state
  directly into React.

Boundary:

- No full Run state machine.
- No runtime Run store.
- No Run database migration.
- No automatic resume.
- No background Run scheduler.
- No frontend Run UI.
- No stale running/waiting tool recovery.
- No stale actionable permission, MCP auth, or elicitation recovery.
- No assistant-prose artifact/ref/checkpoint inference.
- `pending-at-interruption` remains a computed diagnostics signal.
- Interrupted acknowledgement remains `MarkInterruptedDone` / terminal
  cancelled semantics, without a new persisted acknowledgement field.

Validation:

- Reviewed Claude Code source and docs for `QueryEngine`, `query`, session
  restore, task state, local/remote agent task identity, and background agent
  resume.
- Reviewed Agent Builder Phase 6 through Phase 6.9 constraints and the existing
  `Turn / Task / Run` model.
- Performed a docs-only design review; no Go or TypeScript code changed, so no
  Go/TS test suite was required for this phase.

Phase 7 review conclusion:

- Claude Code supports the direction of durable, resumable engineering work,
  but it does not justify jumping straight to an Agent Builder Run table or
  automatic resume. Its lesson is stricter: durable evidence, task identity,
  transcript/session metadata, and explicit continuation semantics must be in
  place before persistence is promoted to a product-level Run object.
- Agent Builder should therefore validate a read-only Run projection first.
  Full Run persistence and UI should be a later phase after DTO parity and
  resume/checkpoint semantics are proven against existing runtime evidence.

### Phase 7.1: Read-only Run Projection Spike

Status: implemented as an internal runtime projection and test-only spike. No
runtime Run store, Run database migration, public HTTP/Wails Run API, automatic
resume, background Run scheduler, or frontend Run UI was implemented.

Scope:

- Add a read-only `RuntimeRunProjection` DTO vocabulary inside the runtime
  package.
- Build `RunProjection(ctx, RuntimeRunProjectionRequest)` from the same
  existing evidence used by `SessionActivity`:
  - messages
  - turns
  - tool calls
  - permissions
  - runtime events
  - `RuntimeAgentTask`
- Keep the projection internal to Go runtime tests. It is not part of the
  transport-neutral `RuntimeService` interface and is not exposed through
  HTTP, Wails, or React.
- Use the projection to validate field semantics before any durable Run table
  or product UI exists.

Implemented:

- Added `RuntimeRunProjectionRequest`, `RuntimeRunProjectionResponse`,
  `RuntimeRunProjection`, checkpoint, diagnostics, user-action, and source DTO
  types.
- Added `runtimeService.RunProjection(...)` as an internal read-only projection
  method.
- Added `runtimeAgentTaskStore.ListBySession(...)` to read existing task
  evidence by parent or child session. This is a query-only helper over the
  existing table; it does not add a schema migration.
- Derived Run ids as `run:session:<session_id>` for the spike. This keeps the
  DTO stable enough for tests without claiming durable product identity.
- Derived status from existing turn/task terminal state:
  - `active`
  - `waiting_user`
  - `interrupted`
  - `completed`
  - `failed`
  - `cancelled`
- Derived checkpoints only from structured interrupted turn summaries and final
  task evidence. The projection does not infer checkpoints from assistant
  prose.
- Added explicit read-only user action DTOs for interrupted checkpoints. They
  describe possible resume/discard UX, but do not execute resume, persist
  acknowledgement, or restore stale actionability.
- Marked projection source as `session_activity_projection`, `readOnly: true`,
  and `sessionActivityParity: true`.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunProjection" -count=1`
- `go test ./internal/runtime -count=1`

Parity coverage:

- `TestRuntimeRunProjectionDerivesFromSessionActivityEvidence` verifies the
  projection uses full `SessionActivity` evidence for turn ids, tool call ids,
  permission ids, produced/verified artifacts, interrupted checkpoints, and
  terminal permission counts.
- `TestRuntimeRunProjectionCursorWindowKeepsSessionActivityParity` verifies a
  cursor-window projection keeps the same selected turn ids and diagnostics as
  the corresponding `SessionActivityCursorWindow`.
- The tests include `RuntimeAgentTask` evidence so task ids and child session
  ids can be summarized without creating a Run store.

Review conclusion:

- Phase 7.1 proves the read-only Run DTO is useful as a projection over current
  runtime evidence.
- `SessionActivity` remains the fallback and parity oracle.
- Runtime events remain evidence selection/hydration inputs only. Event
  payloads are not merged into Run state.
- The spike still does not justify a database schema. Durable Run identity,
  objective ownership, checkpoint persistence, cross-session grouping, and
  workspace/worktree metadata need a separate implementation gate.

Remaining risks:

- Run id assignment is still provisional and session-derived.
- Cross-session Run grouping is only represented by task child session ids; no
  product-level grouping policy exists yet.
- Resume/discard are read-only DTO candidates only. No resume execution,
  acknowledgement persistence, or UI action wiring exists.
- No public API or frontend adoption exists yet, by design.

### Phase 7.2: Run Projection Contract Review And Transport Gate

Status: implemented for read-only transport exposure. No runtime Run store, Run
database migration, automatic resume, background Run scheduler, frontend Run UI,
or resume/discard execution was implemented.

Scope:

- Promote the Phase 7.1 internal read-only Run projection to a transport
  contract.
- Expose Run projection through:
  - transport-neutral `RuntimeService`
  - HTTP/dev-module runtime adapter
  - packaged Wails `RuntimeBridge`
  - client bridge capability typing and HTTP fallback
- Keep `SessionActivity` as the fallback and parity oracle.
- Keep frontend adoption out of scope. The client bridge can call the endpoint,
  but `hydrateWorkbench` and React UI do not consume it.

Implemented:

- Added `RunProjection(ctx, RuntimeRunProjectionRequest)` to
  `RuntimeService`.
- Added `GET /v1/sessions/{session_id}/run-projection?limit=N&cursor=C`.
- Added dev-module fallback support for the same route.
- Added runtime API contract entry for
  `/v1/sessions/{session_id}/run-projection`.
- Added Wails bridge aliases and `RuntimeBridge.RunProjection(...)`.
- Added optional `RunProjection` to the client runtime bridge module and HTTP
  fallback. This is a capability only; it is not wired into UI state.

Validation:

- `go test ./internal/runtime -run "TestRuntimeHTTPServerRoutesNarrowActivityToRuntimeService|TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy|TestRuntimeRunProjection" -count=1`
- `go test ./internal/runtimeapi -count=1`
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgePhase62PackagedHandoffRecoveryContract" -count=1`
- `cd client && npx tsc -b --pretty false`

Contract coverage:

- HTTP and dev-module tests verify session id, cursor, and limit are forwarded
  to the runtime service.
- Wails bridge tests verify `RuntimeBridge.RunProjection` delegates to the
  runtime service and preserves the read-only/parity source flags.
- Runtime projection tests continue to prove `SessionActivity` parity for
  selected full and cursor-window evidence.

Review conclusion:

- Run projection is now safe to read through transport adapters as an additive
  DTO.
- Runtime events may trigger a future Run projection refresh, but event payloads
  still must not be merged into Run state.
- The endpoint is not a persisted Run resource. It is a session-scoped
  projection over existing runtime evidence.
- Resume/discard remain descriptive read-only user-action DTO candidates only.

Remaining risks:

- Public frontend adoption still needs a separate UI design gate.
- Durable Run identity, cross-session grouping, checkpoint persistence,
  workspace/worktree semantics, and resume execution remain unresolved.
- A future persisted Run implementation still requires an explicit migration
  and backfill design.

### Phase 7.3: Run Projection Frontend Read-only Preview Gate

Status: implemented as an additive read-only preview. No runtime Run store, Run
database migration, automatic resume, background Run scheduler, executable
resume/discard control, or persisted Run UI was implemented.

Scope:

- Decide whether the client can safely display a read-only Run summary hydrated
  from `RunProjection`.
- Add a frontend view-model field for the transport DTO without making React
  the runtime source of truth.
- Hydrate the preview from `RuntimeBridge.RunProjection(...)` or the HTTP/dev
  fallback during normal workbench refresh.
- Keep `SessionActivity` as the timeline, diagnostics, permission, artifact,
  interrupted-state fallback and parity oracle.

Implemented:

- Added `RunProjectionViewModel` to the workbench model with only aggregate
  status, count, cursor, and source/parity fields.
- Added client mapping from `RuntimeRunProjectionResponse` to that read-only
  view model.
- `hydrateWorkbench` now calls `RunProjection({ sessionId, limit: 24 })` when
  the active bridge supports it.
- Added `RunProjectionPreview`, a read-only diagnostics-column panel that shows
  status, turn/tool/artifact counts, permission/task/checkpoint counts, cursor
  source, and `SessionActivity` parity.
- Cleared stale preview state when creating a draft/new session and only reused
  a previous preview when its `primarySessionId` still matches the active
  session.

Boundary review:

- Runtime events still only trigger `adapter.refresh`; event payloads do not
  merge into Run, checkpoint, artifact, permission, MCP actionability, or
  interrupted state.
- The preview does not expose `userActions`, `resume`, or `discard`.
- The preview does not infer artifact/ref/checkpoint state from assistant
  prose.
- The preview does not recover stale running/waiting tools, stale permission
  gates, or stale MCP auth/elicitation actionability.
- `SessionActivity` remains the authority for the timeline and diagnostics.

Validation:

- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- `go test ./internal/runtime -run "TestRuntimeRunProjection" -count=1`
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgePhase62PackagedHandoffRecoveryContract" -count=1`
- Browser/Vite smoke against `http://localhost:5180/` after build/typecheck.

Remaining risks:

- The projection id remains session-derived and is not a durable Run id.
- Cross-session grouping, checkpoint persistence, workspace/worktree semantics,
  and executable resume/discard still need a separately approved Run
  implementation phase.
- This is a preview of the current projection contract, not a replacement for
  `SessionActivity`.

### Phase 8: Durable Run Identity And Persistence Design Gate

Status: completed as a documentation/design gate only. No runtime Run store,
database migration, automatic resume, background Run scheduler, executable
resume/discard control, or expanded frontend Run UI was implemented.

Purpose:

- Decide when Agent Builder can move from the Phase 7 read-only Run projection
  to a durable Run identity.
- Ground the design in Claude Code's transcript/session/task model while
  preserving Agent Builder's current `SessionActivity` parity guarantees.
- Define the minimum persistence contract that Phase 8.1 must implement before
  any real resume/discard behavior can be exposed.

Claude Code and external documentation findings:

- Claude Code sessions start with fresh model context, but persistent project
  instructions and memory are loaded at session start. This reinforces that
  durable runtime state must be explicit evidence, not browser or model memory.
  Reference: https://code.claude.com/docs/en/memory
- Claude Code permissions are enforced by the runtime with rules, modes, and
  hooks. Prompt text or memory can influence intent, but cannot grant
  permission. Reference: https://code.claude.com/docs/en/permissions
- Claude Code hooks can participate in permission evaluation, but deny/ask
  rules remain authoritative. This matches Agent Builder's rule that stale
  permission or MCP actionability must not be restored from persisted Run data.
  Reference: https://code.claude.com/docs/en/hooks
- The local Claude Code study docs show session recovery rebuilds from
  transcript, metadata, file history, todos, agent/mode/model metadata, and
  worktree state; background agent resume reads transcript/metadata and
  continues with a new prompt instead of replaying a previous stream.

Design decisions:

- New Runs get a runtime-generated durable id, independent from session id.
  Suggested prefix: `run_` plus a collision-resistant id.
- Legacy/backfilled Runs may use a deterministic compatibility id such as
  `run:session:<session_id>` until migrated to generated ids. This makes
  backfill idempotent and keeps existing Phase 7 projections stable.
- A Run is a top-level user-visible work unit. A Run can reference multiple
  sessions when a task creates child sessions, background agent tasks, or
  isolated worktree sessions.
- `primary_session_id` remains required. Session identity is not hidden by the
  Run; the Run only groups structured runtime evidence.
- Run status is a persisted summary over linked turns/tasks/checkpoints:
  `active`, `waiting_user`, `interrupted`, `completed`, `failed`, `cancelled`,
  or `discarded`. This is not a full Run state machine yet; Phase 8.1 must
  keep status transitions derived from turn/task terminal evidence and explicit
  user actions.
- `SessionActivity` remains the fallback and parity oracle. Persisted Run rows
  are not allowed to become the source of timeline, diagnostics, artifact,
  interrupted, permission, or MCP actionability state.

Proposed schema contract, not a migration:

```text
runtime_runs
  id TEXT PRIMARY KEY
  workspace_id TEXT NOT NULL
  primary_session_id TEXT NOT NULL
  objective TEXT
  status TEXT NOT NULL
  created_at INTEGER NOT NULL
  updated_at INTEGER NOT NULL
  finished_at INTEGER
  discarded_at INTEGER
  source TEXT NOT NULL              -- user_prompt | backfill | task | recovery
  metadata_json TEXT

runtime_run_sessions
  run_id TEXT NOT NULL
  session_id TEXT NOT NULL
  role TEXT NOT NULL                -- primary | child | task | recovery
  task_id TEXT
  turn_id TEXT
  worktree_id TEXT
  created_at INTEGER NOT NULL
  PRIMARY KEY (run_id, session_id)

runtime_run_checkpoints
  id TEXT PRIMARY KEY
  run_id TEXT NOT NULL
  session_id TEXT NOT NULL
  turn_id TEXT
  task_id TEXT
  status TEXT NOT NULL              -- open | resumable | acknowledged | discarded
  summary TEXT
  artifact_refs_json TEXT
  diagnostic_refs_json TEXT
  created_at INTEGER NOT NULL
  acknowledged_at INTEGER
  discarded_at INTEGER
  metadata_json TEXT
```

Backfill and migration requirements for Phase 8.1:

- Backfill must be idempotent. Running the migration twice must not duplicate
  Runs, session links, or checkpoints.
- Backfill should create one compatibility Run per existing session unless
  `runtime_agent_tasks` already proves parent/child session grouping.
- Backfill may link child sessions through `runtime_agent_tasks`, but must not
  infer grouping from assistant prose.
- Backfill must not mark stale running/waiting turns, stale permissions, or
  stale MCP auth/elicitation requests as actionable. Startup recovery should
  normalize them to terminal/cancelled evidence before Run status is computed.
- Backfill must not invent produced artifacts. Produced/verified artifact state
  still comes from completed structured tool/task evidence and diagnostics.

Resume/discard contract:

- Resume is a new explicit user-triggered turn linked to a checkpoint. It is
  not automatic replay and not a continuation of an in-memory request.
- Resume prompt construction must read current workspace state and checkpoint
  summary, then ask the model to continue from that state. It must not replay
  completed tool calls.
- Discard acknowledges a checkpoint or Run for product UX. It must not delete
  evidence and must not change terminal turn/tool/permission/MCP records.
- `MarkInterruptedDone` remains the current acknowledgement path until the
  Phase 8.1 persistence implementation explicitly introduces checkpoint
  acknowledgement rows.

API contract for the first persisted implementation:

- `GET /v1/runs` can list durable Run summaries.
- `GET /v1/runs/{run_id}` can return a persisted Run summary plus a read-only
  `RunProjection` parity payload.
- `GET /v1/sessions/{session_id}/run-projection` remains available as the
  legacy/fallback projection endpoint.
- `POST /v1/runs/{run_id}/resume` and
  `POST /v1/runs/{run_id}/discard` must stay out of Phase 8.1 unless Phase 8.1
  explicitly includes action semantics, audit tests, restart tests, and
  permission/MCP stale-actionability tests.

Frontend contract:

- The Phase 7.3 preview can continue to display `RunProjectionViewModel`.
- A persisted Run UI must remain read-only until resume/discard actions are
  separately implemented and tested.
- React must never be the source of Run status. It may request `GET /v1/runs`
  or `GET /v1/runs/{run_id}` after runtime events, but event payloads may only
  choose which DTO to refresh.

Acceptance criteria for Phase 8.1 implementation:

- Add migrations and stores only after this Phase 8 design is accepted.
- Add contract tests proving new Run ids are durable and session links are
  idempotent.
- Add backfill tests for single-session, child-session, interrupted, failed,
  cancelled, and completed evidence.
- Add restart tests proving stale running/waiting tools, stale permission
  gates, stale MCP auth, and stale MCP elicitation are terminalized or kept
  non-actionable.
- Add parity tests proving persisted Run summaries match the corresponding
  `SessionActivity`/`RunProjection` subset for messages, turns, tool calls,
  permissions, diagnostics, artifact evidence, interrupted summaries, and
  terminal permission/MCP semantics.

Validation:

- Reviewed existing Agent Builder tables and stores for sessions, messages,
  runtime turns, tool calls, permission requests, agent tasks, worktrees,
  runtime events, audit events, compact boundaries, output refs, and MCP
  requests.
- Reviewed local Claude Code study docs for session persistence/recovery,
  permissions, agents/tasks, and background resume.
- Reviewed current Claude Code docs for memory, permissions, hooks, and
  settings.
- Performed a docs-only design review; no Go or TypeScript code changed.

Review conclusion:

- Agent Builder is ready to design a persisted Run identity, but not to expose
  resume/discard execution in the same step.
- The next implementation phase should be narrow: durable Run id, run/session
  links, checkpoint persistence rows, idempotent backfill, read-only APIs, and
  parity/restart tests.
- Background scheduling, automatic resume, and full frontend Run management
  remain explicitly out of scope until persisted evidence proves stable.

### Phase 8.1: Durable Run Identity, Persistence Store, And Backfill

Status: implemented as a read-only persistence foundation.

Implemented:

- Added the `runtime_runs`, `runtime_run_sessions`, and
  `runtime_run_checkpoints` migration.
- Added a `runtimeRunStore` with generated durable Run ids for new user-prompt
  sessions and deterministic `run:session:<session_id>` ids for legacy
  projection backfill.
- Added idempotent session-link and checkpoint upsert behavior. Repeated
  backfill does not duplicate Runs, session links, or checkpoint evidence.
- Added read-only runtime DTOs and transport endpoints:
  `GET /v1/runs`, `GET /v1/runs/{run_id}`, Wails `Runs`, and Wails `Run`.
- `GET /v1/runs/{run_id}` returns persisted Run summary metadata plus a
  read-only `RunProjection` parity payload. The projection is still hydrated
  from `SessionActivity`-derived runtime evidence.
- `Chat` now ensures a durable Run summary for the active/new session before
  creating the turn. This only creates/read-updates summary metadata; it does
  not schedule, resume, or replay work.
- `RunProjection` can backfill/update persisted Run summaries when the Run
  store is available, while preserving `SessionActivity` as the parity oracle.
- Empty projection reads do not complete an already active/waiting/interrupted
  durable Run summary.

Validation:

- Store tests cover generated durable ids, idempotent session links, projection
  backfill, checkpoint dedupe, and the empty-projection active-run guard.
- HTTP tests cover browser/dev transport routes for `GET /v1/runs`,
  `GET /v1/runs/{run_id}`, and dev-module forwarding.
- Wails bridge tests cover `Runs` and `Run` delegation.
- Runtime API contract tests include the new read-only Run routes.

Boundary review:

- No full Run state machine was implemented.
- No background Run scheduler or automatic resume was implemented.
- No executable resume/discard API was implemented.
- No frontend Run management UI was implemented.
- No runtime event payload hydrates Run state; events remain refresh triggers.
- No assistant prose is used to infer artifacts, refs, checkpoints, or Run
  actionability.
- Permission and MCP actionability remain sourced from existing runtime records
  and recovery semantics, not persisted Run rows.

Remaining risks:

- Phase 8.1 persists Run identity and summary metadata only. It does not yet
  implement user-facing checkpoint acknowledgement, resume, or discard.
- Cross-session grouping is limited to existing projection evidence and task
  links; richer worktree/background task grouping still needs an accepted
  action phase.
- Restart parity for stale hosted MCP auth/elicitation remains covered by the
  existing recovery/MCP tests and docs; Phase 8.1 does not add new hosted
  provider credentials or browser OAuth fixtures.

### Phase 8.2: Checkpoint Acknowledgement, Resume, And Discard Action Gate

Status: accepted as a design gate only. No executable action APIs, automatic
resume, background scheduling, frontend Run management UI, or full Run state
machine was implemented in this phase.

Purpose:

- Decide the exact contract for future user-triggered checkpoint
  acknowledgement, resume, and discard actions.
- Convert the Phase 8.1 read-only durable Run foundation into a safe action
  design without letting persisted Run rows become the source of runtime
  actionability.
- Define the minimum tests required before any `POST /v1/runs/{run_id}/resume`
  or `POST /v1/runs/{run_id}/discard` endpoint can be implemented.

Design constraints:

- Resume must create a new explicit user turn. It must not replay a previous
  stream, completed tool call, permission request, MCP auth request, or MCP
  elicitation request.
- Resume prompt construction must use structured checkpoint summary, current
  workspace state, linked session/task metadata, and current model/provider
  configuration. It must not infer task state from assistant prose.
- Discard is an acknowledgement of a checkpoint or Run for UX purposes. It must
  not delete evidence or rewrite terminal turn/tool/permission/MCP records.
- Checkpoint acknowledgement should live in `runtime_run_checkpoints` through
  `acknowledged_at`/`discarded_at` only after the action API phase explicitly
  accepts that storage contract.
- `MarkInterruptedDone` remains the current interrupted acknowledgement path
  until the checkpoint action implementation replaces or bridges it with tests.
- Permission and MCP auth/elicitation actionability must continue to come from
  current runtime stores and recovery normalization. Persisted Run/checkpoint
  rows may describe candidate actions, but must not make stale requests
  actionable.
- Runtime events may trigger Run detail refresh after an action. Event payloads
  must not hydrate resumed/discarded state directly.

Required implementation gates after this design phase:

- Contract tests for any future action endpoint:
  `POST /v1/runs/{run_id}/resume`,
  `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume`,
  `POST /v1/runs/{run_id}/discard`, or checkpoint discard/ack endpoints.
- Restart tests proving stale running/waiting tools, stale permission gates,
  stale MCP auth requests, and stale MCP elicitation requests remain terminal
  or non-actionable after Run detail refresh.
- Parity tests proving action responses still derive timeline, diagnostics,
  artifacts, interrupted summaries, and terminal permission/MCP semantics from
  `SessionActivity`/runtime stores.
- Audit tests proving resume creates a new turn linked to the checkpoint and
  discard records acknowledgement without deleting evidence.
- Frontend transport tests proving React refreshes DTOs after events/actions
  and never uses event payload or browser memory as Run action source.

Exit criteria:

- The team can point to a narrow implementation plan for explicit
  user-triggered resume/discard.
- The plan names the exact storage writes, API shapes, audit records, and
  stale-actionability tests required.
- The plan still excludes automatic resume, background Run scheduling, full Run
  state machine implementation, and expanded frontend Run management UI unless
  a later accepted phase explicitly adds them.

Review conclusion:

- Phase 8.2 is accepted as the action semantics gate.
- The next implementation should start with checkpoint acknowledgement/discard
  because it is evidence-preserving and does not require prompt construction or
  model execution.
- Resume execution remains deferred until checkpoint acknowledgement/discard
  proves restart-safe, audited, and parity-preserving.
- Any future resume implementation must create a new explicit user turn from a
  structured checkpoint; it must not auto-resume or replay previous tool/MCP
  state.

### Phase 8.3: Checkpoint Acknowledgement And Discard Contract

Status: accepted as a narrow checkpoint action contract. Resume execution,
automatic resume, background scheduling, frontend Run management UI, and a full
Run state machine were not implemented.

Scope:

- Implement explicit checkpoint acknowledgement/discard for persisted
  `runtime_run_checkpoints`.
- Add transport routes for checkpoint acknowledgement/discard only. Do not add
  resume execution in this phase.
- Return refreshed persisted Run detail plus `RunProjection` parity payload
  after acknowledgement/discard.
- Keep `SessionActivity`/runtime stores as the source for timeline,
  diagnostics, artifacts, interrupted summaries, permissions, and MCP terminal
  semantics.
- Runtime events may trigger DTO refresh but must not hydrate acknowledgement
  state from payloads.

Out of scope:

- Automatic resume.
- Background Run scheduler.
- Full Run state machine.
- Frontend Run management UI.
- Model prompt construction for resume.
- Any restoration of stale running/waiting tools, permission gates, MCP auth
  requests, or MCP elicitation requests.

Acceptance criteria:

- Store tests cover idempotent checkpoint acknowledgement/discard and prove
  evidence rows are not deleted.
- HTTP/Wails/contract tests cover checkpoint acknowledgement/discard transport.
- Restart/parity tests prove stale permission and MCP actionability is not
  resurrected by Run detail refresh after acknowledgement/discard.
- Docs record that resume execution remains deferred.

Implemented:

- Added `acknowledgedAt` and `discardedAt` to persisted Run checkpoint DTOs.
- Added idempotent `runtimeRunStore` checkpoint acknowledgement/discard writes.
  These writes set timestamps only; they do not change original checkpoint
  status, summary, artifact refs, turn/task records, permission records, or MCP
  request records.
- Added read/write runtime service methods for checkpoint
  acknowledgement/discard that return refreshed Run detail plus the read-only
  projection parity payload.
- Added HTTP/dev-module routes:
  `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/acknowledge`
  and `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/discard`.
- Added Wails bridge methods `AcknowledgeRunCheckpoint` and
  `DiscardRunCheckpoint`.

Validation:

- Store tests cover idempotent acknowledgement, idempotent discard, and
  evidence preservation.
- HTTP tests cover bearer routes and dev-module fallback routes for both
  checkpoint actions.
- Wails bridge tests cover checkpoint action delegation.
- Runtime API contract tests include the new checkpoint action routes.

Review conclusion:

- Checkpoint acknowledgement/discard is now a persisted UX acknowledgement
  layer over existing runtime evidence.
- It does not make stale permission gates, stale MCP auth/elicitation requests,
  stale tools, or stale interrupted turns actionable.
- Resume execution remains the next separate design/implementation problem and
  must create a new explicit user turn rather than replay previous runtime
  state.

### Phase 8.4: Explicit Resume Contract Design Gate

Status: accepted as a design gate only. No resume endpoint, automatic resume,
background scheduling, frontend Run management UI, or full Run state machine
was implemented in this phase.

Purpose:

- Define the exact runtime contract for user-triggered resume from a persisted
  checkpoint.
- Specify how a resume action creates a new turn, links back to the checkpoint,
  audits the action, and refreshes Run detail without replaying stale state.
- Decide the minimum transport and frontend behavior needed before exposing a
  resume control.

Design questions to answer:

- API shape:
  - candidate endpoint:
    `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume`
  - response should return the new `RuntimeChatResponse` or a combined action
    response with new turn id plus refreshed Run detail.
- Prompt construction:
  - use checkpoint summary, current workspace state, linked session/task
    metadata, and current model/provider selection.
  - do not include stale pending permission/MCP auth/elicitation actionability.
  - do not infer artifact/checkpoint state from assistant prose.
- Audit:
  - record run id, checkpoint id, source session id, new turn id, and redacted
    resume prompt summary.
  - preserve previous evidence; do not rewrite terminal turn/tool/permission/MCP
    records.
- Runtime events:
  - emit ordinary turn/session/runtime events for the new turn.
  - event payloads may trigger Run detail refresh but must not hydrate resume
    state directly.
- Frontend:
  - a resume control must call the runtime action endpoint and then refresh Run
    detail/session activity.
  - no local optimistic resume state should survive a failed runtime response.

Acceptance criteria for a future implementation phase:

- Store/service tests prove resume links a new turn to the checkpoint without
  mutating previous checkpoint evidence.
- Restart tests prove stale running/waiting tools, permission gates, MCP auth,
  and MCP elicitation requests remain terminal/non-actionable before and after
  resume.
- HTTP/Wails/contract tests cover the accepted resume endpoint shape.
- Audit tests prove the new turn and checkpoint link are recoverable.
- Frontend transport tests prove events only trigger refresh and React state is
  not the source of resume status.

Review conclusion:

- Phase 8.4 is accepted as the explicit resume design gate.
- The first implementation should create an explicit user-triggered resume
  turn from a checkpoint and return structured action metadata plus refreshed
  Run detail.
- Resume must not replay previous streams, completed tool calls, permission
  requests, MCP auth requests, or MCP elicitation requests.
- Resume prompt construction must be structured and redacted: checkpoint
  summary, source Run/session/checkpoint ids, artifact refs, and current
  workspace context only.
- Frontend resume controls remain out of scope until the runtime endpoint and
  transport tests are proven.

### Phase 8.5: Explicit Checkpoint Resume Contract

Status: accepted as a narrow explicit resume action contract. Automatic
resume, background scheduling, frontend Run management UI, and a full Run state
machine were not implemented.

Scope:

- Implement `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume` as an
  explicit user-triggered action.
- The action creates a new turn in the checkpoint's primary session by calling
  the existing runtime chat path with a structured resume prompt.
- Return a dedicated action DTO with the new turn id, source run/checkpoint ids,
  and refreshed read-only Run detail.
- Record enough structured audit/runtime metadata to prove the new turn was
  created from the checkpoint.
- Keep existing `SessionActivity`/runtime stores as the source for timeline,
  diagnostics, artifact evidence, interrupted summaries, permission state, and
  MCP terminal semantics.

Out of scope:

- Automatic resume.
- Background Run scheduler.
- Full Run state machine.
- Frontend Run management UI or resume button wiring.
- Replaying previous model streams or tool calls.
- Restoring stale running/waiting tools, stale permission gates, stale MCP auth
  requests, or stale MCP elicitation requests.

Acceptance criteria:

- Store/service tests prove resume creates a new turn linked to the checkpoint
  without mutating previous checkpoint evidence.
- HTTP/Wails/contract tests cover the resume action transport.
- Restart/parity tests prove stale permission and MCP actionability is not
  resurrected by resume or by Run detail refresh after resume.
- Docs record that auto resume, background scheduling, and frontend Run UI
  remain deferred.

Implemented:

- Added `RuntimeRunResumeResponse` and `resumedTurnIds` checkpoint metadata.
- Added `runtimeRunStore.LinkCheckpointResume`, backed by checkpoint
  `metadata_json`, to persist the new turn id without mutating checkpoint
  status, summary, artifact refs, acknowledgement, or discard timestamps.
- Added `ResumeRunCheckpoint` service method. It validates the persisted Run
  and checkpoint, rejects non-eligible checkpoints, constructs a structured
  resume prompt, calls the existing `Chat` path to create a new explicit turn,
  links the turn to the checkpoint, writes redacted audit metadata, emits a
  refresh-trigger runtime event, and returns refreshed Run detail.
- Added HTTP/dev-module route:
  `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume`.
- Added Wails bridge method `ResumeRunCheckpoint`.

Validation:

- Store tests cover idempotent resumed turn links and evidence preservation.
- HTTP tests cover bearer and dev-module resume routes.
- Wails bridge tests cover resume delegation.
- Runtime API contract tests include the resume route.

Review conclusion:

- Resume is explicit and user-triggered. It creates a new turn through the
  existing runtime chat path.
- Resume does not replay streams, completed tool calls, permission requests,
  MCP auth requests, or MCP elicitation requests.
- Runtime events remain refresh triggers only; the persisted checkpoint
  metadata plus refreshed Run detail is the source of resume linkage.
- Frontend resume controls remain deferred until a separate UI phase.

### Phase 8.6: Resume Control Frontend Rollout Gate

Status: accepted as a rollout/design gate. No visible resume control, full Run
management UI, automatic resume, background scheduling, or full Run state
machine was implemented in this phase.

Purpose:

- Decide how the existing explicit resume runtime endpoint can be exposed safely
  in the React workbench.
- Define the UI and transport rules for displaying checkpoint resume
  availability without making React the source of actionability.
- Define browser/Vite and Wails smoke coverage before any visible resume
  control ships.

Design constraints:

- The visible control must be derived from refreshed Run detail/checkpoint DTOs,
  not from event payloads or local optimistic state.
- Clicking resume must call `ResumeRunCheckpoint`, then refresh Run detail and
  relevant session activity from runtime APIs.
- Failed resume responses must clear pending local UI state and must not mark a
  checkpoint as resumed.
- The UI must not expose automatic resume, batch resume, background scheduling,
  or Run management beyond the single explicit checkpoint action.
- Stale permission/MCP actionability must still come only from current runtime
  stores after refresh.

Acceptance criteria for a future implementation:

- Browser/Vite tests cover event-triggered Run detail refresh, explicit resume
  action, failed resume response cleanup, and no local resurrection of stale
  actionability.
- Wails/bridge tests cover the same action path through generated bindings or
  the bridge adapter.
- UI tests prove duplicate lifecycle/runtime events do not create duplicate
  controls or duplicate resume submissions.
- Docs record any remaining manual smoke gaps before shipping visible controls.

Review conclusion:

- Phase 8.6 is accepted as the frontend/runtime rollout gate.
- The first visible UI implementation should be narrow: one explicit checkpoint
  resume control rendered from refreshed Run detail DTOs.
- The control must call the runtime `ResumeRunCheckpoint` action and then
  refresh Run detail/session activity.
- React may track transient pending/error UI state, but it must not become the
  source of checkpoint actionability or resumed status.
- No automatic resume, batch resume, background scheduling, full Run management
  UI, or event-payload hydration is accepted.

### Phase 8.7: Narrow Resume Control Frontend Rollout

Status: accepted.

Scope:

- Expose a single visible checkpoint resume action in the existing read-only Run
  projection/detail surface.
- Add adapter support for `ResumeRunCheckpoint` through Wails and HTTP/dev
  transport.
- After a successful resume action, refresh Run detail/session activity from
  runtime APIs. Do not merge event payloads into UI state.
- Keep local state limited to pending/error rendering for the clicked action.
- Add browser/Vite and component/adapter coverage proving duplicate events do
  not duplicate controls or submissions.

Out of scope:

- Automatic resume.
- Background Run scheduler.
- Full Run state machine.
- Full Run management UI.
- Batch resume.
- Any permission/MCP actionability derived from React state or event payloads.

Acceptance criteria:

- Frontend tests cover successful resume, failed resume cleanup, duplicate event
  refresh, and no local resurrection of stale actionability.
- Adapter tests cover HTTP/dev and Wails resume action mapping.
- Existing Go runtime tests continue to pass.
- Docs record any remaining packaged Wails/manual browser smoke gaps.

Implementation notes:

- Extended the frontend Run projection view model with structured checkpoint
  fields from the runtime DTO: status, summary, artifact refs, acknowledged /
  discarded timestamps, resumed turn ids, and `resumeEligible`.
- Added `WorkbenchAdapter.resumeRunCheckpoint(current, runID, checkpointID)`.
  The Wails/HTTP adapter calls runtime `ResumeRunCheckpoint` and then hydrates
  the workbench from runtime APIs. It does not merge the action response into
  timeline, diagnostics, artifacts, permissions, MCP actionability, or Run
  checkpoint state.
- Added HTTP/dev transport mapping for
  `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume`.
- Added one visible resume control to the existing Run projection diagnostics
  panel. It renders only the first refreshed checkpoint DTO with
  `resumeEligible=true`.
- React state is limited to the clicked checkpoint's transient loading/error
  display. Failed resume responses clear pending state and do not mark the
  checkpoint resumed locally.
- The control path is wired through `Workspace` and `WorkbenchShell`; successful
  actions replace the view model with the runtime-hydrated result.

Validation:

- `cd client && npm run build` passed.
- `cd client && npm run lint` passed.
- `git diff --check` passed with only the existing Windows CRLF warning for
  `client/src/runtime/wailsWorkbenchAdapter.ts`.
- `cd desktop && wails3 task common:generate:bindings` passed; the generated
  Wails bridge output contains `ResumeRunCheckpoint`.
- Diff review confirmed no automatic resume, background scheduler, full Run
  state machine, frontend Run management UI, event-payload hydration, prose
  inference, or React-owned resume source of truth was introduced.

Remaining validation gaps:

- The client currently has no Vitest/Testing Library component test harness, so
  the successful/failed click and duplicate-event UI cases were verified by
  build/lint/diff review rather than executable browser/component tests in this
  phase.
- The packaged Wails generated binding path now regenerates with
  `ResumeRunCheckpoint`, but an end-to-end packaged app smoke should still
  verify the visible control through the packaged bridge before treating the
  Wails path as fully shipped.
- No hosted provider credentials were used and no auth state or secrets were
  written to fixtures, logs, screenshots, docs, or React state.

Review conclusion:

- Phase 8.7 is accepted as the first visible explicit checkpoint resume control.
- The implementation keeps runtime DTO refresh as the source of actionability
  and resumed state.
- The remaining work is validation hardening: packaged Wails smoke and an
  executable frontend test harness or equivalent browser smoke for success,
  failure, and duplicate-event cases.

### Phase 8.8: Resume Control Packaged Smoke And Frontend Harness Gate

Status: implemented.

Scope:

- Add or reuse a narrow executable frontend validation path for the visible
  resume control covering eligible DTO rendering, failed resume cleanup, and
  no duplicate controls/submissions after duplicate refresh triggers.
- Add packaged Wails or bridge-layer smoke proving `ResumeRunCheckpoint` is
  available through generated bindings and that success refreshes runtime DTOs
  rather than merging the action response into React state.
- Keep HTTP/dev and Wails transport behavior aligned.
- Record whether a full packaged app smoke is automated or remains a redacted
  manual checklist.

Out of scope:

- Automatic resume.
- Background scheduler.
- Full Run state machine.
- Runtime Run store expansion beyond existing Phase 8 persistence.
- Full frontend Run management UI.
- Batch checkpoint actions.

Acceptance criteria:

- The visible control remains derived only from refreshed runtime checkpoint
  DTOs.
- Failed resume leaves checkpoint actionability/resumed status unchanged until
  the next runtime DTO refresh.
- Duplicate lifecycle/runtime refresh triggers do not duplicate controls or
  submit the same resume twice.
- Packaged Wails binding availability for `ResumeRunCheckpoint` is verified.
- Docs record any remaining manual packaged or browser smoke gap.

Implementation notes:

- Added `runProjectionResumeContract.ts` so the resume checkpoint selection
  rule is executable outside the React component and reusable by the UI.
- Added `client/scripts/phase88-resume-control-smoke.mjs` and
  `npm run smoke:phase88`.
- The smoke script writes its temporary bundle under
  `tmp/runtime-dev/phase88-resume-control-smoke`, executes the checkpoint
  actionability contract, checks the component keeps a stable
  `run-checkpoint-resume` marker and local pending state, and verifies the
  generated Wails bridge exports `ResumeRunCheckpoint`.

Validation:

- `cd desktop && wails3 task common:generate:bindings` passed before the smoke;
  generated output contains `ResumeRunCheckpoint`.
- `cd client && npm run smoke:phase88` passed.
- `cd client && npm run lint` passed.
- `cd client && npm run build` passed.

Remaining validation gaps:

- Phase 8.8 adds an executable contract smoke without introducing a full DOM
  test dependency. It does not yet click the Ant Design button in a browser DOM
  harness.
- A true packaged desktop app smoke is still needed to click the visible
  control through WebView/Wails and observe the runtime DTO refresh end to end.
  The bridge export itself is verified.
- No hosted provider credentials were used and no secrets/auth state were
  written to repo files, logs, screenshots, docs, or React state.

Review conclusion:

- Phase 8.8 narrows the Phase 8.7 validation gap without expanding the product
  surface.
- The resume actionability source remains the runtime checkpoint DTO.
- No automatic resume, background scheduler, full Run state machine, expanded
  runtime Run store, batch action, or frontend Run management UI was introduced.

### Phase 8.9: Packaged Resume Click Smoke And Phase 8 Acceptance

Status: accepted with a packaged click fixture gap.

Scope:

- Run or script a packaged Wails/WebView smoke for the visible checkpoint resume
  control when the runtime fixture can expose an eligible checkpoint.
- Verify the click path calls `ResumeRunCheckpoint`, then refreshes Run
  projection/session activity from runtime APIs.
- Verify failed resume clears pending UI state without locally marking the
  checkpoint resumed.
- Accept Phase 8 only after the packaged click path is covered or explicitly
  recorded as a redacted manual smoke gap.

Out of scope:

- Automatic resume.
- Background scheduler.
- Full Run state machine.
- Full frontend Run management UI.
- Batch checkpoint actions.
- Hosted provider secrets or browser auth automation.

Validation:

- `cd desktop && go test . -run
  "TestRuntimeBridgePhase62PackagedHandoffRecoveryContract|TestRuntimeBridgeActionMethods"
  -count=1` passed.
- `cd desktop && .\scripts\phase62-wails-packaged-smoke.ps1` passed and
  started the packaged app with a runtime root under `tmp/runtime-dev`.
- Phase 8.8 already verified generated Wails bindings expose
  `ResumeRunCheckpoint` and the frontend contract smoke passes.

Manual packaged click gap:

- A true WebView click smoke still needs a deterministic runtime fixture that
  opens the packaged app on a session with an eligible checkpoint visible in
  the Run projection panel.
- The manual smoke checklist for that fixture is:
  1. Start packaged app with `AGENT_BUILDER_DESKTOP_ROOT` under
     `tmp/runtime-dev`.
  2. Open a fixture session whose runtime projection includes exactly one
     `resumeEligible=true` checkpoint.
  3. Verify one `run-checkpoint-resume` control is visible.
  4. Click the control and verify `ResumeRunCheckpoint(runID, checkpointID)` is
     invoked through Wails.
  5. Verify the UI refreshes from runtime Run projection/session activity APIs.
  6. Verify a failed resume clears pending UI state and does not locally mark
     the checkpoint resumed.
- No hosted provider credentials were used and no secrets/auth state were
  written to repo files, logs, screenshots, docs, or React state.

Review conclusion:

- Phase 8 is accepted as the narrow explicit checkpoint resume rollout through
  runtime DTOs and explicit user action.
- Remaining risk is validation infrastructure, not missing product behavior:
  the packaged WebView click path needs a deterministic eligible-checkpoint
  fixture before it can be automated.
- No automatic resume, background scheduler, full Run state machine, expanded
  runtime Run store, batch actions, frontend Run management UI, event-payload
  hydration, or assistant-prose inference was introduced.

### Phase 9: Runtime Run Execution Cutover Design Gate

Status: accepted as a design gate only.

Purpose:

- Decide whether the validated read-only Run projection plus explicit
  checkpoint actions are ready to become a runtime execution source of truth.
- Map the next implementation against the Claude Code runtime lessons already
  captured in Phase 7, rather than inventing a new scheduler model from scratch.
- Define the minimum durable Run lifecycle needed before any replacement of
  session-first execution.

Scope:

- Design the first write-capable Run execution contract and its invariants.
- Identify which state transitions must be persisted, which remain computed,
  and which are still explicitly out of scope.
- Define migration/test gates before introducing any runtime Run store expansion
  or scheduler behavior.

Out of scope until this gate is accepted:

- Implementing a full Run state machine.
- Writing new database migrations.
- Implementing automatic resume.
- Implementing a background scheduler.
- Building a frontend Run management UI.
- Replacing SessionActivity as fallback/parity oracle.

Claude Code mapping carried forward:

- Claude Code's stable behavior comes from a durable transcript/session spine
  plus runtime-owned tools, permissions, tasks, and recovery rules. It does not
  make the browser/client memory the execution source of truth.
- Agent Builder should mirror that shape: a Run may become the runtime
  execution envelope, but messages, tool calls, permissions, tasks, artifacts,
  and checkpoints remain structured runtime evidence.
- Session recovery should still rebuild from persisted/runtime evidence. It
  must not reconstruct actionability from event payloads, assistant prose, or
  React state.

Design decision:

- Do not jump directly from Phase 8 to a full Run state machine.
- The next implementation should introduce a minimal write-capable Run
  execution envelope only after a separate implementation phase is approved.
- That envelope should persist only stable ownership/linkage first:
  `run_id`, workspace/session linkage, objective/source metadata, active turn
  linkage, and terminal summary fields that can be reconciled with
  `SessionActivity`.
- Runtime events remain refresh triggers. They may select Run detail/activity
  reads, but they must not carry enough state for the frontend to mutate Run
  lifecycle, checkpoint, permission, MCP, diagnostics, or artifact state.
- `SessionActivity` remains the fallback and parity oracle until a later phase
  proves the Run execution envelope can replace a specific subset.

Minimum invariants before implementation:

- Every persisted Run transition must be idempotent and replay-safe.
- Running/waiting tool calls, permission gates, MCP auth requests, and
  elicitation requests must not be restored as actionable after restart unless
  a current runtime store explicitly says they are actionable.
- Pending-at-interruption remains computed diagnostics until a later accepted
  phase changes that contract.
- Explicit checkpoint resume stays user-triggered; no automatic resume or
  background scheduler is introduced by the cutover envelope.
- New Run persistence must include focused migration/backfill tests before it
  is allowed to affect production runtime paths.

Recommended next implementation phase:

- Phase 9.1 should be a minimal durable Run execution envelope implementation
  gate, not a full scheduler.
- It should add the smallest persistence/API surface needed to create or link a
  Run at turn start and reconcile it at turn finish.
- It must keep existing session-first execution working and keep
  `SessionActivity` as fallback/parity oracle.
- It must include restart/replay tests proving no stale permission/MCP/tool
  actionability is resurrected.

### Phase 9.1: Minimal Durable Run Execution Envelope

Status: implemented.

Scope:

- Reuse the existing Phase 8 `runtime_runs` and `runtime_run_sessions` schema.
- Link the current turn into the runtime-owned Run envelope after the queued
  turn is persisted.
- Reconcile Run status and terminal summary from `RunProjection` after turn
  finish, cancel, or interrupted acknowledgement.
- Preserve `SessionActivity` as fallback/parity oracle and keep projection
  evidence as the source of diagnostics, artifacts, permissions, MCP state, and
  checkpoints.

Implementation notes:

- Added `runtimeRunStore.LinkTurn`, which writes the latest turn id into
  `runtime_run_sessions.turn_id` and marks the Run active without introducing a
  scheduler or new lifecycle table.
- `Chat` now ensures a runtime Run for the session, persists the queued turn,
  then links the Run envelope to that turn.
- `runChat`, `CancelTurn`, and `MarkInterruptedDone` now call a narrow
  `reconcileRuntimeRunForSession` helper. The helper reads `RunProjection` and
  lets the existing projection/store parity path update durable Run summary.
- `UpsertFromProjection` now preserves an existing user-created Run source and
  objective instead of overwriting them with backfill metadata during
  reconciliation.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunStore|TestRuntimeRunProjection|TestRuntimeRunResume|TestRuntimeTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Review conclusion:

- Phase 9.1 adds the minimum write-capable Run envelope around existing
  session-first turn execution.
- No new database migration was added.
- No automatic resume, background scheduler, full Run state machine, expanded
  runtime Run store, batch action, frontend Run management UI, event-payload
  hydration, or assistant-prose inference was introduced.
- The remaining cutover risk is restart/replay validation for the new envelope
  link when a process exits between queued/running/final reconciliation.

### Phase 9.2: Run Envelope Restart Replay Validation

Status: implemented.

Scope:

- Add focused restart/replay tests for the Phase 9.1 Run envelope linkage.
- Verify a Run linked to an unfinished queued/running/waiting turn is reconciled
  through existing interruption semantics after restart.
- Verify stale running tools, permission gates, MCP auth requests, and
  elicitation requests are not restored as actionable through the Run envelope.
- Verify `SessionActivity` remains fallback/parity oracle after replay.

Out of scope:

- Automatic resume.
- Background scheduler.
- Full Run state machine.
- New Run database migrations.
- Frontend Run management UI.

Implementation notes:

- Added a focused restart/replay validation for a Run envelope linked to an
  unfinished running turn.
- The test simulates startup recovery by interrupting unfinished turns,
  cancelling unfinished tool calls, expiring stale permission requests, and
  cancelling stale MCP auth/actionability requests.
- It verifies `RunProjection` keeps the original durable Run id, moves the Run
  to interrupted status through structured evidence, and does not expose pending
  permission counts.
- It verifies `SessionActivity` has no stale running/pending tool calls and
  `RecoveryStatus` has no pending permission or MCP requests after recovery.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability|TestRuntimeRunProjection"
  -count=1` passed.
- `go test ./internal/runtime ./internal/runtimeapi ./desktop -count=1`
  passed.
- `go test ./... -timeout 180s` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 9.2 validates the Phase 9.1 Run envelope does not resurrect stale
  actionability after replay/recovery.
- `SessionActivity` remains the fallback/parity oracle.
- No automatic resume, background scheduler, full Run state machine, database
  migration, frontend Run management UI, event-payload hydration, or
  assistant-prose inference was introduced.

### Phase 9.3: Run Envelope Acceptance And Cutover Boundary

Status: accepted.

Scope:

- Review and accept the Phase 9.1/9.2 minimal Run envelope.
- Decide the next implementation boundary: either keep hardening the envelope
  with more validation, or plan the first separately approved transition beyond
  session-first execution.
- Record remaining risks before any database migration, scheduler, or frontend
  Run management work is allowed.

Out of scope:

- Automatic resume.
- Background scheduler.
- Full Run state machine.
- New Run database migrations.
- Frontend Run management UI.

Review conclusion:

- Phase 9.1 and Phase 9.2 are accepted as the minimal durable Run envelope
  cutover boundary.
- The runtime now has a write-capable envelope that links session-first turns
  to durable Run identity and reconciles terminal summary through
  `RunProjection`.
- The envelope is not yet a scheduler and is not yet a replacement for
  `SessionActivity`.
- Restart/replay validation proves the envelope does not restore stale tool,
  permission, MCP auth, or elicitation actionability.
- The next step must remain a design gate before any database migration,
  scheduler, full state machine, or frontend Run management work.

Remaining risks:

- The Run envelope stores the current linked turn in existing
  `runtime_run_sessions.turn_id`; it does not yet model multi-turn Run
  lifecycle transitions as first-class persisted state.
- Packaged WebView click smoke for the visible checkpoint resume control still
  needs a deterministic eligible-checkpoint fixture.
- Any future replacement of session-first execution still needs explicit
  migration/backfill, replay, and parity gates.

### Phase 10: Run Lifecycle State Machine And Scheduler Design Gate

Status: accepted as a design gate only.

Purpose:

- Decide whether and how Agent Builder should move from the minimal Run
  envelope to a first-class runtime Run lifecycle.
- Define the minimum persisted state machine needed before scheduler work is
  allowed.
- Preserve Claude Code lessons: durable transcript/session evidence first,
  runtime-owned actionability, no browser-owned execution state.

Scope:

- Design candidate Run lifecycle states and transitions.
- Decide whether a database migration is justified and what backward-compatible
  backfill would require.
- Define scheduler boundaries, especially what remains session-first and what
  may become Run-first.
- Define acceptance tests for restart, cancellation, permission/MCP
  actionability, checkpoint resume, and parity with `SessionActivity`.

Out of scope:

- Implementing the state machine.
- Writing database migrations.
- Implementing automatic resume.
- Implementing a background scheduler.
- Implementing frontend Run management UI.

Candidate lifecycle vocabulary:

- `created`: durable Run envelope exists but no turn has been linked yet.
- `active`: at least one linked turn is queued/running/cancelling.
- `waiting_user`: linked evidence has a current runtime-owned permission/MCP
  gate that is actionable in the current runtime stores.
- `interrupted`: linked evidence contains an interrupted turn/task and no
  current actionable gate should be restored automatically.
- `completed`: linked evidence is terminal and successful.
- `failed`: linked evidence is terminal with failure.
- `cancelled`: linked evidence is terminal by explicit cancellation or
  interrupted acknowledgement.

Transition rules:

- `created -> active` happens only when a turn is durably linked after the turn
  row exists.
- `active -> waiting_user` is computed from current runtime stores, not from
  persisted event payloads.
- `active/waiting_user -> interrupted` happens during restart recovery or
  explicit interruption semantics.
- `active/waiting_user -> completed|failed|cancelled` happens only after
  structured turn/task evidence is terminal.
- `interrupted -> active` can happen only through an explicit user-triggered
  checkpoint resume that creates a new turn. It must not replay tool calls or
  revive stale actionability.

Scheduler boundary:

- Session-first turn execution remains the runtime execution path until a later
  accepted phase proves a Run-first scheduler.
- A scheduler may eventually use the Run envelope for ownership, grouping, and
  cancellation scope, but it must not own permission/MCP actionability without
  current runtime-store confirmation.
- Runtime events may trigger `RunProjection`, `Run`, `TurnActivity`, or
  `SessionActivity` refreshes. Event payloads remain non-authoritative.

Migration decision:

- A migration is justified only when the next implementation needs first-class
  transition history or multi-turn Run ownership that cannot be represented by
  existing `runtime_runs`, `runtime_run_sessions`, and
  `runtime_run_checkpoints`.
- Before any migration, tests must prove backfill idempotency, replay safety,
  and parity with `SessionActivity` for messages, tool calls, permissions,
  diagnostics, artifact evidence, interrupted summaries, terminal MCP
  semantics, and checkpoint actions.

Acceptance tests required before implementation:

- Start, finish, cancel, fail, and restart a linked Run without stale
  tool/permission/MCP actionability.
- Resume an interrupted checkpoint as an explicit new turn and prove the
  original checkpoint evidence is unchanged.
- Backfill legacy sessions into Runs idempotently without changing user-created
  Run ids.
- Keep frontend state derived from runtime DTO refreshes only.

Review conclusion:

- Phase 10 accepts the vocabulary and boundaries for a future state-machine
  implementation.
- The next phase may design a migration and transition-history contract, but it
  must still be a gate before code changes.
- No state machine, migration, automatic resume, background scheduler, or
  frontend Run management UI is implemented in Phase 10.

### Phase 10.1: Run Transition History Migration Design Gate

Status: accepted as a design gate only.

Scope:

- Decide whether to add a transition-history table or extend existing Run
  metadata for first-class lifecycle audit.
- Specify idempotent backfill behavior for existing Phase 8/9 Runs.
- Define exact migration rollback and replay tests.
- Keep `SessionActivity` as fallback/parity oracle.

Out of scope:

- Writing the migration.
- Implementing the state machine.
- Implementing a scheduler.
- Implementing automatic resume.
- Implementing frontend Run management UI.

Decision:

- A dedicated transition-history table is justified before implementing a
  first-class Run lifecycle.
- Extending `runtime_runs.metadata_json` is not enough because transition
  history must be queryable, replayable, idempotently backfilled, and auditable
  without rewriting the durable Run summary row.

Proposed table:

```text
runtime_run_transitions
  id TEXT PRIMARY KEY
  run_id TEXT NOT NULL
  session_id TEXT
  turn_id TEXT
  task_id TEXT
  from_status TEXT
  to_status TEXT NOT NULL
  reason TEXT
  source TEXT NOT NULL
  event_id TEXT
  created_at INTEGER NOT NULL
  metadata_json TEXT
```

Indexes:

- `(run_id, created_at)`
- `(turn_id, created_at)`
- `(session_id, created_at)`
- Unique id should be deterministic for replay-safe transitions, for example
  hash of `run_id`, `turn_id`, `to_status`, `source`, and `created_at` bucket
  when a runtime event id is unavailable.

Backfill rules:

- Backfill may create at most one synthetic transition per terminal historical
  turn unless richer persisted evidence already exists.
- Backfill must not infer lifecycle from assistant prose.
- Backfill must preserve existing generated Run ids and deterministic
  `run:session:<session_id>` ids.
- Re-running backfill must not duplicate transitions.

Implementation gate for Phase 10.2:

- Add the migration and store only.
- Add idempotent insert/list tests and rollback validation.
- Do not wire transitions into runtime execution yet.
- Do not implement scheduler, automatic resume, or frontend Run UI.

Acceptance tests required for Phase 10.2:

- Migration up/down applies cleanly on an empty database and a database with
  existing Phase 8/9 Run rows.
- Store insert is idempotent under repeated replay.
- Listing transitions by run/session/turn is stable and ordered.
- Existing `RunProjection`, `Run`, `SessionActivity`, checkpoint resume, and
  restart-replay tests still pass.

Review conclusion:

- Phase 10.1 accepts a transition-history migration design.
- The migration is still not implemented in this phase.
- `SessionActivity` remains fallback/parity oracle, and runtime events remain
  refresh triggers rather than lifecycle truth.

### Phase 10.2: Run Transition History Store Foundation

Status: implemented.

Scope:

- Implement the accepted `runtime_run_transitions` migration.
- Add a narrow runtime transition store with idempotent insert and ordered list
  methods.
- Keep the store disconnected from runtime execution until a later accepted
  lifecycle wiring phase.

Out of scope:

- Runtime lifecycle state machine wiring.
- Scheduler.
- Automatic resume.
- Frontend Run management UI.
- Replacing `SessionActivity`.

Implementation notes:

- Added `runtime_run_transitions` migration with ordered indexes for run,
  session, and turn queries.
- Added `runtimeRunTransitionStore` with idempotent `Upsert`, `Get`,
  `ListByRun`, `ListBySession`, and `ListByTurn`.
- Deterministic transition ids are generated from stable runtime evidence when
  callers do not provide an id.
- The store is intentionally not wired into runtime execution in this phase.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunTransition" -count=1`
  passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `go test ./... -timeout 180s` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 10.2 adds the migration/store foundation only.
- No runtime state machine wiring, scheduler, automatic resume, frontend Run UI,
  event-payload lifecycle authority, or `SessionActivity` replacement was
  introduced.

### Phase 10.3: Run Transition History Runtime Wiring Gate

Status: accepted as a design gate only.

Scope:

- Decide where transition history should be recorded in existing runtime paths:
  turn start, finish, cancel, interrupted acknowledgement, restart recovery,
  and checkpoint resume.
- Specify idempotency keys for each transition source.
- Define tests proving transition history does not become actionability truth.

Out of scope:

- Implementing a scheduler.
- Automatic resume.
- Frontend Run management UI.
- Replacing `SessionActivity`.

Accepted wiring design:

- `turn_started`: record after the `RuntimeTurn` exists and
  `runtimeRunStore.LinkTurn` has linked the turn to its Run. This can record
  `nil/previous -> active` evidence for the new turn, but it must not infer
  timeline, artifact, permission, MCP, or checkpoint state.
- `turn_finished`: record from `runChat` only after the final `RuntimeTurn`
  row has been written and `RunProjection`/persisted Run reconciliation can
  derive the target status from structured evidence.
- `turn_cancelled`: record from `CancelTurn` after terminal turn and tool-call
  cancellation evidence has been written. The transition writer must not use
  the cancel event payload as lifecycle truth.
- `interrupted_marked_done`: record from `MarkInterruptedDone` after the
  interrupted turn has been persisted as cancelled. This preserves the existing
  `MarkInterruptedDone`/cancelled terminal semantics and does not add a
  persisted acknowledgement field.
- `startup_recovery`: record during runtime startup only after unfinished
  turns/tasks/hooks/tools, expired permissions, and stale MCP auth/elicitation
  requests have been terminalized. Restart recovery must never restore stale
  running/waiting tools, permission gates, MCP auth requests, or elicitation
  requests as actionable.
- `checkpoint_resume`: record after `ResumeRunCheckpoint` creates a new
  explicit user-triggered turn and `LinkCheckpointResume` links that turn to
  the checkpoint. This transition is `interrupted -> active` evidence only; it
  does not replay previous tools and does not auto-resume.
- `checkpoint_acknowledged` and `checkpoint_discarded` stay checkpoint marker
  writes rather than Run lifecycle transitions. They may be audited separately,
  but they must not change Run actionability or terminal status.

Accepted idempotency keys:

- `turn_started`: run id, session id, turn id, source `turn_started`, target
  `active`, and turn `started_at`.
- `turn_finished`: run id, session id, turn id, source `turn_finished`, target
  status derived from reconciled Run/RunProjection, and turn `finished_at`.
- `turn_cancelled`: run id, session id, turn id, source `turn_cancelled`,
  target `cancelled`, and terminal turn `finished_at`.
- `interrupted_marked_done`: run id, session id, turn id, source
  `interrupted_marked_done`, target `cancelled`, and terminal turn
  `finished_at`.
- `startup_recovery`: run id, session id, affected turn/task id when present,
  source `startup_recovery`, target `interrupted`, and the recovered evidence
  id. Replaying startup after evidence is already terminal must not duplicate
  rows.
- `checkpoint_resume`: run id, checkpoint id, resumed turn id, source
  `checkpoint_resume`, target `active`, and resumed turn `started_at`.

Implementation gate for Phase 10.4:

- Add a narrow transition-writer helper that reads current persisted Run and
  structured runtime evidence, then writes `runtime_run_transitions`.
- Wire only the accepted existing runtime paths above.
- Keep transition writes best-effort and non-authoritative for actionability;
  a transition write failure may warn but must not resurrect stale state or
  replace existing terminal evidence.
- Do not expose transition history through HTTP, Wails, or React in the first
  wiring implementation.

Acceptance tests required for Phase 10.4:

- Chat start/finish records stable, ordered transitions without duplicate rows
  under repeated reconciliation.
- Cancel and interrupted acknowledgement record terminal transitions while
  preserving current cancelled semantics.
- Startup recovery records interrupted transitions only after stale tools,
  permissions, and MCP action requests are terminalized; no stale actionability
  is restored.
- Explicit checkpoint resume records `checkpoint_resume` and links the new
  turn without mutating prior checkpoint evidence or replaying previous tools.
- `SessionActivity`, `RunProjection`, and persisted Run detail remain the
  parity oracle for timeline, diagnostics, artifact evidence, interrupted
  summaries, permission state, and terminal MCP semantics.
- HTTP/Wails/frontend tests remain unchanged except for refreshes reading
  existing DTOs; event payloads remain refresh triggers only.

Review conclusion:

- Phase 10.3 accepts where transition history may be recorded and the
  idempotency contract for each source.
- No runtime transition wiring, scheduler, automatic resume, frontend Run UI,
  transport exposure, or `SessionActivity` replacement is implemented in this
  phase.

### Phase 10.4: Run Transition History Runtime Wiring

Status: accepted.

Scope:

- Add a narrow transition writer over the Phase 10.2
  `runtime_run_transitions` store.
- Wire only accepted existing runtime paths from Phase 10.3.
- Keep transition writes non-authoritative and disconnected from transport/UI.

Implementation notes:

- Added `runtimeRunTransitionStore` ownership to `runtimeService` startup,
  restart, and test harness setup.
- Added `recordRunTransition`, turn/task transition helpers, and checkpoint
  resume transition helper.
- `Chat` records `turn_started` only after the turn row exists and
  `runtimeRunStore.LinkTurn` succeeds.
- `runChat` records `turn_finished` after final turn persistence and
  RunProjection reconciliation.
- `CancelTurn` records `turn_cancelled` after terminal turn and stale tool-call
  cancellation evidence has been written.
- `MarkInterruptedDone` records `interrupted_marked_done` after preserving the
  existing cancelled terminal turn semantics.
- Startup recovery records `startup_recovery` for interrupted turns/tasks only
  after unfinished tools, permissions, and stale MCP requests have been
  terminalized.
- `ResumeRunCheckpoint` records `checkpoint_resume` after the explicit new turn
  is created and linked to the checkpoint.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunTransition" -count=1`
  passed.
- `go test ./internal/runtime -run
  "TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn|TestRuntimeRunProjection|TestRuntimeRunStore"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `go test ./... -timeout 180s` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 10.4 wires transition-history writes as audit evidence only.
- No transition-history HTTP/Wails/React DTO, scheduler, automatic resume,
  frontend Run management UI, event-payload lifecycle authority, or
  `SessionActivity` replacement was introduced.
- Transition rows still cannot make stale permission gates, MCP auth,
  elicitation requests, tools, checkpoints, artifacts, diagnostics, or
  interrupted summaries actionable.
- Phase 10.4 is accepted as the backend-only transition-history wiring
  foundation.

### Phase 10.5: Read-only Transition History DTO Design Gate

Status: accepted as a design gate only.

Scope:

- Decide whether transition history should be exposed as a read-only DTO.
- Define the DTO shape, cursor semantics, and transport-neutral API boundary if
  exposure is justified.
- Prove how transition-history reads preserve `SessionActivity`,
  `RunProjection`, and persisted Run detail parity without becoming lifecycle
  or actionability truth.

Out of scope:

- Implementing transition-history transport.
- Frontend Run management UI.
- Scheduler or automatic resume.
- Treating transition rows as source of permission, MCP auth, elicitation,
  checkpoint, artifact, diagnostics, interrupted, or tool actionability.

Accepted DTO design:

```text
RuntimeRunTransitionHistoryRequest
  run_id optional
  session_id optional
  turn_id optional
  cursor optional
  limit optional

RuntimeRunTransitionHistoryResponse
  transitions[] RuntimeRunTransition
  window RuntimeActivityWindow
  source

source
  kind: "run_transition_history"
  read_only: true
  audit_only: true
  session_activity_parity: true
  evidence: ["runtime_run_transitions", "runtime_runs", "runtime_turns",
             "runtime_agent_tasks", "session_activity", "run_projection"]
```

Accepted cursor semantics:

- Use an additive transition-history cursor, not the session-activity cursor:
  `v1:<created_at padded>:transition:<transition_id>`.
- Order by `created_at ASC, id ASC`, matching the store order.
- Cursor filtering is window-only; it must not decide current lifecycle,
  permission, MCP, checkpoint, artifact, diagnostics, or interrupted state.
- A response may include only transition rows plus cursor metadata. It must not
  synthesize timeline cards, diagnostics, checkpoints, user actions, or
  artifact evidence.

Accepted transport boundary if implemented later:

- Runtime service method candidate:
  `RunTransitionHistory(ctx, RuntimeRunTransitionHistoryRequest)`.
- HTTP candidate:
  `GET /v1/run-transitions?run_id=&session_id=&turn_id=&cursor=&limit=`.
- Wails candidate:
  `RuntimeBridge.RunTransitionHistory(req)`.
- Frontend usage, if any, must be read-only diagnostics/audit display and must
  refresh existing `Run`, `RunProjection`, or `SessionActivity` DTOs for actual
  lifecycle and actionability state.

Parity requirements:

- `RuntimeRunTransitionHistoryResponse` must not claim a Run is currently
  active, waiting, interrupted, completed, failed, or cancelled unless the
  caller also refreshes existing Run/RunProjection evidence.
- Transition rows must be traceable back to existing turn/task/checkpoint
  evidence ids when present.
- For a transition window, tests must compare the corresponding
  `SessionActivity`/`RunProjection` subset and prove messages, tool calls,
  permissions, diagnostics, artifact evidence, interrupted summaries, terminal
  permission/MCP semantics, and checkpoint actionability are not derived from
  transition rows.
- Event payloads may trigger this DTO refresh, but may not populate or mutate
  the transition list directly in React.

Implementation gate for Phase 10.6:

- Add contract types and an internal service method only if tests can prove the
  read-only/audit-only boundary.
- Add HTTP/Wails exposure only after the internal method and parity tests pass.
- Do not add frontend consumption in the same phase unless separately accepted.
- Do not implement scheduler, automatic resume, Run management UI, or
  transition-derived actionability.

Review conclusion:

- Phase 10.5 accepts a read-only transition-history DTO contract as optional
  diagnostic/audit evidence.
- The DTO is not required for current frontend behavior and must not replace
  `SessionActivity`, `RunProjection`, or persisted Run detail.

### Phase 10.6: Internal Read-only Transition History DTO

Status: accepted.

Scope:

- Add internal contract types for read-only transition history.
- Add a concrete runtime service method for internal use and tests.
- Prove cursor/window behavior and parity boundaries without transport or UI
  exposure.

Implementation notes:

- Added `RuntimeRunTransitionHistoryRequest`,
  `RuntimeRunTransitionHistoryResponse`, and
  `RuntimeRunTransitionHistorySource`.
- Added concrete `runtimeService.RunTransitionHistory(...)`, intentionally not
  added to the transport-neutral `RuntimeService` interface.
- Added transition-history cursor semantics:
  `v1:<created_at padded>:transition:<transition_id>`.
- Responses include transition rows, `RuntimeActivityWindow`, and read-only
  audit-only source metadata only.
- No HTTP route, Wails bridge method, generated binding, React state, or UI was
  added.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunTransitionHistory|TestRuntimeRunTransition" -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `go test ./... -timeout 180s` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 10.6 proves the internal transition-history DTO can be read as
  audit/diagnostic evidence with stable cursor semantics.
- Lifecycle, checkpoint, permission, MCP, artifact, diagnostics, interrupted,
  and tool actionability still require existing Run/RunProjection/
  SessionActivity refreshes.
- Transition-history transport exposure still requires a later accepted gate.
- Phase 10.6 is accepted as an internal-only DTO foundation.

### Phase 10.7: Transition History Read-only Transport Design Gate

Status: accepted as a design gate only.

Scope:

- Decide whether the internal transition-history DTO should be exposed through
  the transport-neutral runtime boundary.
- Define HTTP/Wails route and method names if exposure is accepted.
- Define contract tests proving transport exposure remains read-only and
  audit-only.

Out of scope:

- Implementing HTTP/Wails transport.
- Generated bindings or frontend consumption.
- Frontend Run management UI.
- Scheduler, automatic resume, or transition-derived actionability.

Accepted transport decision:

- Expose the Phase 10.6 internal DTO through the transport-neutral runtime
  boundary in a later implementation phase.
- Keep the method read-only, audit-only, and explicitly diagnostic.
- Do not add frontend consumption in the same implementation phase.

Accepted service boundary:

```text
RuntimeService.RunTransitionHistory(ctx, RuntimeRunTransitionHistoryRequest)
  -> RuntimeRunTransitionHistoryResponse
```

Accepted HTTP/dev routes:

```text
GET /v1/run-transitions?run_id=<id>&cursor=<cursor>&limit=<n>
GET /v1/run-transitions?session_id=<id>&cursor=<cursor>&limit=<n>
GET /v1/run-transitions?turn_id=<id>&cursor=<cursor>&limit=<n>
```

Accepted Wails bridge:

```text
RuntimeBridge.RunTransitionHistory(req RuntimeRunTransitionHistoryRequest)
  -> RuntimeRunTransitionHistoryResponse
```

Contract rules:

- Exactly one of `run_id`, `session_id`, or `turn_id` should be provided by
  callers. The service may preserve the existing internal priority order for
  defensive compatibility, but tests should cover the intended single-filter
  contract.
- HTTP/dev module routing must support the same path/query shape as direct
  HTTP.
- Wails bridge tests must prove delegation only; generated binding smoke may be
  deferred to a later packaged validation phase if no frontend consumer exists.
- The response must remain transition rows plus window/source metadata only.
  It must not include synthesized lifecycle, timeline, diagnostics, artifact,
  checkpoint, permission, MCP, or interrupted actionability state.
- Runtime events may trigger a refresh of this DTO, but event payloads must not
  be merged into transition history or frontend source-of-truth state.

Acceptance tests required for Phase 10.8:

- Runtime service interface compile-time coverage for `RunTransitionHistory`.
- HTTP direct route returns read-only transition history for run/session/turn
  filters and cursor/limit query values.
- HTTP dev module route delegates to the same service method.
- Wails bridge delegates to the service method and preserves request fields.
- Existing RunProjection, Run detail, SessionActivity, checkpoint resume, and
  stale actionability tests still pass.

Review conclusion:

- Phase 10.7 accepts read-only transport exposure for transition history.
- Frontend consumption, generated binding smoke, and any UI are explicitly
  deferred.
- Transition history remains audit evidence and cannot become lifecycle or
  actionability truth.

### Phase 10.8: Transition History Read-only Transport Exposure

Status: accepted.

Scope:

- Expose the Phase 10.6 read-only transition-history DTO through the
  transport-neutral runtime boundary.
- Add direct HTTP/dev module route coverage.
- Add Wails bridge delegation coverage.

Implementation notes:

- Added `RunTransitionHistory(ctx, RuntimeRunTransitionHistoryRequest)` to
  `RuntimeService`.
- Added direct HTTP `GET /v1/run-transitions` with `run_id`, `session_id`,
  `turn_id`, `cursor`, and `limit` query support.
- Added dev module support for `/v1/run-transitions`.
- Added Wails bridge aliases and `RuntimeBridge.RunTransitionHistory(...)`.
- Added transport tests proving direct HTTP, dev module, and Wails bridge
  delegation preserve request fields and return read-only/audit-only response
  metadata.
- Did not add client adapter consumption, React state, frontend UI, generated
  binding smoke, scheduler, automatic resume, or transition-derived
  actionability.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeHTTPServerRoutesRunTransitionHistoryToRuntimeService|TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy|TestRuntimeRunTransitionHistory"
  -count=1` passed.
- `go test ./desktop -run
  "TestRuntimeBridgeNarrowActivityUsesRuntimeService|TestRuntimeBridgeForwardsDurableRunReads"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `go test ./... -timeout 180s` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 10.8 exposes transition history as read-only transport diagnostics
  only.
- Frontend consumption and generated binding smoke remain deferred until a
  separate accepted validation or UI phase.
- Existing Run detail, RunProjection, and SessionActivity remain the source of
  lifecycle and actionability state.
- Phase 10.8 is accepted as read-only transport exposure only.

### Phase 10.9: Transition History Generated Binding Validation Gate

Status: accepted as a design gate only.

Scope:

- Decide whether generated Wails binding validation is required before any
  frontend adapter consumes `RunTransitionHistory`.
- Define a minimal generated/packaged smoke strategy if validation is required.
- Keep validation focused on binding availability and read-only DTO shape.

Out of scope:

- Frontend adapter consumption.
- React state or UI.
- Scheduler, automatic resume, Run management UI, or transition-derived
  actionability.

Accepted decision:

- Generated binding validation is required before any future frontend adapter
  consumes `RunTransitionHistory`.
- A packaged WebView click smoke is not required yet because there is no
  frontend consumer or visible UI.
- The validation should mirror the Phase 8.8 binding smoke pattern and stay
  limited to generated bridge availability and read-only DTO shape.

Accepted validation strategy for Phase 10.10:

- Run `cd desktop && wails3 task common:generate:bindings`.
- Add a small client-side smoke script under `client/scripts` that reads
  `desktop/frontend/bindings/.../runtimebridge.js`.
- Assert the generated file exports `RunTransitionHistory`.
- Assert the generated function delegates through `$Call.ByID`.
- Assert no client runtime adapter or React source imports/uses
  `RunTransitionHistory` in this phase.
- Keep any temporary smoke outputs under `tmp/runtime-dev`.

Validation commands for Phase 10.10:

- `cd desktop && wails3 task common:generate:bindings`
- `cd client && npm run smoke:phase1010`
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`
- `git diff --check`

Review conclusion:

- Phase 10.9 accepts generated binding validation as the next step.
- It does not accept frontend consumption, React UI, packaged click smoke, or
  transition-derived lifecycle/actionability state.

### Phase 10.10: Transition History Generated Binding Smoke

Status: accepted.

Scope:

- Generate Wails bindings and verify `RunTransitionHistory` is available in the
  generated runtime bridge output.
- Add a small smoke script that can be rerun before any future frontend adapter
  consumption.
- Keep validation limited to binding availability and read-only transport
  shape.

Implementation notes:

- Ran `cd desktop && wails3 task common:generate:bindings`; generated output
  contains `RunTransitionHistory(req)` delegating through `$Call.ByID`.
- Added `client/scripts/phase1010-transition-binding-smoke.mjs`.
- Added `npm run smoke:phase1010`.
- The smoke verifies:
  - generated `runtimebridge.js` exports `RunTransitionHistory(req)`;
  - generated bridge delegates through `$Call.ByID`;
  - `client/src/runtime/wailsWorkbenchAdapter.ts` does not consume
    `RunTransitionHistory`;
  - `client/src/runtime/workbenchTypes.ts` does not expose
    `RunTransitionHistory`.
- Generated binding files are not tracked in this repository state, so the
  committed guard is the rerunnable smoke script and package command.

Validation:

- `cd desktop && wails3 task common:generate:bindings` passed.
- `cd client && npm run smoke:phase1010` passed.
- `cd client && npm run build` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 10.10 validates generated Wails binding availability for
  `RunTransitionHistory`.
- No frontend adapter consumption, React state, UI, packaged click smoke,
  scheduler, automatic resume, or transition-derived actionability was added.
- Phase 10.10 is accepted as generated-binding validation only.

### Phase 10.11: Transition History Frontend Diagnostic Use-case Gate

Status: accepted as a design gate only.

Scope:

- Decide whether there is a real frontend diagnostic use case for consuming
  transition history.
- If a use case exists, define the minimal read-only view model and where it
  should appear without becoming Run management UI.
- If no use case exists, explicitly defer frontend consumption and keep
  transition history as transport-only audit evidence.

Out of scope:

- Implementing frontend adapter consumption.
- React state or UI.
- Scheduler, automatic resume, Run management UI, or transition-derived
  actionability.

Accepted decision:

- There is no current frontend diagnostic use case strong enough to consume
  transition history in the client adapter or React view model.
- Existing user-visible diagnostics should continue to come from refreshed
  `SessionActivity`, `RunProjection`, and persisted Run detail DTOs.
- Transition history remains a transport-accessible read-only audit stream for
  backend tests, support debugging, and future operator diagnostics.
- Generated binding availability from Phase 10.10 is validation evidence only;
  it does not authorize frontend consumption by itself.
- Event payloads may select a transition-history refresh only in a future
  accepted diagnostic phase. Even then, transition rows must remain display-only
  audit evidence and must not be merged into timeline, lifecycle, checkpoint,
  permission, MCP, artifact, diagnostics, interrupted, or tool actionability
  state.

Rejected frontend use cases for this phase:

- A transition-history panel next to `RunProjectionPreview`: rejected because it
  would duplicate lifecycle/status signals already exposed by RunProjection and
  risks making transition rows look authoritative.
- A timeline overlay built from transition rows: rejected because timeline,
  tool, permission, artifact, and diagnostic parity must stay based on
  `SessionActivity` and narrow activity DTOs.
- React-side transition correlation for checkpoint resume: rejected because
  checkpoint actionability is already derived from refreshed runtime checkpoint
  DTOs, not audit rows.

Future acceptance criteria for any frontend consumption:

- The use case must be a concrete diagnostic workflow that cannot be satisfied
  by `SessionActivity`, `RunProjection`, or persisted Run detail.
- The view model must be additive, read-only, and named as audit/diagnostic
  evidence rather than Run lifecycle state.
- The adapter must fetch the full DTO/window from Go after an event trigger; it
  must not merge event payload fields into transition history.
- Tests must prove duplicate lifecycle/checkpoint/permission/artifact events do
  not duplicate visible rows or resurrect stale actionability.
- Tests must prove the corresponding `SessionActivity`/`RunProjection` subset
  remains the source for messages, tool calls, permissions, diagnostics,
  artifact evidence, interrupted summaries, terminal permission/MCP semantics,
  and checkpoint actionability.

Validation:

- Reviewed `RunProjectionPreview` and existing diagnostics surfaces. They
  already expose the narrow user-facing Run status, counts, checkpoint, and
  artifact diagnostics that are safe for React to render.
- Reviewed Phase 10.10 smoke coverage. It proves binding availability and
  intentionally guards against client runtime/workbench type consumption.
- `git diff --check` passed.

Review conclusion:

- Phase 10.11 accepts no frontend transition-history consumption at this time.
- Transition history remains read-only transport/audit evidence.
- `SessionActivity`, `RunProjection`, and persisted Run detail remain frontend
  lifecycle/actionability sources.
- No frontend adapter consumption, React state, UI, scheduler, automatic resume,
  Run management UI, or transition-derived actionability is accepted.

### Phase 10.12: Transition History Phase Acceptance And Next Boundary Gate

Status: accepted as a design gate only.

Scope:

- Review Phase 10 as a whole after transition-history storage, runtime wiring,
  read-only DTO, transport exposure, generated binding validation, and frontend
  non-consumption have all been accepted.
- Decide what implementation boundary is safe next.
- Keep transition history from becoming lifecycle or actionability authority
  by accident.

Accepted Phase 10 closure:

- `runtime_run_transitions` is accepted as durable audit evidence for Run
  lifecycle transitions.
- Transition writes are accepted only in already-approved runtime paths:
  turn start, turn finish, cancellation, interrupted acknowledgement, startup
  recovery, and explicit checkpoint resume.
- `RunTransitionHistory` is accepted as a read-only transport DTO with stable
  cursor/window semantics.
- Generated Wails binding availability is accepted as validation evidence.
- Frontend transition-history consumption remains deferred.

Non-authoritative boundary:

- Transition history must not drive current Run status, checkpoint
  actionability, permission actionability, MCP auth/elicitation actionability,
  artifact evidence, diagnostics, interrupted summaries, or timeline cards.
- Runtime events remain refresh triggers. Event payloads may select DTOs to
  refresh but may not populate lifecycle, actionability, timeline, diagnostics,
  artifact, checkpoint, or transition-history state.
- `SessionActivity`, `RunProjection`, persisted Run detail, current runtime
  permission/MCP stores, and structured checkpoint DTOs remain the source of
  truth for user-visible behavior.

Next accepted boundary:

- Phase 11 should be a design gate for Run lifecycle source-of-truth cutover.
- Phase 11 must decide whether any current lifecycle field may move from
  computed projection toward persisted Run detail, and exactly which existing
  runtime evidence remains authoritative.
- Phase 11 must not implement scheduler, automatic resume, background Run
  execution, frontend Run management UI, or transition-derived actionability.
- Any later implementation must be scoped to one narrow source-of-truth change
  with restart, replay, checkpoint, permission, MCP, artifact, diagnostics, and
  `SessionActivity` parity tests.

Validation:

- Reviewed Phase 10.1 through Phase 10.11 records.
- Confirmed all Phase 10 code-bearing steps already list their Go/client
  validation commands.
- `git diff --check` passed.

Review conclusion:

- Phase 10 is accepted as the transition-history audit foundation.
- The next task is Phase 11: Run Lifecycle Source-of-Truth Cutover Design Gate.
- No scheduler, automatic resume, frontend Run management UI, transition-driven
  actionability, or React-owned runtime state is accepted by Phase 10.

### Phase 11: Run Lifecycle Source-of-Truth Cutover Design Gate

Status: accepted as a design gate only.

Purpose:

- Decide the first safe source-of-truth cutover after Phase 10 transition
  history.
- Prevent accidental promotion of audit rows, event payloads, or React state
  into lifecycle/actionability authority.
- Define the next implementation slice narrowly enough to be tested against
  `SessionActivity`, `RunProjection`, persisted Run detail, checkpoint, MCP,
  permission, artifact, diagnostics, and restart evidence.

Current state:

- Persisted `runtime_runs` already provides durable Run identity, session links,
  checkpoints, source, status, timestamps, and list/detail reads.
- `RunProjection` is still assembled from `SessionActivity`, turns, tool calls,
  permissions, runtime events, and agent tasks.
- `RunProjection` may upsert the persisted Run summary, which means persisted
  Run detail is already a reconciliation cache for structured runtime evidence,
  not an independent scheduler-owned state machine.
- `runtime_run_transitions` records ordered audit evidence only. It is useful
  for debugging and replay validation, but it does not decide current state.

Accepted cutover decision:

- The first source-of-truth implementation should harden persisted Run detail
  reconciliation, not introduce a scheduler or transition-derived state
  machine.
- Persisted Run detail may become the durable read source for Run list/detail
  status, timestamps, session links, and checkpoint markers only when it is
  refreshed from `RunProjection`/structured runtime evidence.
- `RunProjection` remains the parity oracle for lifecycle status, counts,
  checkpoints, diagnostics, artifacts, and user action eligibility.
- `SessionActivity` remains the fallback/parity oracle for timeline, messages,
  tool calls, permissions, diagnostics, artifact evidence, interrupted
  summaries, and terminal MCP semantics.
- Current runtime permission/MCP stores remain the only source for actionable
  permission, auth, and elicitation gates.
- Transition rows remain audit evidence. They may be used in tests to confirm
  replay order, but they must not determine current lifecycle or actionability.

Rejected cutovers:

- Making `runtime_run_transitions` the lifecycle state machine: rejected because
  audit rows cannot safely represent current permission/MCP/checkpoint
  actionability without refreshed runtime evidence.
- Adding a background Run scheduler: rejected until persisted Run detail,
  projection parity, and restart semantics are hardened first.
- Auto-resuming interrupted Runs from checkpoints or transitions: rejected;
  resume remains explicit user-triggered continuation from a structured
  checkpoint.
- Adding frontend Run management UI: rejected because frontend still lacks an
  accepted runtime-owned management contract.

Phase 11.1 implementation gate:

- Add tests and, if needed, small runtime/store changes proving persisted Run
  detail reconciliation is current after:
  - turn start;
  - turn finish success/failure/cancellation;
  - interrupted acknowledgement;
  - startup recovery;
  - explicit checkpoint resume;
  - checkpoint acknowledgement/discard.
- Prove persisted Run detail and `RunProjection` agree on status, finished
  timestamp, session ids, checkpoint markers, resumed turn links, and read-only
  source metadata where applicable.
- Prove divergence handling favors refreshed projection/structured runtime
  evidence, never stale persisted status or transition rows.
- Keep all actionability decisions tied to current runtime permission/MCP
  stores and structured checkpoint DTOs.

Out of scope for Phase 11.1:

- New database migration.
- Scheduler, automatic resume, or background Run execution.
- Frontend Run management UI.
- Transition-derived actionability.
- React state as lifecycle or checkpoint truth.
- Assistant-prose-derived artifact, checkpoint, or lifecycle inference.

Validation required for Phase 11.1:

- Focused Go tests for Run detail/projection reconciliation.
- Existing restart stale-actionability tests covering unfinished tools,
  permissions, MCP auth requests, and MCP elicitation requests.
- Existing checkpoint resume tests proving explicit new-turn semantics.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Review conclusion:

- Phase 11 accepts a narrow persisted Run detail reconciliation hardening as
  the next implementation.
- It does not accept a full Run state machine, scheduler, automatic resume,
  frontend Run management UI, transition-derived actionability, or React-owned
  lifecycle state.

### Phase 11.1: Persisted Run Detail Reconciliation Hardening

Status: accepted.

Scope:

- Harden `Run(ctx, runID)` so returned persisted Run detail reflects the
  reconciliation performed by `RunProjection`.
- Add focused tests proving stale persisted status does not leak through Run
  detail after projection refresh.
- Add focused tests proving checkpoint acknowledgement/discard markers survive
  Run detail reconciliation.

Implementation notes:

- `runtimeService.Run(...)` now re-reads `runtime_runs` after building
  `RunProjection`, because `RunProjection` may upsert refreshed status,
  timestamps, sessions, and checkpoints.
- The fix keeps persisted Run detail as a reconciliation cache for structured
  runtime evidence. It does not use transition rows as current lifecycle truth.
- The `runtime_run_projection_test` fixture now wires a real `runtimeRunStore`
  so projection/detail parity tests exercise persisted Run rows.
- Added `TestRuntimeRunDetailRefreshesPersistedStatusFromProjection`, which
  creates a stale persisted `active` Run and proves `Run(...)` returns the
  refreshed `interrupted` status from projection-backed reconciliation.
- Added `TestRuntimeRunDetailPreservesCheckpointMarkersThroughReconciliation`,
  which proves checkpoint acknowledgement/discard markers and non-actionability
  survive subsequent Run detail refreshes.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunDetailRefreshesPersistedStatusFromProjection" -count=1`
  failed before the fix and passed after the fix.
- `go test ./internal/runtime -run
  "TestRuntimeRun(Projection|Detail|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Persisted Run detail now returns the refreshed reconciliation result rather
  than a stale pre-projection row.
- Checkpoint marker durability is covered at the Run detail boundary.
- No migration, scheduler, automatic resume, frontend Run management UI,
  background Run execution, transition-derived actionability, React-owned
  lifecycle state, or prose-derived lifecycle/checkpoint/artifact inference was
  introduced.

### Phase 11.2: Persisted Run List Reconciliation Hardening

Status: accepted.

Scope:

- Prove `Runs(ctx)` returns list rows reconciled from `RunProjection` and
  structured runtime evidence, not stale persisted status.
- Keep the list contract aligned with `Run(ctx, runID)` after Phase 11.1.
- Add tests first; make only minimal runtime/store fixes if a stale list row can
  leak.

Out of scope:

- New database migration.
- Scheduler, automatic resume, or background Run execution.
- Frontend Run management UI.
- Transition-derived actionability or transition-derived current lifecycle.
- React-owned lifecycle/checkpoint state.

Validation required:

- Focused Go tests for `Runs(ctx)` list/status reconciliation.
- Existing Run detail/projection/checkpoint marker tests.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Implementation notes:

- Added `TestRuntimeRunListRefreshesPersistedStatusFromProjection`.
- The test creates a stale persisted `active` Run list row, then calls
  `Runs(ctx)` and proves list status/finished timestamp are refreshed from
  projection-backed reconciliation.
- No runtime code change was needed; existing `Runs(ctx)` calls
  `backfillRuntimeRuns`, which refreshes each session through `RunProjection`
  before listing persisted runs.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunListRefreshesPersistedStatusFromProjection" -count=1`
  passed.
- `go test ./internal/runtime -run
  "TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Run list reads now have explicit regression coverage matching the Phase 11.1
  Run detail reconciliation contract.
- No migration, scheduler, automatic resume, frontend Run management UI,
  background Run execution, transition-derived actionability, React-owned
  lifecycle state, or prose-derived lifecycle/checkpoint/artifact inference was
  introduced.

### Phase 11.3: Full-window-only Persisted Run Reconciliation

Status: accepted.

Scope:

- Ensure only full `RunProjection` reads can reconcile persisted Run detail.
- Prevent cursor/limit projection windows from overwriting durable Run status,
  timestamps, checkpoints, sessions, or objective with partial evidence.
- Preserve bounded `RunProjection` as read-only window/parity evidence.

Out of scope:

- New database migration.
- Scheduler, automatic resume, or background Run execution.
- Frontend Run management UI.
- Transition-derived current lifecycle or actionability.
- React-owned lifecycle/checkpoint state.

Validation required:

- Focused Go tests proving bounded `RunProjection(limit/cursor)` does not
  mutate persisted Run detail.
- Existing Run detail/list reconciliation tests proving full projections still
  refresh persisted rows.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Implementation notes:

- Added `runtimeRunProjectionCanReconcile(...)` and limited
  `RunProjection` persistence to full reads with no cursor and no positive
  limit.
- Bounded `RunProjection(limit/cursor)` still returns windowed DTOs, cursors,
  and parity evidence, but it no longer mutates `runtime_runs`.
- Added `TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail`.
  The test failed before the fix because `RunProjection(limit=1)` could mutate
  a persisted interrupted Run to completed from partial evidence.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail" -count=1`
  failed before the fix and passed after the fix.
- `go test ./internal/runtime -run
  "TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail|TestRuntimeRunDetailRefreshesPersistedStatusFromProjection|TestRuntimeRunListRefreshesPersistedStatusFromProjection|TestRuntimeRunProjectionCursorWindowKeepsSessionActivityParity"
  -count=1` passed.
- `go test ./internal/runtime -run
  "TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Persisted Run reconciliation is now full-window-only.
- Cursor/limit RunProjection reads remain read-only evidence and cannot clobber
  durable Run status, timestamps, checkpoint rows, sessions, or objective from
  partial evidence.
- No migration, scheduler, automatic resume, frontend Run management UI,
  background Run execution, transition-derived actionability, React-owned
  lifecycle state, or prose-derived lifecycle/checkpoint/artifact inference was
  introduced.

### Phase 11.4: Run Reconciliation Acceptance And Scheduler Readiness Gate

Status: accepted as a design gate only.

Scope:

- Review Phase 11.1 through 11.3 as a complete Run reconciliation hardening
  slice.
- Decide whether the next implementation can safely move beyond reconciliation
  toward lifecycle execution ownership, or whether more read/recovery hardening
  is required first.
- Keep the gate grounded in Claude Code lessons: transcript/evidence first,
  runtime-owned actionability, no browser-owned execution state.

Out of scope:

- Implementing scheduler, automatic resume, or background Run execution.
- Implementing frontend Run management UI.
- Making transition rows lifecycle or actionability truth.
- New database migration.

Acceptance questions:

- Are persisted Run detail/list rows now safe as reconciled read sources when
  hydrated through full projection/detail/list refreshes?
- Are bounded activity/projection windows guaranteed read-only?
- Are checkpoint markers and resumed-turn links protected from projection
  refresh clobbering?
- Do restart, permission, MCP, interrupted, artifact, and checkpoint tests still
  prove stale actionability is not restored?
- Is the next safe implementation a scheduler gate, or another recovery/parity
  hardening slice?

Accepted answers:

- Persisted Run detail and list rows are safe as reconciled read sources only
  when hydrated through full Run detail/list/projection refreshes.
- Bounded activity/projection windows are now guaranteed read-only for persisted
  Run reconciliation.
- Checkpoint acknowledgement/discard markers are protected from projection
  refresh clobbering at the Run detail boundary.
- Resumed-turn links remain explicit checkpoint metadata and are not inferred
  from transition rows or assistant prose.
- Existing restart, permission, MCP, interrupted, artifact, checkpoint, Run
  detail, Run list, and transition tests prove the current read/recovery slice
  does not restore stale actionability.
- The next safe task is not scheduler implementation. It should be a Phase 12
  design gate for Run execution ownership and scheduler boundaries.

Phase 12 design gate requirements:

- Define the first possible Run-owned execution boundary without replacing
  session-first turns prematurely.
- Decide whether a scheduler can use reconciled Run detail for ownership,
  grouping, and cancellation scope while leaving permission/MCP actionability in
  current runtime stores.
- Define how explicit checkpoint resume remains a user-triggered new turn and
  cannot become automatic replay.
- Require tests for restart, cancellation, permission/MCP gates, checkpoint
  resume, transition audit ordering, and `SessionActivity` parity before any
  scheduler code.

Review conclusion:

- Phase 11 is accepted as Run reconciliation hardening only.
- It makes persisted Run detail/list reads safer, but it does not implement or
  authorize a scheduler, automatic resume, background Run execution, frontend
  Run management UI, transition-derived actionability, or React-owned lifecycle
  state.
- Phase 12 should be a design gate before any execution ownership change.

### Phase 12: Run Execution Ownership And Scheduler Design Gate

Status: accepted as a design gate only.

Purpose:

- Decide the first safe move from Run read/reconciliation hardening toward Run
  execution ownership.
- Keep the current session-first turn execution path intact until ownership,
  cancellation, recovery, and actionability contracts are proven.
- Prevent a scheduler implementation from bypassing structured runtime
  evidence, permission/MCP stores, `SessionActivity`, or explicit checkpoint
  semantics.

Current execution model:

- `Chat(ctx, RuntimeChatRequest)` remains the execution entry point.
- Before a turn is linked to execution, the runtime ensures a durable Run for
  the session through `EnsureForSession`.
- After the `RuntimeTurn` row exists, the runtime links the turn to the Run via
  `LinkTurn` and records a `turn_started` transition.
- `runChat` remains the model/tool execution loop. It records final turn
  evidence, reconciles Run detail through `RunProjection`, and records terminal
  transition audit rows.
- `CancelTurn`, startup recovery, `MarkInterruptedDone`, and explicit
  checkpoint resume already update structured turn/tool/permission/MCP evidence
  before transition rows are used as audit evidence.

Accepted scheduler boundary:

- Do not implement a new background scheduler yet.
- Do not replace session-first turns with Run-first execution yet.
- A future scheduler may use reconciled Run detail for ownership, grouping,
  cancellation scope, and operator diagnostics only after those contracts are
  proven.
- Permission, MCP auth, and elicitation actionability must continue to come
  from current runtime stores.
- Checkpoint resume must remain an explicit user-triggered new turn. It must
  not replay prior tools or auto-start from transition/checkpoint evidence.
- Transition rows may validate order and replay behavior, but they must not
  decide current lifecycle or actionability.
- Frontend event payloads may trigger DTO refreshes only; they must not merge
  scheduler/run state into React.

Rejected implementation paths:

- A Run-first background scheduler that directly owns tool execution.
- Automatic resume of interrupted Runs or checkpoints.
- Restoring stale running/waiting tools, permission gates, MCP auth requests,
  or MCP elicitation requests on startup.
- Frontend Run management UI or React-owned lifecycle state.
- Using transition history as the lifecycle state machine.

Phase 12.1 implementation gate:

- Add contract tests for the existing session-first execution path proving Run
  ownership preflight:
  - `Chat` ensures a durable Run before execution;
  - the persisted turn is linked to the Run before `turn_started` transition
    audit is considered useful;
  - cancellation, startup recovery, interrupted acknowledgement, and checkpoint
    resume keep Run ownership links stable;
  - no permission/MCP/checkpoint actionability is derived from transition rows
    or event payloads.
- Make only minimal runtime fixes if those contracts fail.
- Do not add a scheduler implementation, migration, automatic resume,
  background execution worker, frontend Run management UI, or transition-driven
  actionability.

Validation required for Phase 12.1:

- Focused Go tests for Run ownership preflight and link stability.
- Existing Run detail/list/projection reconciliation tests.
- Existing restart stale-actionability and checkpoint resume tests.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Review conclusion:

- Phase 12 accepts Run execution ownership work only as a contract-first
  hardening path.
- The next implementation is Phase 12.1 Run ownership preflight coverage, not a
  scheduler implementation.
- No scheduler, automatic resume, background Run execution, frontend Run
  management UI, transition-derived actionability, React-owned lifecycle state,
  or new migration is accepted by Phase 12.

### Phase 12.1: Run Ownership Preflight Contract Coverage

Status: accepted.

Scope:

- Add contract coverage proving `turn_started` transition audit rows are useful
  only after the persisted turn is linked to the durable Run/session row.
- Prove interrupted acknowledgement keeps existing Run-turn ownership links
  stable.
- Prove checkpoint resume transition evidence does not mutate checkpoint
  markers and explicit resumed-turn links remain structured checkpoint metadata.

Implementation notes:

- Added `runtimeRunSessionLinkedToTurn(...)` as a small internal store helper.
- `recordRunTurnTransition(...)` now skips `turn_started` transitions unless
  `runtime_run_sessions.turn_id` is already linked to the turn.
- Added
  `TestRuntimeRunTransitionWriterRequiresRunTurnLinkBeforeStartedTransition`.
  It proves a `turn_started` transition is not recorded before `LinkTurn`, then
  records normally after the link exists.
- Extended interrupted acknowledgement transition coverage to assert the Run
  turn link survives `MarkInterruptedDone`.
- Extended checkpoint resume transition coverage to assert explicit resumed
  turn ids stay in checkpoint metadata without acknowledging/discarding the
  checkpoint.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunTransitionWriter(RequiresRunTurnLinkBeforeStartedTransition|RecordsTurnLifecycleIdempotently|MarkInterruptedDonePreservesCancelledSemantics|RecordsStartupRecoveryForTurnAndTask|RecordsCheckpointResumeFromNewTurn)"
  -count=1` passed.
- `go test ./internal/runtime -run "TestRuntimeRunTransitionWriter" -count=1`
  passed.
- `go test ./internal/runtime -run
  "TestRuntimeRunTransitionWriter|TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Run ownership preflight is now explicit at the transition writer boundary:
  `turn_started` audit evidence requires a persisted Run/session/turn link.
- Terminal acknowledgement and checkpoint resume contracts preserve ownership
  links and checkpoint evidence.
- No migration, scheduler implementation, automatic resume, background Run
  execution, frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or prose-derived lifecycle/checkpoint/artifact
  inference was introduced.

### Phase 12.2: Run Cancellation Ownership Contract Coverage

Status: accepted.

Scope:

- Add focused coverage for cancellation ownership on the existing session-first
  execution path.
- Prove `CancelTurn` preserves the Run/session/turn link, terminalizes current
  turn/tool evidence, reconciles persisted Run detail, and records transition
  audit after structured evidence is terminal.
- Prove cancellation does not derive permission, MCP, checkpoint, or artifact
  actionability from transition rows or event payloads.

Out of scope:

- New database migration.
- Scheduler implementation or background Run execution worker.
- Automatic resume.
- Frontend Run management UI.
- Transition-derived lifecycle/actionability.
- React-owned lifecycle/checkpoint state.

Validation required:

- Focused Go tests for cancellation ownership/link stability.
- Existing Run ownership preflight and reconciliation tests.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Implementation notes:

- Added `TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership`.
- The test exercises real `CancelTurn` over the existing session-first runtime
  path with a durable Run, linked turn, waiting tool call, and backend session.
- It proves cancellation:
  - preserves the `runtime_run_sessions` turn link;
  - terminalizes the turn and waiting tool call;
  - records `turn_cancelled` transition audit from terminal turn evidence;
  - reconciles persisted Run detail/projection to `cancelled`;
  - emits `turn.cancelled` without deriving actionability from event payloads.
- No runtime code change was needed.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership" -count=1`
  passed.
- `go test ./internal/runtime -run
  "TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership|TestRuntimeRunTransitionWriter|TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Cancellation ownership is covered at the existing session-first execution
  boundary.
- `CancelTurn` preserves Run ownership links and records transition audit only
  after structured turn/tool evidence is terminal.
- No migration, scheduler implementation, automatic resume, background Run
  execution, frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or prose-derived lifecycle/checkpoint/artifact
  inference was introduced.

### Phase 12.3: Run Startup Recovery Ownership Contract Coverage

Status: accepted.

Scope:

- Add focused coverage for startup recovery ownership on existing durable Run
  rows.
- Prove startup recovery interrupts unfinished turn/task evidence, preserves
  Run/session/turn ownership links, cancels stale tool/permission/MCP
  actionability, reconciles Run detail, and records recovery transition audit
  only after stale evidence is terminalized.

Out of scope:

- New database migration.
- Scheduler implementation or background Run execution worker.
- Automatic resume.
- Frontend Run management UI.
- Transition-derived lifecycle/actionability.
- React-owned lifecycle/checkpoint state.

Validation required:

- Focused Go tests for startup recovery ownership/link stability.
- Existing Run ownership preflight, cancellation, and reconciliation tests.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Implementation notes:

- Extended `TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability`
  with Run startup recovery ownership assertions.
- The test now wires `runtime_run_transitions`, preserves the
  `runtime_run_sessions` turn link across startup-style interruption, records a
  `startup_recovery` transition only after the interrupted turn has terminal
  evidence, and verifies persisted Run detail/projection reconcile to
  `interrupted`.
- The same fixture continues to prove unfinished tool calls are cancelled,
  pending permissions are expired, and stale MCP auth requests are cancelled
  rather than restored as actionable.
- No runtime code change was needed.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability"
  -count=1` passed.
- `go test ./internal/runtime -run
  "TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability|TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership|TestRuntimeRunTransitionWriter|TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Startup recovery ownership is covered at the existing recovery contract
  boundary.
- Recovery preserves Run ownership links, terminalizes stale evidence before
  transition audit, and keeps stale permission/MCP/tool actionability from
  being restored.
- No migration, scheduler implementation, automatic resume, background Run
  execution, frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or prose-derived lifecycle/checkpoint/artifact
  inference was introduced.

### Phase 12.4: Run Checkpoint Resume Ownership Contract Coverage

Status: accepted.

Scope:

- Add focused coverage for explicit checkpoint resume ownership on the existing
  session-first execution path.
- Prove `ResumeRunCheckpoint` creates a new user-triggered turn, links resumed
  turn metadata to the checkpoint, records checkpoint resume transition audit,
  and does not mutate source checkpoint evidence into acknowledged/discarded or
  auto-resumed state.

Out of scope:

- New database migration.
- Scheduler implementation or background Run execution worker.
- Automatic resume.
- Frontend Run management UI.
- Transition-derived lifecycle/actionability.
- React-owned lifecycle/checkpoint state.

Validation required:

- Focused Go tests for explicit checkpoint resume ownership/link stability.
- Existing Run ownership preflight, cancellation, startup recovery, and
  reconciliation tests.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1`.
- `git diff --check`.

Implementation notes:

- Added
  `TestRuntimeRunTransitionWriterRequiresResumedTurnBeforeCheckpointResume`.
- The test proves checkpoint resume transition audit is not recorded before the
  resumed turn exists.
- It also proves a created resumed turn can be linked through checkpoint
  metadata and then audited without acknowledging, discarding, or otherwise
  mutating the source checkpoint evidence.
- Existing
  `TestRuntimeRunTransitionWriterRecordsCheckpointResumeFromNewTurn` continues
  to prove checkpoint resume transition ordering and idempotency.
- No runtime code change was needed.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunTransitionWriter(RequiresResumedTurnBeforeCheckpointResume|RecordsCheckpointResumeFromNewTurn)"
  -count=1` passed.
- `go test ./internal/runtime -run
  "TestRuntimeRunTransitionWriter|TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability|TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership|TestRuntimeRun(Projection|Detail|List|Envelope|Store|Transition)|TestRuntimeRunStore|TestRuntimeRunTransition|TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Explicit checkpoint resume ownership is covered at the transition/checkpoint
  metadata boundary.
- Resume transition audit requires a concrete resumed turn and remains
  explicit user-triggered evidence, not automatic replay.
- No migration, scheduler implementation, automatic resume, background Run
  execution, frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or prose-derived lifecycle/checkpoint/artifact
  inference was introduced.

### Phase 12.5: Run Ownership Contract Acceptance And Scheduler Gate

Status: accepted as a design gate only.

Scope:

- Review Phase 12.1 through 12.4 as a complete Run ownership contract slice.
- Decide whether the next boundary can move to scheduler design or still needs
  more ownership/recovery hardening.
- Keep scheduler implementation out of this gate.

Acceptance questions:

- Does `turn_started` transition audit require a durable Run/session/turn link?
- Does cancellation preserve Run ownership and terminalize structured evidence
  before transition audit?
- Does startup recovery preserve Run ownership and terminalize stale
  tool/permission/MCP evidence before recovery audit?
- Does explicit checkpoint resume require a concrete resumed turn and keep
  source checkpoint evidence unchanged?
- Are all actionability decisions still owned by current runtime stores and
  refreshed DTOs rather than transition rows, event payloads, or React state?

Out of scope:

- Scheduler implementation or background Run execution worker.
- Automatic resume.
- Frontend Run management UI.
- New database migration.
- Transition-derived lifecycle/actionability.

Accepted answers:

- `turn_started` transition audit now requires an existing durable
  Run/session/turn link.
- Cancellation preserves Run ownership, terminalizes turn/tool evidence, and
  records transition audit after terminal evidence.
- Startup recovery preserves Run ownership, terminalizes stale
  tool/permission/MCP evidence, and records recovery transition audit after
  terminal evidence.
- Explicit checkpoint resume requires a concrete resumed turn before transition
  audit and keeps source checkpoint evidence unchanged.
- Permission, MCP auth, elicitation, checkpoint, artifact, interrupted, and
  lifecycle actionability still come from current runtime stores and refreshed
  DTOs, not transition rows, event payloads, or React state.

Accepted next boundary:

- Phase 13 may be a scheduler design gate.
- Phase 13 must define scheduler ownership without implementing it.
- The scheduler design may use reconciled Run detail for ownership, grouping,
  cancellation scope, and diagnostics.
- The scheduler design must keep session-first turn execution as the current
  implementation until a later accepted implementation phase.
- The scheduler design must preserve explicit checkpoint resume as a
  user-triggered new turn and must not introduce automatic resume.

Review conclusion:

- Phase 12 is accepted as Run ownership contract hardening.
- It does not implement scheduler, automatic resume, background Run execution,
  frontend Run management UI, transition-derived actionability, React-owned
  lifecycle state, or a new migration.
- The next task is Phase 13: Run Scheduler Design Gate.

### Phase 13: Run Scheduler Design Gate

Status: accepted as a design gate only.

Scope:

- Define what a future Run scheduler may own before any scheduler code is
  introduced.
- Preserve the current session-first execution path as the implementation until
  a later accepted scheduler implementation phase.
- Define the minimum contracts a scheduler implementation must satisfy around
  Run ownership, cancellation, diagnostics, checkpoints, permissions, MCP
  actionability, and activity parity.

Accepted scheduler ownership:

- A scheduler may create and own Run-level execution plans after a durable Run
  row and Run/session link exist.
- A scheduler may use reconciled Run detail for grouping, ownership, display
  status, cancellation scope, and diagnostics routing.
- A scheduler may assign future work to explicit user-triggered turns,
  checkpoint-resume turns, or task turns, but each executable unit must still
  have durable session/turn evidence before transition audit becomes useful.
- A scheduler may emit runtime events as refresh triggers that identify which
  DTO family changed.
- A scheduler may write audit/diagnostic evidence after the underlying
  structured runtime evidence has been persisted.

State that remains outside scheduler ownership:

- Permission request actionability remains owned by the permission/runtime
  stores and refreshed DTOs.
- MCP auth and elicitation actionability remains owned by the MCP/runtime
  stores and refreshed DTOs.
- Checkpoint actionability remains owned by structured checkpoint DTO state and
  explicit user actions.
- Artifact evidence remains owned by completed scheduler/tool output and
  persisted structured refs, not by scheduler intent, event payloads, or
  assistant prose.
- Timeline, diagnostics, interrupted summaries, terminal permission semantics,
  and terminal MCP semantics remain covered by `SessionActivity` and
  `RunProjection`/Run detail parity.
- Transition history remains audit evidence only. It may validate order and
  replay, but it cannot become lifecycle or actionability truth.

Required scheduler implementation entry criteria:

- Define a concrete scheduler DTO/API shape before adding a worker.
- Prove scheduler-created work cannot run without a durable Run/session/turn
  link.
- Prove cancellation terminalizes owned turn/tool evidence before recording
  transition audit.
- Prove startup recovery cancels stale running/waiting tool evidence, stale
  permissions, stale MCP auth requests, and stale MCP elicitation requests
  before any scheduler replay.
- Prove explicit checkpoint resume creates a new user-triggered turn and does
  not mutate source checkpoint evidence into acknowledged, discarded, or
  auto-resumed state.
- Prove scheduler reads preserve `SessionActivity` subset parity for messages,
  tool calls, permissions, diagnostics, artifact evidence, interrupted
  summaries, and terminal permission/MCP semantics.
- Prove runtime event payloads only select DTO refreshes and are never merged
  directly into timeline, diagnostics, artifact, interrupted, checkpoint,
  permission, MCP, or Run lifecycle state.

Out of scope:

- Implementing a scheduler or background Run worker.
- Automatic resume.
- Frontend Run management UI.
- New database migration.
- Transition-derived lifecycle/actionability.
- React-owned scheduler or Run lifecycle state.
- Inferring artifact, checkpoint, or lifecycle state from assistant prose.

Review conclusion:

- Phase 13 accepts a scheduler boundary, not scheduler behavior.
- The next implementation should be a narrow contract phase that introduces the
  smallest scheduler-facing DTO/API or store preflight needed to prove these
  boundaries, before any background worker or automatic execution loop exists.
- No migration, scheduler implementation, automatic resume, background Run
  execution, frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or prose-derived state was introduced.

### Phase 13.1: Scheduler-facing Contract Preflight

Status: accepted.

Scope:

- Add the smallest scheduler-facing preflight contract needed before any
  scheduler worker exists.
- Prove future scheduler-created work cannot be considered executable unless
  durable Run/session/turn ownership evidence exists.
- Keep the preflight read-only and internal; it must not expose scheduler
  actions through HTTP, Wails, or React.

Implementation notes:

- Added `RuntimeRunSchedulerPreflightRequest`,
  `RuntimeRunSchedulerPreflightResponse`, and
  `RuntimeRunSchedulerPreflightSource`.
- Added internal `runtimeRunSchedulerPreflight(...)`.
- The preflight reads only `runtime_runs`, `runtime_run_sessions`, and
  `runtime_turns`.
- `CanSchedule` is true only when:
  - the turn exists;
  - the session id matches the turn;
  - the Run exists;
  - the Run contains the session;
  - the Run/session link points at the same turn; and
  - the turn is not terminal.
- The preflight source is explicitly read-only and reports
  `StartsWorker=false`.

Rejected behavior:

- No scheduler worker or background execution loop.
- No automatic resume.
- No HTTP/Wails bridge route or frontend Run management UI.
- No transition-derived lifecycle/actionability.
- No permission/MCP/checkpoint/artifact actionability decision.
- No database migration.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerPreflight|TestRuntimeRunTransitionWriterRequiresRunTurnLinkBeforeStartedTransition|TestRuntimeRunTransitionWriterRequiresResumedTurnBeforeCheckpointResume"
  -count=1` passed.

Review conclusion:

- Phase 13.1 creates a future scheduler entry contract without making the
  scheduler executable.
- Scheduler readiness now has a concrete read-only preflight gate tied to
  durable Run/session/turn evidence.
- Runtime truth remains in existing stores and DTO refreshes; events,
  transition history, assistant prose, and React state were not promoted to
  lifecycle or actionability sources.

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

Implement Phase 13.2: Scheduler Preflight Acceptance And Integration Gate. Keep
it as a gate/review phase only unless a focused contract test is needed; decide
whether the internal preflight is sufficient for a later scheduler implementation
or whether another read-only contract boundary is required. Do not add
migrations, scheduler implementation, automatic resume, frontend Run management
UI, background Run execution, transition-derived actionability, or React-owned
lifecycle state.
