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
