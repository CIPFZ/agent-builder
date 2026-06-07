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
