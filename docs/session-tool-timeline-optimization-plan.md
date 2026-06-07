# Session Tool Timeline Optimization Plan

## Goal

Make the session conversation timeline feel close to Codex and Claude Desktop:
messages stay readable, while thinking, permissions, and tool calls become a
compact process stream that can support long-running agent tasks.

The implementation keeps using Ant Design and Ant Design X where they fit.
Runtime state remains owned by Go; React maps runtime DTOs into view models and
renders them.

## Current Chain

1. The Go runtime persists sessions, turns, messages, tool calls, permissions,
   policy decisions, and audit events.
2. The Wails/HTTP bridge exposes session state through:
   - `/v1/sessions`
   - `/v1/sessions/{id}/messages`
   - `/v1/sessions/{id}/activity`
   - `/v1/permissions`
   - `/v1/policy`
3. `wailsWorkbenchAdapter.ts` hydrates a `WorkbenchViewModel` by merging:
   - assistant/user messages
   - grouped thinking items
   - tool calls
   - permission requests
   - turn progress
4. `Timeline.tsx` dispatches each timeline item to:
   - `ThinkingItem`
   - `ToolCallCard`
   - `PermissionGate`
   - message bubbles

This flow is workable, but long tasks need a lighter process stream and richer
tool metadata.

## Product Direction

### Timeline Rhythm

The conversation should read as:

1. User message.
2. Lightweight thinking row, collapsed by default when content exists.
3. Tool process groups:
   - `Read 8 files`
   - `Edited 2 files`
   - `Ran 3 commands`
4. Permission prompts embedded near the relevant tool when pending.
5. Assistant result.

Most tool rows should be compact by default. Details expand only when needed.
Failures should show their error output without forcing the user to hunt for it.

### Tool Categories

Frontend can initially classify tool calls heuristically. Runtime should provide
this directly over time.

- `file_read`: list files read, collapsed details.
- `file_write`: list created/overwritten files.
- `file_edit`: edited files, additions/deletions, diff preview.
- `shell`: command, duration, exit status, stdout/stderr.
- `generic`: fallback for MCP/custom tools.

### Visual Rules

- Summary rows use a small icon, muted title, status text, and chevron.
- Expanded content uses a light gray panel.
- Shell output is monospace and bounded.
- Successful commands are collapsed by default.
- Failed commands expose the error block by default.
- File edit diff uses green/red backgrounds for changed lines.
- Permission prompts are not heavy cards unless user input is required.

## Runtime Gaps

Current persisted tool-call data already includes command, stdout, stderr,
exit code, refs, sandbox state, and policy state. The main gap is stable display
metadata so React does not need to parse summaries.

Runtime now returns computed display metadata without a database migration:

```ts
interface ToolCallDisplayViewModel {
  kind?: 'file_read' | 'file_write' | 'file_edit' | 'shell' | 'generic' | string;
  title?: string;
  detail?: string;
  command?: string;
  exitCode?: number;
  durationMs?: number;
}
```

Future runtime work should derive file path lists and diff summaries from refs.
It should also normalize artifact-producing metadata for shell and MCP/custom
tools so the timeline and turn diagnostics do not need to parse prose.

Planned follow-up:

- Add filesystem-backed artifact verification in the long-conversation
  hardening plan's Phase 1.1.
- Add runtime metadata for shell-created explicit file paths when the command
  clearly writes a local target.
- Add runtime metadata for MCP/custom structured artifact refs and local path
  targets.
- Keep React as a consumer of runtime metadata only; no frontend-only artifact
  inference.

## Implementation Status

### Phase 1: Frontend Process Stream

Status: implemented.

- Refactored `ToolCallCard` into a compact expandable process row.
- Refactored `ThinkingItem` into the same lightweight rhythm.
- Refactored `PermissionGate` so pending approvals are compact and resolved
  approvals are process states.
- Resolved permissions are hidden from the main timeline; pending permissions
  remain visible and actionable.
- Pending permissions are attached under the matching tool process block when
  the tool call is visible in the timeline.
- Tool output panels use bounded monospace output and the same scrollbar color
  behavior as the chat area.
- Output, artifact, and diff refs are surfaced as compact metadata counts in
  tool details.
- Fixed touched user-facing Chinese labels.

Validation:

- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- Browser reload on `http://localhost:4174/`; verified thinking, tool, and
  permission rows render without console errors.

### Phase 2: Timeline Grouping

Status: implemented.

- Adjacent tool calls with the same `turnId` are grouped in the React render
  layer.
- Groups are conservative: permissions, messages, thinking rows, tool kind, and
  tool status break the group so approval context remains visible and failed
  reads are not merged into large successful read groups.

Validation:

- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- Browser reload verified existing non-adjacent tool calls are not merged.

### Phase 3: Runtime Metadata

Status: implemented for the Phase 3 display normalization surface.

- Runtime returns computed `display` metadata for tool calls.
- Frontend uses `display.kind`, `display.title`, `display.detail`,
  `display.target`, `display.command`, `display.exitCode`, and
  `display.durationMs` first, falling back to legacy fields.
- Runtime now also exposes additive display fields for `workingDir`,
  `primaryTarget`, `targets`, `stdoutExcerpt`, `stderrExcerpt`,
  `inputExcerpt`, `outputExcerpt`, `failureReason`, `artifactCount`,
  `diffCount`, `artifactRefs`, `diffRefs`, `artifactSummary`, and
  `diffSummary`.
- Runtime extracts file targets from common JSON input keys such as `path`,
  `file_path`, `filepath`, `file`, `target`, and `uri`.
- Runtime extracts multi-target fields such as `paths`, `files`, `targets`,
  `patterns`, `artifact_refs`, `artifacts`, and `diff_refs`.
- Runtime falls back to truncated JSON-like input summaries for common target
  fields when the original tool input is unavailable or too large.
- Runtime classifies file search tools separately from file reads and shell
  commands.
- Runtime no longer uses generic `read`/`write` summary keywords to decide the
  primary tool kind, preventing paths or shell commands from changing a tool's
  category.
- Runtime classifies shell before file tools, so shell commands containing file
  names or file-tool keywords stay `shell`.
- Runtime classifies MCP/plugin/custom tools conservatively as `generic` and
  extracts structured artifact refs/path targets from machine-readable output.
- Shell display metadata includes command, working directory, exit code,
  duration, stdout/stderr excerpts, failure reason for failed or nonzero-exit
  shell calls, artifact counts, and existing sandbox/policy metadata on the
  call.
- File edit display uses diff refs when present and a conservative synthetic
  `diffCount` for structured edit summaries that expose additions/removals.
- Runtime startup cancels unfinished persisted tool calls so old `running`
  states do not survive a backend restart.
- Existing persisted fields remain the source of truth.
- `ToolCallCard` displays runtime metadata first for command, cwd, targets,
  stdout/stderr excerpts, failure reason, artifact/diff counts, and refs.
- Nonzero shell exit codes render as failed visual status and expand by
  default, even when persisted status is `completed`.

Validation:

- `go test ./internal/runtime`
- `go test ./...`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `cd client && npm run build`
- Runtime HTTP/Vite/browser smoke:
  - runtime HTTP `http://127.0.0.1:5189`
  - Vite `http://127.0.0.1:5179`
  - fake OpenAI-compatible server `http://127.0.0.1:5193`
  - session `0ba53bcb-1774-4196-a939-c0c791d6d95c`
  - turn `1780789254668-f0650ef246d730d8`
  - verified shell stdout/stderr/exit display, write target/diff refs, read
    path containing `write`, glob/search kind, multiedit target/diff count,
    diagnostics warning recovery, and no duplicate cards after refresh.
  - follow-up local validation on `http://127.0.0.1:5190` /
    `http://127.0.0.1:5180` verified `failureReason`, long-output truncation,
    grouped shell status, browser refresh recovery from `SessionActivity`, and
    event-triggered tool-card visibility before final assistant completion.

### Phase 4: Long Task Validation

Status: implemented for the first runtime slice; broader full-repo audit is the
next validation scenario.

Runtime/tool changes:

- The `write` tool now supports `mode: "append"` in addition to the default
  overwrite behavior.
- The main coder prompt instructs long reports, documentation, audits, and
  large generated files to write a header first, then append one section at a
  time.
- This avoids failures where a very large one-shot `write` call produces
  invalid or truncated JSON before the file is written.
- Smoke validation created `tmp/runtime-dev/long-write-smoke.md` through a real
  HTTP runtime turn using one overwrite call followed by multiple append calls.
- The same validation exposed and fixed a display bug where `view` calls whose
  file path contained the word `write` were classified as `file_write`; runtime
  now prioritizes the tool name before input-summary keyword fallbacks.

Validation task:

```text
梳理 C:\Users\ytq\work\ai\crush 的全量全模块全代码，并将结果放到
C:\Users\ytq\work\ai\crush\docs。
```

Validation focus:

- Session stays recoverable after refresh.
- Tool rows stay readable at large volume.
- Running state updates without layout jumps.
- Permissions are actionable and later recoverable.
- Generated docs are complete and traceable.

## Test Plan

### Static Checks

- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`
- `go test ./internal/runtime`
- Broaden to `go test ./...` before committing or packaging.

### Browser Checks

- Existing short conversation with command output.
- Failed command output.
- Long stdout output.
- Permission pending and allowed states.
- Sidebar expanded and collapsed.
- Narrow composer layout.
- Scroll-to-bottom button position.

### Runtime Checks

- Create a session.
- Run at least one shell command.
- Trigger a file read.
- Trigger a file edit.
- Trigger a permission request.
- Refresh the UI and verify the timeline reconstructs.

### Long Task Checks

- Start the crush full-code audit task.
- Let it run through multiple tool phases.
- Verify the timeline does not become unusable.
- Verify final docs exist in `C:\Users\ytq\work\ai\crush\docs`.
- Verify session can be reopened and still shows the process stream.

## Stress Validation

### Stress 01: Multi-turn report session

Status: passed.

- Session: `67ee5724-0ad3-4698-84b8-52af7f245e2b`
- Turns: 2
- Messages: 46
- Tool calls: 28
- Output:
  - `tmp/runtime-dev/stress-01/overview.md`
  - `tmp/runtime-dev/stress-01/checklist.md`

Coverage:

- Multi-turn continuity.
- Read/search/write process rows.
- Long report written with overwrite + append.
- Activity hydration after refresh.

### Stress 02: Shell policy and command output

Status: first run found a bug; fixed and passed on rerun.

Initial finding:

- Commands containing stderr redirect `2>&1` were split incorrectly at `&`.
- This made `go test ... 2>&1`, `npm run lint 2>&1`, and
  `git status --short 2>&1` look like overwrite redirections.
- The policy denied them as destructive and the agent did not produce the
  requested report.

Fix:

- `splitShellStatements` now keeps `>&` together as shell redirection syntax.
- Added regression coverage in `TestClassifyShellCommandStderrRedirectIsNotOverwrite`.

Rerun status: passed.

- Session: `509ae04a-5a93-4cff-bfd8-1a5c1c198fe5`
- Messages: 31
- Tool calls: 17
- Denied tool calls: 0
- Failed tool calls: 0
- Output:
  - `tmp/runtime-dev/stress-02b/command-validation.md`

### Stress 03: Wide module scan

Status: passed.

- Session: `4247c098-a21a-438b-8a5c-61789cdd2317`
- Messages: 40
- Tool calls: 27
- Denied tool calls: 0
- Failed tool calls: 0
- Tool kinds observed:
  - `file_edit`
  - `file_read`
  - `file_search`
  - `file_write`
  - `shell`
- Output:
  - `tmp/runtime-dev/stress-03/module-map.md`
  - `tmp/runtime-dev/stress-03/ui-cases.md`

Coverage:

- Wide keyword search.
- At least 8 files read across runtime, tools, adapter, and UI layers.
- Multiple generated artifacts.
- Large file-write timeline rendering.

### Phase 5: Runtime Event Refresh

Status: implemented.

- `WorkbenchAdapter` now exposes optional runtime event subscription.
- The Wails adapter connects to `EventsEndpoint()` and the HTTP/Vite adapter
  connects to `/v1/events` with query-token auth for `EventSource`.
- When a WebView does not expose `EventSource`, the adapter falls back to
  lightweight `/v1/events` polling and still only uses events as refresh
  triggers.
- `WorkbenchShell` uses runtime events as a fast refresh trigger and keeps the
  existing busy polling as a fallback.
- React still hydrates from `SessionActivity`; SSE events only trigger refresh
  and do not become the source of truth.

Validation:

- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`

### Phase 6: Interrupted Recovery Surface

Status: implemented as part of the long-conversation hardening Phase 5 slice.

- Runtime exposes interrupted recovery metadata through hydrated
  `SessionActivity` turns.
- React renders the interrupted recovery surface next to the diagnostics panel
  and keeps runtime events as refresh triggers only.
- Tool cards continue to use Phase 3 display metadata for target paths, refs,
  diff counts, policy metadata, command/cwd/exit, and output excerpts.
- No timeline duplicate items were observed after browser reload of the
  interrupted validation session.

Follow-up before the next timeline/runtime phase:

- Phase 5.1 added browser validation for pending permission recovery, denied
  permission diagnostics, and nonzero shell interrupted recovery.
- Phase 5.1 fixed the new-chat active-session handoff; the next prompt after
  new chat no longer reuses the previous session id.
- Phase 5.1 added a close-live MCP/custom structured-ref interrupted fixture
  that validates hydrated `SessionActivity` and the browser UI. A true external
  MCP server end-to-end fixture is still useful before promoting generic tool
  artifact refs to a stronger production guarantee.

### Phase 7: Run Design Gate Follow-up

Status: planned only after the long-conversation hardening Phase 6 design gate.

- Phase 6 defines Run as an additive future contract, not as a replacement for
  timeline/tool/diagnostics state.
- `SessionActivity` remains the current source of truth for timeline,
  diagnostics, and interrupted recovery surfaces.
- Runtime events remain refresh triggers only.
- Before any Run UI or runtime store is implemented, close the external MCP
  interrupted structured refs fixture, Wails packaged new-chat/recovery smoke,
  pending-at-interruption lifecycle semantics, and narrow hydration design
  follow-ups.
