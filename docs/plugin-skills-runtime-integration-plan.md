# Plugin And Skills Runtime Integration Plan

Status: in progress as of 2026-06-04.

This phase follows the completed plugin center UI milestone. The goal is to
move plugin-center data ownership back to the runtime boundary while keeping
React as a DTO consumer.

## Scope

- Add a runtime `RuntimePlugin` DTO as a capability-bundle summary.
- Expose `GET /v1/plugins` through the HTTP/dev transport and Wails bridge.
- Derive plugin summaries from runtime-owned Skills and MCP server DTOs.
- Keep marketplace/package governance out of scope until the later plugin
  governance phase.
- Update the React plugin center to consume `settings.plugins` instead of
  hard-coded plugin examples.
- Keep Skills tab backed by runtime skill DTOs and MCP counts backed by runtime
  MCP server DTOs.

## Current Implementation

- `RuntimePlugin` and `RuntimePluginsResponse` were added to runtime contract
  types.
- `runtimeService.Plugins` now summarizes:
  - a `Runtime Skills` bundle when skills are discovered;
  - one plugin bundle per configured MCP server, including tool/resource/prompt
    counts and lifecycle state.
- HTTP fallback supports `GET /v1/plugins`.
- Wails bridge exposes `Plugins`.
- Workbench adapter maps plugin DTOs into `RuntimePluginViewModel` and refreshes
  plugin snapshots when skills or MCP server state changes.
- Plugin center reads `settings.plugins`, provides an empty state when runtime
  has no plugin bundles, and no longer carries React-owned plugin truth.

## Validation

Completed during implementation:

```powershell
go test ./internal/runtime -run "Plugin|Skill|MCP" -count=1
go test ./desktop -count=1
cd client; npm run lint
cd client; npm run build
```

Browser verification must be run against `http://127.0.0.1:5174/` after the dev
server picks up the new runtime DTO path.

## Remaining Risks

- This is not a full plugin package model. It is a runtime capability-bundle
  summary derived from skills and MCP until package governance is designed.
- Plugin enable/disable is only authoritative where runtime has an underlying
  capability toggle. Bulk enable/disable for the skills bundle remains out of
  scope.
- Plugin details currently summarize DTO metadata; richer package README,
  trust, signing, marketplace, and install/update semantics remain later work.
