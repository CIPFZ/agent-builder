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

### Phase 13.2: Scheduler Preflight Acceptance And Integration Gate

Status: accepted as a review gate only.

Scope:

- Review whether Phase 13.1's internal preflight is sufficient as the first
  scheduler-facing execution gate.
- Decide whether another read-only boundary is required before any scheduler
  worker implementation.
- Keep this phase free of runtime behavior changes unless a focused contract
  gap is found.

Accepted review:

- Phase 13.1 is sufficient for the execution preflight boundary: a future
  scheduler cannot treat work as executable unless durable Run/session/turn
  evidence exists and the turn is non-terminal.
- The preflight is intentionally internal and read-only. It is not an HTTP,
  Wails, or React capability.
- The preflight does not replace `SessionActivity`, `RunProjection`, persisted
  Run detail, transition history, permission stores, MCP stores, checkpoint
  DTOs, artifact refs, or interrupted summaries.
- No additional code change is needed in this gate.

Remaining scheduler gap:

- A scheduler implementation still needs a concrete read-only Run scheduler
  plan DTO before any worker exists.
- That plan DTO must describe intended work ownership, ordering, cancellation
  scope, diagnostics routing, and refresh targets without starting work,
  replaying checkpoints, or changing actionability.
- The plan DTO must be checked by the Phase 13.1 preflight before execution in
  any future worker phase.

Rejected behavior:

- No scheduler worker or background execution loop.
- No automatic resume.
- No HTTP/Wails bridge route or frontend Run management UI.
- No transition-derived lifecycle/actionability.
- No permission/MCP/checkpoint/artifact actionability decision.
- No database migration.

Validation:

- Review confirmed `runtimeRunSchedulerPreflight(...)` is not exposed through
  `RuntimeService`, HTTP routes, Wails bridge bindings, or React adapters.
- Review confirmed the preflight source is read-only and reports
  `StartsWorker=false`.
- `git diff --check` passed.

Review conclusion:

- Phase 13.2 accepts Phase 13.1 as the scheduler execution preflight gate.
- The next safe task is Phase 14: Run Scheduler Plan DTO Design Gate.
- Phase 14 must define plan/read contracts only. It must not implement a
  scheduler worker, automatic resume, background execution, frontend Run
  management UI, transition-derived actionability, React-owned lifecycle state,
  or database migration.

### Phase 14: Run Scheduler Plan DTO Design Gate

Status: accepted as a design gate only.

Scope:

- Define the read-only Run scheduler plan DTO contract a future worker would
  consume.
- Preserve session-first execution until a later accepted implementation phase.
- Require Phase 13.1 preflight before any plan item can become executable.
- Keep scheduler plan output as planning evidence only, never lifecycle or
  actionability truth.

Accepted plan DTO shape:

```text
RuntimeRunSchedulerPlanRequest
  run_id
  session_id
  mode: user_turn | checkpoint_resume | task_turn
  turn_id?
  checkpoint_id?
  task_id?
  cursor?
  limit?

RuntimeRunSchedulerPlanResponse
  plan: RuntimeRunSchedulerPlan
  source: RuntimeRunSchedulerPlanSource

RuntimeRunSchedulerPlan
  run_id
  primary_session_id
  session_ids[]
  objective
  status_from_run_detail
  items[]
  cancellation_scope
  diagnostics_route
  refresh_targets[]
  activity_window?

RuntimeRunSchedulerPlanItem
  id
  kind: user_turn | checkpoint_resume | task_turn
  order_key
  session_id
  turn_id?
  checkpoint_id?
  task_id?
  can_schedule
  preflight_reason?
  required_preflight
  refresh_targets[]
  cancellation_scope
  diagnostics_route

RuntimeRunSchedulerPlanSource
  kind: run_scheduler_plan
  read_only: true
  starts_worker: false
  session_activity_parity: true
  evidence[]
```

Accepted source-of-truth rules:

- Run identity, primary session, linked sessions, checkpoints, and persisted
  summary come from persisted Run detail reconciled from `RunProjection`.
- Turn executability comes from Phase 13.1 preflight over `runtime_runs`,
  `runtime_run_sessions`, and `runtime_turns`.
- Timeline, messages, tool calls, permissions, diagnostics, artifact evidence,
  interrupted summaries, and terminal permission/MCP semantics remain from
  `SessionActivity`/activity windows and `RunProjection` parity.
- Checkpoint resume plan items may only describe an explicit user-triggered
  future turn. They must not auto-resume.
- Artifact refs may appear only as existing structured refs from completed
  tool/task/checkpoint evidence.
- Runtime events may carry only plan/read refresh targets such as `run`,
  `runProjection`, `turnActivity`, `sessionActivityWindow`,
  `sessionActivity`, or `schedulerPlan`.

Required plan item rules:

- `can_schedule=true` requires the Phase 13.1 preflight to pass.
- A plan item without a durable Run/session/turn link must remain
  non-executable with a preflight reason.
- A terminal turn must remain non-executable.
- A checkpoint item must reference a concrete checkpoint and must not mark it
  acknowledged, discarded, or resumed.
- A task item must remain read-only until a later task scheduling gate accepts
  executable task ownership.

Out of scope:

- Implementing the DTO in Go.
- Exposing scheduler plan through HTTP, Wails, or React.
- Scheduler worker or background execution loop.
- Automatic resume.
- Frontend Run management UI.
- Database migration.
- Transition-derived lifecycle/actionability.
- React-owned scheduler or Run lifecycle state.
- Inferring artifact, checkpoint, or lifecycle state from assistant prose.

Implementation entry criteria for Phase 14.1:

- Add internal DTO types and builder only.
- Keep the builder read-only and internal.
- Make `RuntimeRunSchedulerPlanSource.StartsWorker=false`.
- Use Phase 13.1 preflight for item executability.
- Add tests proving missing Run/session/turn links and terminal turns produce
  non-executable plan items.
- Add tests proving checkpoint plan items do not acknowledge, discard, resume,
  or mutate checkpoint evidence.

Validation:

- Design review only.
- `git diff --check` passed.

Review conclusion:

- Phase 14 accepts the scheduler plan DTO contract, not scheduler behavior.
- The next safe task is Phase 14.1: internal read-only scheduler plan DTO
  implementation.
- Phase 14.1 must not add a worker, automatic resume, background execution,
  frontend Run management UI, migration, transition-derived actionability, or
  React-owned lifecycle state.

### Phase 14.1: Internal Read-only Scheduler Plan DTO

Status: accepted.

Scope:

- Add internal scheduler plan DTO types and a read-only builder.
- Use Phase 13.1 preflight to decide plan item executability.
- Keep the plan internal; do not expose it through HTTP, Wails, or React.

Implementation notes:

- Added `RuntimeRunSchedulerPlanRequest`,
  `RuntimeRunSchedulerPlanResponse`, `RuntimeRunSchedulerPlan`,
  `RuntimeRunSchedulerPlanItem`, and `RuntimeRunSchedulerPlanSource`.
- Added internal `runtimeRunSchedulerPlan(...)`.
- The plan source is read-only, reports `StartsWorker=false`, and records
  evidence as `runtime_runs`, `runtime_run_sessions`, `runtime_turns`, and
  `runtime_run_checkpoints`.
- Plan-level routing includes cancellation scope, diagnostics route, and
  refresh targets only. These are descriptive read contracts, not events or
  execution commands.
- `user_turn` items use Phase 13.1 preflight for `CanSchedule`.
- Missing Run/session/turn links and terminal turns remain non-executable plan
  items with preflight reasons.
- `checkpoint_resume` items are non-executable until an explicit resumed turn
  exists and do not acknowledge, discard, resume, or mutate checkpoint
  evidence.
- `task_turn` items remain non-executable until a later task scheduling gate
  accepts executable task ownership.

Rejected behavior:

- No scheduler worker or background execution loop.
- No automatic resume.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No transition-derived lifecycle/actionability.
- No permission/MCP/checkpoint/artifact actionability decision.
- No database migration.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunScheduler(Plan|Preflight)"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 14.1 implements the internal read-only scheduler plan DTO without
  making scheduler behavior executable.
- The plan builder preserves the Phase 13.1 preflight boundary and does not
  mutate checkpoint evidence.
- Runtime truth remains in existing stores and DTO refreshes; events,
  transition history, assistant prose, and React state were not promoted to
  lifecycle or actionability sources.

### Phase 14.2: Scheduler Plan DTO Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review the internal scheduler plan DTO before any transport or worker phase.
- Decide whether the next safe boundary is read-only transport exposure,
  another backend contract test, or a worker design gate.
- Keep this phase free of runtime behavior changes unless a focused contract
  gap is found.

Accepted review:

- Phase 14.1 is sufficient as the internal scheduler plan contract.
- The plan source is read-only, reports `StartsWorker=false`, and is not
  exposed through `RuntimeService`, HTTP routes, Wails bridge bindings, or
  React adapters.
- Plan item executability is derived from Phase 13.1 preflight.
- Missing Run/session/turn links, mismatched sessions, and terminal turns remain
  non-executable.
- Checkpoint plan items remain descriptive and do not acknowledge, discard,
  resume, or mutate checkpoint evidence.
- Task items remain non-executable until a later task scheduling ownership gate.

Transport decision:

- Read-only transport exposure is not required before a minimal scheduler worker
  design. The plan is currently a backend-internal contract for future worker
  code, not a frontend surface.
- If transport is added later, it must be read-only and must not create frontend
  Run management UI or event-payload-derived state.

Remaining scheduler gap:

- A future worker design must decide how to apply the internal plan and
  preflight to the existing session-first `Chat` path.
- The first worker design should be user-triggered and foreground only. It must
  not introduce automatic resume, stale actionability replay, or unattended
  background execution.

Rejected behavior:

- No scheduler worker or background execution loop.
- No automatic resume.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No transition-derived lifecycle/actionability.
- No permission/MCP/checkpoint/artifact actionability decision.
- No database migration.

Validation:

- Review confirmed `runtimeRunSchedulerPlan(...)` is not exposed through
  `RuntimeService`, HTTP routes, Wails bridge bindings, or React adapters.
- Review confirmed `RuntimeRunSchedulerPlanSource.StartsWorker=false`.
- `git diff --check` passed.

Review conclusion:

- Phase 14.2 accepts the internal scheduler plan DTO.
- The next safe task is Phase 15: Minimal User-triggered Run Scheduler Worker
  Design Gate.
- Phase 15 must design worker behavior only. It must not implement a worker,
  automatic resume, unattended background execution, frontend Run management UI,
  transition-derived actionability, React-owned lifecycle state, or database
  migration.

### Phase 15: Minimal User-triggered Run Scheduler Worker Design Gate

Status: accepted as a design gate only.

Scope:

- Design the first executable scheduler step without implementing it.
- Constrain the first worker to the existing user-triggered `Chat` path.
- Keep execution foreground from the user's action perspective: the worker may
  delegate to the current asynchronous `runChat` goroutine, but it must not add
  an unattended background queue, poller, or automatic resume loop.
- Require the internal scheduler plan and Phase 13.1 preflight before
  delegating to execution.

Accepted worker shape:

```text
Chat(...)
  -> ensure/select session
  -> ensure durable Run
  -> persist queued RuntimeTurn
  -> link Run/session/turn
  -> build RuntimeRunSchedulerPlan for the turn
  -> require plan item can_schedule=true
  -> record turn_started transition audit
  -> write existing audit/event/budget/context evidence
  -> delegate to existing runChat(...)
```

Accepted ownership:

- The worker may own applying a single `user_turn` plan item to the existing
  execution path.
- The worker may fail fast before delegation when plan/preflight rejects the
  turn.
- The worker may emit refresh-trigger events that identify DTO families to
  refresh.
- The worker may record diagnostic/audit evidence only after durable runtime
  evidence exists.

State that remains outside worker ownership:

- Permission request actionability.
- MCP auth and elicitation actionability.
- Checkpoint resume actionability and acknowledgement/discard state.
- Artifact evidence and produced refs.
- Timeline, diagnostics, interrupted summaries, terminal permission semantics,
  and terminal MCP semantics.
- Transition history as lifecycle or actionability truth.
- React state as scheduler or Run truth.

Rejected first-worker behavior:

- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No task scheduling execution.
- No frontend Run management UI.
- No database migration.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Implementation entry criteria for Phase 15.1:

- Add an internal foreground scheduler delegate around the existing `Chat`
  turn-start path only.
- Preserve the public `Chat(ctx, RuntimeChatRequest)` contract.
- Keep `runChat(...)` as the execution delegate.
- If plan/preflight rejects, terminalize the queued turn before any
  `turn_started` transition or `runChat` delegation.
- Prove success path still links Run/session/turn before `turn_started`
  transition audit.
- Prove failed preflight does not call `runChat`, does not create stale
  actionability, and leaves Run detail/projection consistent.
- Prove checkpoint resume remains explicit and is not auto-triggered by the
  worker.

Validation:

- Design review only.
- `git diff --check` passed.

Review conclusion:

- Phase 15 accepts a minimal worker design that can begin replacing direct Chat
  delegation in a controlled way.
- The first implementation should be Phase 15.1: foreground user-turn scheduler
  delegate.
- Phase 15.1 may add internal execution wiring, but it must not add automatic
  resume, unattended background execution, frontend Run management UI,
  transition-derived actionability, React-owned lifecycle state, task execution
  scheduling, or database migration.

### Phase 15.1: Foreground User-turn Scheduler Delegate

Status: accepted.

Scope:

- Add internal execution wiring around the existing `Chat` turn-start path.
- Require scheduler plan/preflight success before `turn_started` transition
  audit and before delegating to `runChat(...)`.
- Keep `Chat(ctx, RuntimeChatRequest)` as the public user-triggered entry point.

Implementation notes:

- Added `runtimeRunSchedulerDelegateUserTurn(...)`.
- Added `failRuntimeRunScheduledTurn(...)`.
- `Chat` now links Run/session/turn, builds the internal scheduler plan for the
  queued turn, requires `CanSchedule=true`, and only then records
  `turn_started` transition audit.
- If the delegate rejects the turn, the queued turn is marked `failed`, a
  `turn.failed` refresh event is recorded, and `runChat(...)` is not started.
- Rejection also marks the in-memory request state finished/failed so status
  reads do not resurrect a stale busy turn.
- The successful path still delegates to the existing `runChat(...)` execution
  function. No new queue, poller, or background worker loop was added.

Rejected behavior:

- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No task scheduling execution.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No database migration.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunScheduler(Delegate|Plan|Preflight)|TestRuntimeRunTransitionWriterRequiresRunTurnLinkBeforeStartedTransition"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 15.1 introduces the first real scheduler execution boundary, but only
  as a foreground delegate for user-triggered `Chat`.
- Successful turns must pass the internal plan/preflight gate before
  `turn_started` transition audit and `runChat(...)`.
- Failed preflight terminalizes the queued turn and does not start execution.
- Failed preflight also terminalizes the in-memory request state.
- Runtime truth remains in existing stores and DTO refreshes; events,
  transition history, assistant prose, and React state were not promoted to
  lifecycle or actionability sources.

### Phase 15.2: Foreground Scheduler Delegate Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review Phase 15.1 before extending scheduler ownership.
- Decide whether the next safe boundary is checkpoint-resume delegate hardening,
  task scheduling design, or transport/read exposure.
- Keep this phase free of runtime behavior changes unless a focused contract
  gap is found.

Accepted review:

- Phase 15.1 is accepted as the first real scheduler execution boundary.
- The delegate is foreground and user-triggered through existing `Chat`.
- The delegate uses the internal plan DTO and Phase 13.1 preflight before
  `turn_started` transition audit and before `runChat(...)`.
- Preflight rejection terminalizes both durable turn state and in-memory request
  state, emits a refresh event, and does not start execution.
- No transport, frontend UI, automatic resume, unattended background execution,
  task scheduling, migration, or scheduler-owned actionability was added.

Next boundary decision:

- Checkpoint-resume delegate hardening is the next safe task.
- `ResumeRunCheckpoint(...)` already creates an explicit user-triggered turn
  through `Chat`, so it should inherit the foreground scheduler delegate.
- The next phase should prove this with focused coverage and ensure checkpoint
  resume still does not become automatic replay or mutate source checkpoint
  evidence.

Rejected behavior:

- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No task scheduling execution.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No database migration.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- Review confirmed `runtimeRunSchedulerDelegateUserTurn(...)` is internal and
  only called from `Chat`.
- Review confirmed failed preflight path does not record `turn_started`.
- `git diff --check` passed.

Review conclusion:

- Phase 15.2 accepts the foreground scheduler delegate.
- The next safe task is Phase 16: Checkpoint Resume Scheduler Delegate
  Hardening.
- Phase 16 may add focused backend coverage or narrow internal wiring, but it
  must not add automatic resume, unattended background execution, frontend Run
  management UI, transition-derived actionability, React-owned lifecycle state,
  task execution scheduling, or database migration.

### Phase 16: Checkpoint Resume Scheduler Delegate Hardening

Status: accepted.

Scope:

- Prove explicit checkpoint resume inherits the foreground scheduler delegate
  boundary through the new resumed turn.
- Keep checkpoint resume explicit and user-triggered.
- Preserve source checkpoint evidence.

Implementation notes:

- Added
  `TestRuntimeRunSchedulerDelegateAcceptsExplicitCheckpointResumeTurnOnly`.
- The test first proves a checkpoint plan item is not executable without a
  concrete explicit resumed turn.
- It then simulates the explicit resumed turn created by `Chat`, links
  Run/session/turn, and proves `runtimeRunSchedulerDelegateUserTurn(...)`
  accepts that turn through the same foreground preflight path.
- It links the resumed turn to the checkpoint, records checkpoint resume
  transition audit, and verifies source checkpoint evidence is not
  acknowledged, discarded, or otherwise mutated.

Rejected behavior:

- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No task scheduling execution.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No database migration.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerDelegate|TestRuntimeRunSchedulerPlanCheckpointItemDoesNotMutateEvidence|TestRuntimeRunTransitionWriterRequiresResumedTurnBeforeCheckpointResume|TestRuntimeRunTransitionWriterRecordsCheckpointResumeFromNewTurn"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 16 hardens explicit checkpoint resume under the foreground scheduler
  delegate without adding automatic resume.
- Checkpoint planning remains non-executable until an explicit resumed turn
  exists.
- Source checkpoint evidence remains intact after resumed-turn linking and
  transition audit.
- Runtime truth remains in existing stores and DTO refreshes; events,
  transition history, assistant prose, and React state were not promoted to
  lifecycle or actionability sources.

### Phase 16.1: Scheduler Delegate Acceptance And Next Boundary Gate

Status: accepted as a review gate only.

Scope:

- Review the user-turn and checkpoint-resume foreground delegate slice.
- Decide whether the next safe boundary is task scheduling design,
  transport/read exposure, or more backend delegate coverage.
- Keep this phase free of runtime behavior changes unless a focused contract
  gap is found.

Accepted review:

- User-triggered `Chat` now has an internal foreground scheduler delegate before
  `turn_started` transition audit and before `runChat(...)`.
- Preflight rejection terminalizes durable turn state and in-memory request
  state without starting execution.
- Explicit checkpoint resume remains user-triggered and inherits the foreground
  delegate through its resumed turn.
- Checkpoint planning remains non-executable until a concrete resumed turn
  exists.
- Source checkpoint evidence remains intact after resumed-turn linking and
  transition audit.

Next boundary decision:

- Task scheduling ownership is the next meaningful scheduler boundary.
- Read-only transport exposure is still not required because no frontend
  scheduler UI is accepted and current frontend behavior remains DTO refresh
  only.
- The next phase should be a task scheduling design gate before any executable
  task worker is introduced.

Rejected behavior:

- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No task scheduling execution.
- No HTTP/Wails bridge route, generated binding, adapter, or frontend Run UI.
- No database migration.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- Review confirmed scheduler delegate code is internal to runtime.
- Review confirmed no transport/frontend exposure was added.
- `git diff --check` passed.

Review conclusion:

- Phase 16.1 accepts the foreground scheduler delegate slice for user turns and
  explicit checkpoint resume.
- The next safe task is Phase 17: Task Scheduling Ownership Design Gate.
- Phase 17 must design task scheduling ownership only. It must not implement
  task execution scheduling, automatic resume, unattended background execution,
  frontend Run management UI, transition-derived actionability, React-owned
  lifecycle state, or database migration.

### Phase 17: Task Scheduling Ownership Design Gate

Status: accepted as a design gate only.

Scope:

- Define how future task scheduling relates to Runs, foreground user turns,
  existing agent task stores, cancellation, diagnostics, artifact evidence, and
  permission/MCP actionability.
- Preserve existing agent task recorder/store behavior until a later accepted
  implementation phase.
- Keep task scheduling execution out of this gate.

Accepted ownership model:

- A task must be owned by a parent Run and parent turn before it can become
  executable scheduler work.
- A task may have a child session, worktree, role, model/provider, allowed tool
  scope, capability scope, and parent tool-call link.
- `runtime_agent_tasks`, task messages, task results, and completed tool/task
  artifact refs remain the structured evidence sources.
- Run task transition audit may record ordering and diagnostics after task
  evidence exists, but it cannot become task lifecycle or actionability truth.
- `CancelAgentTask(...)` remains the cancellation entry point until a later
  scheduler-owned cancellation implementation is accepted.
- Startup recovery remains responsible for terminalizing stale queued/running
  task evidence before any scheduler replay.

Required future task scheduler plan rules:

- Task plan items must require parent Run/session/turn ownership evidence.
- Task plan items must remain non-executable if the parent turn is missing,
  terminal in an incompatible state, or not linked to the Run.
- Task plan items must preserve existing task scope controls:
  allowed tools, capability scope, worktree/cwd, role, provider/model, and
  parent tool-call association.
- Task plan execution must not restore stale permission requests, MCP auth
  requests, or MCP elicitation requests as actionable.
- Task completion may contribute produced refs only through completed task/tool
  structured output.
- Partial, interrupted, failed, cancelled, or disconnected task evidence must
  not create artifact evidence unless structured completed output already
  exists.

Out of scope:

- Implementing task scheduler execution.
- Automatic resume.
- Unattended background scheduler queue, poller, or worker loop.
- Frontend Run management UI.
- Scheduler-owned permission/MCP/checkpoint/artifact actionability.
- Database migration.
- Transition-derived lifecycle/actionability.
- Assistant-prose-derived lifecycle/checkpoint/artifact inference.

Implementation entry criteria for Phase 17.1:

- Add read-only task scheduler plan/preflight coverage only, or another focused
  backend contract test if a gap is found.
- Prove task plan items cannot become executable without parent
  Run/session/turn ownership.
- Prove task plan items preserve task scope and do not widen allowed tools,
  capability scope, worktree, cwd, provider/model, or role.
- Prove cancellation/recovery semantics remain owned by existing task stores
  and runtime recovery until a later implementation gate.

Validation:

- Design review only.
- `git diff --check` passed.

Review conclusion:

- Phase 17 accepts task scheduling ownership boundaries, not task scheduler
  execution.
- The next safe task is Phase 17.1: Task Scheduler Plan/Preflight Contract.
- Phase 17.1 must stay read-only or test-only. It must not implement task
  execution scheduling, automatic resume, unattended background execution,
  frontend Run management UI, transition-derived actionability, React-owned
  lifecycle state, or database migration.

### Phase 17.1: Task Scheduler Plan/Preflight Contract

Status: accepted.

Scope:

- Add read-only task scheduler planning/preflight contracts.
- Prove task plan items cannot become executable without parent
  Run/session/turn ownership.
- Prove task plan items preserve existing task scope and do not widen allowed
  tools, capability scope, worktree, cwd, provider/model, or role.

Implementation notes:

- Extended `RuntimeRunSchedulerPlanItem` with `OwnershipVerified` and
  `TaskScope`.
- Added `RuntimeRunSchedulerTaskScope`.
- Extended internal `runtimeRunSchedulerPlan(...)` task item handling to read
  `runtime_agent_tasks`.
- Task plan items now:
  - load the task by id;
  - copy task scope fields into the read-only plan item;
  - run Phase 13.1 preflight against the task's parent session/turn and Run;
  - set `OwnershipVerified=true` only when parent Run/session/turn ownership
    is valid; and
  - remain `CanSchedule=false` with `task_scheduler_not_accepted` even when
    ownership is valid.
- Initialized `agentTasks` in the shared runtime transition writer test
  fixture.

Rejected behavior:

- No task scheduler execution.
- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No frontend Run management UI.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No database migration.
- No transition-derived lifecycle/actionability.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerPlanTaskItem|TestRuntimeRunSchedulerPlanCheckpointItemDoesNotMutateEvidence|TestRuntimeRunSchedulerPlanBuildsReadOnlyExecutableTurnItem"
  -count=1` passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop
  -count=1` passed.
- `git diff --check` passed.

Review conclusion:

- Phase 17.1 adds task scheduler read-only plan/preflight coverage without
  making task execution schedulable.
- Parent Run/session/turn ownership is now represented separately from
  executable task scheduling.
- Task scope is preserved in the plan DTO and is not widened by planning.
- Runtime truth remains in existing task stores, Run stores, activity/projection
  DTOs, and structured task/tool output.

### Phase 17.2: Task Scheduler Plan Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review Phase 17.1 before any task execution scheduling.
- Decide whether the next safe boundary is task cancellation ownership
  hardening or another backend contract test.
- Keep this phase free of runtime behavior changes unless a focused contract
  gap is found.

Accepted review:

- Task scheduler planning is read-only.
- Task items read `runtime_agent_tasks` and copy scope fields into the plan DTO.
- Parent Run/session/turn ownership is represented by `OwnershipVerified`.
- Task items remain non-executable even when ownership is valid because task
  scheduler execution is not accepted.
- Missing parent Run/session/turn ownership keeps task items non-executable
  with the scheduler preflight reason.
- No task state, task scope, artifact evidence, cancellation state, permission
  actionability, MCP actionability, transition-derived lifecycle, or frontend
  state is mutated by planning.

Next boundary decision:

- Task cancellation ownership hardening is the next safe task.
- `CancelAgentTask(...)` already exists and terminalizes task/result/message
  evidence, but the scheduler boundary now needs focused coverage that task
  cancellation stays store-owned and does not become scheduler-owned
  actionability or task execution.

Rejected behavior:

- No task scheduler execution.
- No automatic resume.
- No unattended background scheduler queue, poller, or worker loop.
- No frontend Run management UI.
- No scheduler-owned permission/MCP/checkpoint/artifact actionability.
- No database migration.
- No transition-derived lifecycle/actionability.
- No assistant-prose-derived lifecycle/checkpoint/artifact inference.

Validation:

- Review confirmed task plan items remain `CanSchedule=false`.
- Review confirmed no transport/frontend exposure was added.
- `git diff --check` passed.

Review conclusion:

- Phase 17.2 accepts the read-only task scheduler plan/preflight contract.
- The next safe task is Phase 18: Task Cancellation Ownership Design Gate.
- Phase 18 must define cancellation ownership before any scheduler-owned task
  execution or cancellation implementation. It must not implement task
  scheduler execution, automatic resume, unattended background execution,
  frontend Run management UI, transition-derived actionability, React-owned
  lifecycle state, or database migration.

### Phase 18: Task Cancellation Ownership Design Gate

Status: accepted as a design gate only.

Scope:

- Define how `CancelAgentTask(...)`, task result/message evidence, Run task
  transition audit, and future scheduler task items interact.
- Preserve current task cancellation behavior until a later accepted
  implementation phase.
- Keep scheduler-owned task cancellation execution out of this gate.

Accepted cancellation ownership:

- `CancelAgentTask(...)` remains the current cancellation entry point.
- Cancellation truth is the terminal task row plus task result/message evidence.
- When a child session exists, runtime may request child-session cancellation
  through the backend, but the durable task row/result/message evidence remains
  the source of truth.
- Run task transition audit may record cancellation ordering after task
  evidence is terminal, but transition rows cannot decide task lifecycle or
  actionability.
- Future scheduler task items may describe cancellation scope and ownership
  checks, but they must not become cancellation actionability or execution
  authority until a later gate.

Required future cancellation rules:

- A task cancellation request must load the current task from
  `runtime_agent_tasks`.
- If the task is already final, cancellation must not rewrite final status,
  artifact refs, result evidence, or scheduler plan state.
- If the task is active, cancellation must terminalize task row, task result,
  and parent-to-child control message evidence before any transition audit is
  considered useful.
- Cancellation must preserve parent Run/session/turn/task ownership links.
- Cancellation must not restore stale permission requests, MCP auth requests,
  MCP elicitation requests, or tool calls as actionable.
- Cancellation must not create produced artifact refs unless completed
  structured task/tool output already exists.

Out of scope:

- Implementing scheduler-owned task cancellation.
- Implementing task scheduler execution.
- Automatic resume.
- Unattended background scheduler queue, poller, or worker loop.
- Frontend Run management UI.
- Scheduler-owned permission/MCP/checkpoint/artifact actionability.
- Database migration.
- Transition-derived lifecycle/actionability.
- Assistant-prose-derived lifecycle/checkpoint/artifact inference.

Implementation entry criteria for Phase 18.1:

- Add focused backend coverage for active task cancellation ownership.
- Prove `CancelAgentTask(...)` terminalizes task/result/message evidence and
  preserves parent Run/session/turn/task links.
- Prove cancelling an already-final task does not rewrite final evidence.
- Prove cancellation does not make scheduler task plan items executable and
  does not mutate task scope.

Validation:

- Design review only.
- `git diff --check` passed.

Review conclusion:

- Phase 18 accepts task cancellation ownership boundaries, not scheduler-owned
  task cancellation.
- The next safe task is Phase 18.1: Task Cancellation Ownership Contract
  Coverage.
- Phase 18.1 should add focused tests only unless a narrow internal helper is
  required. It must not implement task scheduler execution, automatic resume,
  unattended background execution, frontend Run management UI,
  transition-derived actionability, React-owned lifecycle state, or database
  migration.

### Phase 18.1: Task Cancellation Ownership Contract Coverage

Status: implemented.

Implementation:

- Added backend coverage proving active `CancelAgentTask(...)` terminalizes the
  task row and durable task result/message evidence.
- Added ownership assertions for parent Run/session/turn/tool/task links across
  the cancellation path.
- Added coverage proving cancelling an already-final task leaves final task
  status, finished time, artifact refs, and result evidence unchanged while
  recording only a rejected control message.
- Added scheduler plan coverage proving cancelled task plan items remain
  read-only, non-executable, ownership-checked, and scope-preserving.
- Fixed cancellation detail propagation so the response/result/message evidence
  keeps the cancellation reason without adding a task-table column or
  migration.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeCancelAgentTask|TestRuntimeRunSchedulerPlanTaskItem" -count=1`
  passed.

Review conclusion:

- Phase 18.1 accepts cancellation ownership contract coverage only.
- No task scheduler execution, automatic resume, unattended scheduler queue,
  frontend Run management UI, transition-derived actionability,
  React-owned lifecycle state, or database migration was added.
- `CancelAgentTask(...)` remains the cancellation entry point and scheduler
  task items remain read-only planning evidence.
- The next safe task is Phase 18.2: Task Cancellation Ownership Acceptance
  Gate.

### Phase 18.2: Task Cancellation Ownership Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review Phase 18.1 before moving toward any scheduler-owned task execution.
- Confirm task cancellation ownership is stable and remains backend/runtime
  owned.
- Decide the next safe boundary without implementing task scheduler execution,
  automatic resume, unattended background execution, frontend Run management UI,
  transition-derived actionability, React-owned lifecycle state, or database
  migration.

Accepted review:

- `CancelAgentTask(...)` remains the cancellation entry point.
- Active cancellation terminalizes durable task row/result/message evidence and
  preserves parent Run/session/turn/tool/task links.
- Already-final cancellation records only rejected control evidence and does
  not rewrite final task/result/artifact evidence.
- Scheduler task plan items may carry cancellation scope and task scope
  evidence, but remain read-only, non-executable, and non-authoritative for
  cancellation actionability.
- Cancellation events remain refresh triggers only. Event payloads do not
  hydrate lifecycle, artifact evidence, permission/MCP actionability, or Run
  status.

Next boundary decision:

- The next safe task is Phase 19: Task Scheduler Execution Design Gate.
- Phase 19 must define the minimum accepted task scheduler execution boundary
  before implementation. It must explicitly address parent Run/session/turn/task
  ownership, task scope enforcement, cancellation ownership, evidence ordering,
  and frontend/backend transport exposure.
- Phase 19 must not implement a task scheduler worker, automatic resume,
  unattended queue/poller, frontend Run management UI, database migration, or
  transition/event/prose/React-derived lifecycle/actionability.

Validation:

- Review confirmed Phase 18.1 changed only cancellation detail propagation,
  backend tests, and docs.
- Review confirmed no frontend adapter, Run UI, migration, task scheduler
  execution, automatic resume, unattended worker, or source-of-truth promotion
  was added.
- `git diff --check` passed.

Review conclusion:

- Phase 18.2 accepts task cancellation ownership as stable enough to inform the
  next design gate.
- Task execution remains unimplemented for task plan items until a later
  accepted implementation phase.
- The next safe task is Phase 19: Task Scheduler Execution Design Gate.

### Phase 19: Task Scheduler Execution Design Gate

Status: accepted as a design gate only.

Purpose:

- Define the minimum safe boundary for scheduler-owned task execution before
  implementation.
- Align with the Claude Code lesson that tasks/subagents must have explicit
  tool/session/task state, stop/output controls, and durable transcripts or
  event readers instead of UI-derived lifecycle.
- Preserve Agent Builder's runtime-owned truth model: Go stores and DTO reads
  remain authoritative; React state, event payloads, transition history, and
  assistant prose do not become task lifecycle or actionability sources.

Existing implementation inventory:

- Foreground user turns already pass through `runtimeRunSchedulerDelegateUserTurn(...)`
  before `runChat(...)`.
- Runtime task records already come from the existing AgentTool/coordinator
  path through `AgentTaskStarted/Progress/Completed/Failed`.
- Task stop/output/list/get/message tools already call runtime DTO/action
  methods; `CancelAgentTask(...)` remains the cancellation entry point.
- `runtimeRunSchedulerPlan(...)` can describe `task_turn` items and copy task
  scope evidence, but task items remain non-executable with
  `task_scheduler_not_accepted`.

Accepted design:

- The first scheduler-owned task execution boundary must be foreground and
  explicitly user/model/tool-triggered. It must not be an unattended worker,
  queue, poller, or automatic resume path.
- A task execution delegate must load the current `runtime_agent_tasks` row,
  verify parent Run/session/turn ownership, verify the parent turn is linked to
  the Run, and reject terminal or unowned tasks before any execution side
  effect.
- Task scope is an execution constraint, not UI decoration. Allowed tools,
  capability scope, cwd/worktree, role, provider/model, parent tool-call id,
  and child session id must be preserved and enforced by runtime policy before
  child tool calls run.
- Cancellation ownership stays with `CancelAgentTask(...)`. A task scheduler
  delegate may observe cancellation state and terminal evidence, but must not
  invent a second cancellation source of truth.
- Artifact evidence can be produced only from completed structured task/tool
  output. Unfinished, partial, denied, disconnected, or cancelled task execution
  must not create produced refs.
- Event payloads may choose refresh targets only. After any task event, the
  frontend must refresh runtime DTOs such as task detail, turn activity,
  session activity window, Run projection, or scheduler plan; it must not merge
  payloads into lifecycle, diagnostics, artifact evidence, permission/MCP
  actionability, or Run status.
- The scheduler plan source must stay explicit about whether it is read-only or
  starts a worker. Any future executable task plan item must flip that contract
  only in the same implementation phase that proves side effects and fallback
  parity.

Required implementation entry criteria for Phase 19.1:

- Add focused backend tests for a task execution delegate contract before
  enabling task plan executability.
- Prove unowned, terminal, missing, cancelled, or stale task rows are rejected
  without execution side effects.
- Prove accepted task execution preserves task scope and parent
  Run/session/turn/tool ownership.
- Prove cancellation during or before execution terminalizes through
  `CancelAgentTask(...)` or recorder terminal evidence, not through transition
  history, event payloads, assistant prose, or React state.
- Prove completed structured task output is the only path that can contribute
  produced artifact refs.
- Prove DTO refresh/fallback behavior remains compatible with full
  `SessionActivity` parity.

Rejected behavior:

- No task scheduler worker, queue, poller, or background daemon.
- No automatic resume of tasks, child sessions, stale permission gates, stale
  MCP auth requests, or stale MCP elicitation requests.
- No frontend Run management UI.
- No database migration.
- No transition/event/prose/React-derived lifecycle, artifact evidence,
  permission/MCP actionability, or Run status.
- No provider credentials, browser auth state, hosted MCP secrets, or task
  auth state in fixtures, logs, docs, screenshots, or React state.

Validation:

- Design review only.
- Reviewed the existing runtime scheduler delegate, task scheduler plan, task
  tools, AgentTask recorder, and coordinator subagent path.
- Read-only Claude Code comparison confirmed the reference architecture exposes
  explicit Agent/TaskCreate/TaskGet/TaskStop/TaskOutput style boundaries and
  separates subagent state/transcripts from UI-only state.
- `git diff --check` passed.

Review conclusion:

- Phase 19 accepts the task scheduler execution design boundary, not an
  implementation.
- The next safe task is Phase 19.1: Task Scheduler Execution Delegate Contract
  Coverage.
- Phase 19.1 must start with backend contract tests and may add only the
  narrowest internal helper needed to express the delegate contract. It must
  not add a worker, queue, automatic resume, frontend Run UI, migration, or
  source-of-truth promotion.

### Phase 19.1: Task Scheduler Execution Delegate Contract Coverage

Status: implemented.

Implementation:

- Added internal `runtimeRunSchedulerDelegateTaskTurn(...)` as a contract
  helper for task execution preflight. It reads the scheduler plan and task
  row, but does not start execution.
- The helper rejects missing task items, unowned task rows, terminal task rows,
  cancelled/interrupted task rows, and owned active task rows while task
  scheduler execution remains unaccepted.
- Added backend tests proving rejected task delegate candidates do not write
  runtime events, run transitions, task messages, task results, or artifact
  evidence.
- Added backend tests proving owned active task candidates preserve parent
  Run/session/turn/tool ownership and task scope evidence while remaining
  non-executable with `task_scheduler_not_accepted`.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerDelegate(TaskTurn|Allows|Rejects|Accepts)|TestRuntimeRunSchedulerPlanTaskItem"
  -count=1` passed.

Review conclusion:

- Phase 19.1 adds delegate contract coverage only.
- No task scheduler worker, queue, poller, automatic resume, frontend Run UI,
  database migration, or transition/event/prose/React-derived lifecycle or
  actionability source was added.
- Task plan items remain non-executable until a later accepted implementation
  phase changes both the plan contract and delegate side-effect coverage.
- The next safe task is Phase 19.2: Task Scheduler Execution Delegate
  Acceptance Gate.

### Phase 19.2: Task Scheduler Execution Delegate Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review Phase 19.1 before any task plan item becomes executable.
- Confirm the new delegate helper is rejection-only and side-effect free.
- Decide the next safe implementation boundary without adding a worker, queue,
  automatic resume, frontend Run UI, database migration, or
  transition/event/prose/React-derived lifecycle/actionability.

Accepted review:

- `runtimeRunSchedulerDelegateTaskTurn(...)` is accepted as the internal
  preflight/delegate contract for future task execution.
- The helper loads runtime plan/task evidence and rejects missing, unowned,
  terminal, cancelled, interrupted, and currently non-accepted owned active
  tasks without writing runtime events, run transitions, task messages, task
  results, or artifact evidence.
- Owned active task candidates preserve task scope and parent
  Run/session/turn/tool ownership evidence in the plan DTO.
- Task plan items remain non-executable with `task_scheduler_not_accepted`.
- Cancellation remains owned by `CancelAgentTask(...)` or recorder terminal
  evidence; events remain refresh triggers only.

Next boundary decision:

- The next safe task is Phase 20: Foreground Task Plan Executability
  Implementation Gate.
- Phase 20 may only consider a foreground, explicit, model/tool-triggered path
  that flips owned active task plan items from rejection-only to executable
  after proving side effects, task scope enforcement, cancellation ordering,
  artifact evidence, and `SessionActivity` parity.
- Phase 20 must not add a background worker, queue, poller, automatic resume,
  frontend Run management UI, database migration, or event/prose/React-derived
  source of truth.

Validation:

- Review confirmed Phase 19.1 added one internal helper, focused backend tests,
  and docs.
- Review confirmed no task execution path was enabled and no frontend adapter,
  Run UI, migration, worker, queue, automatic resume, or source-of-truth
  promotion was added.
- `git diff --check` passed.

Review conclusion:

- Phase 19.2 accepts the delegate contract as stable enough for a later
  foreground executability implementation gate.
- Task scheduler execution remains disabled until Phase 20 explicitly changes
  the plan/delegate contract with tests.
- The next safe task is Phase 20: Foreground Task Plan Executability
  Implementation Gate.

### Phase 20: Foreground Task Plan Executability Implementation Gate

Status: implemented.

Implementation:

- Updated `runtimeRunSchedulerTaskPlanItem(...)` so an owned, active task with
  verified parent Run/session/turn ownership is now `CanSchedule=true`.
- Added explicit `terminal_task` task-plan rejection for final task rows after
  ownership verification.
- Updated `runtimeRunSchedulerDelegateTaskTurn(...)` tests so owned active task
  candidates are accepted by the delegate while still producing no runtime
  events, run transitions, task messages, task results, or artifact evidence.
- Kept missing, unowned, completed, cancelled, and interrupted task candidates
  rejected without side effects.
- Updated scheduler plan tests so cancelled task rows remain non-executable and
  owned active task rows preserve scope while becoming foreground-schedulable.

Accepted behavior:

- This phase flips plan/delegate foreground schedulability only. It does not
  add a worker, queue, poller, automatic resume, frontend Run UI, transport
  exposure, database migration, or new source of truth.
- A successful task delegate preflight means the existing foreground caller may
  proceed; it does not itself start child execution or infer lifecycle.
- Task cancellation remains owned by `CancelAgentTask(...)` or recorder
  terminal evidence.
- Completed structured task/tool output remains the only produced-ref path.
- Runtime DTO reads remain authoritative; event payloads remain refresh
  triggers only.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerDelegate(TaskTurn|Allows|Rejects|Accepts)|TestRuntimeRunSchedulerPlanTaskItem"
  -count=1` passed.

Review conclusion:

- Phase 20 implements the minimum foreground task-plan executability flip.
- No background scheduler, automatic resume, frontend Run UI, migration,
  transition/event/prose/React-derived lifecycle/actionability, or artifact
  inference was added.
- The next safe task is Phase 20.1: Foreground Task Executability Parity And
  Recorder Evidence Coverage.

### Phase 20.1: Foreground Task Executability Parity And Recorder Evidence Coverage

Status: implemented.

Implementation:

- Added backend coverage proving a foreground-schedulable owned active task
  delegate candidate creates no refs before recorder evidence exists.
- Added recorder completion coverage proving completed task output creates
  task artifact refs through structured recorder evidence.
- Added full `SessionActivity` plus cursor-window parity coverage for task
  completion and task artifact events after recorder completion.
- Added terminal precedence coverage proving the completed task plan becomes
  `terminal_task` and non-executable after recorder terminal evidence.

Validation:

- `go test ./internal/runtime -run
  "TestRuntimeRunSchedulerDelegateTaskTurn(ActivityParity|Allows|Rejects)|TestRuntimeRunSchedulerPlanTaskItem"
  -count=1` passed.

Review conclusion:

- Phase 20.1 strengthens evidence/parity coverage for the Phase 20 foreground
  schedulability flip.
- Recorder completed output remains the produced-ref path; delegate preflight
  alone does not write refs, task messages/results, runtime events, or
  transitions.
- Full `SessionActivity` remains the fallback and parity oracle; cursor-window
  events remain additive refresh evidence.
- The next safe task is Phase 20.2: Foreground Task Executability Acceptance
  Gate.

### Phase 20.2: Foreground Task Executability Acceptance Gate

Status: accepted as a review gate only.

Scope:

- Review Phase 20 and Phase 20.1 before exposing any executable task plan path
  through transport or UI.
- Confirm foreground task schedulability is internal, evidence-backed, and
  compatible with full `SessionActivity` fallback/parity.
- Decide the next safe boundary without adding a background worker, queue,
  poller, automatic resume, frontend Run UI, database migration, or
  event/prose/React-derived source of truth.

Accepted review:

- Owned active task plan items with verified parent Run/session/turn ownership
  may be internally foreground-schedulable.
- `runtimeRunSchedulerDelegateTaskTurn(...)` may accept that candidate, but it
  does not start execution or write lifecycle evidence by itself.
- Missing, unowned, completed, cancelled, and interrupted task rows remain
  non-executable.
- Recorder terminal evidence remains authoritative. Completed recorder output
  is the covered task artifact ref path, and completed task rows become
  `terminal_task` again.
- Full `SessionActivity` remains the fallback and parity oracle; cursor-window
  events remain additive refresh evidence.

Next boundary decision:

- The next safe task is Phase 21: Task Scheduler Transport And UI Exposure
  Design Gate.
- Phase 21 must decide if, when, and how any executable task plan/delegate
  signal is exposed through HTTP/Wails/frontend adapters. It must preserve DTO
  refresh source-of-truth boundaries and must not introduce Run management UI
  without a separate accepted UI phase.

Validation:

- Review confirmed Phase 20/20.1 changed internal scheduler plan/delegate
  behavior and backend tests only.
- Review confirmed no worker, queue, poller, automatic resume, frontend Run UI,
  database migration, or event/prose/React-derived source of truth was added.
- `git diff --check` passed.

Review conclusion:

- Phase 20.2 accepts internal foreground task schedulability as stable.
- Transport and UI exposure remain unaccepted until a separate design gate.
- The next safe task is Phase 21: Task Scheduler Transport And UI Exposure
  Design Gate.

### Phase 21: Task Scheduler Transport And UI Exposure Design Gate

Status: accepted as a design gate only.

Purpose:

- Decide whether the internal foreground task schedulability signal should be
  exposed through HTTP, Wails, generated bindings, runtime adapters, or React.
- Preserve the current runtime source-of-truth boundary before any frontend
  implementation.

Accepted transport design:

- Do not expose a task scheduler execute action yet.
- Do not expose `runtimeRunSchedulerDelegateTaskTurn(...)` through HTTP or
  Wails yet. It remains an internal backend preflight/delegate contract.
- The next transport-safe step, if needed, is a read-only scheduler plan DTO
  endpoint/method that returns `RuntimeRunSchedulerPlanResponse` for a Run and
  candidate turn/checkpoint/task id.
- A read-only transport plan response may include `CanSchedule`,
  `OwnershipVerified`, `TaskScope`, `CancellationScope`, `DiagnosticsRoute`,
  and `RefreshTargets`, but it must not start execution, cancel tasks, mutate
  task rows, write refs, write task messages/results, or record transitions.
- Event payloads may select refresh targets only. Frontend must refresh
  runtime DTOs such as `RunProjection`, `TurnActivity`,
  `SessionActivityCursorWindow`, full `SessionActivity`, task detail, task
  result, and read-only scheduler plan. It must not merge event payloads into
  lifecycle, diagnostics, artifact evidence, permission/MCP actionability, or
  Run status.
- Any future executable task action requires a separate implementation gate
  after read-only transport coverage proves HTTP/Wails/browser parity.

Frontend/UI decision:

- No frontend Run management UI is accepted by this gate.
- No executable task button/control is accepted by this gate.
- If the frontend consumes read-only scheduler plan later, it may use it only
  to decide refresh affordances/disabled states that are reconciled against
  runtime DTOs. React state must not become task lifecycle or actionability
  truth.

Required implementation entry criteria for Phase 21.1:

- Add transport-neutral RuntimeService/HTTP/dev/Wails read-only scheduler plan
  exposure, or explicitly accept that no transport exposure is currently
  needed.
- If exposed, add contract tests for:
  - user-turn, checkpoint, and task candidate request shapes;
  - browser/dev HTTP and Wails bridge parity where generated bindings exist;
  - no mutation of task rows, refs, messages, results, transitions, permission
    state, MCP auth/elicitation state, or runtime events;
  - event-triggered refresh choosing DTO reads rather than payload merging.
- Keep full `SessionActivity` as fallback and parity oracle.

Rejected behavior:

- No task scheduler execute route, adapter method, generated binding, or React
  action.
- No background worker, queue, poller, or automatic resume.
- No frontend Run management UI.
- No database migration.
- No stale permission/MCP actionability restore.
- No event/prose/React-derived lifecycle, artifact evidence, permission/MCP
  actionability, or Run status.

Validation:

- Design review only.
- Reviewed existing frontend/backend integration notes, Wails bridge route
  patterns, HTTP route history, and current internal scheduler plan/delegate
  state.
- `git diff --check` passed.

Review conclusion:

- Phase 21 accepts read-only scheduler plan transport as the next safe exposure
  boundary, not executable task scheduling.
- The next safe task is Phase 21.1: Read-only Scheduler Plan Transport
  Contract.
- Phase 21.1 must not add execute/cancel actions, frontend Run management UI,
  worker/queue/auto-resume behavior, database migration, or source-of-truth
  promotion.

## 2026-06-10: Phase 21.1 Read-only Scheduler Plan Transport Contract

Phase 21.1 exposes the already accepted scheduler plan DTO as a read-only
transport contract.

Implemented:

- Added `RuntimeService.RunSchedulerPlan(ctx, RuntimeRunSchedulerPlanRequest)`
  as the transport-neutral read-only entry point.
- Added `GET /v1/run-scheduler-plan` with `run_id`, `session_id`, `mode`,
  `turn_id`, `checkpoint_id`, `task_id`, `cursor`, and `limit` query mapping.
- Added dev module support for `/v1/run-scheduler-plan` using the same request
  mapping.
- Added Wails bridge aliases for scheduler plan DTOs and
  `RuntimeBridge.RunSchedulerPlan(...)`.
- Kept `runtimeRunSchedulerDelegateTaskTurn(...)` backend-internal and did not
  add execute/cancel task transport actions.

Contract tests:

- HTTP route coverage proves request mapping for run/session/mode/turn/
  checkpoint/task/cursor/limit and validates `source.readOnly=true`,
  `source.startsWorker=false`, and `source.sessionActivityParity=true`.
- Dev module coverage proves browser/dev transport can read scheduler plans
  without special generated bindings.
- Wails bridge coverage proves the request is forwarded unchanged and preserves
  the read-only source contract.
- Side-effect assertions verify scheduler plan transport does not call `Chat`
  and does not call `CancelAgentTask`.

Validation:

- `go test ./internal/runtime -run "TestRuntimeHTTPServerRoutesRunSchedulerPlanToRuntimeService|TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy" -count=1`
  passed.
- `go test ./desktop -run "TestRuntimeBridgeNarrowActivityUsesRuntimeService" -count=1`
  passed.

Review conclusion:

- Phase 21.1 is a read-only transport contract only.
- No background worker, queue, poller, automatic resume, database migration,
  frontend Run management UI, scheduler execute action, or scheduler cancel
  action was introduced.
- Runtime DTO reads remain the source of truth. Event payloads may only choose
  refresh targets; they still cannot hydrate lifecycle, artifact evidence,
  permission/MCP actionability, or Run status.

## 2026-06-10: Phase 21.2 Read-only Scheduler Plan Transport Acceptance

Phase 21.2 accepts the Phase 21.1 scheduler plan transport as stable read-only
planning evidence.

Acceptance review:

- Reviewed the RuntimeService, HTTP, dev module, and Wails bridge changes.
- Confirmed the only new transport surface is `RunSchedulerPlan` /
  `GET /v1/run-scheduler-plan`.
- Confirmed no scheduler execute/cancel route, adapter method, React action,
  frontend Run management UI, background worker, queue, poller, automatic
  resume, or database migration was added.
- Confirmed `runtimeRunSchedulerDelegateTaskTurn(...)` remains
  backend-internal.
- Confirmed scheduler plan reads carry explicit source metadata:
  `readOnly=true`, `startsWorker=false`, and `sessionActivityParity=true`.
- Confirmed event payloads still select refresh targets only and do not become
  lifecycle, artifact, permission/MCP actionability, or Run status truth.

Validation:

- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- Frontend/browser/Wails callers may request scheduler plan DTOs to display or
  reason about disabled/enabled planning affordances.
- Callers must still refresh `SessionActivity`, `RunProjection`, persisted Run
  detail, task detail/result, permission/MCP DTOs, or artifact/ref DTOs for
  authoritative state.
- The scheduler plan transport does not authorize task execution.

Next safe boundary:

- Phase 22 should be a design gate for an explicit foreground task execute
  action, if that product path is still desired.
- Phase 22 must decide ownership, idempotency, cancellation ordering, artifact
  evidence, permission/MCP semantics, and event refresh behavior before any
  implementation.
- Phase 22 should still reject background workers, automatic resume, database
  migrations, stale actionability restoration, and frontend-owned lifecycle
  truth unless separately accepted.

## 2026-06-10: Phase 22 Explicit Foreground Task Execute Action Design Gate

Phase 22 defines the first acceptable shape for task execution transport. It is
a design gate only; no execute route, bridge method, frontend button, worker,
queue, or scheduler implementation is added by this phase.

Accepted execution shape:

- A future execute action may be explicit, foreground, and user-triggered only.
- The action may target one owned active task candidate that already appears in
  `RunSchedulerPlan` with `canSchedule=true` and `ownershipVerified=true`.
- The action must run through backend runtime code, not React state or event
  payload data.
- The backend must re-read task row, parent Run/session/turn ownership,
  scheduler plan/preflight, effective scope, and current cancellation state at
  execution time.
- The action must be idempotent by task id. Duplicate execute requests for the
  same active task must not create duplicate child turns, duplicate task
  messages/results, duplicate refs, or duplicate lifecycle events.
- Cancellation remains owned by `CancelAgentTask(...)` and recorder terminal
  evidence. Execute must observe cancellation before start and during
  foreground execution.
- Completed structured task/tool output is the only produced-ref source.
  Partial, unfinished, disconnected, or cancelled task work must not create
  artifact evidence.
- Permission and MCP auth/elicitation decisions must come from current runtime
  DTO/state only. Execute must not resurrect stale permission gates or stale
  MCP auth/elicitation requests after restart.
- Runtime events emitted by execution may only trigger DTO refreshes:
  `RunSchedulerPlan`, task detail/result, `TurnActivity`,
  `SessionActivityCursorWindow`, full `SessionActivity`, `RunProjection`, Run
  detail, refs, permissions, and MCP requests.

Rejected execution shape:

- No background worker, queue, poller, daemon, unattended scheduler, or batch
  task execution.
- No automatic resume after restart.
- No stale running/waiting tool recovery.
- No stale actionable permission gate recovery.
- No stale actionable MCP auth/elicitation recovery.
- No database migration or new persisted Run state machine field.
- No frontend Run management UI and no React-owned task lifecycle state.
- No event-payload, transition-history, assistant-prose, or React-state merge
  into lifecycle, artifact evidence, permission/MCP actionability, or Run
  status.

Required implementation entry criteria for Phase 22.1:

- Define a backend-only execute contract that calls
  `runtimeRunSchedulerDelegateTaskTurn(...)` or equivalent revalidation before
  starting work.
- Define the child turn/session linkage and task result/message ownership that
  prevents duplicate evidence.
- Define idempotency behavior for already-running, already-completed,
  cancelled, interrupted, missing, and unowned task rows.
- Define permission/MCP behavior for foreground execution without restoring
  stale actionability after restart.
- Define artifact evidence rules proving only completed scheduler output can
  produce refs.
- Define transport response source metadata that explicitly says whether the
  response is an action result, whether a worker was started, and which DTOs
  callers must refresh.
- Add tests before any frontend exposure.

Validation:

- Design review only.
- Reviewed Phase 21.1/21.2 scheduler plan transport, internal scheduler
  delegate, task cancellation ownership, and current source-of-truth rules.
- `git diff --check` passed.

Review conclusion:

- Phase 22 accepts the concept of a future explicit foreground task execute
  action, but not its implementation.
- The next safe task is Phase 22.1: Foreground Task Execute Backend Contract.
- Phase 22.1 must still avoid background scheduling, automatic resume,
  database migrations, frontend Run management UI, stale actionability
  resurrection, and event/prose/React-derived truth.

## 2026-06-10: Phase 22.1 Foreground Task Execute Backend Contract

Phase 22.1 adds the backend-only execute contract shape. It does not start task
execution yet and does not expose HTTP, Wails, generated bindings, or frontend
controls.

Implemented:

- Added `RuntimeRunSchedulerExecuteTaskRequest`,
  `RuntimeRunSchedulerExecuteTaskResponse`, and
  `RuntimeRunSchedulerExecuteTaskSource`.
- Added internal `runtimeRunSchedulerExecuteTask(...)`.
- The internal contract requires `runId` and `taskId`, re-reads the durable Run
  and task row, and revalidates through
  `runtimeRunSchedulerDelegateTaskTurn(...)`.
- Accepted owned active candidates return `accepted=true`,
  `executionStarted=false`, `startsWorker=false`, `backendOnly=true`, and
  refresh targets.
- Rejected candidates return the scheduler/delegate rejection reason and still
  report `startsWorker=false`.

Contract tests:

- `TestRuntimeRunSchedulerExecuteTaskAcceptsOwnedActiveCandidateWithoutStartingWorker`
  proves owned active candidates are accepted as backend contract evidence
  without starting a worker or writing events/messages/results/refs.
- `TestRuntimeRunSchedulerExecuteTaskIsIdempotentBeforeExecutionImplementation`
  proves duplicate contract calls for the same task do not mutate task rows or
  duplicate evidence.
- `TestRuntimeRunSchedulerExecuteTaskRejectsInvalidCandidatesWithoutSideEffects`
  proves unowned and terminal tasks are rejected without side effects.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask|TestRuntimeRunSchedulerDelegateTaskTurn" -count=1`
  passed.

Review conclusion:

- Phase 22.1 establishes the backend execute action contract, not execution.
- No worker, queue, poller, automatic resume, database migration, HTTP/Wails
  route, generated binding, frontend Run UI, stale actionability recovery, or
  event/prose/React-derived truth was added.
- The next safe task is Phase 22.2 acceptance, or Phase 22.3 to implement the
  foreground execution body behind the backend-only contract after acceptance.

## 2026-06-10: Phase 22.2 Foreground Task Execute Backend Contract Acceptance

Phase 22.2 accepts the Phase 22.1 backend-only execute contract as the stable
entry point for a future foreground task execution implementation.

Acceptance review:

- Confirmed `runtimeRunSchedulerExecuteTask(...)` is internal runtime code
  only.
- Confirmed no `RuntimeService` method, HTTP route, dev module route, Wails
  bridge method, generated binding, frontend adapter method, or UI control was
  added.
- Confirmed accepted responses report `executionStarted=false`,
  `startsWorker=false`, `backendOnly=true`, and `idempotentByTaskId=true`.
- Confirmed the contract revalidates via
  `runtimeRunSchedulerDelegateTaskTurn(...)` and re-reads durable Run/task
  evidence before accepting a candidate.
- Confirmed duplicate calls do not mutate task rows or duplicate task
  messages/results, refs, events, transitions, or lifecycle evidence.
- Confirmed rejected unowned/terminal candidates do not write evidence or
  resurrect stale actionability.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask|TestRuntimeRunSchedulerDelegateTaskTurn" -count=1`
  passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- Future foreground execution must be implemented behind
  `runtimeRunSchedulerExecuteTask(...)` or an equivalent backend-only contract
  that preserves the same revalidation and idempotency semantics.
- The first execution implementation may flip `executionStarted=true` only
  after tests prove no duplicate turns/messages/results/refs/events,
  cancellation ordering, permission/MCP behavior, and completed-output-only
  artifact evidence.
- Transport/frontend exposure remains unaccepted until the foreground execution
  body is implemented and accepted.

Next safe boundary:

- Phase 22.3 should implement the foreground execution body behind the internal
  contract, still without HTTP/Wails/client exposure.
- Phase 22.3 must not add background scheduling, automatic resume, database
  migrations, stale actionability recovery, frontend Run UI, or
  event/prose/React-derived truth.

## 2026-06-10: Phase 22.3 Foreground Task Execute Body Behind Backend Contract

Phase 22.3 implements the first internal foreground execution body behind
`runtimeRunSchedulerExecuteTask(...)`. This is still runtime-only and does not
expose HTTP, Wails, generated bindings, frontend controls, or a background
scheduler.

Implemented:

- `runtimeRunSchedulerExecuteTask(...)` now distinguishes accepted states:
  - `queued` task: revalidated and moved to `running`;
  - `running` task: accepted as an idempotent duplicate without writing new
    evidence.
- Queued task start writes one processed parent-to-child instruction message.
- Queued task start records one `task_started` lifecycle event/audit entry.
- Queued task start records one run transition with source `task_started`.
- The response returns `executionStarted=true` only for the first queued task
  start. Duplicate/running calls return `executionStarted=false`.

Still not implemented:

- No child agent execution body is started.
- No result, completion, failed, cancelled, or artifact evidence is produced by
  the start action.
- No HTTP/dev/Wails/client transport or frontend UI is exposed.
- No worker, queue, poller, automatic resume, database migration, stale
  actionability recovery, or event/prose/React-derived source of truth is
  introduced.

Contract tests:

- Running task execute calls remain idempotent and side-effect free.
- Queued task execute calls start the task once, write exactly one instruction
  message, one `task_started` event, and one task-start transition, and create
  no result or artifact refs.
- Duplicate queued-task execute calls become `already_running` without
  duplicate messages/events/transitions.
- Unowned and terminal tasks remain rejected without side effects.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask" -count=1`
  passed.

Review conclusion:

- Phase 22.3 implements a minimal foreground start body, not a complete task
  runner.
- Artifact evidence remains completed-output-only.
- Transport/frontend exposure remains unaccepted until a later gate accepts
  actual child-agent execution and its permission/MCP/cancellation behavior.

## 2026-06-10: Phase 22.4 Foreground Task Start Body Acceptance

Phase 22.4 accepts the internal foreground task start body from Phase 22.3.

Acceptance review:

- Confirmed `runtimeRunSchedulerExecuteTask(...)` remains internal runtime code.
- Confirmed no `RuntimeService`, HTTP/dev module, Wails bridge, generated
  binding, frontend adapter, or UI surface was added.
- Confirmed queued task starts write only start evidence: task status
  `running`, one instruction message, one `task_started` event/audit entry,
  and one task-start transition.
- Confirmed duplicate/running calls return `already_running` and do not
  duplicate messages, events, transitions, task results, refs, or lifecycle
  evidence.
- Confirmed unowned and terminal tasks remain rejected without side effects.
- Confirmed no completion/result/artifact evidence is produced by task start.

Validation:

- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- Internal foreground task start is accepted as the first execution body behind
  the backend-only execute contract.
- Artifact refs remain completion-only and must come from completed structured
  scheduler/recorder output.
- A later child-agent runner must preserve this idempotent start behavior and
  prove cancellation, permission/MCP, failure, completion, and artifact
  semantics before any transport/frontend exposure.

Next safe boundary:

- Phase 22.5 should design the actual child-agent foreground runner behind the
  accepted internal start body.
- Phase 22.5 must decide how to call agent/coordinator execution without
  background scheduling, automatic resume, stale actionability restoration, or
  frontend-owned state.

## 2026-06-10: Phase 22.5 Child-agent Foreground Runner Design Gate

Phase 22.5 designs the runtime-to-agent runner boundary behind the accepted
internal task start body. It is a design gate only; no child-agent runner,
transport exposure, frontend control, background worker, automatic resume,
database migration, or stale actionability recovery is added.

Reviewed implementation points:

- `agent.coordinator.runSubAgent(...)` already owns child task execution for
  current tool-driven subagents. It creates the child task session, registers
  the child `SessionAgent` for follow-up/cancel routing, evaluates scheduler
  policy, runs the child agent non-interactively, propagates cost, and writes
  task started/progress/completed/failed evidence through `AgentTaskRecorder`.
- `Coordinator.SendToSession(...)` and `Coordinator.Cancel(...)` route active
  child sessions through `childAgents`; this routing is runtime process state
  and must not be treated as durable resume state after restart.
- `runtimeRunSchedulerExecuteTask(...)` currently provides only backend-only
  foreground start evidence: it revalidates ownership through
  `runtimeRunSchedulerDelegateTaskTurn(...)`, moves queued tasks to `running`,
  writes one instruction message, one `task_started` event, and one
  task-start transition, and remains idempotent for already-running tasks.
- `runtimeSchedulerRecorder.AgentTaskCompleted(...)` is the correct completion
  evidence path for produced refs. Failed/cancelled task evidence must stay
  terminal and must not produce artifact refs from partial output.

Accepted runner shape:

- The eventual foreground runner should be an explicit backend-internal
  dependency behind `runtimeRunSchedulerExecuteTask(...)`, not a HTTP/Wails/
  client/frontend action in the first implementation.
- Runtime should depend on a narrow child-agent runner interface, for example
  an internal shape equivalent to:

  ```text
  ExecuteAgentTask(ctx, RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error)
  ```

  The request should be built from fresh runtime evidence after scheduler
  revalidation: run id, parent session id, parent turn id, parent tool call id,
  child session id/session title, task id, role/name/kind, prompt summary or
  durable prompt body, allowed tools, capability scope, cwd/worktree, provider,
  and model. The interface may be implemented by an adapter around coordinator
  sub-agent execution, but runtime tests should be able to inject a fake runner.
- The runner must reuse coordinator/agent execution semantics instead of
  inventing a parallel scheduler. The implementation path should either export
  a narrow coordinator foreground-task method or add an adapter in the agent
  package that calls the same sub-agent machinery used by `runSubAgent(...)`.
  Directly duplicating session creation, child agent registration, provider
  options, permission evaluation, or recorder writes in runtime is rejected.
- Execution remains foreground and request-scoped. A successful call may block
  until the child agent reaches completed/failed/cancelled terminal evidence;
  it must not enqueue background work, spawn a daemon, poll after return, or
  automatically resume after restart.
- Idempotency remains task-id based. If the durable task is already running,
  completed, failed, cancelled, or interrupted, the runner must not start a
  second child execution or duplicate child sessions/messages/results/refs.
  Running duplicates may return the existing running task as
  `task_already_running` until a later accepted phase adds safe foreground join
  semantics.
- Cancellation remains owned by `CancelAgentTask(...)` and recorder terminal
  evidence. The runner must observe `ctx.Done()` and fresh task cancellation
  state before start and before completion writes. Cancellation after restart
  must terminalize stale running task evidence; it must not recover a
  process-local child agent or stale tool state.
- Permission and MCP auth/elicitation state must be read from current runtime
  DTO/state during foreground execution. The runner must not restore stale
  actionable permission gates or stale MCP auth/elicitation requests from task
  rows, event payloads, React state, assistant prose, or transition history.
- Produced refs remain completion-only. Only completed structured child-agent
  recorder output may populate task/result artifact refs. Partial, unfinished,
  disconnected, failed, cancelled, or interrupted child work must not create
  artifact evidence.
- Runtime events emitted by the runner are refresh triggers only. Payloads may
  help callers decide which DTOs to refresh, but callers must re-read task
  detail/result, `TurnActivity`, `SessionActivityCursorWindow`, full
  `SessionActivity`, `RunProjection`, refs, permission, and MCP request DTOs
  for truth.

Required implementation entry criteria for the next implementation phase:

- Add a backend-internal child-agent runner interface and wire it into
  `runtimeService` for tests without exposing transport or UI.
- Preserve `runtimeRunSchedulerExecuteTask(...)` revalidation and start
  idempotency before invoking the runner.
- Prove queued task execution reaches completed, failed, and cancelled terminal
  evidence through recorder-compatible paths.
- Prove duplicate execute calls do not duplicate child sessions, messages,
  results, refs, lifecycle events, or transitions.
- Prove cancellation before start, during execution, and after an already-final
  task remains terminal and artifact-safe.
- Prove permission/MCP actionability is current-state only and event payloads
  are refresh triggers only.
- Prove completed output is the only produced-ref source and failed/cancelled/
  partial child execution creates no artifact evidence.

Rejected runner shape:

- No background scheduler, queue, poller, daemon, unattended execution, or
  automatic resume.
- No runtime Run store, Run state-machine migration, or new database migration.
- No HTTP/dev/Wails/generated binding/client adapter/frontend Run UI exposure.
- No stale running/waiting tool recovery.
- No stale actionable permission gate or MCP auth/elicitation recovery.
- No event-payload, assistant-prose, transition-history, or React-state source
  of truth.
- No runtime-side clone of coordinator sub-agent logic that bypasses
  `AgentTaskRecorder` evidence.

Validation:

- Design review only.
- Reviewed coordinator child-agent execution, active child routing,
  runtime task start, task cancellation, task recorder completion/failure, and
  task tool DTO paths.
- `git diff --check` passed.

Review conclusion:

- Phase 22.5 accepts the foreground child-agent runner direction but not its
  implementation.
- The next safe task is Phase 22.6: implement a backend-internal child-agent
  runner contract/fake-runner harness behind `runtimeRunSchedulerExecuteTask`.
- Phase 22.6 must still avoid transport/frontend exposure, background workers,
  automatic resume, database migrations, stale actionability recovery, and
  event/prose/React-derived truth.

## 2026-06-10: Phase 22.6 Child-agent Foreground Runner Backend Contract

Phase 22.6 implements the backend-internal child-agent runner contract behind
`runtimeRunSchedulerExecuteTask(...)`. It does not connect the real
coordinator runner yet and does not expose HTTP, dev module, Wails, generated
bindings, client adapter, or frontend controls.

Implemented:

- Added backend-internal `runtimeAgentTaskRunner` with
  `ExecuteAgentTask(ctx, RuntimeAgentTaskExecutionRequest)`.
- Added `RuntimeAgentTaskExecutionRequest` and
  `RuntimeAgentTaskExecutionResult` DTOs for runtime/agent boundary evidence.
  These DTOs are not added to `RuntimeService` or transport.
- Added an internal request builder that copies fresh durable task/run evidence
  after scheduler revalidation and start recording:
  run/task ids, parent session/turn/tool call, child session, title/kind/role/
  name, prompt summary, provider/model, allowed tools, capability scope,
  cwd/worktree, and `StartedAt`.
- The request explicitly marks `startAlreadyRecorded=true`,
  `backendOnly=true`, and `eventPayloadRefreshOnly=true` so a future real
  adapter must not duplicate start evidence or treat events as state truth.
- `runtimeRunSchedulerExecuteTask(...)` now invokes the injected runner only
  after queued-task start evidence is recorded. If no runner is injected, the
  Phase 22.4 start-only behavior remains unchanged.
- Duplicate/running and terminal/unowned candidates still return before runner
  invocation, preserving task-id idempotency and no stale actionability
  recovery.
- After runner return, runtime re-reads the task from durable storage for the
  response instead of trusting runner payload data.

Contract tests:

- Existing start-only tests still pass with no runner injected.
- A fake foreground runner proves the runtime passes fresh ownership/scope
  evidence and source flags into the runner.
- The fake runner writes completion through `runtimeSchedulerRecorder` and
  proves completed output produces exactly one result message and one runtime
  artifact ref.
- A duplicate execute after terminal completion is rejected before runner
  invocation and does not duplicate child sessions/messages/results/refs.
- A fake cancelled runner writes cancelled terminal evidence with partial
  summary text and proves cancelled/partial output creates no artifact refs.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask" -count=1`
  passed.

Review conclusion:

- Phase 22.6 adds only a backend-internal, test-injectable runner contract.
- It still does not implement the real coordinator adapter or expose execution
  through transport/frontend.
- Runtime durable DTOs remain the source of truth; runner result payloads and
  events are not used to hydrate lifecycle, artifact, permission/MCP
  actionability, or Run status directly.
- The next safe task is Phase 22.7 acceptance of the backend runner contract,
  then a later implementation gate can connect a real coordinator adapter if
  accepted.

## 2026-06-10: Phase 22.7 Child-agent Foreground Runner Backend Contract Acceptance

Phase 22.7 accepts the Phase 22.6 backend-internal runner contract as the
stable boundary for future child-agent foreground execution work.

Acceptance review:

- Confirmed `runtimeAgentTaskRunner` is an unexported runtime dependency and is
  only injected on `runtimeService` for internal execution/tests.
- Confirmed `RuntimeAgentTaskExecutionRequest` and
  `RuntimeAgentTaskExecutionResult` are not exposed through `RuntimeService`,
  HTTP/dev routes, Wails bridge, generated bindings, client adapters, or React
  UI.
- Confirmed `runtimeRunSchedulerExecuteTask(...)` still revalidates durable
  Run/task ownership through `runtimeRunSchedulerDelegateTaskTurn(...)` before
  start or runner invocation.
- Confirmed runner invocation happens only after queued-task start evidence is
  recorded, and the request explicitly states
  `startAlreadyRecorded=true`, `backendOnly=true`, and
  `eventPayloadRefreshOnly=true`.
- Confirmed no-runner behavior preserves the Phase 22.4 start-only contract.
- Confirmed running duplicates return `task_already_running` without runner
  invocation, and terminal/unowned candidates are rejected before runner
  invocation.
- Confirmed runtime re-reads the durable task after runner return instead of
  trusting runner result payloads as lifecycle or artifact truth.
- Confirmed fake-runner tests prove completed recorder evidence is the only
  produced-ref path and cancelled/partial output produces no artifact refs.

Validation:

- `go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask" -count=1`
  passed.
- `go test ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- Future real coordinator integration must implement this backend-internal
  runner boundary or an equivalent one that preserves the same source flags,
  durable re-read semantics, idempotency, cancellation ordering, and
  completion-only artifact evidence.
- The real adapter must reuse coordinator/sub-agent execution semantics and
  recorder-compatible terminal evidence; it must not clone coordinator logic in
  runtime or bypass `AgentTaskRecorder`.
- Transport/frontend exposure remains unaccepted until the real adapter is
  implemented and accepted with permission/MCP, cancellation, failure,
  completion, and artifact evidence coverage.

Review conclusion:

- Phase 22.7 accepts the backend runner contract.
- The next safe task is Phase 23: Real Coordinator Foreground Runner Adapter
  Design Gate.
- Phase 23 should design, not yet implement, how to expose a narrow
  coordinator/agent method that can execute an already-started runtime task
  without duplicating task start evidence or restoring stale process state.

## 2026-06-10: Phase 23 Real Coordinator Foreground Runner Adapter Design Gate

Phase 23 designs the real coordinator adapter that can eventually implement the
accepted `runtimeAgentTaskRunner` contract. It is a design gate only; no
coordinator method, adapter implementation, transport exposure, frontend
control, background worker, automatic resume, database migration, or stale
actionability recovery is added.

Adapter problem statement:

- Current `coordinator.runSubAgent(...)` creates a task session, registers a
  process-local child agent, evaluates policy, starts the child agent, writes
  start/progress/completed/failed task recorder evidence, and unregisters the
  child agent on return.
- The runtime execute path already records durable start evidence before
  runner invocation. A real foreground adapter therefore cannot call
  `runSubAgent(...)` as-is without duplicating start messages/events and
  potentially creating a second child session.
- The accepted Phase 22.6 runner request represents an already-started runtime
  task. The adapter must run that task foreground-only and write terminal
  evidence through recorder-compatible paths without treating process-local
  `childAgents` as durable resume state.

Accepted adapter direction:

- Add a narrow agent/coordinator-facing contract in a later implementation
  phase, for example:

  ```text
  ExecuteStartedAgentTask(ctx, StartedAgentTaskExecutionRequest) (StartedAgentTaskExecutionResult, error)
  ```

  The request should map one-to-one from
  `RuntimeAgentTaskExecutionRequest`, including task id, parent session/turn/
  tool call, child session id, title/kind/role/name, prompt summary or durable
  task prompt body, model/provider, tools, scope, cwd/worktree, and source
  flags.
- Refactor coordinator sub-agent execution into reusable pieces instead of
  cloning logic in runtime:
  - shared policy evaluation using current permission/MCP state;
  - child session/agent registration for foreground follow-up/cancel routing;
  - model/provider option resolution;
  - non-interactive child agent `Run`;
  - parent session cost propagation;
  - terminal recorder writes.
- Split start evidence from terminal execution. The new adapter path must
  accept `startAlreadyRecorded=true` and skip `AgentTaskStarted`/
  `AgentTaskProgress` start writes unless a future accepted migration changes
  the start owner.
- Preserve `Coordinator.Cancel(sessionID)` routing only while the foreground
  call is active. After process restart, missing child-agent process state must
  remain non-resumable and must be handled by runtime cancellation/interruption
  evidence, not auto-resume.
- The runtime-side adapter that satisfies `runtimeAgentTaskRunner` should be
  thin: map runtime DTO to agent/coordinator DTO, call the coordinator method,
  then let runtime re-read durable task/result/ref DTOs.

Required Phase 23.1 implementation entry criteria:

- Add only backend/internal agent/coordinator contract types and tests first.
- Prove an already-started task does not duplicate start evidence.
- Prove active foreground registration enables follow-up/cancel routing during
  the call and is removed after return.
- Prove cancellation maps to cancelled terminal recorder evidence and no
  artifact refs.
- Prove failed child execution writes failed terminal evidence and no artifact
  refs unless completed structured output exists.
- Prove completed child execution writes completion/result evidence and
  produced refs only from completed recorder output.
- Prove permission/MCP actionability is current-state only and is not restored
  from event payloads, assistant prose, React state, or process-local child
  maps after restart.

Rejected adapter shape:

- Do not call `runSubAgent(...)` directly from runtime in a way that duplicates
  task session/start evidence.
- Do not create a background scheduler, queue, daemon, poller, or join loop.
- Do not auto-resume a started child task after restart.
- Do not persist process-local child agent handles or add database migrations.
- Do not expose HTTP/dev/Wails/generated binding/client adapter/frontend
  execution controls.
- Do not let event payloads, transition history, assistant prose, or React
  state hydrate lifecycle, artifacts, permission/MCP actionability, or Run
  status.

Validation:

- Design review only.
- Reviewed `coordinator.runSubAgent(...)`,
  `Coordinator.SendToSession(...)`, `Coordinator.Cancel(...)`,
  `AgentTaskRecorder`, Phase 22.6 runner contract, runtime task cancellation,
  and recorder completion/failure evidence.
- `git diff --check` passed.

Review conclusion:

- Phase 23 accepts the real adapter direction but not implementation.
- The next safe task is Phase 23.1: Agent/coordinator Started Task Execution
  Contract.
- Phase 23.1 must remain backend/internal and must not expose transport/UI or
  add background scheduling, automatic resume, database migrations, stale
  actionability recovery, or event/prose/React-derived truth.

## 2026-06-10: Phase 23.1 Agent/coordinator Started Task Execution Contract

Phase 23.1 adds the backend/internal coordinator contract for executing an
already-started agent task. It is not wired into runtime yet and does not add
HTTP/dev/Wails transport, generated bindings, client adapters, frontend UI,
background scheduling, automatic resume, database migrations, or stale
actionability recovery.

Implemented:

- Added `StartedAgentTaskExecutionRequest` and
  `StartedAgentTaskExecutionResult` in `internal/agent`.
- Added `coordinator.ExecuteStartedAgentTask(...)` for backend/internal use.
- The method requires pre-recorded start evidence via
  `StartAlreadyRecorded=true` and rejects calls without it.
- The method registers the child `SessionAgent` in the existing process-local
  `childAgents` map only while the foreground call is active, preserving
  follow-up/cancel routing during execution and unregistering after return.
- The method reuses coordinator model/provider option resolution, CWD/worktree
  context propagation, non-interactive `SessionAgent.Run`, parent session cost
  propagation, and `AgentTaskRecorder` terminal evidence.
- The method intentionally skips `AgentTaskStarted` and `AgentTaskProgress`
  start writes because runtime already owns durable start evidence.
- Agent run failures map to failed terminal recorder evidence; context
  cancellation maps to cancelled terminal recorder evidence.
- Policy denial after pre-recorded start evidence maps to failed terminal
  recorder evidence without running the agent.
- Completed runs write completed terminal recorder evidence and return terminal
  result metadata with `NoStaleResume=true` and `CompletionOnlyRefs=true`.

Contract tests:

- Completed execution skips duplicate start/progress evidence and writes one
  completed record for the existing task/child session.
- Active foreground execution routes `SendToSession(...)` follow-up messages to
  the child agent and unregisters the child after return.
- Active foreground cancellation routes through `Coordinator.Cancel(...)` and
  writes cancelled terminal evidence without artifact refs.
- Policy denial writes failed terminal evidence without running the agent.
- Calls without pre-recorded start evidence are rejected before agent run.

Validation:

- `go test ./internal/agent -run "TestExecuteStartedAgentTask|TestRunSubAgent" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Review conclusion:

- Phase 23.1 establishes the coordinator-side started-task execution contract.
- Runtime is still not wired to this contract; `runtimeAgentTaskRunner` remains
  test-injected only.
- No transport/frontend execution surface, background scheduler, automatic
  resume, migration, stale actionability recovery, or event/prose/React source
  of truth was added.
- The next safe task is Phase 23.2 acceptance, followed by a later runtime
  adapter wiring gate if accepted.

## 2026-06-10: Phase 23.2 Agent/coordinator Started Task Execution Contract Acceptance

Phase 23.2 accepts the Phase 23.1 coordinator-side started-task execution
contract as the stable agent-layer boundary for future runtime wiring.

Acceptance review:

- Confirmed `StartedAgentTaskExecutionRequest` and
  `StartedAgentTaskExecutionResult` live in `internal/agent` and are not
  exposed through runtime transport, generated bindings, client adapters, or
  frontend UI.
- Confirmed `coordinator.ExecuteStartedAgentTask(...)` requires
  `StartAlreadyRecorded=true` and rejects calls without pre-recorded durable
  start evidence.
- Confirmed the method does not call `AgentTaskStarted` or
  `AgentTaskProgress`, preserving runtime ownership of start evidence.
- Confirmed active child registration is process-local and scoped to the
  foreground call, enabling follow-up/cancel routing only while the child agent
  is actually active.
- Confirmed completion, provider/policy failure, agent failure, and context
  cancellation write terminal recorder-compatible evidence and do not create
  artifact refs from partial/cancelled output.
- Confirmed runtime remains unwired: `runtimeAgentTaskRunner` is still
  test-injected and no real coordinator adapter is installed.

Validation:

- `go test ./internal/agent -run "TestExecuteStartedAgentTask|TestRunSubAgent" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- A future runtime adapter may map `RuntimeAgentTaskExecutionRequest` to
  `StartedAgentTaskExecutionRequest` and call
  `coordinator.ExecuteStartedAgentTask(...)`, but only after a separate wiring
  design/implementation phase accepts agent selection, prompt sourcing,
  permission/MCP behavior, cancellation ordering, and terminal evidence
  coverage.
- The runtime adapter must remain foreground/request-scoped and must let
  runtime re-read durable task/result/ref DTOs after return.
- Transport/frontend exposure remains blocked.

Review conclusion:

- Phase 23.2 accepts the coordinator-side started-task execution contract.
- The next safe task is Phase 23.3: Runtime-to-coordinator Foreground Runner
  Wiring Design Gate.
- Phase 23.3 should design how runtime selects/builds the child agent and maps
  request evidence to coordinator execution without adding transport/UI,
  background scheduling, automatic resume, migrations, stale actionability
  recovery, or event/prose/React-derived truth.

## 2026-06-10: Phase 23.3 Runtime-to-coordinator Foreground Runner Wiring Design Gate

Phase 23.3 designs how runtime should eventually wire
`runtimeAgentTaskRunner` to the real coordinator started-task contract. It is a
design gate only; no runtime wiring, coordinator adapter installation,
transport exposure, frontend control, background worker, automatic resume,
database migration, or stale actionability recovery is added.

Reviewed wiring points:

- Runtime owns `runtimeRunSchedulerExecuteTask(...)` and the durable
  `RuntimeAgentTaskExecutionRequest` emitted after scheduler revalidation and
  start evidence.
- Agent owns `coordinator.ExecuteStartedAgentTask(...)`, which expects an
  already-started task and requires a concrete `SessionAgent`.
- Backend/workspace currently holds `Workspace.App.AgentCoordinator`, while
  runtime holds `runtimeBackend` plus the active `workspaceID`.
- The task agent configuration already exists as `config.AgentTask`; existing
  `agent_tool` builds the task sub-agent from this config and allowed tools.

Accepted wiring direction:

- Add a small runtime-side adapter in a later phase, for example:

  ```text
  runtimeCoordinatorTaskRunner{
      backend *backend.Backend
      workspaceID string
  }
  ```

  It should satisfy `runtimeAgentTaskRunner` by:
  - resolving the current backend workspace by id;
  - requiring an initialized `AgentCoordinator`;
  - selecting/building the configured task agent through the coordinator/agent
    package, not from runtime;
  - mapping `RuntimeAgentTaskExecutionRequest` to
    `agent.StartedAgentTaskExecutionRequest`;
  - calling `ExecuteStartedAgentTask(...)`;
  - returning only action metadata while runtime re-reads durable task/result/
    ref DTOs after return.
- Agent selection should initially support the existing `config.AgentTask`
  subagent role only. Unknown or unsupported task roles must fail the already
  started task with terminal failed evidence instead of guessing a model or
  falling back to the coder agent.
- Prompt sourcing must be explicit. Runtime task rows currently persist
  `PromptSummary`, not a full durable prompt body. The wiring phase must decide
  whether Phase 23.4 adds a durable task prompt field or limits execution to
  tasks whose instruction message can be read as the prompt source. It must not
  infer prompts from assistant prose.
- Child session id must come from durable task evidence. The adapter must not
  create a second child session for an already-started task.
- Cancellation ordering remains runtime-first: `CancelAgentTask(...)` owns
  durable cancellation; coordinator `Cancel(childSessionID)` is best-effort
  active-process routing while the foreground call is running.
- Permission/MCP actionability must be current-state only. The adapter may
  rely on coordinator policy evaluation and runtime recorder state during the
  foreground call, but must not restore stale permission gates or MCP auth/
  elicitation requests from events, React state, assistant prose, or durable
  task rows after restart.
- The adapter must keep event payloads as refresh triggers only. Runtime must
  re-read task detail/result, refs, activity, projection, permission, and MCP
  DTOs after runner return.

Required Phase 23.4 implementation entry criteria:

- Add backend/internal adapter wiring only; do not expose HTTP/dev/Wails/client
  or UI controls.
- Prove missing backend/workspace/coordinator fails terminally without leaving
  a started task running.
- Prove unsupported role/model/prompt-source gaps fail terminally and do not
  run the coder agent by accident.
- Prove prompt source is durable and structured: task instruction message or a
  newly accepted durable task prompt field, never assistant prose.
- Prove successful runtime execution calls coordinator once, does not duplicate
  start evidence, and re-reads durable task state after return.
- Prove cancellation before/during runner execution terminalizes as cancelled
  and produces no artifact refs.
- Prove completed output remains the only produced-ref source.

Rejected wiring shape:

- No runtime-side recreation of task agent build logic beyond a thin call into
  agent/coordinator code.
- No fallback to coder agent for unknown task role.
- No prompt reconstruction from assistant prose, event payloads, or React
  state.
- No child session recreation for already-started tasks.
- No background worker, queue, daemon, poller, automatic resume, migration,
  transport/frontend execute action, stale actionability recovery, or event/
  prose/React state source of truth.

Validation:

- Design review only.
- Reviewed runtime runner contract, coordinator started-task contract,
  backend/workspace coordinator ownership, task agent config, and runtime task
  cancellation/recorder evidence paths.
- `git diff --check` passed.

Review conclusion:

- Phase 23.3 accepts the runtime-to-coordinator wiring direction but not
  implementation.
- The next safe task is Phase 23.4: Runtime-to-coordinator Foreground Runner
  Adapter Contract.
- Phase 23.4 must first resolve durable prompt sourcing and unsupported-role
  terminal failure behavior before connecting real task execution.

## 2026-06-10: Phase 23.4 Runtime-to-coordinator Foreground Runner Adapter Contract

Phase 23.4 implements the backend/internal runtime-side adapter contract for
mapping started runtime tasks to the coordinator started-task execution shape.
It does not install the adapter into `runtimeService`, does not resolve a real
backend workspace/coordinator yet, and does not expose HTTP/dev/Wails,
generated bindings, client adapters, frontend UI, background scheduling,
automatic resume, database migrations, or stale actionability recovery.

Implemented:

- Added `Prompt` to `RuntimeAgentTaskExecutionRequest`.
- `runtimeRunSchedulerExecuteTask(...)` now writes a structured durable prompt
  source into the start instruction message payload:
  `prompt_source=runtime_task_instruction` and `prompt=<start prompt>`.
- Added `runtimeCoordinatorTaskRunner`, an internal runner contract that maps
  `RuntimeAgentTaskExecutionRequest` to
  `agent.StartedAgentTaskExecutionRequest` through an injected
  `runtimeStartedAgentTaskExecutor`.
- The adapter reads prompt text only from the explicit runtime request or from
  the durable structured task instruction message. It does not infer prompt
  text from assistant prose, events, transition history, or React state.
- The adapter currently supports only `config.AgentTask` role. Unsupported
  roles fail terminally through `runtimeSchedulerRecorder.AgentTaskFailed`
  without calling the executor.
- Missing executor or missing prompt source also fail terminally and leave no
  artifact evidence.
- Successful executor calls return metadata only; runtime remains responsible
  for durable task/result/ref re-reads after runner return.

Contract tests:

- The runtime execute start path persists structured prompt source payload in
  the instruction message.
- The coordinator adapter reads the durable instruction prompt and maps
  task/session/turn/source flags into the started-task executor request.
- Unsupported roles fail terminally, do not call the executor, and do not
  create artifact refs.
- Missing prompt source fails terminally, does not call the executor, and does
  not create artifact refs.

Validation:

- `go test ./internal/runtime -run "TestRuntimeCoordinatorTaskRunner|TestRuntimeRunSchedulerExecuteTask" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Review conclusion:

- Phase 23.4 resolves durable prompt sourcing for the adapter contract and
  terminal failure behavior for unsupported roles/prompt gaps.
- The adapter is still not installed into `runtimeService` and still does not
  call a real backend workspace/coordinator.
- No transport/frontend execution surface, background scheduler, automatic
  resume, migration, stale actionability recovery, or event/prose/React source
  of truth was added.
- The next safe task is Phase 23.5 acceptance, then a later gate may install a
  real backend/coordinator executor if accepted.

## 2026-06-10: Phase 23.5 Runtime-to-coordinator Foreground Runner Adapter Contract Acceptance

Phase 23.5 accepts the Phase 23.4 runtime-side coordinator adapter contract as
the stable backend/internal mapping boundary for future real executor wiring.

Acceptance review:

- Confirmed `runtimeCoordinatorTaskRunner` is internal runtime code and is not
  installed into `runtimeService`.
- Confirmed no `RuntimeService` method, HTTP/dev route, Wails bridge,
  generated binding, client adapter, or React UI execution affordance was
  added.
- Confirmed task start now persists a structured prompt source in the
  instruction message payload and does not rely on assistant prose.
- Confirmed the adapter reads prompt text only from the explicit request or
  the durable `runtime_task_instruction` payload.
- Confirmed unsupported roles, missing executor, and missing prompt source
  fail terminally through recorder-compatible task failure evidence and produce
  no artifact refs.
- Confirmed successful adapter calls return metadata only; runtime still
  re-reads durable task/result/ref DTOs after runner return.
- Confirmed events remain refresh triggers only and do not hydrate lifecycle,
  artifacts, permission/MCP actionability, or Run status.

Validation:

- `go test ./internal/runtime -run "TestRuntimeCoordinatorTaskRunner|TestRuntimeRunSchedulerExecuteTask" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- A future real executor may be installed behind this adapter only after a
  separate design/implementation gate proves backend workspace resolution,
  coordinator readiness, task-agent selection, cancellation ordering, and
  completed-output-only produced refs.
- The first real executor must remain backend/internal and foreground-scoped.
- Transport/frontend exposure remains blocked.

Review conclusion:

- Phase 23.5 accepts the runtime-side adapter contract.
- The next safe task is Phase 24: Real Backend/coordinator Executor Install
  Design Gate.
- Phase 24 must design how to resolve backend workspace/coordinator and build
  the task agent without recreating coordinator logic in runtime.

## 2026-06-10: Phase 24 Real Backend/coordinator Executor Install Design Gate

Phase 24 designs how to install a real backend/coordinator executor behind the
accepted runtime adapter contract. It is a design gate only; no executor is
installed, no runtime service wiring is changed, and no transport/frontend
execution surface is added.

Reviewed install points:

- `runtimeCoordinatorTaskRunner` already maps runtime task evidence and
  durable prompt source into an injected started-task executor.
- `agent.coordinator.ExecuteStartedAgentTask(...)` can execute an
  already-started task, but its request currently requires a concrete
  `SessionAgent`.
- The existing `agent_tool` builds the configured task agent using
  `config.AgentTask`, `taskPrompt(...)`, and `coordinator.buildAgent(...)`.
- Runtime should not import or duplicate task-agent build logic. It currently
  holds `backend.Backend` and workspace id, while backend/workspace owns
  `Workspace.App.AgentCoordinator`.

Accepted install direction:

- Add an agent/coordinator-side configured executor method in a later phase,
  for example:

  ```text
  ExecuteConfiguredStartedAgentTask(ctx, StartedAgentTaskExecutionRequest) (StartedAgentTaskExecutionResult, error)
  ```

  This method should:
  - accept a started-task request without requiring runtime to supply
    `SessionAgent`;
  - reject non-`config.AgentTask` roles terminally;
  - build the task agent using the same task prompt, task agent config,
    allowed tools, provider/model, skills, permission, and recorder wiring as
    `agent_tool`;
  - call `ExecuteStartedAgentTask(...)` with the constructed child agent;
  - avoid `AgentTaskStarted`/`AgentTaskProgress` duplicate start writes.
- Add a backend/workspace method only after that coordinator method exists,
  for example:

  ```text
  ExecuteStartedAgentTask(ctx, workspaceID string, req agent.StartedAgentTaskExecutionRequest)
  ```

  It should resolve workspace, require initialized coordinator, call the
  configured coordinator method, and return terminal result metadata.
- Install `runtimeCoordinatorTaskRunner` into `runtimeService` only after the
  backend method is implemented and tested. Runtime should pass the backend
  workspace id and a backend executor adapter; it should not build agents.
- Missing backend, missing workspace, or uninitialized coordinator must fail
  the already-started task terminally through runtime recorder evidence rather
  than leaving it running.
- Cancellation remains foreground/request-scoped: runtime durable
  `CancelAgentTask(...)` owns cancellation, while backend/coordinator cancel
  routing remains best-effort for an active child session only.

Required Phase 24.1 implementation entry criteria:

- First implement the coordinator configured started-task executor, not runtime
  installation.
- Add tests proving it builds the configured task agent path without duplicate
  start evidence.
- Add tests for missing task agent config, unsupported role, provider/model
  failure, policy denial, cancellation, and completion terminal evidence.
- Prove failed/cancelled/partial paths produce no artifact refs.
- Keep everything backend/internal; do not add HTTP/dev/Wails/client/frontend
  execution actions.

Rejected install shape:

- No runtime-side agent construction or copied `buildAgent`/`taskPrompt`
  logic.
- No direct fallback to coder agent.
- No prompt inference from assistant prose/events/React state.
- No background worker, queue, poller, daemon, automatic resume, migration,
  transport/frontend execution action, stale actionability recovery, or event/
  prose/React source of truth.

Validation:

- Design review only.
- Reviewed runtime adapter contract, coordinator started-task contract,
  app/backend workspace coordinator ownership, `agent_tool` task-agent build
  path, and runtime cancellation/recorder evidence paths.
- `git diff --check` passed.

Review conclusion:

- Phase 24 accepts the real executor install direction but not implementation.
- The next safe task is Phase 24.1: Coordinator Configured Started Task
  Executor Contract.
- Phase 24.1 must keep agent construction in coordinator code and leave
  runtime adapter installation for a later accepted phase.

## 2026-06-10: Phase 24.1 Coordinator Configured Started Task Executor Contract

Phase 24.1 implements the coordinator-owned configured started-task executor.
It does not install the runtime adapter, does not add backend transport, and
does not expose HTTP/dev/Wails, generated bindings, client adapters, frontend
UI, background scheduling, automatic resume, database migrations, or stale
actionability recovery.

Implemented:

- Added `Coordinator.ExecuteConfiguredStartedAgentTask(...)`.
- Added coordinator-side configured task-agent construction through the
  existing `config.AgentTask`, `taskPrompt(...)`, and `buildAgent(...)` path.
- Added an internal test hook for task-agent construction so contract tests can
  avoid real provider calls while production code still uses coordinator-owned
  agent construction.
- The configured executor rejects unsupported roles terminally before building
  an agent and does not fall back to the coder agent.
- Missing task agent config/build failures are mapped to failed terminal
  recorder-compatible evidence for the already-started task.
- On success, the configured executor injects the constructed task agent and
  delegates to `ExecuteStartedAgentTask(...)`, preserving no duplicate start/
  progress writes and foreground-only child registration semantics.

Contract tests:

- Configured execution builds the task agent through the coordinator-owned
  builder, skips duplicate start/progress evidence, writes one completed
  record, and applies configured allowed tools.
- Unsupported roles fail terminally without invoking the task-agent builder and
  produce no artifact refs.
- Missing task agent config fails terminally and produces no artifact refs.

Validation:

- `go test ./internal/agent -run "TestExecuteConfiguredStartedAgentTask|TestExecuteStartedAgentTask|TestRunSubAgent" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Review conclusion:

- Phase 24.1 establishes the coordinator-owned configured started-task executor
  contract.
- Runtime remains unwired; `runtimeCoordinatorTaskRunner` still uses only an
  injected executor in tests.
- No transport/frontend execution surface, background scheduler, automatic
  resume, migration, stale actionability recovery, or event/prose/React source
  of truth was added.
- The next safe task is Phase 24.2 acceptance, then a later backend/runtime
  wiring gate if accepted.

## 2026-06-10: Phase 24.2 Coordinator Configured Started Task Executor Contract Acceptance

Phase 24.2 accepts the Phase 24.1 coordinator-owned configured started-task
executor contract.

Acceptance review:

- Confirmed `Coordinator.ExecuteConfiguredStartedAgentTask(...)` is
  backend/internal agent API only and is not exposed through runtime transport,
  generated bindings, client adapters, or frontend UI.
- Confirmed task-agent construction stays in coordinator code through the
  existing `config.AgentTask`, `taskPrompt(...)`, and `buildAgent(...)` path.
- Confirmed runtime does not build agents and the runtime adapter is still not
  installed.
- Confirmed unsupported roles fail terminally before agent construction and do
  not fall back to the coder agent.
- Confirmed missing task agent config/build failures write failed terminal
  evidence for the already-started task.
- Confirmed successful configured execution delegates to
  `ExecuteStartedAgentTask(...)` and does not duplicate start/progress
  evidence.

Validation:

- `go test ./internal/agent -run "TestExecuteConfiguredStartedAgentTask|TestExecuteStartedAgentTask|TestRunSubAgent" -count=1`
  passed.
- `go test ./internal/agent ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1`
  passed.
- `git diff --check` passed.

Accepted contract:

- A future backend/runtime wiring phase may call
  `ExecuteConfiguredStartedAgentTask(...)` through a backend/workspace adapter.
- Runtime must still use durable prompt source and re-read task/result/ref DTOs
  after runner return.
- Transport/frontend exposure remains blocked until backend/runtime wiring is
  implemented and accepted.

Review conclusion:

- Phase 24.2 accepts the coordinator configured executor contract.
- The next safe task is Phase 24.3: Backend/runtime Executor Wiring Design
  Gate.
- Phase 24.3 should design the backend workspace method and runtime install
  path without implementing it yet.

## 2026-06-10: Phase 24.3 Backend/runtime Executor Wiring Design Gate

Phase 24.3 designs the backend/runtime wiring needed to install the accepted
coordinator configured executor behind `runtimeCoordinatorTaskRunner`. It is a
design gate only; no backend method, runtime installation, transport exposure,
frontend control, background scheduling, automatic resume, database migration,
or stale actionability recovery is added.

Accepted wiring shape:

- Add a backend-internal method in a later phase, for example:

  ```text
  ExecuteStartedAgentTask(ctx, workspaceID string, req agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error)
  ```

  It should resolve the workspace by id, require initialized
  `AgentCoordinator`, call `ExecuteConfiguredStartedAgentTask(...)`, and return
  terminal result metadata.
- Add a thin runtime executor adapter that satisfies
  `runtimeStartedAgentTaskExecutor` by calling the backend method. It should
  not build agents, select models, or inspect frontend state.
- Install `runtimeCoordinatorTaskRunner` into `runtimeService` only when
  runtime has both a live backend pointer and active workspace id. Startup or
  workspace changes must not auto-resume old tasks.
- If backend, workspace, or coordinator is missing after runtime has already
  recorded task start evidence, the runtime adapter must fail the task
  terminally through recorder-compatible evidence and produce no artifact refs.
- Runtime must keep durable re-read semantics after runner return. Runner or
  backend result payloads may be action metadata only.
- Cancellation remains owned by `CancelAgentTask(...)`. The installed runner
  may rely on coordinator process-local cancel routing only during the active
  foreground request.

Required Phase 24.4 implementation entry criteria:

- Add backend/internal method and runtime executor adapter only; do not expose
  HTTP/dev/Wails/client/frontend execution actions.
- Prove missing backend/workspace/coordinator terminally fails already-started
  tasks without artifact refs.
- Prove successful execution calls backend/coordinator once and does not
  duplicate start/progress evidence.
- Prove runtime re-reads durable task/result/ref DTOs after return.
- Prove cancellation before/during execution terminalizes as cancelled and
  produces no artifact refs.
- Prove event payloads remain refresh triggers only.

Rejected wiring shape:

- No runtime-side task agent construction or model/provider selection.
- No fallback to coder agent.
- No prompt inference from assistant prose, events, transition history, or
  React state.
- No background worker, queue, poller, daemon, automatic resume, migration,
  transport/frontend execution action, stale actionability recovery, or event/
  prose/React source of truth.

Validation:

- Design review only.
- Reviewed backend workspace ownership, app coordinator initialization,
  coordinator configured executor contract, runtime adapter contract, and
  runtime cancellation/recorder evidence paths.
- `git diff --check` passed.

Review conclusion:

- Phase 24.3 accepts the backend/runtime wiring direction but not
  implementation.
- The next safe task is Phase 24.4: Backend/runtime Executor Wiring Contract.
- Phase 24.4 must remain backend/internal and must still avoid transport/UI
  exposure, background scheduling, automatic resume, migrations, stale
  actionability recovery, and event/prose/React-derived truth.

## 2026-06-10: Phase 24.4 Backend/runtime Executor Wiring Contract

Phase 24.4 implements the backend/internal executor wiring needed to connect
the accepted coordinator configured started-task executor to the runtime
runner. It remains internal only: no HTTP/dev route, Wails binding, generated
client adapter, React control, background worker, automatic resume, database
migration, or stale actionability recovery is added.

Implemented contract:

- Added `Backend.ExecuteStartedAgentTask(ctx, workspaceID, req)` as an
  internal backend method. It resolves the workspace, requires an initialized
  `AgentCoordinator`, and delegates to
  `ExecuteConfiguredStartedAgentTask(...)`.
- Added `runtimeBackendStartedAgentTaskExecutor` as a thin runtime adapter
  around backend/workspace routing. It performs only availability checks and
  delegates execution; it does not build agents, select models, inspect
  frontend state, or infer prompts.
- Installed `runtimeCoordinatorTaskRunner` after runtime startup has a live
  backend, workspace id, and DB-backed runtime stores. Installation does not
  execute or resume tasks; it only gives later explicit scheduler execution a
  backend/coordinator delegate.
- Hardened `runtimeCoordinatorTaskRunner.ExecuteAgentTask(...)` so a backend
  or coordinator error that returns no terminal result is converted into
  durable failed task evidence through the existing scheduler recorder path.
  The failure is terminal, refresh-target-only, no-stale-resume, and
  completion-only-refs.
- Preserved explicit prompt sourcing: request prompt first, then durable
  `runtime_task_instruction` task message payload. Assistant prose, runtime
  events, transition history, and React state are still rejected as sources of
  truth.

Validation:

- Backend routing test proves the backend calls the workspace coordinator once
  and returns terminal coordinator metadata.
- Backend guard test proves missing workspace and missing coordinator return
  errors instead of falling back to a coder agent.
- Runtime installation test proves the service installs a coordinator runner
  backed by the backend/workspace adapter, and adapter guard tests prove
  missing backend or workspace id is rejected before delegation.
- Runtime runner tests prove durable instruction prompt sourcing, unsupported
  role failure, missing prompt-source failure, and non-terminal executor error
  terminalization. These failure paths create no artifact refs.
- Narrow test command passed:

  ```text
  go test ./internal/backend ./internal/runtime -run "TestBackendExecuteStartedAgentTask|TestRuntimeCoordinatorTaskRunner|TestRuntimeBackendStartedAgentTaskExecutor|TestRuntimeServiceInstallsBackendAgentTaskRunner" -count=1
  ```
- Related package regression passed:

  ```text
  go test ./internal/agent ./internal/backend ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1
  ```

Review conclusion:

- Phase 24.4 accepts backend/runtime executor wiring as internal-only.
- The implementation keeps runtime DTO/durable re-read semantics as the source
  of truth; event payloads remain refresh triggers only.
- Cancellation remains owned by existing runtime task cancellation paths; no
  new process recovery or automatic resume semantics were added.
- Remaining risk before any user-facing execution affordance: run broader
  related-package tests and perform an acceptance pass that reviews duplicate
  lifecycle/progress evidence, cancellation ordering, and transport boundaries.
- The next safe task is Phase 24.5: Backend/runtime Executor Wiring Acceptance.

## 2026-06-10: Phase 24.5 Backend/runtime Executor Wiring Acceptance

Phase 24.5 accepts the Phase 24.4 backend/runtime executor wiring contract.
This is an acceptance/review phase only; it does not add execution routes,
client bindings, frontend controls, background workers, automatic resume,
database migrations, or stale permission/MCP actionability recovery.

Acceptance review:

- `Backend.ExecuteStartedAgentTask(...)` is backend-internal and only resolves
  workspace/coordinator before delegating to the coordinator-owned
  `ExecuteConfiguredStartedAgentTask(...)`.
- `runtimeCoordinatorTaskRunner` is installed during runtime startup only
  after a live backend, workspace id, and DB-backed runtime stores exist.
  Installation does not execute queued work, resume interrupted tasks, or
  change startup recovery semantics.
- Runtime still uses explicit prompt sources only:
  `RuntimeAgentTaskExecutionRequest.Prompt` or the durable
  `runtime_task_instruction` task message payload. Assistant prose, runtime
  events, transition history, and React state remain rejected as prompt/state
  sources.
- Backend/coordinator errors that return no terminal result are converted into
  durable failed task evidence by the existing recorder path, with no artifact
  refs and no stale resume.
- Existing task cancellation remains owned by `CancelAgentTask(...)` and
  recorder terminal evidence. The backend runner installation does not add a
  scheduler-owned cancellation source or process-recovery resume path.
- No HTTP/dev route, Wails binding, generated frontend client action, React
  Run UI, runtime Run store migration, background queue/worker, poller, daemon,
  or automatic resume path was added by Phase 24.4.
- Event payloads remain refresh triggers only; runtime DTO reads and durable
  task/result/ref stores remain the source of truth.

Validation:

- Phase 24.4 implementation tests passed:

  ```text
  go test ./internal/backend ./internal/runtime -run "TestBackendExecuteStartedAgentTask|TestRuntimeCoordinatorTaskRunner|TestRuntimeBackendStartedAgentTaskExecutor|TestRuntimeServiceInstallsBackendAgentTaskRunner" -count=1
  ```

- Related package regression passed:

  ```text
  go test ./internal/agent ./internal/backend ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1
  ```

- Phase 24.5 acceptance diff review passed:
  - no `client/` changes;
  - no `desktop/` binding or bridge changes;
  - no HTTP execution route changes;
  - no database migration changes;
  - no runtime-side agent construction or model selection;
  - no auto-resume or background scheduler execution loop.

Review conclusion:

- Phase 24.5 accepts Phase 24.4 as the first real internal backend/coordinator
  executor installation.
- The implementation is still not a user-facing execution feature. It is a
  controlled internal delegate used only when existing explicit scheduler
  execution reaches an accepted AgentTask candidate.
- Remaining risk before exposing any operator-facing execution affordance:
  live foreground child-agent smoke coverage with configured credentials,
  cancellation during a real provider call, and permission/MCP behavior during
  real child execution.
- The next safe task is Phase 25: Real Child-agent Execution Smoke And
  Cancellation Validation. It should validate the installed backend runner in
  live/fake-provider smoke paths without adding frontend controls, transport
  execution routes, background workers, auto-resume, or migrations.

## 2026-06-10: Phase 25 Real Child-agent Execution Smoke And Cancellation Validation

Phase 25 validates the installed backend/runtime executor with an internal
fake-coordinator smoke path. It exercises the real runtime scheduler execution
entry, installed `runtimeCoordinatorTaskRunner`, backend workspace routing, and
coordinator interface without adding HTTP/dev routes, Wails bindings, frontend
controls, background workers, automatic resume, database migrations, or stale
actionability recovery.

Implemented validation/hardening:

- Added runtime scheduler execute smoke coverage for:
  - queued AgentTask start through the installed backend runner;
  - backend workspace `AgentCoordinator.ExecuteConfiguredStartedAgentTask(...)`
    invocation exactly once;
  - durable prompt sourcing from `runtime_task_instruction`;
  - completed backend runner output producing artifact refs only through
    completion recorder evidence;
  - no duplicate task-start message/event evidence;
  - cancelled backend runner output producing terminal cancelled evidence with
    zero artifact refs.
- Hardened `runtimeSchedulerRecorder.AgentTaskFailed(...)` so failed and
  cancelled task evidence ignores incoming `ArtifactRefs`. This prevents
  unfinished, partial, failed, or cancelled child execution from creating
  artifact evidence even if a coordinator accidentally passes partial refs.
- For cancelled task results, the recorder now stores the terminal reason in
  `CancellationDetail` instead of treating cancellation as a generic error
  detail.

Validation:

- Focused runtime scheduler execute smoke passed:

  ```text
  go test ./internal/runtime -run "TestRuntimeRunSchedulerExecuteTask" -count=1
  ```

- Related package regression passed:

  ```text
  go test ./internal/agent ./internal/backend ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1
  ```

Review conclusion:

- Phase 25 accepts the internal fake-coordinator child-agent execution smoke
  and the no-artifact-on-failed/cancelled hardening.
- The test is intentionally credential-free and writes no secrets, OAuth
  state, provider auth state, screenshots, or live logs to the repository.
- The implementation remains backend/internal. No client execution affordance,
  transport action, background scheduler loop, auto-resume path, Run migration,
  or stale permission/MCP actionability recovery was added.
- Remaining risk: this phase validates the backend/coordinator interface with
  fake coordinator evidence, not a real hosted/provider call. Provider-specific
  live smoke still requires credentials and must be redacted/manual unless a
  safe local fake provider can cover the behavior.
- The next safe task is Phase 25.1: Real Provider/Hosted MCP Redacted Smoke
  Checklist And Gap Review. It should document or execute redacted credential-
  safe live smoke for child-agent execution, cancellation, permissions, MCP,
  and completed-output refs without storing secrets or adding UI/transport
  execution actions.

## 2026-06-10: Phase 25.1 Real Provider/Hosted MCP Redacted Smoke Checklist And Gap Review

Phase 25.1 reviews the live-provider/hosted-MCP validation gap after the
internal backend runner smoke. It does not add code, transport routes, Wails
bindings, frontend controls, background workers, automatic resume, migrations,
or stale permission/MCP actionability recovery.

Credential-safe result:

- Real hosted provider execution, browser OAuth, and provider-specific hosted
  MCP elicitation were not automated because this workspace has no safe
  operator-held credentials or browser auth state available for automation.
- A redacted local checklist was recorded at
  `tmp/runtime-dev/phase-25.1-provider-hosted-smoke-redacted.md`. This path is
  ignored by git and contains no secrets, OAuth tokens, cookies, browser
  profiles, provider auth state, raw headers, screenshots, or live provider
  logs.
- Deterministic repo coverage remains the accepted validation substitute until
  a credentialed operator can run the manual smoke:
  - Phase 25 proves scheduler execution reaches the installed backend runner
    and workspace coordinator path.
  - Phase 25 proves completed backend runner evidence is the only task ref
    source and failed/cancelled evidence produces zero artifact refs.
  - Phase 6.8/6.9 coverage proves restart does not restore stale actionable
    MCP auth or elicitation requests, replay export keeps redacted terminal
    evidence, and narrow reads do not recreate actionability from events.

Manual smoke checklist retained:

- Use operator-held provider credentials outside repo fixtures.
- Use an operator-controlled browser profile outside the workspace for OAuth.
- Start a queued AgentTask through the internal scheduler path and verify the
  prompt source is the durable `runtime_task_instruction` payload.
- Verify completed child-agent/provider output creates refs only from terminal
  completion recorder evidence.
- Cancel during foreground child-agent/provider execution and verify terminal
  cancelled evidence with zero artifact refs.
- If permission, MCP auth, or elicitation is reached, record only redacted
  request kind/status through runtime APIs.
- Restart before answering hosted MCP auth/elicitation and verify
  `RecoveryStatus`, activity DTOs, and replay export do not expose stale
  actionability.

Review conclusion:

- Phase 25.1 accepts the provider/hosted MCP live-smoke gap as credential-
  gated and safely redacted, not as a blocker for the internal backend runner
  validation track.
- No secrets or auth state were committed or written to repo docs.
- The next safe task is Phase 25.2: Backend Runner Acceptance And Next Exposure
  Gate. It should decide whether the backend runner is ready for a later
  explicitly-approved transport/UI design gate, or whether more internal fake
  provider coverage is needed first.

## 2026-06-10: Phase 25.2 Backend Runner Acceptance And Next Exposure Gate

Phase 25.2 accepts the internal backend runner validation track and defines the
next exposure boundary. It does not implement transport exposure, Wails
bindings, generated client adapters, frontend controls, background workers,
automatic resume, database migrations, or stale actionability recovery.

Acceptance review:

- Phase 24.4 installed the runtime coordinator runner behind the existing
  explicit scheduler execution path.
- Phase 24.5 accepted the backend/runtime wiring as internal-only and confirmed
  runtime still does not build agents, choose models, infer prompts from prose,
  or use event/React state as source of truth.
- Phase 25 validated the installed backend runner through scheduler execution,
  backend workspace routing, and coordinator interface evidence. Completion
  refs come only from terminal completion evidence.
- Phase 25 also hardened failed/cancelled recorder semantics so partial,
  failed, cancelled, or unfinished child work cannot create artifact refs.
- Phase 25.1 recorded provider/hosted MCP live smoke as a credential-gated
  redacted manual gap. This is acceptable for the internal runner track but
  must remain visible before user-facing exposure.

Exposure decision:

- The backend runner is ready for a design gate for explicit transport exposure
  of the already-existing scheduler execute action.
- It is not yet accepted for direct frontend implementation or visible Run/task
  controls.
- The next design gate must specify:
  - HTTP/dev route and Wails method shape, if accepted;
  - generated binding/client adapter contract, if accepted;
  - request idempotency by run id + task id;
  - current durable task/run preflight checks before execution;
  - response shape as action metadata plus refresh targets only;
  - required runtime DTO re-read after action;
  - no event payload merge into task, artifact, permission, MCP, or timeline
    state;
  - no background queue/worker/poller/daemon;
  - no automatic resume or stale actionability recovery;
  - no frontend Run management UI until transport contracts pass.

Review conclusion:

- Phase 25.2 accepts internal backend runner readiness for a transport exposure
  design gate only.
- More internal fake-provider coverage is useful but not required before the
  design gate, because the current blocker is contract exposure semantics, not
  backend runner installation.
- The next safe task is Phase 26: Explicit Scheduler Execute Transport Design
  Gate. It must remain design-only unless a later implementation phase is
  explicitly accepted.

## 2026-06-10: Phase 26 Explicit Scheduler Execute Transport Design Gate

Phase 26 designs, but does not implement, a transport contract for explicitly
executing an already accepted scheduler task candidate through the internal
backend runner. No HTTP/dev route, Wails binding, generated client adapter,
frontend control, background worker, automatic resume, database migration, or
stale actionability recovery is added in this phase.

Accepted design direction:

- Expose only the existing explicit scheduler task execution action, not a
  general Run executor:

  ```text
  POST /v1/runs/{run_id}/tasks/{task_id}/execute
  RuntimeBridge.ExecuteRunTask(ctx, runID, taskID)
  ```

- The action must delegate to
  `runtimeRunSchedulerExecuteTask(ctx, RuntimeRunSchedulerExecuteTaskRequest)`
  and return `RuntimeRunSchedulerExecuteTaskResponse`.
- The response is action metadata plus refresh targets only. The UI/adapter
  must re-read durable DTOs after the action:
  - `Run(runID)` or `RunProjection(sessionID/runID)`
  - `RunSchedulerPlan(runID, taskID)`
  - `AgentTask(taskID)`
  - `TurnActivity(parentTurnID)`
  - `SessionActivityWindow` or full `SessionActivity` fallback
- Idempotency is by durable `(run_id, task_id)` state:
  - queued task: accepted and may start foreground execution once;
  - running task: accepted, `ExecutionStarted=false`, no duplicate start
    evidence;
  - terminal/unowned/missing task: rejected by current preflight with no side
    effects.
- Transport must not start a worker, queue, poller, daemon, or automatic
  resume. It is a foreground request only.
- Transport event payloads remain refresh triggers only. They must not be
  merged into timeline, diagnostics, task status, artifact evidence,
  permission state, MCP auth/elicitation state, or Run state.
- Frontend controls remain out of scope until a later implementation phase
  accepts route/binding coverage and a separate UI behavior contract.

Required implementation gate after this design:

- Add service interface method and bridge aliases for
  `RuntimeRunSchedulerExecuteTaskRequest/Response`.
- Add HTTP direct route and dev-module route tests proving method/path/body
  validation and service delegation.
- Add Wails bridge tests proving method delegation and response shape.
- Add client adapter contract tests only if adapter exposure is accepted in the
  implementation phase; otherwise keep frontend unchanged.
- Re-run scheduler execute tests proving no duplicate lifecycle evidence,
  failed/cancelled no-artifact semantics, and completed-output-only refs.
- Confirm no generated bindings/front-end controls are added unless explicitly
  included by the implementation phase.

Rejected shapes:

- No full Run executor.
- No runtime Run state machine or Run database migration.
- No background scheduler/worker/poller/daemon.
- No automatic resume.
- No stale running/waiting tool recovery.
- No stale permission gate or MCP auth/elicitation actionability recovery.
- No React state, assistant prose, transition history, or event payload source
  of truth.
- No frontend Run/task execution UI in the design gate.

Review conclusion:

- Phase 26 accepts the explicit scheduler task execute transport design.
- The next safe task is Phase 26.1: Explicit Scheduler Execute Transport
  Contract Implementation. It may add backend/service HTTP/Wails transport
  coverage for the explicit action, but must still keep frontend visible
  controls out of scope unless a later UI phase is accepted.

## 2026-06-10: Phase 26.1 Explicit Scheduler Execute Transport Contract Implementation

Phase 26.1 implements the accepted backend/service HTTP/Wails transport
contract for explicit scheduler task execution. It does not add frontend
visible controls, client adapter execute methods, generated bindings,
background workers, automatic resume, database migrations, full Run executor
behavior, or stale permission/MCP actionability recovery.

Implemented contract:

- Added `RuntimeService.ExecuteRunTask(ctx, runID, taskID)`.
- Implemented `runtimeService.ExecuteRunTask(...)` as a thin wrapper over the
  existing `runtimeRunSchedulerExecuteTask(...)` internal action.
- Added direct HTTP route:

  ```text
  POST /v1/runs/{run_id}/tasks/{task_id}/execute
  ```

- Added dev-module route support for the same method/path.
- Added `RuntimeBridge.ExecuteRunTask(ctx, runID, taskID)` and Wails-facing
  DTO aliases for `RuntimeRunSchedulerExecuteTaskResponse`.
- Kept response semantics as action metadata plus refresh targets. The route
  does not merge action/event payloads into task, artifact, permission, MCP,
  timeline, or Run state.

Validation:

- Focused transport/scheduler tests passed:

  ```text
  go test ./internal/runtime ./desktop -run "TestRuntimeHTTPServerRoutesRunSchedulerExecuteTaskToRuntimeService|TestRuntimeHTTPServerRoutesRunSchedulerPlanToRuntimeService|TestRuntimeRunSchedulerExecuteTask|TestRuntimeBridgeForwardsActivityWindowAndRunProjection|TestRuntimeBridgeForwardsDurableRunReads" -count=1
  ```

- Related package regression passed:

  ```text
  go test ./internal/agent ./internal/backend ./internal/runtime ./internal/db ./internal/runtimeapi ./desktop -count=1
  ```

Review conclusion:

- Phase 26.1 accepts backend/service HTTP/Wails transport for the explicit
  scheduler task execute action.
- The implementation still has no visible frontend control and no client
  adapter execute method. Frontend reads remain durable DTO based.
- The next safe task is Phase 26.2: Explicit Scheduler Execute Adapter
  Contract Gate. It should decide whether to expose this transport through the
  browser/Wails workbench adapter without adding visible UI controls yet.

## 2026-06-10: Phase 26.2 Explicit Scheduler Execute Adapter Contract Gate

Phase 26.2 accepts a frontend adapter contract for the explicit scheduler task
execute transport, but does not implement it. No visible UI control, generated
binding update, background worker, automatic resume, database migration, full
Run executor, or stale actionability recovery is added in this design gate.

Accepted adapter shape:

- Add an optional `executeRunTask(current, runID, taskID)` method to
  `WorkbenchAdapter`.
- Add optional `ExecuteRunTask(runID, taskID)` to the runtime bridge module
  type and HTTP bridge.
- The adapter method may call Wails `ExecuteRunTask` when available, otherwise
  the HTTP fallback:

  ```text
  POST /v1/runs/{run_id}/tasks/{task_id}/execute
  ```

- The adapter must ignore action payloads as source-of-truth and call
  `hydrateWorkbench(current, bridge)` after the action returns.
- The adapter may surface transport/action errors to callers, but it must not
  synthesize task status, artifact refs, permission state, MCP actionability,
  timeline entries, diagnostics, or Run state from the error or response body.
- `staticWorkbenchAdapter.executeRunTask(...)` should remain a no-op refresh/
  fallback, not an offline state mutation.
- No React component should call the method in this phase; visible controls are
  reserved for a later UI design/implementation gate.

Required Phase 26.3 implementation criteria:

- Update TS DTO/module types for `RuntimeRunSchedulerExecuteTaskResponse`.
- Add `runtimeHTTPBridge.ExecuteRunTask`.
- Add `WorkbenchAdapter.executeRunTask` and Wails adapter implementation that
  calls the action and then hydrates durable DTOs.
- Keep `WorkbenchShell` and `RunProjectionPreview` UI unchanged.
- Add a client contract smoke that proves the method exists, calls
  `hydrateWorkbench`/durable reads after action, and does not inspect action
  response payload for UI state.
- Run client typecheck/build or the repo's available TS validation command.

Review conclusion:

- Phase 26.2 accepts hidden adapter exposure only.
- The next safe task is Phase 26.3: Explicit Scheduler Execute Adapter
  Contract Implementation.

## 2026-06-10: Phase 26.3 Explicit Scheduler Execute Adapter Contract Implementation

Phase 26.3 implements hidden workbench adapter support for the explicit
scheduler task execute transport. It does not add visible frontend controls,
generated Wails binding updates, background workers, automatic resume,
database migrations, stale actionability recovery, full Run executor behavior,
or event/prose/React source-of-truth behavior.

Implemented contract:

- Added `WorkbenchAdapter.executeRunTask(current, runID, taskID)`.
- Added optional runtime bridge module `ExecuteRunTask(runID, taskID)` typing.
- Added HTTP fallback call to:

  ```text
  POST /v1/runs/{run_id}/tasks/{task_id}/execute
  ```

- Implemented `wailsWorkbenchAdapter.executeRunTask(...)` so it calls the
  explicit action and then immediately calls `hydrateWorkbench(current,
  bridge)`.
- Added `staticWorkbenchAdapter.executeRunTask(...)` as runtime-unavailable
  fallback only; it does not mutate offline state.
- Kept `WorkbenchShell` and `RunProjectionPreview` unchanged. No visible UI
  control calls the hidden adapter method.
- Added `client/scripts/phase263-execute-adapter-smoke.mjs` and
  `npm run smoke:phase263` to verify the contract.

Validation:

- Hidden adapter smoke passed:

  ```text
  npm run smoke:phase263
  ```

- Client build passed:

  ```text
  npm run build
  ```

Review conclusion:

- Phase 26.3 accepts hidden adapter support for explicit scheduler task
  execution.
- The adapter treats action response payload as non-authoritative and relies on
  durable rehydration.
- The next safe task is Phase 26.4: Explicit Scheduler Execute UI Gate. It
  should decide whether and where to expose a visible control, with no
  background workers, auto-resume, migrations, full Run executor behavior, or
  stale actionability recovery.

## 2026-06-10: Phase 26.4 Explicit Scheduler Execute UI Gate

Phase 26.4 accepts the visible scheduler execute UI contract as a design gate
only. It does not add React controls, new runtime actions, generated Wails
bindings, database migrations, background workers, automatic resume, stale
actionability recovery, or full Run executor behavior.

Current frontend/runtime finding:

- `RunProjectionPreview` currently renders aggregate Run evidence and the
  already-accepted checkpoint resume action.
- `RunProjectionViewModel` does not yet carry durable scheduler task candidate
  rows, task ownership proof, queue state, execution eligibility, or execution
  denial reasons.
- The hidden `WorkbenchAdapter.executeRunTask(...)` method exists, but no
  React component calls it.
- Therefore a visible execute button cannot be safely added from the existing
  aggregate projection alone. The UI would otherwise have to infer
  actionability from counts, events, or React-local state, which is explicitly
  out of scope.

Accepted future UI contract:

- The visible control may appear in `RunProjectionPreview` only after a durable
  read model exposes scheduler task candidates as explicit rows.
- Each row must include stable `runID`, `taskID`, display title/source,
  scheduler status, ownership/session evidence, `executeEligible`, and a
  non-secret disabled/denial reason.
- The control should be a restrained icon+label action on the task row, enabled
  only when the durable row says execution is eligible. Aggregate counters,
  runtime events, action responses, assistant prose, and React state must not
  synthesize eligibility.
- On click, React may call `WorkbenchAdapter.executeRunTask(current, runID,
  taskID)`, but the adapter action response remains metadata only. The adapter
  must rehydrate durable DTOs before the UI updates.
- Event envelopes may choose which durable DTO/window to refresh. Event payloads
  must not directly merge timeline, diagnostics, artifact evidence,
  permission/MCP actionability, scheduler task state, or Run status.
- Duplicate lifecycle/permission/artifact/ref/terminal events must be absorbed
  by durable rereads and stable row IDs; they must not duplicate timeline items
  or resurrect stale permission/MCP/actionability state.

Required implementation gate before visible controls:

- Add a transport-neutral scheduler task candidate read model to the workbench
  DTO/view model, preserving full `SessionActivity` and Run projection parity
  as the oracle.
- Add browser/Vite and Wails/bridge contract coverage proving event-triggered
  refresh uses durable reads, hidden execute actions rehydrate before display,
  fallback full `SessionActivity` remains valid, and duplicate terminal events
  do not resurrect stale actionability.
- Keep the implementation additive and read-model first; do not introduce a
  runtime Run store, database migration, background scheduler, auto-resume, or
  frontend-owned Run state.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 26.4 accepts the UI location and refresh/action contract, but rejects a
  visible execute control until durable scheduler task candidate rows exist.
- The next safe task is Phase 26.5: Scheduler Task Candidate Read Model Gate.
  It should define the exact DTO and transport contract that a future visible
  execute control can consume without making React/events/action responses the
  source of truth.

## 2026-06-10: Phase 26.5 Scheduler Task Candidate Read Model Gate

Phase 26.5 defines the read-model contract required before a visible scheduler
execute control can exist. It does not implement React controls, background
execution, automatic resume, database migrations, stale actionability recovery,
or a full Run state machine.

Existing backend contract:

- Runtime already exposes a read-only scheduler plan transport:

  ```text
  GET /v1/run-scheduler-plan?run_id=...&task_id=...
  ```

- `RuntimeRunSchedulerPlan` includes durable source evidence, refresh targets,
  `SessionActivity` parity, and `RuntimeRunSchedulerPlanItem` rows.
- A task plan item already carries `runID` through the enclosing plan plus
  item-level `taskID`, `sessionID`, `turnID`, `kind`, `orderKey`,
  `canSchedule`, `preflightReason`, `ownershipVerified`, `requiredPreflight`,
  `refreshTargets`, `cancellationScope`, `diagnosticsRoute`, and `taskScope`.
- The backend plan source remains read-only and `StartsWorker=false`.

Accepted frontend read model:

```ts
interface RunSchedulerTaskCandidateViewModel {
  id: string;
  runID: string;
  taskID: string;
  kind: string;
  orderKey?: string;
  sessionID?: string;
  turnID?: string;
  title?: string;
  source?: string;
  status?: string;
  executeEligible: boolean;
  disabledReason?: string;
  ownershipVerified: boolean;
  requiredPreflight: boolean;
  refreshTargets?: string[];
  cancellationScope?: string;
  diagnosticsRoute?: string;
  taskScope?: {
    allowedTools?: string[];
    capabilityScope?: string[];
    cwd?: string;
    worktree?: string;
    role?: string;
    provider?: string;
    model?: string;
    parentToolCallID?: string;
    childSessionID?: string;
  };
}
```

Mapping rules:

- `executeEligible` maps only from durable `item.canSchedule`.
- `disabledReason` maps only from durable `item.preflightReason` or a
  transport/read error message surfaced outside actionability state.
- `ownershipVerified` maps only from durable `item.ownershipVerified`.
- `title`, `source`, and `status` may be derived from durable task/plan fields
  if present; they must not be inferred from assistant prose or event payloads.
- Task rows must keep stable `id` and `taskID` keys so duplicate lifecycle,
  permission, artifact/ref, or terminal events cannot create duplicate rows.
- Terminal task rows may remain visible for diagnostics, but must map to
  `executeEligible=false` and must not resurrect stale permission, MCP auth, or
  elicitation actionability.

Accepted adapter/read contract:

- Additive adapter support may expose a hidden read method such as
  `readRunSchedulerPlan(current, request)` or hydrate candidates through the
  existing workbench hydration path.
- Browser/Vite must use the HTTP fallback. Wails may call `RunSchedulerPlan`
  only when available. Both paths must produce the same view model.
- Runtime events may request a scheduler-plan refresh target, but the payload
  may only choose the durable read; it must not merge candidate rows or
  actionability into React state.
- After `executeRunTask(...)`, the adapter must re-read durable workbench/run
  DTOs and scheduler plan rows before the UI can change.
- Full `SessionActivity` remains fallback and parity oracle for messages, tool
  calls, permissions, diagnostics, artifact evidence, interrupted summaries,
  and terminal permission/MCP semantics.

Required implementation criteria for the next phase:

- Add TS DTO/view-model types for the scheduler plan and candidate rows.
- Add hidden adapter read support for `RunSchedulerPlan` in browser HTTP and
  optional Wails bridge paths.
- Map plan items to `RunSchedulerTaskCandidateViewModel` without React-owned
  actionability or action-response state.
- Add source/contract smoke coverage proving candidates are durable-read based,
  event payloads only trigger refreshes, and `RunProjectionPreview` still has
  no visible execute button.
- Run the client build or equivalent TS validation.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 26.5 accepts the candidate read model and transport-neutral adapter
  contract.
- The next safe task is Phase 26.6: Scheduler Task Candidate Read Model
  Implementation. It should add hidden frontend adapter/read-model support and
  smoke coverage, while keeping visible execution controls out of scope.

## 2026-06-10: Phase 26.6 Scheduler Task Candidate Read Model Implementation

Phase 26.6 implements hidden frontend scheduler task candidate read-model
support. It does not add visible execute controls, background workers,
automatic resume, database migrations, stale actionability recovery, frontend
Run state ownership, or full Run executor behavior.

Implemented:

- Added `RunSchedulerTaskCandidateViewModel`,
  `RunSchedulerTaskScopeViewModel`, and `RunSchedulerPlanRequestViewModel`.
- Added hidden `WorkbenchAdapter.readRunSchedulerPlan(current, request)`.
- Added `RunProjectionViewModel.schedulerTaskCandidates` as an additive hidden
  field for durable task candidate rows.
- Added optional Wails bridge typing for `RunSchedulerPlan(...)`.
- Added browser/Vite HTTP fallback for:

  ```text
  GET /v1/run-scheduler-plan
  ```

- During workbench hydration, the adapter now reads scheduler plan rows for
  durable task IDs from the current Run projection and maps them into stable
  candidate rows.
- Candidate rows dedupe by stable `runID:taskID`.
- `executeEligible` maps only from durable `item.canSchedule`.
- `disabledReason` maps only from durable `item.preflightReason` when the item
  is not executable.
- `RunProjectionPreview` and `WorkbenchShell` remain unchanged; no visible UI
  calls `readRunSchedulerPlan` or `executeRunTask`.

Validation:

```text
npm run smoke:phase266
npm run build
```

Review conclusion:

- Phase 26.6 accepts hidden frontend durable scheduler candidate hydration.
- Actionability still comes from durable scheduler plan reads, not events,
  action responses, assistant prose, or React-local state.
- The next safe task is Phase 26.7: Scheduler Candidate Browser/Wails Contract
  Smoke. It should add targeted browser/bridge-level coverage for event-
  triggered candidate refresh and duplicate terminal/event non-resurrection
  before any visible execute control is implemented.

## 2026-06-10: Phase 26.7 Scheduler Candidate Browser/Wails Contract Smoke

Phase 26.7 adds frontend contract smoke coverage for event-triggered scheduler
candidate refresh behavior. It does not add visible scheduler execute controls,
background workers, automatic resume, database migrations, stale actionability
recovery, frontend Run state ownership, or full Run executor behavior.

Implemented:

- Added `client/scripts/phase267-scheduler-refresh-contract-smoke.mjs`.
- Added `npm run smoke:phase267`.
- The smoke verifies:
  - `WorkbenchShell` event handling only schedules `adapter.refresh(...)`.
  - Event payloads are not merged into scheduler candidates, diagnostics,
    artifacts, permission/MCP actionability, or Run state.
  - Terminal turn/tool and artifact/ref events remain refresh triggers.
  - Scheduler candidates are hydrated through durable `RunSchedulerPlan` reads.
  - Duplicate events cannot duplicate candidate rows because rows dedupe by
    stable `runID:taskID`.
  - `executeEligible` and disabled state continue to map from durable
    `canSchedule`/`preflightReason`.
  - `RunProjectionPreview` and `WorkbenchShell` still do not expose
    `readRunSchedulerPlan` or `executeRunTask` UI calls.

Validation:

```text
npm run smoke:phase267
npm run smoke:phase266
npm run build
```

Review conclusion:

- Phase 26.7 accepts the event-triggered durable refresh contract for hidden
  scheduler candidates.
- Browser/Wails source-of-truth boundaries remain intact: events choose
  refresh timing, durable reads provide state.
- The next safe task is Phase 26.8: Visible Scheduler Execute Control Gate. It
  should decide whether the existing hidden candidate read model is sufficient
  to add a visible restrained task-row control, or whether another runtime
  contract gap remains.

## 2026-06-10: Phase 26.8 Visible Scheduler Execute Control Gate

Phase 26.8 accepts a minimal visible scheduler execute control for a later
implementation phase. This is a design gate only; it does not add the visible
control, background workers, automatic resume, database migrations, stale
actionability recovery, frontend Run state ownership, or full Run executor
behavior.

Gate finding:

- The hidden candidate read model from Phase 26.6 is sufficient for a minimal
  safe control because it provides durable `runID`, `taskID`,
  `executeEligible`, `disabledReason`, ownership/preflight evidence, refresh
  targets, and stable row keys.
- The current read model is not rich enough for a polished task manager. It
  does not yet provide a durable human summary, task progress text, or full
  task status beyond scheduler eligibility. The first visible control must
  therefore be intentionally narrow and diagnostic.

Accepted visible UI contract:

- Surface: `RunProjectionPreview`, below the existing metric/tags area and
  above checkpoint resume.
- Rendering: show at most a small list of scheduler task candidate rows from
  `run.schedulerTaskCandidates`.
- Copy: use durable `title`/`taskID`, durable `source`/role if present, and
  `disabledReason` when blocked. Do not infer from assistant prose or runtime
  event payloads.
- Button: restrained icon+label action. Enabled only when
  `candidate.executeEligible === true` and an `onRunTaskExecute` handler is
  supplied.
- Click behavior: call `onRunTaskExecute(run.id, candidate.taskID)` and let the
  adapter call `executeRunTask(...)` followed by durable hydration. Do not merge
  action response payload into UI state.
- Pending UI: local loading state may disable only the clicked row while the
  action is in flight. It must not mark the task running/completed/failed.
- Error UI: local action error text may be shown as an ephemeral action error,
  but must not become durable task status, diagnostics, artifacts, permission,
  MCP actionability, or Run state.
- Terminal/blocked rows may stay visible as diagnostics, but must remain
  disabled and must not resurrect stale permission/MCP auth or elicitation
  actionability.

Required implementation criteria:

- Add `onRunTaskExecute` plumbing from `WorkbenchShell` to `Workspace` to
  `RunProjectionPreview`.
- Render candidate rows without cards-inside-cards and keep text constrained.
- Add source smoke proving:
  - `RunProjectionPreview` only reads `schedulerTaskCandidates`.
  - Enablement uses `candidate.executeEligible`.
  - Clicks call `onRunTaskExecute(run.id, candidate.taskID)`.
  - No event payload, action response, assistant prose, or React state becomes
    scheduler/task source of truth.
- Run client build.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 26.8 accepts a minimal visible execute control implementation.
- The next safe task is Phase 26.9: Visible Scheduler Execute Control
  Implementation. It should add the UI plumbing and source smoke only; no full
  scheduler UI, background worker, auto-resume, migration, stale actionability
  recovery, or full Run executor behavior.

## 2026-06-10: Phase 26.9 Visible Scheduler Execute Control Implementation

Phase 26.9 implements the minimal visible scheduler execute control accepted
by Phase 26.8. It does not add a full scheduler UI, background workers,
automatic resume, database migrations, stale actionability recovery, frontend
Run state ownership, or full Run executor behavior.

Implemented:

- Added `WorkbenchShell` execute plumbing that calls
  `adapter.executeRunTask(...)` and replaces UI state only with the hydrated
  durable view model returned by the adapter.
- Added `Workspace.onRunTaskExecute` plumbing.
- Added scheduler candidate rows to `RunProjectionPreview`, rendered only from
  durable `run.schedulerTaskCandidates`.
- Enabled each row's `Execute` button only when
  `candidate.executeEligible === true` and an execute handler is available.
- Button clicks call `onExecuteTask(run.id, candidate.taskID)`.
- Local pending/error state is limited to the clicked row/action feedback and
  does not synthesize task status, diagnostics, artifacts, permission/MCP
  actionability, or Run state.
- Added scoped CSS for compact task rows inside the existing diagnostics panel.
- Added `client/scripts/phase269-scheduler-execute-ui-smoke.mjs` and
  `npm run smoke:phase269`.
- Updated earlier source smokes whose old "no visible control" assertions were
  intentionally superseded, while keeping their durable-read/action-response
  boundaries intact.

Validation:

```text
npm run smoke:phase269
npm run smoke:phase267
npm run smoke:phase266
npm run smoke:phase263
npm run build
```

Browser/Vite check:

- Reloaded `http://localhost:5180/` in the in-app browser.
- React root rendered with `workbench-shell` present.
- Browser console error log was empty.

Review conclusion:

- Phase 26.9 accepts the first visible scheduler execute affordance.
- Source-of-truth boundaries remain intact: durable candidate rows determine
  eligibility, explicit action responses are ignored as UI state, and adapter
  hydration remains authoritative after execution.
- The next safe task is Phase 26.10: Scheduler Execute UI Runtime Smoke And
  Acceptance. It should validate the visible control against a runtime
  candidate fixture or live local runtime, including disabled terminal rows,
  duplicate refresh events, and post-click durable rehydration.

## 2026-06-10: Phase 26.10 Scheduler Execute UI Runtime Smoke And Acceptance

Phase 26.10 adds fixture-backed acceptance coverage for the visible scheduler
execute UI. It does not add background workers, automatic resume, database
migrations, stale actionability recovery, frontend Run state ownership, a full
scheduler UI, or full Run executor behavior.

Implemented:

- Added `client/scripts/phase2610-scheduler-ui-runtime-fixture-smoke.mjs`.
- Added `npm run smoke:phase2610`.
- The fixture smoke covers:
  - executable queued candidate evidence;
  - terminal/blocked candidate evidence with durable disabled reason;
  - duplicate task evidence deduped by stable `runID:taskID`;
  - preview enablement using durable `candidate.executeEligible`;
  - click routing through durable `run.id` and `candidate.taskID`;
  - local action errors remaining ephemeral;
  - adapter execution followed by durable workbench hydration;
  - shell state replacement from the hydrated view model, not action metadata.

Validation:

```text
npm run smoke:phase2610
npm run smoke:phase269
npm run build
```

Browser/Vite live check:

- `http://localhost:5180/` rendered without console errors.
- The current live page had no `RunProjectionPreview` and no scheduler
  candidate rows, so no real click was performed.
- This is recorded as a redacted runtime fixture gap rather than fabricating
  live candidate state.

Review conclusion:

- Phase 26.10 accepts fixture-backed UI/runtime contract coverage.
- Remaining risk: a real local runtime session with durable scheduler
  candidates still needs an end-to-end click smoke to verify post-click
  candidate transition in the live UI.
- The next safe task is Phase 26.11: Live Scheduler Candidate Seed And Click
  Smoke Gate. It should decide how to create a non-secret, local-only durable
  scheduler candidate fixture for end-to-end UI clicking without adding
  background workers, auto-resume, migrations, stale actionability recovery, or
  full Run executor behavior.

## 2026-06-10: Phase 26.11 Live Scheduler Candidate Seed And Click Smoke Gate

Phase 26.11 accepts the design for a live scheduler candidate seed and click
smoke. This is a gate only; it does not add the seed, click smoke, background
workers, automatic resume, database migrations, stale actionability recovery,
frontend Run state ownership, a full scheduler UI, or full Run executor
behavior.

Existing reusable runtime evidence:

- Backend runtime tests already create durable scheduler candidates with:
  - `runtimeRunSchedulerPlanLinkedTurnFixture(...)`;
  - `RuntimeRun`/`RuntimeTurn` links through the runtime stores;
  - `RuntimeAgentTask` rows written through `service.agentTasks.Upsert(...)`;
  - `runtimeRunSchedulerPlan(...)` reads that return owned executable and
    terminal/non-executable task items.
- These fixtures use temp runtime databases and do not require credentials,
  browser OAuth, generated secrets, or repository fixtures containing auth
  state.

Accepted seed approach:

- Build the live click smoke from runtime-owned durable evidence, not React
  mocks.
- Prefer a Go runtime/http integration fixture that starts a local runtime HTTP
  server over a temp SQLite database seeded with:
  - one active Run linked to one queued parent Turn;
  - one queued owned AgentTask candidate;
  - one terminal owned AgentTask candidate;
  - optional duplicate terminal/ref events to prove durable rereads do not
    duplicate rows.
- Run the frontend against that local runtime endpoint in a browser/Vite smoke
  when feasible.
- Keep all transient database files, logs, pid files, screenshots, and smoke
  output under `tmp/runtime-dev`.
- Do not add a dev-only persistent endpoint, production seed command,
  migration, or React-local fixture mode.

Accepted click-smoke assertions:

- The queued candidate row is visible and its Execute button is enabled.
- The terminal candidate row is visible and disabled with durable
  `terminal_task`/preflight reason.
- Clicking the queued row calls the explicit execute transport once.
- UI changes after click come only from adapter durable hydration.
- Duplicate lifecycle/terminal/ref events do not duplicate rows and do not
  resurrect stale permission/MCP auth or elicitation actionability.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 26.11 accepts a runtime-owned temp seed and browser click smoke design.
- The next safe task is Phase 26.12: Live Scheduler Candidate Seed And Click
  Smoke Implementation. It should implement the local-only fixture/smoke path
  under `tmp/runtime-dev` constraints and keep production runtime behavior
  unchanged.

## 2026-06-11: Phase 26.12 Live Scheduler Candidate Seed And Click Smoke Implementation

Phase 26.12 implements the runtime-owned durable seed portion of the live
scheduler candidate smoke. It does not add a production seed endpoint,
background worker, automatic resume, database migration, stale actionability
recovery, frontend Run state ownership, full scheduler UI, or full Run
executor behavior.

Implemented:

- Added `internal/runtime/runtime_scheduler_ui_seed_test.go`.
- The test seeds runtime-owned durable evidence through existing stores:
  - one active persisted Run linked to one queued parent Turn;
  - one queued owned AgentTask candidate;
  - one terminal owned AgentTask candidate.
- The test verifies through the runtime HTTP handler:
  - queued task scheduler plan is executable and ownership verified;
  - terminal task scheduler plan is disabled with durable `terminal_task`
    preflight reason;
  - explicit execute transport accepts the queued candidate once;
  - post-execute durable task state becomes running with no artifact refs;
  - scheduler execute source remains backend-only and idempotent by task ID.

Validation:

```text
go test ./internal/runtime -run "TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute|TestRuntimeRunSchedulerPlan|TestRuntimeRunSchedulerExecute" -count=1
```

Implementation note:

- A direct HTTP `/v1/sessions/{id}/run-projection` read was not used in this
  smoke because the production Run projection path calls runtime readiness
  checks (`ensureStarted`) and therefore requires normal provider/model
  configuration. The seed smoke intentionally avoids weakening that product
  guard or introducing a dev-only bypass.

Review conclusion:

- Phase 26.12 accepts runtime-owned durable seed coverage for scheduler UI
  candidates and explicit execute transport.
- Remaining risk: a full browser click smoke still needs an end-to-end local
  runtime/Vite setup with normal runtime readiness satisfied, so the UI can
  render the candidate rows from `RunProjectionViewModel.schedulerTaskCandidates`
  and click the visible button.
- The next safe task is Phase 26.13: End-to-End Browser Scheduler Click Smoke
  Gate. It should decide whether to satisfy runtime readiness with a local
  test provider/config fixture, or keep the click smoke as a manual local
  validation checklist.

## 2026-06-11: Phase 26.13 End-to-End Browser Scheduler Click Smoke Gate

Phase 26.13 reviews the remaining full browser click gap. It is a design gate
only and does not add a test provider, runtime readiness bypass, production
seed endpoint, background worker, automatic resume, database migration, stale
actionability recovery, frontend Run state ownership, full scheduler UI, or
full Run executor behavior.

Gate finding:

- The visible scheduler execute control is now covered by source smokes and a
  fixture-backed runtime seed smoke.
- A true browser click smoke additionally requires a ready runtime because
  frontend hydration reads `/v1/sessions/{id}/run-projection`, and the
  production Run projection path correctly calls `ensureStarted`.
- Satisfying `ensureStarted` automatically is not just a scheduler UI concern:
  it crosses provider/model configuration and test-provider behavior.
- Introducing a dev-only readiness bypass or React-only candidate fixture would
  weaken the source-of-truth boundary this phase is trying to validate.

Accepted path:

- Do not add a runtime readiness bypass.
- Do not add a React fixture mode for scheduler candidates.
- Keep the full browser click smoke as a manual/local validation checklist
  until a separate test-provider/config design is accepted.
- If automation is needed later, design a local test provider/config fixture as
  its own phase, with no secrets and all transient files under
  `tmp/runtime-dev`.

Manual local checklist:

1. Start a normal local runtime with a configured non-secret provider/model.
2. Seed or create a runtime-owned Run/Turn/AgentTask candidate through existing
   runtime paths.
3. Start Vite with `VITE_AGENT_BUILDER_RUNTIME_URL` pointing at that runtime.
4. Open `http://localhost:5180/`.
5. Verify `RunProjectionPreview` shows queued and terminal scheduler rows.
6. Verify the queued row's Execute button is enabled and the terminal row is
   disabled with durable preflight reason.
7. Click Execute once and verify the UI refreshes from durable hydration.
8. Verify duplicate lifecycle/ref events do not duplicate rows or resurrect
   stale permission/MCP auth or elicitation actionability.
9. Do not write secrets, provider auth state, screenshots containing secrets,
   or runtime logs containing credentials into the repo.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 26.13 accepts the manual/local browser click validation boundary.
- The next safe task is Phase 26.14: Scheduler Execute Phase Acceptance And
  Risk Review. It should review Phases 26.1-26.13, confirm no forbidden Run
  persistence/auto-resume/background scheduler behavior was introduced, and
  decide whether to pause this track or open a separate test-provider/config
  automation phase.

## 2026-06-11: Phase 26.14 Scheduler Execute Phase Acceptance And Risk Review

Phase 26.14 accepts the Phase 26 explicit scheduler execute track through the
current boundary. It is a review/acceptance phase only and does not add code,
background workers, automatic resume, database migrations, stale actionability
recovery, frontend Run state ownership, a full scheduler UI, or full Run
executor behavior.

Reviewed scope:

- Backend/service HTTP and Wails transport for explicit scheduler task
  execution.
- Frontend adapter action that executes and then rehydrates durable DTOs.
- Durable scheduler task candidate read model mapped from `RunSchedulerPlan`.
- Event-triggered refresh contract: events schedule reads only.
- Minimal visible `RunProjectionPreview` execute affordance backed by durable
  candidate rows.
- Runtime-owned seed smoke for queued and terminal scheduler candidates.
- Manual/local browser click boundary when normal runtime readiness is not
  available.

Verification summary:

- No `internal/db` migration files changed in the Phase 26.1-26.13 file set.
- No runtime Run state machine was added.
- No runtime Run store expansion was added beyond existing accepted Run/turn/
  task evidence use.
- No background scheduler, queue, poller, daemon, or automatic resume loop was
  added.
- No stale permission gate or stale MCP auth/elicitation actionability recovery
  was added.
- React renders candidate rows and local pending/error affordance only; durable
  `schedulerTaskCandidates` remain the source for execution eligibility.
- Action response payloads remain metadata; the adapter rehydrates durable DTOs
  after explicit actions.
- Runtime events remain refresh triggers and are not merged into timeline,
  diagnostics, artifacts, permission/MCP actionability, scheduler candidates,
  or Run state.

Validation already run across the accepted implementation:

```text
npm run smoke:phase263
npm run smoke:phase266
npm run smoke:phase267
npm run smoke:phase269
npm run smoke:phase2610
npm run build
go test ./internal/runtime -run "TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute|TestRuntimeRunSchedulerPlan|TestRuntimeRunSchedulerExecute" -count=1
```

Residual risks:

- Full browser click automation still needs a normally ready runtime with
  provider/model configuration. This should not be solved by a readiness
  bypass or React fixture mode.
- The visible scheduler row uses limited durable display data (`taskID`, role,
  status/preflight reason). A richer task manager should wait for a separate
  durable task display/read-model phase.
- The execute action starts queued tasks through the accepted foreground
  explicit path only; it is not a background scheduler.

Review conclusion:

- Pause the scheduler execute track at Phase 26.14.
- If more automation is desired, open a separate phase for non-secret local
  test-provider/config readiness automation and browser click smoke.
- The next safe task is Phase 27: Test Provider/Config Readiness Automation
  Gate, if the project wants to automate full browser scheduler click smoke
  without manual local provider setup.

## 2026-06-11: Phase 27 Test Provider/Config Readiness Automation Gate

Phase 27 reviews whether to automate a full browser scheduler click smoke by
making the local runtime normally ready without secrets. This is a design gate
only and does not add a test provider, provider catalog entry, production seed
endpoint, runtime readiness bypass, background worker, automatic resume,
database migration, stale actionability recovery, frontend Run state ownership,
full scheduler UI, or full Run executor behavior.

Current readiness model:

- `/v1/sessions/{id}/run-projection` correctly calls `ensureStarted`.
- `ensureStarted` requires either:
  - selected configured provider/model records with a non-empty API key; or
  - a local desktop `model.json` loaded through `applyLocalModelConfig(...)`.
- Existing tests already cover selected model persistence and local model config
  application.
- Existing fake provider fixtures are runtime tests, not a product provider
  catalog or frontend-configured provider path.

Gate finding:

- A full automated browser click smoke should not bypass `ensureStarted`.
- It should not add a React-only scheduler candidate fixture.
- It should not add a production provider catalog entry that points at a test
  server.
- The safest automation path is a local-only test harness that:
  - creates all temporary runtime/config files under `tmp/runtime-dev`;
  - starts a local fake OpenAI-compatible provider bound to loopback;
  - writes a temp desktop `model.json` pointing at that fake provider with a
    dummy non-secret token;
  - starts the runtime HTTP server against a temp runtime database seeded with
    durable Run/Turn/AgentTask evidence;
  - starts Vite with `VITE_AGENT_BUILDER_RUNTIME_URL` pointing at that runtime;
  - drives the in-app/browser test to click the visible Execute button;
  - records only redacted, non-secret logs.

Accepted constraints for any future implementation:

- All temporary scripts, logs, pid files, screenshots, and artifacts must live
  under `tmp/runtime-dev`.
- The fake provider must be local loopback only and must not require network
  credentials.
- The fake provider/config must not be added to the embedded provider catalog
  or committed as user configuration.
- The runtime must become ready through the normal config path; no
  `ensureStarted` bypass is accepted.
- The smoke must still prove post-click UI state comes from durable hydration,
  not action response payloads or event payloads.

Required implementation criteria for a future Phase 27.1:

- Add a local-only smoke harness, preferably script-driven, that starts and
  tears down:
  - fake provider;
  - temp runtime/config root;
  - runtime HTTP server;
  - Vite dev server if needed.
- Seed runtime-owned durable scheduler candidate evidence through existing
  stores or accepted test-only harness code.
- Use the browser to verify queued/terminal rows and click exactly one queued
  Execute button.
- Verify no duplicate rows and no stale permission/MCP actionability after
  duplicate refresh events.
- Run the existing Phase 26 smoke/build tests as regression coverage.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 27 accepts the design for non-secret local readiness automation, but
  does not implement it.
- The next safe task is Phase 27.1: Local Test Provider Browser Click Smoke
  Implementation, if the project wants to continue automation now.

## 2026-06-11: Phase 27.1 Local Test Provider Readiness Smoke Implementation

Phase 27.1 implements the runtime readiness portion required before a full
browser scheduler click smoke. It does not add a production provider catalog
entry, production seed endpoint, runtime readiness bypass, background worker,
automatic resume, database migration, stale actionability recovery, frontend
Run state ownership, full scheduler UI, or full Run executor behavior.

Implemented:

- Added `internal/runtime/runtime_test_provider_readiness_test.go`.
- The test creates a temp runtime root under `tmp/runtime-dev`.
- It starts a loopback fake OpenAI-compatible provider.
- It writes a temp desktop `model.json` with a dummy non-secret token pointing
  at the loopback provider.
- It starts runtime readiness through the normal `Status(...)` /
  `ensureStarted` path.
- After readiness succeeds, it seeds durable Run/Turn/AgentTask evidence
  through existing runtime stores.
- It verifies:
  - `RunProjection(session-1)` succeeds without readiness bypass;
  - the projection includes the durable AgentTask candidate;
  - `RunSchedulerPlan(runID, taskID)` returns an executable,
    ownership-verified candidate.

Validation:

```text
go test ./internal/runtime -run "TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection|TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute" -count=1
```

Review conclusion:

- Phase 27.1 accepts local non-secret provider/config readiness coverage.
- Remaining gap: browser/Vite click automation still needs a harness that
  starts runtime HTTP and Vite against this ready temp runtime, then clicks the
  visible Execute button.
- The next safe task is Phase 27.2: Browser Scheduler Click Harness Gate. It
  should decide whether to implement the process orchestration now or keep
  click validation manual.

## 2026-06-11: Phase 27.2 Browser Scheduler Click Harness Gate

Phase 27.2 reviews whether to implement full browser process orchestration for
the scheduler Execute button now. It is a design gate only and does not add a
browser harness, long-running process launcher, runtime readiness bypass,
production seed endpoint, background worker, automatic resume, database
migration, stale actionability recovery, frontend Run state ownership, full
scheduler UI, or full Run executor behavior.

Gate finding:

- Phase 27.1 closes the critical runtime readiness gap through normal local
  config and loopback fake provider.
- A full click harness is materially broader than a source or Go smoke. It must
  orchestrate:
  - temp runtime root and temp runtime DB under `tmp/runtime-dev`;
  - loopback fake provider;
  - runtime HTTP server with known token;
  - seeded durable Run/Turn/AgentTask evidence after readiness;
  - Vite dev server with `VITE_AGENT_BUILDER_RUNTIME_URL` and token;
  - in-app/browser automation against the Vite page;
  - robust process cleanup on Windows.
- Adding that directly without a harness contract risks flaky long-running
  processes and stale ports/pids under the user's active development browser.

Accepted next boundary:

- Do not implement the browser process harness in this phase.
- Keep the manual/local browser click checklist from Phase 26.13 as the current
  full-click validation path.
- Before implementation, define a small harness contract covering:
  - fixed vs dynamic ports;
  - pid/log paths under `tmp/runtime-dev`;
  - cleanup on success/failure;
  - how the browser selects the test session;
  - how seeded candidate IDs are exposed without React fixtures;
  - how screenshots/logs avoid secrets.

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 27.2 accepts deferring browser process orchestration until a dedicated
  harness contract exists.
- The next safe task is Phase 27.3: Scheduler Execute Automation Track
  Acceptance And Pause. It should summarize Phases 27-27.2 and decide whether
  to pause or open a dedicated harness-contract phase.

## 2026-06-11: Phase 27.3 Scheduler Execute Automation Track Acceptance And Pause

Phase 27.3 accepts the scheduler execute automation track through Phase 27.2.
It is a review/acceptance phase only and does not add code, browser process
orchestration, runtime readiness bypass, production seed endpoint, background
worker, automatic resume, database migration, stale actionability recovery,
frontend Run state ownership, full scheduler UI, or full Run executor behavior.

Reviewed scope:

- Phase 27 accepted a non-secret local readiness strategy for future browser
  click automation.
- Phase 27.1 implemented the readiness smoke through normal runtime config:
  temp desktop `model.json`, loopback fake OpenAI-compatible provider, and
  durable Run/Turn/AgentTask evidence under `tmp/runtime-dev`.
- Phase 27.2 rejected direct browser process orchestration without a dedicated
  harness contract.
- Existing Phase 26 scheduler execute coverage remains the accepted execution
  boundary: explicit user-triggered action, runtime revalidation, durable DTO
  hydration, and no React-owned scheduler source state.

Verification summary:

- Runtime readiness is now automatable without secrets and without bypassing
  `ensureStarted`.
- Full browser clicking still needs runtime HTTP, Vite, browser automation,
  port ownership, pid/log cleanup, and session selection rules.
- That orchestration is too broad to add implicitly as part of scheduler
  execution.
- The manual/local browser click checklist remains valid until a harness
  contract is accepted.

Validation already run across the accepted implementation:

```text
go test ./internal/runtime -run "TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection|TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute" -count=1
git diff --check
```

Residual risks:

- Full browser scheduler click automation is still not automated end to end.
- A future harness can become flaky if it uses global ports, leaves child
  processes running, writes logs outside `tmp/runtime-dev`, or depends on the
  user's active development browser state.
- Screenshots and logs must remain redacted because provider/auth state may be
  present in local runtime configuration.

Review conclusion:

- Pause the scheduler execute automation track at Phase 27.3.
- The next safe task is Phase 28: Browser Scheduler Click Harness Contract
  Gate. It should define the harness contract before any process launcher,
  browser automation, or packaged/Wails click smoke is implemented.

## 2026-06-11: Phase 28 Browser Scheduler Click Harness Contract Gate

Phase 28 defines the local-only harness contract for future browser scheduler
click automation. It is a design gate only and does not add scripts, process
launchers, runtime readiness bypasses, production seed endpoints, background
workers, automatic resume, database migrations, stale actionability recovery,
frontend Run state ownership, full scheduler UI, or full Run executor behavior.

Harness purpose:

- Prove the visible scheduler Execute button works against a normally ready
  runtime and durable scheduler candidate evidence.
- Prove the browser refresh path uses runtime DTO reads after events/actions,
  not event payloads, action payloads, React fixtures, or assistant prose.
- Prove duplicate lifecycle/permission/artifact/ref/terminal events do not
  duplicate candidate rows or resurrect stale permission/MCP actionability.

Accepted local-only contract:

1. Workspace and output:
   - All temporary files must live under
     `tmp/runtime-dev/phase28-browser-scheduler-click/`.
   - The harness may write only redacted logs, pid files, screenshots, and
     non-secret config under that directory.
   - It must not write provider secrets, OAuth state, browser auth state, or
     screenshots containing secrets into the repo.
2. Runtime readiness:
   - Runtime readiness must use the normal local config path accepted in
     Phase 27.1: temp desktop root, temp `model.json`, loopback fake
     OpenAI-compatible provider, and dummy non-secret token.
   - The harness must not bypass `ensureStarted`, add embedded provider
     catalog entries, or add production seed endpoints.
3. Runtime data:
   - Durable Run/Turn/AgentTask candidate evidence must be seeded through
     existing runtime stores or accepted test-only harness helpers.
   - Candidate identity must be exposed to the browser by selecting the seeded
     session/run through runtime DTOs or a test-only non-production harness
     manifest under `tmp/runtime-dev`, not by React fixtures.
4. Process orchestration:
   - Runtime HTTP, fake provider, and Vite must bind to loopback only.
   - Ports should be dynamically allocated where possible; any fixed fallback
     must first check ownership and fail closed if already occupied.
   - Each child process must have a pid file and redacted stdout/stderr log
     under the phase temp directory.
   - Cleanup must terminate only the pids recorded for this harness run and
     must verify paths resolve inside the phase temp directory.
5. Browser automation:
   - Browser automation should navigate to the Vite URL configured with
     `VITE_AGENT_BUILDER_RUNTIME_URL` or the dev proxy target for the harness
     runtime.
   - The test should select the seeded session through normal UI/runtime
     hydration, verify exactly one queued executable candidate, click Execute
     once, then verify durable post-click hydration.
   - The test must also verify terminal candidates are disabled and duplicate
     refresh events do not create duplicate rows.
6. Source-of-truth rules:
   - Runtime events may choose which DTO to refresh.
   - Event payloads must not directly update scheduler candidates, timelines,
     diagnostics, artifact evidence, interrupted summaries, permission state,
     or MCP actionability.
   - Action responses may confirm request metadata but must not become durable
     scheduler state; the browser must re-read runtime DTOs.
7. Packaged/Wails boundary:
   - A packaged/Wails smoke can be added only after the Vite/browser harness is
     stable or if it reuses the same runtime-owned seed and redaction rules.
   - Wails bindings are adapter-specific; browser development must continue to
     support HTTP/dev transport fallback.

Required validation for a future implementation phase:

- Start and stop fake provider, runtime HTTP, Vite, and browser automation with
  all outputs under the phase temp directory.
- Run the browser click smoke against durable scheduler candidates.
- Run existing scheduler smoke/build coverage:

  ```text
  npm run smoke:phase263
  npm run smoke:phase266
  npm run smoke:phase267
  npm run smoke:phase269
  npm run smoke:phase2610
  npm run build
  go test ./internal/runtime -run "TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection|TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute|TestRuntimeRunSchedulerPlan|TestRuntimeRunSchedulerExecute" -count=1
  ```

Validation:

- Documentation-only gate reviewed with:

  ```text
  git diff --check
  ```

Review conclusion:

- Phase 28 accepts the harness contract and keeps browser click automation
  blocked until implementation follows this contract.
- The next safe task is Phase 28.1: Local Browser Scheduler Click Harness
  Implementation. It may add local-only scripts/tests under the accepted
  contract, but must not add production seed endpoints, readiness bypasses,
  background scheduling, automatic resume, database migrations, or
  frontend-owned scheduler state.

## 2026-06-11: Phase 28.1 Local Browser Scheduler Click Harness Implementation

Phase 28.1 implements the local-only Vite/browser scheduler click harness. It
does not add production seed endpoints, runtime readiness bypasses, background
workers, automatic resume, database migrations, stale actionability recovery,
frontend Run state ownership, full scheduler UI, packaged/Wails automation, or
full Run executor behavior.

Implemented:

- Added `internal/runtime/runtime_browser_scheduler_harness_test.go`.
  - The helper is skipped by default and runs only when
    `AGENT_BUILDER_PHASE281_BROWSER_HARNESS=1`.
  - It uses `AGENT_BUILDER_DESKTOP_ROOT` under
    `tmp/runtime-dev/phase28-browser-scheduler-click/`.
  - It starts a loopback fake OpenAI-compatible provider and writes temp
    `model.json` with a dummy non-secret token.
  - It reaches readiness through normal `Status(...)` / `ensureStarted`.
  - It creates a normal runtime session, selects it through runtime service,
    seeds durable Run/Turn/AgentTask evidence, starts runtime HTTP on
    loopback, and writes a redacted harness manifest under the phase temp dir.
  - It disables the test-only foreground task runner after readiness so the
    browser click validates explicit scheduler start/hydration without running
    a background agent worker.
- Added `client/scripts/phase281-browser-scheduler-click-harness.mjs` and
  `npm run smoke:phase281`.
  - The script starts the Go helper, local Vite, and Playwright.
  - It writes pid/log/spec/config/screenshot output under
    `tmp/runtime-dev/phase28-browser-scheduler-click/`.
  - It uses local `@playwright/test` as a dev dependency instead of relying on
    global npx state.
  - The generated Playwright spec verifies:
    - backend `RunProjection` contains the queued and terminal task IDs;
    - backend `RunSchedulerPlan` marks the queued task schedulable and the
      terminal task blocked;
    - the visible `RunProjectionPreview` renders exactly two durable scheduler
      candidate rows despite duplicate terminal refresh events;
    - the queued row has an enabled Execute button;
    - the terminal row is disabled;
    - one browser click starts the queued task through runtime HTTP;
    - post-click status is verified by re-reading durable task DTOs;
    - no pending permission actionability is resurrected.
- Fixed the integration gap found by the harness:
  - Workbench hydrates `RunProjection` with `limit=24`.
  - Bounded projections intentionally must not mutate persisted Run detail.
  - Before this phase, bounded projections returned synthetic
    `run:session:<id>` IDs even when a durable Run already existed, causing UI
    `RunSchedulerPlan` calls to fail ownership checks.
  - `RunProjection` now read-only binds a bounded projection to the existing
    durable run ID when a run/session link exists, without backfilling or
    mutating persisted Run state.
  - Added a regression assertion to
    `TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail`.

Validation:

```text
go test ./internal/runtime -run "TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail|TestPhase281BrowserSchedulerClickHarnessServer|TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection|TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute" -count=1
go test ./internal/runtime -run "TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail|TestPhase281BrowserSchedulerClickHarnessServer|TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection|TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute|TestRuntimeRunSchedulerPlan|TestRuntimeRunSchedulerExecute" -count=1
npm run lint
npm run build
npm run smoke:phase263
npm run smoke:phase266
npm run smoke:phase267
npm run smoke:phase269
npm run smoke:phase2610
npm run smoke:phase281
```

Review conclusion:

- Phase 28.1 accepts the local Vite/browser scheduler click harness.
- Full packaged/Wails click validation remains unimplemented.
- The browser smoke validates explicit scheduler start and durable DTO
  hydration, not real coordinator worker completion.
- The next safe task is Phase 28.2: Browser Scheduler Harness Acceptance And
  Packaged/Wails Gate. It should review Phase 28.1 and decide whether to add a
  packaged/Wails smoke that reuses the same runtime-owned seed and redaction
  rules.

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

Implement Phase 28.2: Browser Scheduler Harness Acceptance And Packaged/Wails
Gate. Review Phase 28.1 and decide whether packaged/Wails click validation is
needed now, using the same runtime-owned seed and redaction rules.
