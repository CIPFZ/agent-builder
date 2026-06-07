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

Status: implemented for the current display surface.

- Runtime returns computed `display` metadata for tool calls.
- Frontend uses `display.kind`, `display.title`, `display.detail`,
  `display.target`, `display.command`, `display.exitCode`, and
  `display.durationMs` first, falling back to legacy fields.
- Runtime extracts file targets from common JSON input keys such as `path`,
  `file_path`, `filepath`, `file`, `target`, and `uri`.
- Runtime falls back to truncated JSON-like input summaries for common target
  fields when the original tool input is unavailable or too large.
- Runtime classifies file search tools separately from file reads and shell
  commands.
- Runtime startup cancels unfinished persisted tool calls so old `running`
  states do not survive a backend restart.
- Existing persisted fields remain the source of truth.

Validation:

- `go test ./internal/runtime`
- `cd client && npm run lint`
- `cd client && npx tsc -b --pretty false`

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
