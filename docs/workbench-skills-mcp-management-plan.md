# Workbench Skills And MCP Management Plan

Status: completed for the Phase 6 management-surface milestone on
2026-06-03.

This phase follows the completed runtime parity closure gate. The goal is to
make skills and MCP visible and operable from the desktop workbench without
moving capability truth into React.

## Scope

- Settings -> Skills consumes runtime skill DTOs and shows enabled state,
  source path, state, diagnostics, policy hints, and allowed tools.
- Settings -> Skills can refresh discovery and enable or disable a runtime
  skill through the runtime adapter.
- Settings -> MCP consumes runtime MCP server DTOs and shows configured
  servers, transport, lifecycle state, counts, diagnostics, and enable state.
- Settings -> MCP can add or edit the baseline server DTO fields supported by
  runtime, refresh a server, enable or disable a server, load
  tools/resources/prompts, and toggle MCP tools.
- Vite/browser development continues to use the HTTP/dev transport fallback;
  Wails bindings remain an adapter path only.
- Composer context shows a runtime-derived enabled skill/MCP capability
  summary.

## Completion Record

Implemented files:

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/runtime/staticWorkbenchAdapter.tsx`
- `client/src/app/shell/WorkbenchShell.tsx`
- `client/src/features/settings/SettingsPanel.tsx`
- `client/src/features/settings/SettingsPanel.module.css`
- `client/src/features/composer/Composer.tsx`
- `client/src/features/composer/Composer.module.css`

Validated behavior:

- Settings -> Skills loads runtime DTOs, refreshes discovery, and supports
  enable/disable round-trip with state restored.
- Settings -> MCP loads runtime DTOs, shows empty state, displays a temporary
  disabled MCP server from runtime API, loads details, toggles enable/disable,
  and returns to empty state after cleanup.
- Composer shows the runtime-derived capability summary, for example
  `4 skills / 0 MCP / 0 tools`.
- Browser verification used the Codex in-app browser at
  `http://127.0.0.1:5174/`.

Validation completed:

```powershell
go test ./internal/runtime -run "Scenario|Replay|Recovery|Permission|Policy|Hook|MCP|AgentTask|Sandbox|Worktree|Ref|Turn|Tool|Skill|Capability" -count=1
go test ./desktop -count=1
cd client; npm run lint
cd client; npm run build
```

Known browser console output:

- Existing Ant Design deprecation warnings for Dropdown `overlayClassName`,
  List, and Modal `destroyOnClose` remain. No new runtime or application error
  was observed during the Skills/MCP verification.

## Non-Goals

- No marketplace, plugin package governance, remote teammate runtime, or
  managed organization policy surface.
- No React-owned mock skill, MCP, capability, request, or policy state.
- No copied Claude Code branding, UI copy, or proprietary implementation.
- No project-scoped skill/MCP config until the Projects phase defines project
  ownership.

## Runtime Boundary

React consumes these runtime DTO groups through `WorkbenchAdapter`:

- `RuntimeSkillViewModel`
- `RuntimeMCPServerViewModel`
- `RuntimeMCPToolViewModel`
- `RuntimeMCPResourceViewModel`
- `RuntimeMCPPromptViewModel`

The adapter maps Go DTOs returned by existing runtime APIs:

- `GET /v1/skills`
- `POST /v1/skills/refresh`
- `POST /v1/skills/{name}/enabled`
- `GET /v1/mcp/servers`
- `PUT /v1/mcp/servers/{name}`
- `POST /v1/mcp/servers/{name}/enabled`
- `POST /v1/mcp/servers/{name}/refresh`
- `GET /v1/mcp/servers/{name}/tools`
- `POST /v1/mcp/servers/{name}/tools/{tool}/enabled`
- `GET /v1/mcp/servers/{name}/resources`
- `GET /v1/mcp/servers/{name}/prompts`

## Acceptance

- Skills and MCP settings load from the runtime APIs in the in-app browser.
- Refresh and enable toggles call runtime adapter methods and update from DTO
  responses.
- MCP server details load tools/resources/prompts from runtime APIs.
- Frontend lint/build and focused Go Skill/MCP tests pass.
- Final validation includes desktop tests and manual in-app browser operation
  against `http://127.0.0.1:5174/`.

## Remaining Risks

- MCP stdio args are edited as a simple whitespace-separated field; quoted arg
  parsing can be improved when advanced server editing is required.
- Server creation currently covers baseline transport fields only. Env,
  headers, and secret management should be handled in a later governance pass.
- Project-scoped Skills/MCP config waits for the Projects phase.
- End-to-end MCP tool use in a chat turn still requires a real configured MCP
  server; this phase verified the management and DTO surfaces plus the existing
  runtime MCP closure tests.

## Next Phase

After this phase is complete, the roadmap should move to Projects and
project-scoped sessions/config, unless runtime diagnostics are prioritized for
MCP request history or capability audit explanations.
