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
