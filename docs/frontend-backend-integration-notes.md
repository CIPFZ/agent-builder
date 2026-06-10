# Frontend / Backend Integration Notes

Status: active notes for current Agent Builder frontend/backend integration.

## 2026-05-31: Provider Settings Integration Issue

### Symptom

Settings -> 服务商 showed an empty supported-provider list in the Vite browser
preview, even though the backend API returned provider data.

The failing UI path was:

```text
Settings -> 服务商 -> 添加服务商 -> 支持的服务商
```

The backend path was correct:

```text
GET /v1/config/providers
```

It returned the provider catalog, including `AIHubMix`, `DeepSeek`, and
`Custom`.

### Root Cause

The frontend adapter path was mixing desktop-only Wails bindings with browser
development runtime assumptions.

Important findings:

- Wails generated binding files can exist under `/bindings/...`, but they import
  the real Wails runtime, such as `/wails/runtime.js`.
- The Vite browser preview is not the Wails desktop runtime, so generated Wails
  bindings are not a reliable transport there.
- The Codex in-app browser used during development did not expose standard
  browser request APIs:

```text
window.fetch === undefined
window.XMLHttpRequest === undefined
```

Because of that, replacing the adapter with axios would not solve the problem.
Axios browser transports also depend on XHR or fetch.

There was a second adapter failure mode: `ProviderCatalog` could be fetched
successfully, but a later workbench hydration request such as sessions, models,
or configured providers could fail or return an unexpected shape. The adapter
then fell back to the static view model, which erased the provider catalog and
made the UI look empty.

### Resolution

The runtime adapter must choose transport by environment:

```text
Wails desktop runtime
  -> Wails generated binding

Vite / browser development
  -> HTTP runtime API through /runtime-api proxy
  -> dev module / JSONP fallback when fetch and XHR are unavailable
```

Provider settings hydration must also be resilient:

- Treat provider catalog loading as an independent runtime-owned data source.
- Do not let optional workbench data failures erase already-loaded provider
  catalog data.
- Validate response shapes before mapping them into frontend view models.
- Keep static view models as empty boot placeholders only; do not put business
  mock/default provider data in React components.

## Integration Rules

- React is a product UI and view-model consumer, not the runtime state source.
- Runtime state and configuration belong in Go and SQLite where persistence is
  required.
- Wails is one adapter, not the product protocol.
- HTTP/dev transports must expose the same DTO contract as Wails methods.
- Frontend components must call a runtime adapter interface, not generated
  Wails bindings directly.
- Do not assume a development browser supports `fetch` or `XMLHttpRequest`.
- Do not assume axios changes the underlying transport availability.
- When a module has multiple backend requests, isolate optional failures so one
  failing request does not wipe unrelated successfully loaded state.

## Debug Checklist

When a frontend/backend feature looks empty or inactive:

1. Verify the backend API directly.
2. Verify the Vite proxy or dev transport directly.
3. Verify the runtime adapter receives the DTO.
4. Verify DTO-to-view-model mapping does not throw.
5. Verify React receives the final view model.
6. Verify the Ant Design component renders with a known in-memory value only as
   a temporary diagnostic step, then remove the diagnostic.

Do not leave diagnostic mock data, DOM dataset markers, or frontend-only default
business data in the codebase.

## 2026-06-07: Runtime Event Refresh Cursor

Runtime events are transport refresh triggers, not timeline state. The frontend
adapter must keep the latest event sequence and reconnect or poll with
`after=<sequence>`. EventSource receives named `runtime-event` SSE messages;
polling fallback uses `/v1/events?after=<sequence>`.

The UI must still rebuild messages, tool calls, permissions, and diagnostics
from hydrated `SessionActivity`. Lifecycle events may trigger immediate
hydration; high-frequency message/token/progress events should be coalesced.

Follow-up risks:

- Phase 2 still hydrates the whole active workbench view on visible lifecycle
  events. Very large sessions may need a narrower session-scoped refresh later.
- Current validation covers the Vite/browser path, runtime SSE tests, Wails
  binding regeneration, and deterministic fake-model long tool bursts. A
  packaged production installer smoke and a real external long-running model
  run should still be repeated before calling this production-ready.

## 2026-06-07: Interrupted Recovery DTO

Phase 5 keeps `SessionActivity` as the source of truth for interrupted UI.
Runtime events such as `turn.interrupted` and `tool.call.cancelled` only
trigger hydration. React maps the hydrated `turns[].interrupted` DTO into a
read-only recovery surface next to the diagnostics panel.

Frontend actions are intentionally low risk:

- Inspect expands structured runtime metadata.
- Copy copies the runtime-provided summary text.
- Follow-up sends a new user turn seeded from the summary; it does not replay
  the interrupted turn.
- Mark done calls the runtime action that persists the interrupted turn as
  `cancelled`.

Validation note:

- During Phase 5 browser validation, clicking "new chat" before submit still
  allowed the adapter to pass the previously active session id into `Chat`.
  This is a pre-existing active-session handoff issue and should be fixed
  separately from interrupted recovery.

Follow-up:

- Add a Phase 5.1 integration fix so `NewChat` clears any submitted session id
  before the next `Chat` request. The workbench may keep a transient composer
  view model, but the request must not reuse a stale session id after the
  runtime has cleared the active session.
- Validate the fix through the HTTP/Vite path and Wails bridge path with a
  browser flow: start an existing session, click new chat, submit a prompt, and
  verify the prompt creates or targets the new session only after hydration.
- Keep runtime events as refresh triggers only. The fix should still hydrate
  session, timeline, diagnostics, and interrupted recovery state from
  `SessionActivity`.

## 2026-06-07: New Chat Active-Session Handoff

Phase 5.1 fixed the stale active-session handoff observed during interrupted
recovery validation.

Resolution:

- Clicking new chat now clears the visible draft conversation immediately.
- The runtime adapter sets a one-shot draft-submit guard after `NewChat`.
- The next `Chat` request omits `sessionId` even if an older hydrated view
  model still contains the previous active session id.
- The guard is cleared after the runtime returns the new session id, or when
  the user explicitly selects an existing session.

This guard only constrains request parameters. It is not a frontend-owned
session source. Sessions, timeline, diagnostics, and interrupted recovery still
hydrate from runtime `SessionActivity`; runtime events still only trigger
refresh.

Browser validation on the HTTP/Vite path opened an old session, clicked new
chat, submitted `phase51-new-chat`, and confirmed the old session did not
receive the prompt while the runtime-created new session did.

## 2026-06-07: Phase 6 Run Design Gate Boundary

Phase 6 is a documentation/design gate only. It defines a minimal future Run
contract, but it does not add a runtime Run store, database migration, frontend
Run UI, or full Run state machine.

Frontend/backend rules for any future Run work:

- `SessionActivity` remains the current source of truth for timeline,
  diagnostics, and interrupted recovery UX.
- Runtime events remain refresh triggers only. They must not become React Run,
  timeline, diagnostics, or interrupted recovery state.
- A future Run view may summarize objective, artifacts, turn ids, task ids,
  checkpoints, verification, and user-triggered resume/discard actions, but it
  must hydrate those fields from runtime DTOs.
- React must not infer Run artifacts or checkpoint state from assistant prose.
- Resume must create a new user-triggered turn from an explicit checkpoint
  summary. It must not replay an interrupted turn automatically.
- Discard/acknowledgement must preserve underlying turn, tool, permission, ref,
  and audit evidence.

Follow-up validation before implementation:

- External MCP server end-to-end interrupted structured refs fixture.
- Wails packaged smoke for new-chat handoff and interrupted recovery bridge
  path.
- Pending-at-interruption lifecycle semantics review.
- Narrow session-scoped and turn-scoped hydration design for very large
  sessions.

## 2026-06-07: Phase 6.1 External MCP Structured Refs

Phase 6.1 closes the external MCP interrupted structured-ref fixture gap without
adding Run state.

The runtime now has an end-to-end stdio MCP fixture where a fake provider calls
a real external MCP tool, the MCP tool returns machine-readable JSON artifact
refs, the turn is interrupted before final assistant completion, and hydrated
`SessionActivity` restores diagnostics, target/display metadata, runtime
artifact refs, and the interrupted recovery summary.

Frontend/backend rules remain unchanged:

- `SessionActivity` is still the source of truth for timeline, diagnostics, and
  interrupted recovery UX.
- Runtime events still only trigger hydration refreshes.
- React must not infer produced artifacts, Run checkpoints, or recovery state
  from assistant prose.
- Phase 6.1 does not add a runtime Run store, database migration, frontend Run
  UI, automatic resume, or persisted interrupted acknowledgement field.

## 2026-06-07: Phase 6.2 Wails Packaged Smoke

Phase 6.2 adds a focused packaged desktop/Wails bridge smoke without adding Run
state.

The packaged smoke is `desktop/scripts/phase62-wails-packaged-smoke.ps1`. It
uses `AGENT_BUILDER_DESKTOP_ROOT` under `tmp/runtime-dev`, starts
`desktop/bin/AgentBuilder.exe`, and verifies the packaged runtime creates its
desktop runtime directories there. It also runs a desktop bridge contract test
covering `NewChat`, draft `Chat`, `Events(after)`, `SessionActivity`, and
`MarkInterruptedDone`.

Frontend/backend rules remain unchanged:

- `SessionActivity` is still the source of truth for timeline, diagnostics, and
  interrupted recovery UX.
- Runtime events still only trigger hydration refreshes.
- The one-shot new-chat guard is request hygiene; it is not a frontend-owned
  session state source.
- React must not infer produced artifacts, checkpoint state, or recovery state
  from assistant prose.
- Phase 6.2 does not add a runtime Run store, database migration, frontend Run
  UI, automatic resume, stale tool recovery, or a persisted interrupted
  acknowledgement field.

Coverage boundary:

- The smoke covers packaged executable startup and Wails bridge method
  contract. It is not a full WebView click-through automation and does not
  replace future live packaged long-turn validation.

## 2026-06-08: Phase 6.4 Narrow Activity Hydration Design

Phase 6.4 documents the future narrow activity hydration boundary for very
large sessions without adding runtime APIs, Wails methods, database migrations,
Run state, or frontend Run UI.

Current baseline:

- Full `SessionActivity(sessionID)` remains the source of truth for timeline,
  diagnostics, tool metadata, permissions, policy, and interrupted recovery.
- Runtime events are still refresh triggers only.
- React must keep mapping hydrated runtime DTOs into view models. It must not
  infer artifacts, diagnostics, interrupted recovery, or checkpoint state from
  event payloads, assistant prose, or local component state.

Future narrow hydration rules:

- Additive session-scoped reads may hydrate a bounded activity window around
  the active session tail.
- Additive turn-scoped reads may hydrate one turn's messages, tool calls,
  permissions, diagnostics, artifacts, events, and interrupted summary.
- Optional diagnostics/artifact/interrupted slices must be computed in Go from
  the same runtime evidence and helper code used by full `SessionActivity`.
- The frontend adapter may choose a narrow read based on the event envelope,
  but the returned hydrated DTO remains the only state source:
  - `turn.*`, `tool.call.*`, and `permission.*` events with a `turn_id` should
    refresh the owning turn activity.
  - message/progress bursts can stay coalesced and refresh a small session
    window.
  - session-level events refresh session metadata plus the active window.
- Full `SessionActivity` must remain the compatibility fallback and parity
  oracle until narrow hydration is implemented and validated.

Boundary:

- No automatic resume.
- No stale running/waiting tool recovery.
- No restored actionable permission gate after restart.
- No persisted interrupted acknowledgement field; `MarkInterruptedDone`
  continues to persist cancelled terminal acknowledgement semantics.
- No Run store, Run database migration, or frontend Run UI.

## 2026-06-08: Phase 6.5 MCP Structured Content

Phase 6.5 hardens the MCP wrapper path for native MCP `structuredContent`
without adding frontend-owned state or Run state.

Runtime/backend rules:

- The MCP Go client wrapper captures SDK `CallToolResult.StructuredContent`
  into tool response metadata.
- The scheduler/runtime path persists that metadata as structured tool output
  and can derive artifact refs and diagnostics from it.
- MCP servers do not need to mirror JSON structured refs as text content for
  runtime diagnostics to see machine-readable artifact evidence.
- Streamable HTTP and SSE MCP use the same SDK `CallTool` result shape after
  transport normalization; hosted auth, elicitation, disconnect, and replay
  timing remain live-provider validation risks.

Frontend rules remain unchanged:

- `SessionActivity` remains the source of truth for timeline, diagnostics,
  tool metadata, permissions, policy, and interrupted recovery.
- Runtime events still only trigger hydration refreshes.
- React must not infer artifacts, diagnostics, interrupted recovery, or
  checkpoint state from assistant prose, event payloads, or local component
  state.
- Phase 6.5 does not add automatic resume, stale tool recovery, stale
  permission actionability, narrow hydration APIs, Run store, database
  migration, or frontend Run UI.

Follow-up boundary:

- Phase 6.6 should validate live/deterministic streamable HTTP MCP and SSE MCP
  restart behavior, hosted auth, and elicitation timing while keeping
  `SessionActivity` authoritative.
- Phase 6.7 may implement the Phase 6.4 narrow hydration design only as
  additive runtime DTO/API/Wails surfaces with full `SessionActivity` as the
  parity oracle and fallback.
- Neither phase should introduce Run persistence, frontend Run UI, automatic
  resume, stale tool recovery, stale permission actionability, or
  prose-derived artifact/checkpoint inference.

## 2026-06-08: Phase 6.6 HTTP/SSE MCP Restart Validation

Phase 6.6 adds deterministic streamable HTTP MCP and SSE MCP restart fixtures
for native `structuredContent` artifact refs. The runtime path remains:
completed scheduler output -> persisted tool metadata/refs -> hydrated
`SessionActivity` diagnostics and interrupted recovery.

Frontend/backend rules remain unchanged:

- `SessionActivity` remains the source of truth for timeline, diagnostics,
  artifact refs, permissions, policy, MCP request evidence, and interrupted
  recovery.
- Runtime events remain refresh triggers only.
- React must not infer artifacts, diagnostics, interrupted recovery,
  checkpoints, or Run state from assistant prose, event payloads, partial MCP
  transport state, or component state.
- Partial structured MCP output from an unfinished tool is not artifact
  production evidence after restart. Recovery cancels the unfinished tool and
  keeps it terminal/read-only.
- Header-based MCP auth stays in runtime transport config. Hosted OAuth and
  elicitation flows remain provider/SDK validation risks and must not restore
  stale actionability after restart.
- Phase 6.6 does not add automatic resume, stale tool recovery, stale
  permission or MCP request actionability, narrow hydration APIs, Run store,
  database migration, or frontend Run UI.

## 2026-06-08: Phase 6.7 Narrow Activity Hydration

Phase 6.7 adds additive narrow activity reads while keeping full
`SessionActivity` as the fallback and parity oracle.

New runtime reads:

- `GET /v1/turns/{turn_id}/activity`
- `GET /v1/sessions/{session_id}/activity-window?limit=N`
- Wails bridge methods `TurnActivity(turnID)` and
  `SessionActivityWindow(sessionID, limit)`

Frontend/backend rules:

- Runtime events remain refresh triggers only. The adapter may use the event
  envelope to choose `TurnActivity` or `SessionActivityWindow`, but the
  returned runtime DTO is the only source for timeline, diagnostics,
  permissions, artifact evidence, and interrupted recovery.
- Failed or unavailable narrow reads fall back to full `SessionActivity`.
- The frontend may merge narrow runtime DTO items into the previously hydrated
  runtime view model, but it must not infer new artifact, checkpoint,
  permission, MCP request, or interrupted state from event payloads, assistant
  prose, or local component state.
- Vite/browser development, including the dev-module fallback path for browsers
  without `fetch` or `XMLHttpRequest`, must expose the same narrow DTO contract
  as Wails.

Boundary:

- No automatic resume.
- No stale running/waiting tool recovery.
- No restored actionable permission gate.
- No restored actionable MCP auth or elicitation request.
- No Run store, Run database migration, persisted interrupted acknowledgement
  field, or frontend Run UI.

## 2026-06-08: Phase 6.8 Hosted MCP Follow-up

Phase 6.8 hardens MCP restart semantics without adding frontend-owned state or
Run state.

Runtime/backend rules:

- Startup recovery marks stale pending/required MCP auth and elicitation
  requests as terminal `cancelled`. This uses the existing MCP request status,
  event, audit, and replay paths; it does not add a persisted acknowledgement
  field.
- Completed MCP scheduler output remains the only artifact-producing evidence.
  A streamable HTTP or SSE transport disconnect after completed output does not
  remove the persisted completed tool call or create partial artifact evidence.
- Unfinished or partial MCP tool output remains non-producing evidence and is
  cancelled on restart.
- Hosted OAuth and provider-specific elicitation flows must be validated with a
  manual smoke checklist when credentials or browser auth are required. Secrets
  and auth state must not be written to repo fixtures, logs, React state, or
  screenshots.

Frontend rules remain unchanged:

- Runtime events are refresh triggers only. MCP auth/elicitation event payloads
  may select a runtime read, but must not recreate actionable request state.
- Full `SessionActivity` remains the timeline/diagnostics/interrupted fallback.
  Narrow activity may expose terminal MCP request events as read-only evidence,
  not as UI actionability.
- React must not infer artifacts, diagnostics, MCP request state, checkpoints,
  or recovery state from assistant prose, event payloads, partial MCP transport
  state, or component state.

Boundary:

- No automatic resume.
- No stale running/waiting tool recovery.
- No restored actionable permission gate.
- No restored actionable MCP auth or elicitation request after restart.
- No Run store, Run database migration, persisted interrupted acknowledgement
  field, frontend Run UI, durable narrow cursor, or frontend rollout change.

## 2026-06-08: Phase 6.9 Narrow Activity Cursor

Phase 6.9 adds a durable cursor contract for narrow session activity windows
without changing the source-of-truth boundary.

Runtime/API rules:

- `GET /v1/sessions/{session_id}/activity-window?limit=N&cursor=C` hydrates a
  mixed-evidence window ordered by message, turn, tool call, permission, and
  runtime event anchors.
- `RuntimeActivityWindow` now returns cursor metadata: `cursor`,
  `firstCursor`, `lastCursor`, `hasMoreBefore`, `hasMoreAfter`, and
  `evidenceCount`.
- `SessionActivityCursorWindow(sessionID, cursor, limit)` is additive.
  Existing `SessionActivityWindow(sessionID, limit)` remains available as a
  compatibility fallback.
- Full `SessionActivity` remains the fallback and parity oracle. Cursor-window
  DTOs are assembled from the same runtime evidence and diagnostics helpers.

Frontend rules:

- The adapter may use an event envelope to choose `TurnActivity`,
  `SessionActivityCursorWindow`, the legacy `SessionActivityWindow`, or full
  `SessionActivity`.
- Event payloads must not be merged into timeline, diagnostics, artifact,
  interrupted, permission, or MCP actionability state.
- Cursor-window results may merge runtime-returned DTO items into the current
  hydrated view model, but duplicate lifecycle/permission/artifact/ref/terminal
  events must not create duplicate timeline items or resurrect stale
  actionability.
- Vite/browser HTTP and dev-module fallback paths must pass cursor/limit
  through to the runtime service. Packaged Wails uses
  `SessionActivityCursorWindow` when generated bindings expose it, then falls
  back to legacy narrow/full activity.

Hosted MCP rules:

- Restart recovery still cancels stale actionable MCP auth/elicitation
  requests. Narrow activity can show terminal MCP request/event evidence only;
  it must not recreate actionable hosted auth or elicitation UI from events.
- Completed scheduler output remains the only source of produced artifact refs.
  Partial/unfinished/disconnected MCP output remains non-producing and
  cancelled after restart.
- Real hosted OAuth/provider-specific elicitation smoke must stay manual and
  redacted when credentials or browser auth are required. Do not store secrets,
  tokens, cookies, browser profiles, screenshots, provider auth state, or
  React state in repo fixtures or docs.

Boundary:

- No automatic resume.
- No stale running/waiting tool recovery.
- No restored actionable permission gate.
- No restored actionable MCP auth or elicitation request.
- No Run store, Run database migration, persisted interrupted acknowledgement
  field, frontend Run UI, or prose-derived artifact/checkpoint inference.

## 2026-06-08: Phase 7 Claude Code Runtime Mapping And Run DTO Gate

Phase 7 is a documentation/design gate for a future read-only Run DTO. It maps
Agent Builder's runtime surfaces to Claude Code's `QueryEngine`, transcript,
session metadata, task state, background agent output, and explicit resume
protocol before any Run implementation.

Frontend/backend rules:

- A future Run surface must hydrate from runtime DTOs assembled in Go. React
  must not own Run lifecycle, checkpoint, artifact, permission, or MCP
  actionability state.
- The first Run DTO must be read-only and derived from existing sessions,
  turns, tool calls, permissions, `RuntimeAgentTask`, runtime events, replay,
  and `SessionActivity`.
- `SessionActivity` remains the fallback and parity oracle. A Run DTO may
  summarize corresponding activity evidence, but it must not hide or rewrite
  primitive turn/tool/permission/task evidence.
- Runtime events may select a Run DTO refresh in the same way they select
  `TurnActivity` or `SessionActivityCursorWindow`; event payloads must not be
  merged into Run state.
- Future resume UX must create an explicit user-triggered turn from a
  structured checkpoint summary. It must not auto-resume from React state,
  stale runtime events, or assistant prose.

Boundary:

- No frontend Run UI in Phase 7.
- No runtime Run store or Run database migration.
- No automatic resume or background Run scheduler.
- No stale running/waiting tool, stale permission gate, or stale MCP
  auth/elicitation actionability recovery.
- No artifact, ref, or checkpoint inference from assistant prose.

## 2026-06-08: Phase 7.1 Internal Run Projection Spike

Phase 7.1 adds an internal Go runtime `RuntimeRunProjection` and tests, but it
does not expose a frontend/backend Run contract yet.

Frontend/backend rules:

- There is no HTTP, Wails, dev-module, or React Run surface in Phase 7.1.
- The projection is assembled in Go from `SessionActivity`, turns, tool calls,
  permissions, runtime events, and `RuntimeAgentTask` evidence.
- `SessionActivity` remains the public fallback and parity oracle.
- Future transport exposure must preserve the same rule as activity hydration:
  runtime events may select a Run projection refresh, but payloads must not be
  merged into Run, checkpoint, artifact, permission, MCP actionability, or
  interrupted state.
- The read-only resume/discard action DTOs are candidate UX descriptions only.
  They must not be wired to automatic resume or persisted acknowledgement
  without a separate approved phase.

Boundary:

- No frontend Run UI.
- No runtime Run store or Run database migration.
- No automatic resume or background Run scheduler.
- No stale running/waiting tool, stale permission gate, or stale MCP
  auth/elicitation actionability recovery.

## 2026-06-08: Phase 7.2 Run Projection Transport Gate

Phase 7.2 exposes the read-only Run projection through transport adapters while
keeping React adoption out of scope.

Frontend/backend rules:

- `GET /v1/sessions/{session_id}/run-projection?limit=N&cursor=C` returns the
  same read-only runtime projection that Phase 7.1 validated.
- The Wails bridge exposes `RunProjection(req)`.
- The client runtime bridge type and HTTP fallback include optional
  `RunProjection`, but `hydrateWorkbench` and React UI do not call it.
- Runtime events may be used by a future UI only to choose when to refresh this
  DTO. Event payloads must not be merged into Run, checkpoint, artifact,
  permission, MCP actionability, or interrupted state.
- `SessionActivity` remains the fallback and parity oracle.

Boundary:

- No frontend Run UI.
- No runtime Run store or Run database migration.
- No automatic resume or background Run scheduler.
- No executable resume/discard control.
- No stale running/waiting tool, stale permission gate, or stale MCP
  auth/elicitation actionability recovery.

## 2026-06-08: Phase 7.3 Run Projection Read-only Preview

Phase 7.3 lets the client display a narrow, read-only Run projection preview
without changing the frontend/backend source-of-truth boundary.

Frontend/backend rules:

- `hydrateWorkbench` may call `RunProjection({ sessionId, limit })` after it
  identifies the active session.
- The preview maps only aggregate fields into `RunProjectionViewModel`: status,
  counts, cursor/source metadata, and the `SessionActivity` parity flag.
- Runtime events remain refresh triggers only. Event payloads do not populate
  or merge Run projection state.
- `SessionActivity` remains the source for timeline, diagnostics, permissions,
  artifact evidence, and interrupted recovery surfaces.
- The client must clear stale projection state on draft/new sessions and may
  reuse a previous projection only when its `primarySessionId` matches the
  active session.

Boundary:

- No runtime Run store or Run database migration.
- No automatic resume or background Run scheduler.
- No executable resume/discard controls and no `userActions` wiring.
- No stale running/waiting tool, stale permission gate, or stale MCP
  auth/elicitation actionability recovery.

## 2026-06-09: Phase 8 Durable Run Persistence Design Gate

Phase 8 is a design gate for future persisted Run identity. It does not change
the current frontend runtime integration.

Frontend/backend rules:

- The frontend continues to read the Phase 7.3 `RunProjectionViewModel` as a
  read-only preview.
- Future persisted Run APIs may add `GET /v1/runs` and
  `GET /v1/runs/{run_id}`, but those APIs must still hydrate from Go runtime
  evidence and return read-only DTOs until action semantics are accepted.
- Runtime events may choose whether to refresh a Run list, Run detail, or
  session projection. Event payloads must not populate Run state.
- `SessionActivity` remains the source for timeline, diagnostics, permissions,
  artifact evidence, interrupted recovery, and terminal MCP semantics.
- React state must clear or rehydrate persisted Run DTOs on session/workspace
  changes; it must not carry stale Run actionability across sessions.

Boundary:

- No frontend Run management UI in Phase 8.
- No executable resume/discard controls.
- No automatic resume or background Run scheduler.
- No persisted Run store or database migration until Phase 8.1 is separately
  accepted.

## 2026-06-09: Phase 8.1 Read-only Durable Run APIs

Phase 8.1 adds a persisted Run identity foundation, but the frontend contract
remains read-only.

Transport:

- HTTP/dev transport now exposes `GET /v1/runs` and
  `GET /v1/runs/{run_id}`.
- Wails exposes matching `Runs` and `Run` bridge methods.
- `GET /v1/runs/{run_id}` returns a persisted Run summary plus a read-only
  projection payload. The projection remains hydrated from runtime evidence and
  must be treated as a parity payload, not as an action source.

Frontend rules:

- Runtime events may trigger refreshes of the Run list, Run detail, or session
  projection. Event payloads must not be merged into Run/timeline/diagnostics/
  artifact/permission/MCP state.
- React may cache the returned DTO for rendering, but Go runtime evidence
  remains the source of truth.
- Do not add resume/discard controls or background Run UI from these endpoints.
  Action semantics need a separate accepted phase.

## 2026-06-09: Phase 8.2 Action Design Gate Frontend Boundary

Phase 8.2 is an accepted design gate for future checkpoint acknowledgement,
resume, and discard actions. It does not add frontend controls or new
executable endpoints.

Frontend boundary:

- A future resume/discard UI must call explicit runtime action endpoints and
  then refresh Run detail/session activity. It must not synthesize action
  results from event payloads or React state.
- Resume must be represented as a new user-triggered turn returned by the Go
  runtime, not as a continuation of a browser-held stream.
- Discard/acknowledgement state must come from runtime DTOs after persistence,
  not from optimistic local-only state that could resurrect stale actionability.
- Stale permission gates and MCP auth/elicitation requests must remain
  non-actionable unless current runtime stores return them as pending.

Phase 8.3 frontend boundary:

- Checkpoint acknowledgement/discard endpoints and refreshed Run detail are
  available through HTTP/dev transport and Wails.
- The frontend may refresh Run detail after those actions, but must not add
  resume execution controls or a Run management surface in Phase 8.3.
- Runtime events remain refresh triggers only.
- Local optimistic acknowledgement/discard state must be replaced by runtime
  DTO refresh; it must not become the source of actionability.

## 2026-06-09: Phase 8.4 Explicit Resume Design Gate Boundary

Phase 8.4 is an accepted design gate for explicit user-triggered resume. It did
not add a resume endpoint or frontend resume control.

Frontend boundary:

- A future resume control must call an explicit runtime action endpoint and
  refresh Run detail/session activity from Go runtime responses.
- React must not treat a clicked resume button, event payload, or cached Run DTO
  as proof that work resumed.
- Failed resume responses must leave no local optimistic resumed state.
- Stale permission/MCP actionability must still be determined only by current
  runtime stores.

Phase 8.5 frontend boundary:

- The explicit resume action endpoint and structured action response are
  available through HTTP/dev transport and Wails.
- The frontend must not wire a visible resume control in Phase 8.5 unless the
  runtime endpoint, Wails/HTTP transport, and refresh behavior are all covered
  by tests.
- Runtime events remain refresh triggers only; the action response and
  subsequent DTO refresh are the only allowed sources of resume status.
- Visible resume controls remain deferred to a separate frontend rollout gate.

## 2026-06-09: Phase 8.6 Resume Control Rollout Gate Boundary

Phase 8.6 is accepted as a frontend/runtime rollout gate. It decides how to
expose the explicit resume endpoint without moving actionability into React.

Frontend boundary:

- Visible resume controls must be derived from refreshed Run detail DTOs.
- Runtime events may trigger refresh only; payloads must not create or update
  resume controls directly.
- A resume click must call the runtime endpoint and then refresh Run detail and
  session activity.
- Failed resume responses must clear pending UI state and must not mark a
  checkpoint as resumed locally.

Phase 8.7 frontend boundary:

- Implement only one explicit checkpoint resume action in the existing
  read-only Run projection/detail surface.
- Keep pending/error state local and transient; runtime DTO refresh remains the
  source of checkpoint actionability and resumed status.
- No batch resume, automatic resume, background scheduling, or full Run
  management UI.

Phase 8.7 implementation note:

- `RunProjectionViewModel` now carries structured checkpoint DTO fields from
  the runtime projection. The resume button is rendered only from refreshed
  checkpoint `resumeEligible` state.
- `WorkbenchAdapter.resumeRunCheckpoint` calls the runtime action and then
  hydrates the workbench from runtime APIs. The action response and runtime
  events are not merged into timeline, diagnostics, artifacts, permissions, MCP
  actionability, or checkpoint state.
- The browser/dev HTTP transport maps
  `POST /v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume`.
- `wails3 task common:generate:bindings` regenerates a Wails bridge export for
  `ResumeRunCheckpoint`; a packaged app smoke is still needed before treating
  the packaged Wails path as fully shipped.

Phase 8.8 validation note:

- `npm run smoke:phase88` executes the checkpoint resume selection contract,
  verifies the Run projection component keeps a stable resume-control marker
  and local pending state, and checks the generated Wails bridge exports
  `ResumeRunCheckpoint`.
- This is a contract and bridge smoke. It does not replace a packaged WebView
  click smoke for the Ant Design button and runtime DTO refresh path.

Phase 8.9 validation note:

- Focused desktop bridge tests and the existing packaged startup smoke passed.
- The packaged click path still needs a deterministic eligible-checkpoint
  fixture before it can be automated through WebView. Until that fixture exists,
  runtime DTO refresh and Wails binding availability remain covered by
  contract/bridge smoke rather than a clicked packaged UI flow.

## 2026-06-09: Phase 9 Run Execution Cutover Design Gate

Phase 9 accepts only the design direction for a future write-capable Run
execution envelope.

Frontend boundary:

- The client continues to consume runtime DTOs and must not become the Run
  lifecycle source of truth.
- Runtime events remain refresh triggers only.
- No frontend Run management UI is introduced by Phase 9.
- A future Phase 9.1 implementation may expose new runtime DTO fields, but the
  UI must continue to reconcile them against `SessionActivity` fallback/parity
  until a replacement gate is explicitly accepted.

## 2026-06-09: Phase 9.1 Minimal Durable Run Execution Envelope

Phase 9.1 adds runtime-owned Run linkage around existing session-first turn
execution.

Frontend boundary:

- No new frontend Run UI is added.
- Existing Run projection reads remain the frontend surface.
- Runtime events remain refresh triggers only.
- The client must not infer Run lifecycle, checkpoint, permission, MCP,
  diagnostics, or artifact state from the new envelope linkage.

## 2026-06-09: Phase 9.2 Run Envelope Restart Replay Validation

Phase 9.2 validates that the Phase 9.1 Run envelope linkage does not change
frontend/backend source-of-truth boundaries after restart-style recovery.

Frontend boundary:

- No new UI behavior is added.
- Run projection remains a runtime DTO read.
- Stale tool, permission, MCP auth, and elicitation actionability must remain
  absent unless current runtime stores expose them after refresh.

## 2026-06-09: Phase 9.3 Run Envelope Acceptance

Phase 9.3 accepts the minimal durable Run envelope as a backend/runtime
boundary, not as a frontend product surface.

Frontend boundary:

- No Run management UI is introduced.
- The frontend keeps reading Run projection DTOs and session activity.
- Future Run lifecycle or scheduler work requires a separate design gate before
  any UI state or transport contract changes.

## 2026-06-09: Phase 10 Run Lifecycle Design Gate

Phase 10 accepts lifecycle vocabulary and scheduler boundaries only.

Frontend boundary:

- No frontend Run management UI is added.
- Lifecycle state must come from runtime DTO refreshes.
- Waiting/actionable state must still come from current permission/MCP runtime
  stores, never from cached Run state or event payloads.
- Any future transport changes for lifecycle history require a separate
  migration/contract gate.

## 2026-06-09: Phase 10.1 Run Transition History Migration Gate

Phase 10.1 accepts a backend migration design for Run transition history only.

Frontend boundary:

- No transport or UI changes are introduced by the design gate.
- Transition history must not become a frontend source of lifecycle truth until
  a later runtime DTO contract exposes it.
- Current Run projection and session activity reads remain unchanged.

## 2026-06-09: Phase 10.2 Run Transition History Store Foundation

Phase 10.2 adds only the backend transition-history migration and narrow store.

Frontend boundary:

- No transport, generated binding, or UI changes are introduced.
- The frontend still reads existing Run projection DTOs and session activity.
- Transition rows are not exposed as lifecycle truth in this phase.
- Runtime events remain refresh triggers only and must not be merged into
  lifecycle, permission, MCP, artifact, diagnostics, or checkpoint state.

## 2026-06-09: Phase 10.3 Run Transition Runtime Wiring Gate

Phase 10.3 accepts where backend transition-history writes may be wired later.

Frontend boundary:

- No transport, generated binding, or UI changes are introduced by the gate.
- The first runtime wiring implementation must not expose transition-history
  DTOs to React.
- Existing Run projection, Run detail, and session activity refreshes remain
  the frontend integration points.
- Transition history must not become permission, MCP auth, elicitation,
  checkpoint, artifact, diagnostics, or interrupted actionability truth.

## 2026-06-09: Phase 10.4 Run Transition Runtime Wiring

Phase 10.4 wires backend transition-history writes into accepted runtime paths.

Frontend boundary:

- No HTTP, Wails, generated binding, or React surface is added.
- Existing Run projection, Run detail, and session activity refreshes remain
  the only frontend integration points.
- Transition rows are backend audit evidence only and are not lifecycle,
  checkpoint, permission, MCP, artifact, diagnostics, or interrupted
  actionability truth.
- Runtime events continue to trigger refreshes only; event payloads are not
  merged into frontend lifecycle state.

## 2026-06-09: Phase 10.5 Read-only Transition History DTO Gate

Phase 10.5 accepts an optional read-only transition-history DTO design.

Frontend boundary:

- No HTTP, Wails, generated binding, or React surface is added by the gate.
- If implemented later, transition-history reads are audit/diagnostic evidence
  only.
- The frontend must refresh existing Run detail, RunProjection, or
  SessionActivity DTOs for lifecycle, checkpoint, permission, MCP, artifact,
  diagnostics, and interrupted state.
- Event payloads may choose to refresh a transition-history DTO, but may not
  merge transition rows into frontend source-of-truth state.

## 2026-06-09: Phase 10.6 Internal Transition History DTO

Phase 10.6 implements the read-only transition-history DTO internally only.

Frontend boundary:

- No HTTP route, Wails bridge method, generated binding, React state, or UI is
  added.
- `RunTransitionHistory` is a concrete runtime method for internal tests and is
  not part of the transport-neutral `RuntimeService` interface.
- Existing frontend reads remain Run detail, RunProjection, and
  SessionActivity.
- Transition-history transport exposure still requires a later accepted gate.

## 2026-06-09: Phase 10.7 Transition History Transport Gate

Phase 10.7 accepts read-only transport exposure for transition history.

Frontend boundary:

- The accepted next implementation may add RuntimeService, HTTP/dev, and Wails
  bridge transport for `RunTransitionHistory`.
- Frontend consumption remains out of scope.
- Generated binding smoke can be deferred until a frontend consumer or packaged
  validation need exists.
- Transport responses remain audit/diagnostic evidence only and must not become
  lifecycle, checkpoint, permission, MCP, artifact, diagnostics, interrupted,
  or tool actionability truth.

## 2026-06-09: Phase 10.8 Transition History Transport Exposure

Phase 10.8 exposes transition history through RuntimeService, HTTP/dev, and
Wails bridge delegation.

Frontend boundary:

- No client adapter consumption, React state, UI, or generated binding smoke is
  added.
- HTTP/dev exposes `GET /v1/run-transitions`.
- Wails bridge exposes `RunTransitionHistory(req)`, but frontend use remains
  deferred.
- Existing Run detail, RunProjection, and SessionActivity remain the only
  frontend lifecycle/actionability sources.

## 2026-06-09: Phase 10.9 Transition History Binding Validation Gate

Phase 10.9 accepts generated Wails binding validation before frontend use.

Frontend boundary:

- The next validation may generate Wails bindings and check the generated
  `RunTransitionHistory` export.
- No client runtime adapter consumption, React state, or UI is accepted.
- Packaged WebView click smoke is deferred until a visible frontend consumer
  exists.
- Binding availability does not make transition history a lifecycle or
  actionability source.

## 2026-06-09: Phase 10.10 Transition History Binding Smoke

Phase 10.10 adds a rerunnable generated-binding smoke for transition history.

Frontend boundary:

- `npm run smoke:phase1010` verifies the generated Wails bridge exports
  `RunTransitionHistory(req)`.
- The smoke also verifies the client runtime adapter and workbench types do not
  consume or expose `RunTransitionHistory`.
- No React state, UI, packaged click smoke, or frontend adapter consumption is
  added.
- Existing Run detail, RunProjection, and SessionActivity remain the frontend
  lifecycle/actionability sources.

## 2026-06-09: Phase 10.11 Transition History Frontend Diagnostic Gate

Phase 10.11 rejects immediate frontend consumption of transition history.

Frontend boundary:

- Generated Wails binding availability is validation evidence only; it does not
  authorize React or adapter consumption.
- There is no current diagnostic workflow that requires a transition-history
  panel, timeline overlay, or checkpoint correlation surface in the frontend.
- Existing RunProjection, persisted Run detail, SessionActivity, and narrow
  activity DTOs remain the frontend sources for lifecycle, timeline,
  diagnostics, artifact evidence, interrupted recovery, permissions, MCP
  semantics, and checkpoint actionability.
- Any future transition-history UI must be accepted separately as additive,
  read-only audit evidence and must fetch a full runtime DTO/window after event
  triggers instead of merging event payload fields into React state.

## 2026-06-09: Phase 10.12 Transition History Phase Acceptance

Phase 10.12 accepts transition history as a backend audit foundation and
defines the next boundary as a Run lifecycle source-of-truth design gate.

Frontend boundary:

- Transition history remains transport-accessible audit evidence, not a
  frontend lifecycle source.
- The frontend still derives user-visible state from refreshed runtime DTOs:
  RunProjection, persisted Run detail, SessionActivity, narrow activity,
  current permission/MCP stores, and structured checkpoint DTOs.
- The next design gate may review lifecycle source-of-truth cutover, but it
  must not add frontend Run management UI, scheduler behavior, automatic
  resume, background Run execution, or transition-derived actionability.

## 2026-06-09: Phase 11 Run Lifecycle Source-of-Truth Gate

Phase 11 accepts persisted Run detail reconciliation hardening as the next
implementation boundary.

Frontend boundary:

- Persisted Run detail may become a more reliable durable read source only
  after it is reconciled from `RunProjection` and structured runtime evidence.
- The frontend should still refresh runtime DTOs rather than cache lifecycle or
  checkpoint state locally.
- Transition rows remain audit evidence and must not drive frontend lifecycle,
  checkpoint, permission, MCP, artifact, diagnostics, or interrupted state.
- No frontend Run management UI, background scheduler behavior, automatic
  resume, or transition-derived actionability is accepted by this gate.

## 2026-06-09: Phase 11.1 Persisted Run Detail Reconciliation

Phase 11.1 hardens backend Run detail reads after projection reconciliation.

Frontend boundary:

- `Run(runID)` now returns persisted Run detail after `RunProjection` has
  refreshed the durable row.
- Frontend callers still treat runtime DTO responses as the source of truth and
  should not cache lifecycle or checkpoint actionability in React state.
- Checkpoint acknowledgement/discard markers remain persisted runtime DTO
  fields and are not derived from transition rows or event payloads.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is added.

## 2026-06-09: Phase 11.2 Run List Reconciliation Boundary

Phase 11.2 should harden Run list reconciliation to match the Phase 11.1 detail
contract.

Frontend boundary:

- Run list rows should be trusted only after backend reconciliation from
  structured runtime evidence.
- The frontend must still refresh DTOs rather than locally mutating Run list
  status from events, transition rows, or React state.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted by this boundary.

Phase 11.2 result:

- `Runs()` list reconciliation already refreshed stale rows through
  projection-backed backfill; the phase added regression coverage only.
- Frontend list consumers should continue to use backend DTO refreshes as the
  source of truth.

## 2026-06-09: Phase 11.3 Full-window-only Run Reconciliation Boundary

Phase 11.3 should prevent bounded projection windows from mutating persisted
Run detail.

Frontend boundary:

- `RunProjection(limit/cursor)` remains a read-only window for UI preview and
  parity checks.
- Only full projection/detail/list refreshes may reconcile durable Run rows.
- Frontend consumers must not treat partial projection windows as complete Run
  lifecycle, checkpoint, artifact, or diagnostics truth.

Phase 11.3 result:

- Bounded `RunProjection(limit/cursor)` reads no longer persist reconciliation
  back into `runtime_runs`.
- Full Run detail/list refreshes still reconcile durable rows through full
  projection reads.

## 2026-06-09: Phase 11.4 Reconciliation Acceptance Gate

Phase 11.4 should review the Phase 11 reconciliation slice before any larger
lifecycle execution work.

Frontend boundary:

- Persisted Run detail/list reads are reconciled backend DTOs, not React-owned
  state.
- Bounded projection/activity windows remain read-only preview evidence.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted by this gate.

Phase 11.4 result:

- Phase 11 is accepted as read/reconciliation hardening only.
- The next step is a Phase 12 Run execution ownership and scheduler design
  gate, not scheduler implementation.
- Frontend behavior remains DTO refresh only; no Run management UI or
  transition-derived actionability is accepted.

## 2026-06-09: Phase 12 Run Execution Ownership Gate

Phase 12 accepts contract-first Run ownership hardening before any scheduler
implementation.

Frontend boundary:

- The current session-first execution path remains the execution surface.
- Frontend events may trigger Run/detail/activity refreshes only; event payloads
  must not become scheduler, lifecycle, checkpoint, permission, MCP, artifact,
  diagnostics, or interrupted state.
- No frontend Run management UI, background scheduler behavior, automatic
  resume, or transition-derived actionability is accepted.
- Phase 12.1 should add backend ownership/link stability coverage, not frontend
  adapter or React changes.

## 2026-06-09: Phase 12.1 Run Ownership Preflight Coverage

Phase 12.1 adds backend-only Run ownership/link stability coverage.

Frontend boundary:

- `turn_started` transition audit evidence now requires the backend
  Run/session/turn link to exist first.
- Frontend event payloads still cannot infer lifecycle or actionability from
  transition rows.
- No frontend adapter, React state, Run management UI, scheduler behavior, or
  automatic resume change is introduced.

## 2026-06-09: Phase 12.2 Run Cancellation Ownership Boundary

Phase 12.2 should add backend-only cancellation ownership coverage.

Frontend boundary:

- Cancellation UI/transport should continue to call runtime endpoints and then
  refresh DTOs.
- Frontend must not synthesize cancelled Run state from event payloads or
  transition rows.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted.

Phase 12.2 result:

- Backend cancellation ownership coverage proves `CancelTurn` preserves Run
  links, terminalizes turn/tool evidence, records audit transition after
  terminal evidence, and reconciles Run detail/projection to `cancelled`.
- No frontend behavior changes were added.

## 2026-06-09: Phase 12.3 Run Startup Recovery Ownership Boundary

Phase 12.3 should add backend-only startup recovery ownership coverage.

Frontend boundary:

- Startup recovery must refresh backend DTOs after stale evidence is
  terminalized.
- Frontend must not restore stale tool, permission, MCP auth, or MCP
  elicitation actionability from events, transition rows, or React state.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted.

Phase 12.3 result:

- Backend recovery coverage proves Run links survive startup-style recovery,
  stale tool/permission/MCP actionability is terminalized, and recovery
  transition audit is recorded from terminal evidence.
- No frontend behavior changes were added.

## 2026-06-09: Phase 12.4 Run Checkpoint Resume Ownership Boundary

Phase 12.4 should add backend-only explicit checkpoint resume ownership
coverage.

Frontend boundary:

- Resume remains a runtime endpoint action followed by DTO refresh.
- Frontend must not auto-resume or synthesize resumed checkpoint state from
  transition rows, event payloads, or React state.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted.

Phase 12.4 result:

- Backend checkpoint resume coverage proves transition audit requires a
  concrete resumed turn and resumed-turn links stay checkpoint metadata.
- No frontend behavior changes were added.

## 2026-06-09: Phase 12.5 Run Ownership Acceptance Gate

Phase 12.5 should review Run ownership contract coverage before any scheduler
implementation is designed.

Frontend boundary:

- Frontend behavior remains DTO refresh only.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability is accepted.
- Any future scheduler design must preserve runtime-owned permission/MCP and
  checkpoint actionability.

Phase 12.5 result:

- Phase 12 is accepted as backend Run ownership contract hardening only.
- The next step is a Phase 13 scheduler design gate, not scheduler
  implementation.
- Frontend behavior remains DTO refresh only.

## 2026-06-09: Phase 13 Run Scheduler Design Gate

Phase 13 defines scheduler boundaries before any scheduler implementation.

Frontend boundary:

- The current UI must keep treating backend DTO refreshes as the source of
  truth.
- Future scheduler events may identify that Run detail, Run projection,
  TurnActivity, SessionActivityWindow, or full SessionActivity should refresh,
  but event payloads must not be merged directly into lifecycle, timeline,
  diagnostics, artifacts, interrupted recovery, checkpoint, permission, MCP
  auth, or MCP elicitation state.
- Permission, MCP auth, MCP elicitation, checkpoint, artifact, interrupted, and
  terminal semantics remain runtime-owned and DTO-backed.
- No frontend Run management UI, background scheduler behavior, automatic
  resume, transition-derived actionability, or React-owned scheduler state is
  accepted by this gate.

Phase 13 result:

- Scheduler ownership is limited to future Run-level planning/grouping,
  cancellation scope, diagnostics routing, and refresh-trigger events after
  durable Run/session/turn evidence exists.
- Frontend work remains read/refresh only until a later accepted scheduler
  implementation phase defines concrete DTOs and user actions.

## 2026-06-09: Phase 13.1 Scheduler-facing Contract Preflight

Phase 13.1 adds an internal scheduler preflight contract only.

Frontend boundary:

- The preflight is not exposed through HTTP, Wails, or React.
- Frontend behavior remains unchanged: events can trigger DTO refreshes only.
- The frontend must not synthesize scheduler readiness, lifecycle,
  permission/MCP actionability, checkpoint state, artifact evidence, or
  interrupted recovery from event payloads, transition rows, or React state.

Phase 13.1 result:

- The backend now has a read-only internal preflight DTO proving whether a
  future scheduler may consider a turn executable.
- `CanSchedule` requires durable Run/session/turn evidence and a non-terminal
  turn.
- No frontend Run management UI, scheduler behavior, automatic resume, or
  transition-derived actionability was introduced.

## 2026-06-09: Phase 13.2 Scheduler Preflight Acceptance Gate

Phase 13.2 accepts the internal preflight and keeps frontend behavior unchanged.

Frontend boundary:

- The scheduler preflight remains internal to the backend.
- No HTTP, Wails, React, or generated binding surface is added for preflight.
- Future scheduler plan events may only trigger DTO refreshes; they must not
  hydrate lifecycle, timeline, diagnostics, artifacts, interrupted recovery,
  checkpoint, permission, MCP auth, or MCP elicitation state.

Phase 13.2 result:

- The next frontend-relevant boundary is a read-only scheduler plan DTO design,
  not Run management UI or scheduler execution.
- Existing frontend sources remain Run detail, RunProjection, TurnActivity,
  SessionActivityWindow, and full SessionActivity refreshes.

## 2026-06-09: Phase 14 Run Scheduler Plan DTO Design Gate

Phase 14 defines a future scheduler plan DTO contract only.

Frontend boundary:

- No scheduler plan HTTP, Wails, generated binding, adapter, or React surface is
  added in this gate.
- A future scheduler plan DTO may be used as read-only backend planning
  evidence, but it must not become frontend lifecycle, permission, MCP,
  checkpoint, artifact, interrupted, diagnostics, or timeline truth.
- Future plan events may only identify refresh targets such as Run detail,
  RunProjection, TurnActivity, SessionActivityWindow, full SessionActivity, or
  schedulerPlan.
- Frontend Run management UI remains out of scope until a later accepted UI
  phase.

Phase 14 result:

- The accepted plan DTO shape is read-only and reports `startsWorker=false`.
- Plan item executability must be derived from the backend Phase 13.1
  preflight, not event payloads or React state.
- Checkpoint plan items may describe explicit user-triggered future work only;
  they must not auto-resume or mutate checkpoint evidence.

## 2026-06-09: Phase 14.1 Internal Read-only Scheduler Plan DTO

Phase 14.1 implements the scheduler plan DTO internally only.

Frontend boundary:

- No HTTP route, Wails bridge method, generated binding, runtime adapter method,
  or React consumer is added.
- Plan refresh targets are descriptive backend fields only. They do not hydrate
  lifecycle, timeline, diagnostics, artifacts, interrupted recovery,
  checkpoint, permission, MCP auth, or MCP elicitation state.
- `canSchedule` remains backend preflight output, not frontend state.

Phase 14.1 result:

- The backend can build read-only scheduler plan evidence for a Run/turn or
  checkpoint request.
- The source reports `startsWorker=false`.
- Checkpoint plan items do not acknowledge, discard, resume, or mutate
  checkpoint evidence.

## 2026-06-09: Phase 14.2 Scheduler Plan DTO Acceptance Gate

Phase 14.2 accepts the internal scheduler plan DTO and keeps it backend-only.

Frontend boundary:

- No scheduler plan transport is added before a concrete frontend need exists.
- Future transport exposure, if accepted, must remain read-only and must not
  create Run management UI or React-owned scheduler state.
- Frontend refresh behavior remains unchanged.

Phase 14.2 result:

- The next boundary is a minimal user-triggered scheduler worker design gate,
  not frontend transport or UI.
- The first worker design must preserve DTO refresh source-of-truth and must not
  use event payloads to hydrate lifecycle or actionability.

## 2026-06-09: Phase 15 Minimal User-triggered Scheduler Worker Design Gate

Phase 15 designs the first scheduler worker boundary without implementing it.

Frontend boundary:

- The first worker is backend-internal and still entered through the existing
  user-triggered `Chat` action.
- No frontend Run management UI, scheduler controls, generated bindings, or
  adapter methods are added.
- Events from the future worker may only trigger DTO refreshes. They must not
  hydrate lifecycle, timeline, diagnostics, artifacts, interrupted recovery,
  checkpoint, permission, MCP auth, or MCP elicitation state.

Phase 15 result:

- The accepted first worker shape is a foreground backend delegate around the
  current `Chat` turn-start path.
- It must use the internal plan DTO and Phase 13.1 preflight before delegating
  to `runChat`.
- Failed preflight must terminalize the queued turn and must not start
  execution or resurrect stale actionability.

## 2026-06-09: Phase 15.1 Foreground User-turn Scheduler Delegate

Phase 15.1 implements the first scheduler execution boundary internally.

Frontend boundary:

- The public frontend action remains `Chat`.
- No scheduler transport, generated binding, adapter method, or Run management
  UI is added.
- The new `turn.failed` event from preflight rejection remains a refresh
  trigger only; frontend state still comes from refreshed DTOs.

Phase 15.1 result:

- Backend `Chat` now requires the internal scheduler plan/preflight before
  `turn_started` transition audit and before `runChat` starts.
- Failed preflight terminalizes the queued turn and does not start execution.
- Failed preflight also terminalizes in-memory request state so status refreshes
  do not show stale busy work.
- No automatic resume, unattended background execution, or frontend scheduler
  state was introduced.

## 2026-06-09: Phase 15.2 Foreground Scheduler Delegate Acceptance Gate

Phase 15.2 accepts the foreground scheduler delegate and keeps frontend behavior
unchanged.

Frontend boundary:

- `Chat` remains the frontend entry point.
- No scheduler transport or frontend Run management UI is added.
- Preflight rejection events remain refresh triggers only.

Phase 15.2 result:

- The next relevant boundary is checkpoint-resume backend hardening, because
  explicit resume already enters through `Chat`.
- Frontend resume behavior remains runtime action followed by DTO refresh.

## 2026-06-09: Phase 16 Checkpoint Resume Scheduler Delegate Hardening

Phase 16 adds backend coverage for explicit checkpoint resume under the
foreground scheduler delegate.

Frontend boundary:

- Resume remains an explicit runtime action followed by DTO refresh.
- No automatic resume, scheduler transport, or frontend Run management UI is
  added.
- Checkpoint resume events remain refresh triggers only.

Phase 16 result:

- Checkpoint planning stays non-executable until an explicit resumed turn
  exists.
- The explicit resumed turn uses the same backend foreground delegate/preflight
  path as other user turns.
- Source checkpoint evidence is not acknowledged, discarded, or mutated by the
  scheduler delegate path.

## 2026-06-09: Phase 16.1 Scheduler Delegate Acceptance Gate

Phase 16.1 accepts the user-turn and checkpoint-resume foreground delegate
slice.

Frontend boundary:

- Frontend behavior remains existing actions followed by DTO refresh.
- No scheduler transport or Run management UI is added.
- The next boundary is backend task scheduling design, not frontend exposure.

Phase 16.1 result:

- User-turn and explicit checkpoint resume scheduler delegate contracts are
  accepted.
- Task scheduling ownership remains unimplemented and requires a separate
  design gate.

## 2026-06-09: Phase 17 Task Scheduling Ownership Design Gate

Phase 17 defines backend task scheduling ownership only.

Frontend boundary:

- No task scheduler execution, scheduler transport, or Run management UI is
  added.
- Existing task views/actions remain backed by task DTO refreshes.
- Events remain refresh triggers only and must not hydrate task lifecycle,
  permission/MCP actionability, artifact evidence, or Run status.

Phase 17 result:

- Future task plan items must require parent Run/session/turn ownership.
- Task scope fields such as allowed tools, capability scope, worktree/cwd,
  role, provider/model, and parent tool-call link must be preserved.
- Task completion may contribute artifact refs only through completed structured
  task/tool output.

## 2026-06-10: Phase 17.1 Task Scheduler Plan/Preflight Contract

Phase 17.1 adds backend-only read-only task plan/preflight coverage.

Frontend boundary:

- No task scheduler execution, scheduler transport, adapter method, or Run
  management UI is added.
- Task plan fields are backend read-only contract evidence, not frontend state.
- Existing frontend task behavior remains DTO refresh only.

Phase 17.1 result:

- Task plan items verify parent Run/session/turn ownership separately from
  executability.
- Even with valid ownership, task items remain non-executable until a later
  task scheduler implementation gate.
- Task scope fields are copied into the read-only plan and are not widened.

## 2026-06-10: Phase 17.2 Task Scheduler Plan Acceptance Gate

Phase 17.2 accepts the read-only task scheduler plan/preflight contract.

Frontend boundary:

- Task scheduler plan remains backend-internal and read-only.
- No task scheduler transport, adapter method, or Run management UI is added.
- Existing task cancellation and task detail UI behavior remains DTO refresh
  only.

Phase 17.2 result:

- The next boundary is task cancellation ownership design.
- Future scheduler task items must not own cancellation actionability until that
  boundary is accepted.

## 2026-06-10: Phase 18 Task Cancellation Ownership Design Gate

Phase 18 defines backend task cancellation ownership only.

Frontend boundary:

- Existing task cancellation remains a runtime action followed by DTO refresh.
- No scheduler-owned task cancellation UI, transport, or Run management UI is
  added.
- Task cancellation events remain refresh triggers only.

Phase 18 result:

- `CancelAgentTask(...)` remains the cancellation entry point.
- Task row, task result, and task message evidence remain cancellation truth.
- Future scheduler task items may describe cancellation scope, but they must not
  become cancellation actionability or execution authority.

## 2026-06-10: Phase 18.1 Task Cancellation Ownership Contract Coverage

Phase 18.1 adds backend contract coverage for the accepted cancellation
boundary.

Frontend boundary:

- Existing task cancellation remains a runtime action followed by DTO refresh.
- Task cancellation events remain refresh triggers only; payloads must not
  hydrate lifecycle, artifact evidence, permission/MCP actionability, or Run
  status in React state.
- Scheduler task plan items remain read-only evidence and must not be surfaced
  as executable task cancellation controls.

Phase 18.1 result:

- Active cancellation now has tests for terminal task row, task result, and
  parent-to-child control message evidence.
- Already-final cancellation has tests proving final task/result/artifact
  evidence is not rewritten.
- Scheduler task plans have tests proving cancelled task items remain
  non-executable and scope-preserving.

## 2026-06-10: Phase 18.2 Task Cancellation Ownership Acceptance Gate

Phase 18.2 accepts the backend task cancellation ownership contract.

Frontend boundary:

- Existing task cancellation UI may continue to call the runtime cancellation
  action and refresh task DTOs.
- Scheduler task plans remain read-only evidence and must not become frontend
  actionability.
- Events remain refresh triggers only; event payloads must not hydrate task
  lifecycle, artifact evidence, permission/MCP actionability, or Run status in
  React state.

Phase 18.2 result:

- `CancelAgentTask(...)` remains the cancellation entry point.
- Cancellation evidence is stable enough to inform the next design gate.
- The next frontend/backend risk to design is task scheduler execution
  transport exposure, but no task scheduler transport, worker, queue, or Run
  management UI is accepted yet.

## 2026-06-10: Phase 19 Task Scheduler Execution Design Gate

Phase 19 defines the frontend/backend boundary for future scheduler-owned task
execution before implementation.

Frontend boundary:

- No task scheduler transport method, adapter method, worker, queue, or Run
  management UI is added in this phase.
- Future task execution events may select DTO refresh targets only. Event
  payloads must not hydrate task lifecycle, diagnostics, artifact evidence,
  permission/MCP actionability, or Run status in React state.
- Any future executable task plan item must be backed by runtime DTO refreshes
  and full `SessionActivity` parity checks, not frontend-only task state.
- Task cancellation UI remains backed by `CancelAgentTask(...)` followed by
  DTO refresh.

Phase 19 result:

- The first task execution boundary must be foreground and explicit, not an
  unattended background scheduler or automatic resume path.
- Task execution must preserve parent Run/session/turn/tool ownership and
  task scope fields before side effects.
- Completed structured task/tool output is the only accepted produced-ref path.
- Phase 19.1 should add backend contract coverage before any transport or UI
  exposure is considered.

## 2026-06-10: Phase 19.1 Task Scheduler Execution Delegate Contract Coverage

Phase 19.1 adds backend-only task delegate contract coverage.

Frontend boundary:

- No task scheduler transport method, adapter method, worker, queue, or Run
  management UI is added.
- The new task delegate helper is internal and rejection-only while task plan
  items remain non-executable.
- Frontend task state remains DTO refresh based; events continue to be refresh
  triggers only.

Phase 19.1 result:

- Missing, unowned, terminal, cancelled, interrupted, and currently
  non-accepted owned task candidates are rejected without runtime events, run
  transitions, task messages, task results, or artifact evidence.
- Owned active task candidates preserve scope and parent ownership evidence in
  the plan DTO, but remain non-executable.

## 2026-06-10: Phase 19.2 Task Scheduler Execution Delegate Acceptance Gate

Phase 19.2 accepts the backend rejection-only delegate contract.

Frontend boundary:

- Task scheduler plan items remain non-executable and backend-internal.
- No task scheduler transport method, adapter method, worker, queue, automatic
  resume, or Run management UI is accepted.
- Events remain refresh triggers only and must not hydrate task lifecycle,
  artifact evidence, permission/MCP actionability, or Run status in React
  state.

Phase 19.2 result:

- The task delegate contract is stable enough to inform a later foreground
  executability implementation gate.
- The next frontend/backend risk is exposing any executable task plan path; it
  must remain DTO-refresh based and parity-checked before UI/transport exposure.

## 2026-06-10: Phase 20 Foreground Task Plan Executability Implementation Gate

Phase 20 flips only the internal foreground task plan/delegate schedulability
contract.

Frontend boundary:

- No task scheduler transport method, adapter method, worker, queue, automatic
  resume, or Run management UI is added.
- Owned active task plan items may now be schedulable internally, but frontend
  state still comes from runtime DTO refreshes.
- Event payloads remain refresh triggers only and must not hydrate task
  lifecycle, artifact evidence, permission/MCP actionability, or Run status.

Phase 20 result:

- Owned active task candidates with verified parent Run/session/turn ownership
  can pass the internal foreground delegate.
- Missing, unowned, completed, cancelled, and interrupted task candidates remain
  rejected without side effects.
- The delegate still does not start a worker or write runtime events, task
  messages/results, transitions, or artifact evidence by itself.

## 2026-06-10: Phase 20.1 Foreground Task Executability Parity And Recorder Evidence

Phase 20.1 adds backend coverage for the Phase 20 foreground schedulability
flip.

Frontend boundary:

- Full `SessionActivity` remains the fallback and parity oracle.
- Cursor-window task events remain additive refresh evidence; frontend state
  must still come from DTO refreshes.
- Delegate preflight alone does not create task refs, messages, results,
  runtime events, or transitions.

Phase 20.1 result:

- Recorder completed output is covered as the produced-ref path for task
  artifacts.
- Completed task evidence wins over executable plan refreshes: completed task
  items become `terminal_task` and non-executable.

## 2026-06-10: Phase 20.2 Foreground Task Executability Acceptance Gate

Phase 20.2 accepts internal foreground task schedulability.

Frontend boundary:

- No executable task plan path is exposed through HTTP, Wails, or frontend
  adapters yet.
- Full `SessionActivity` remains the fallback and parity oracle.
- Frontend task state remains DTO-refresh based; event payloads remain refresh
  triggers only.

Phase 20.2 result:

- Internal foreground task schedulability is stable enough to inform a
  transport/UI exposure design gate.
- Transport and UI exposure remain unaccepted until Phase 21 defines the
  adapter and source-of-truth constraints.

## 2026-06-10: Phase 21 Task Scheduler Transport And UI Exposure Design Gate

Phase 21 defines the next transport-safe boundary for task scheduler planning.

Frontend boundary:

- No task scheduler execute action, adapter method, generated binding, or React
  control is accepted.
- No frontend Run management UI is accepted.
- The only accepted next exposure is read-only scheduler plan DTO transport.
- Events remain refresh triggers only; payloads may choose which DTO to refresh
  but must not hydrate task lifecycle, artifact evidence, permission/MCP
  actionability, or Run status in React state.

Phase 21 result:

- `runtimeRunSchedulerDelegateTaskTurn(...)` remains backend-internal.
- A future read-only scheduler plan transport may expose
  `RuntimeRunSchedulerPlanResponse` for user-turn, checkpoint, or task
  candidates.
- Any executable task action requires a later implementation gate after
  read-only HTTP/Wails/browser parity is covered.

Phase 21.1 note:

- The read-only scheduler plan DTO is now exposed through
  `RuntimeService.RunSchedulerPlan(...)`, direct HTTP
  `GET /v1/run-scheduler-plan`, dev module forwarding, and Wails
  `RuntimeBridge.RunSchedulerPlan(...)`.
- The route is a DTO read only. It does not start a worker, call `Chat`, call
  `CancelAgentTask`, write scheduler evidence, or restore stale actionability.
- Frontend code may use runtime events only to choose a scheduler plan refresh.
  The scheduler plan response remains planning evidence and must not replace
  `SessionActivity`, `RunProjection`, persisted Run detail, permission/MCP
  DTOs, or artifact evidence as source of truth.

Phase 21.2 note:

- The read-only scheduler plan transport is accepted as stable planning
  evidence for HTTP/dev/Wails callers.
- It does not authorize execution. Any frontend affordance based on scheduler
  plan data must remain disabled/read-only until a separate execute action gate
  accepts ownership, idempotency, cancellation, permission/MCP, artifact, and
  refresh semantics.
- The next safe boundary is an explicit foreground task execute action design
  gate, not a background scheduler or automatic resume implementation.

Phase 22 note:

- A future task execute action is accepted only as an explicit foreground
  user-triggered path. It must re-read runtime DTO/state at execution time and
  cannot rely on scheduler plan data cached in React.
- Frontend exposure remains rejected until backend idempotency, cancellation,
  artifact evidence, permission/MCP semantics, and refresh behavior have
  contract tests.
- Events may trigger refreshes for scheduler plan, task detail/result, activity
  windows, RunProjection, Run detail, refs, permissions, and MCP requests, but
  event payloads must not merge into lifecycle or actionability state.
- Background scheduling, automatic resume, stale permission/MCP actionability
  recovery, database migrations, and frontend Run management UI remain out of
  scope.

Phase 22.1 note:

- Runtime now has an internal backend-only
  `runtimeRunSchedulerExecuteTask(...)` contract and DTOs for a future
  foreground task execute action.
- The contract revalidates durable Run/task ownership through the scheduler
  delegate and returns source metadata, but it currently returns
  `executionStarted=false` and `startsWorker=false`.
- No HTTP/dev/Wails/client adapter surface is exposed in this phase.

Phase 22.2 note:

- The backend-only execute contract is accepted as the future foreground
  execution entry point.
- Frontend and transport exposure remain rejected until the execution body has
  contract coverage for idempotency, cancellation ordering, permission/MCP
  semantics, and completed-output-only artifact evidence.

Phase 22.3 note:

- The internal execute contract can now start a queued task by moving it to
  `running`, recording one instruction message, one `task_started` event, and
  one task-start transition.
- Duplicate calls for a running task remain idempotent and do not duplicate
  evidence.
- This remains internal runtime-only. No child agent runner, HTTP/dev/Wails
  route, frontend adapter, or UI control is exposed.

Phase 22.4 note:

- The internal task start body is accepted.
- Transport/frontend exposure remains blocked until an actual child-agent
  foreground runner is designed, implemented, and accepted with cancellation,
  permission/MCP, failure/completion, and artifact evidence coverage.

Phase 22.5 note:

- The child-agent foreground runner is accepted as a backend-internal design
  direction only.
- Frontend and transport surfaces must continue to treat runtime events as
  refresh triggers and re-read runtime DTOs for task/result/ref/permission/MCP
  truth.
- No execute transport, generated binding, adapter method, React control, or
  frontend-owned task lifecycle state is accepted until the backend runner
  contract is implemented and reviewed.

Phase 22.6 note:

- The backend now has an internal, test-injectable child-agent runner contract
  behind `runtimeRunSchedulerExecuteTask`.
- This does not add a `RuntimeService` method, HTTP/dev route, Wails bridge,
  generated binding, frontend adapter method, or React control.
- Frontend behavior remains read/refresh only: event payloads can choose DTO
  refreshes, but task lifecycle, refs, permission/MCP actionability, and Run
  status must still come from runtime DTO reads.

Phase 22.7 note:

- The backend runner contract is accepted as internal-only.
- There is still no execute transport or frontend action. Browser/Wails/React
  code must continue to use read-only scheduler/task/activity DTOs and treat
  events as refresh triggers.
- A future coordinator adapter must be accepted before any transport or UI
  affordance can call foreground task execution.

Phase 23 note:

- The real coordinator adapter is design-only.
- Frontend and transport still have no execute action. The eventual adapter
  must first prove backend cancellation, permission/MCP, terminal evidence, and
  completion-only artifact semantics before any browser/Wails affordance is
  considered.

Phase 23.1 note:

- The coordinator started-task execution contract exists in backend/internal
  code only.
- Runtime, HTTP/dev, Wails, generated bindings, client adapters, and React UI
  are still not wired to real task execution.
- Frontend remains read/refresh only until runtime adapter wiring and transport
  exposure are separately accepted.

Phase 23.2 note:

- The coordinator started-task execution contract is accepted, but runtime
  wiring and all transport/frontend execution affordances remain blocked.
- Browser/Wails/React code must continue to treat task execution as unavailable
  and use read-only scheduler/task/activity DTOs for state.

Phase 23.3 note:

- Runtime-to-coordinator wiring is design-only.
- There is still no browser/Wails/React execute path. Any future transport
  exposure remains blocked until backend wiring proves durable prompt sourcing,
  terminal failure/cancellation behavior, and completion-only refs.

Phase 23.4 note:

- Runtime has an internal adapter contract and structured prompt-source
  payload, but the adapter is not installed and no execute transport/UI exists.
- Browser/Wails/React code remains read-only for task execution state and must
  continue to refresh authoritative runtime DTOs rather than deriving state
  from events or local UI memory.

Phase 23.5 note:

- The internal runtime-side adapter contract is accepted, but real executor
  installation and all transport/frontend execution affordances remain blocked.
- Frontend behavior is unchanged: read scheduler/task/activity DTOs and treat
  runtime events as refresh triggers only.

Phase 24 note:

- Real backend/coordinator executor installation is design-only.
- There is still no execute transport or frontend action; future backend work
  must first keep task-agent construction coordinator-owned and prove terminal
  evidence/cancellation semantics.

Phase 24.1 note:

- The coordinator configured started-task executor exists internally, but
  runtime is not wired to it and no transport/frontend execution affordance is
  available.
- Frontend behavior remains read-only with runtime DTO refreshes as the source
  of truth.

Phase 24.2 note:

- The coordinator configured executor contract is accepted, but backend/runtime
  wiring and all browser/Wails/React execution affordances remain blocked.
- Frontend behavior remains unchanged.

Phase 24.3 note:

- Backend/runtime executor wiring is design-only.
- No browser/Wails/React execution affordance is accepted; frontend remains
  read-only and refresh-driven.
