# Workbench Runtime Feature Roadmap

Status: active roadmap with Phase 1-6 frontend/backend/runtime workbench
milestones completed through Skills/MCP management.

This document continues the provider-settings work and defines the next
implementation sequence for turning Agent Builder into a usable runtime-backed
desktop workbench.

## Current Baseline

Update 2026-06-03:

- Phase 1 through Phase 4 are completed for the current desktop workbench
  milestone: configured provider/model chat, durable sessions/model switching,
  runtime timeline rendering, and permission decision UI.
- Phase 5 is completed for the runtime closure gate: scenario coverage now
  exercises pending permission reload/deny, multi-session active recovery, MCP
  pending/denied replay with redaction, AgentTask mailbox ordering/rejection,
  and the existing policy, hook, sandbox/worktree, compact/ref, adapter, and
  DTO contract checks.
- The tool/thinking/permission integration plan is completed and archived as a
  finished milestone:
  [`tool-thinking-permission-integration-plan.md`](./tool-thinking-permission-integration-plan.md).
- Runtime parity closure stabilization and scenario coverage is recorded in:
  [`runtime-parity-closure-stabilization-plan.md`](./runtime-parity-closure-stabilization-plan.md).
- Skills and MCP management surfaces are completed, with React consuming
  runtime DTOs only:
  [`workbench-skills-mcp-management-plan.md`](./workbench-skills-mcp-management-plan.md).
- The next implementation stage is Projects and multi-session scope.

The Settings -> 服务商 module now has the first real frontend/backend path:

- Supported provider catalog comes from Go runtime, not frontend mock data.
- User-configured providers are saved in SQLite.
- DeepSeek was verified with:
  - OpenAI compatible protocol: `https://api.deepseek.com`
  - Model discovery: works and returns `deepseek-v4-flash`, `deepseek-v4-pro`
  - Test and latency: works through runtime API
- Vite/browser development must not depend on Wails generated bindings. The
  frontend runtime adapter must support HTTP/dev transport fallback.

Do not add new frontend-only business data. React consumes view models and
calls runtime adapter methods; Go runtime and SQLite own durable state.

## Target Capabilities

The next product milestone is:

1. Use a configured model from the main page for real conversation.
2. Persist sessions and switch between sessions, providers, and models.
3. Render conversation details including tool calls, thinking, code, files, and
   runtime status.
4. Configure and enforce permissions.
5. Use skills and MCP from the workbench.
6. Integrate projects, where each project owns multiple sessions.

## Architecture Principles

- Runtime state belongs in Go.
- SQLite stores durable project, provider, session, message, turn, permission,
  MCP, skill, audit, and project membership state.
- React is the product UI and keeps only local UI state such as open modal,
  selected panel, draft composer text, and sidebar collapse.
- Runtime adapter maps Go DTOs to UI view models. Components must not invent
  fallback business state.
- Wails and HTTP/dev transports must expose the same runtime operations.
- Events notify the UI that state changed; APIs remain the source of truth.

## Phase 1: Use Configured Provider In Main Chat

Status: completed for the current desktop workbench milestone.

Goal: send the composer prompt through the currently selected configured
provider and model.

Backend work:

- Add a selected runtime model concept that references a configured provider:

```text
selected_model
  id
  configured_provider_id
  provider_id
  model
  scope: global | project | session
  project_id nullable
  session_id nullable
  created_at
  updated_at
```

- Convert configured provider records into runtime provider config at turn
  execution time.
- Stop relying on legacy `model.json` as the product path. Keep it only as a
  development or compatibility path until removed.
- Make `Chat` / turn creation fail with a clear runtime error if no provider or
  model is selected.
- Add or reuse API:
  - `GET /v1/config/models`
  - `PUT /v1/config/selected-model`
  - `POST /v1/sessions/{id}/turns`
  - `POST /v1/turns` only as shorthand that creates or uses an active session

Frontend work:

- Add a model selector in the main composer footer using configured providers
  and discovered/default models.
- Show the current selected provider/model in the composer.
- On send, call runtime adapter with selected session and selected model.
- Disable only while runtime is busy; do not permanently disable creation or
  send controls.

Acceptance:

- User can configure DeepSeek, select `deepseek-v4-flash` or
  `deepseek-v4-pro`, and send a prompt from the main page.
- Response appears in the conversation.
- Refreshing the window can recover the active session and messages.

## Phase 2: Sessions And Model Switching

Status: completed for the current desktop workbench milestone.

Goal: sessions are durable and can use different providers/models.

Backend work:

- Confirm session tables can store project, provider, model, title, timestamps,
  status, and active marker.
- Add session-level model selection:

```text
sessions
  id
  project_id nullable
  title
  selected_provider_id nullable
  selected_model nullable
  created_at
  updated_at
```

- Create or update APIs:
  - `GET /v1/sessions`
  - `POST /v1/sessions`
  - `POST /v1/sessions/{id}/select`
  - `PUT /v1/sessions/{id}`
  - `DELETE /v1/sessions/{id}`
  - `GET /v1/sessions/{id}/messages`

Frontend work:

- Sidebar session list loads from runtime.
- New chat creates a durable session.
- Selecting a session reloads messages and session model state.
- Model selector can change the active session model without changing other
  sessions.

Acceptance:

- Create two sessions using different models/providers.
- Switch between sessions and see correct messages and model label.
- Restart/reload and preserve sessions.

## Phase 3: Conversation Rendering

Status: completed for the current desktop workbench milestone.

Goal: the chat surface becomes a runtime timeline, not plain text.

Backend work:

- Ensure runtime exposes:
  - messages
  - turns
  - tool calls
  - permission requests
  - refs/artifacts
  - audit events
  - event stream sequence
- Normalize message parts:

```text
message
  role: user | assistant | system | tool
  content
  parts[]
  provider
  model
  turn_id
  finished
  error
```

- Normalize tool call lifecycle:

```text
tool_call
  id
  turn_id
  name
  input_summary
  output_summary
  status: queued | running | waiting_permission | completed | failed | cancelled
  started_at
  completed_at
```

Frontend work:

- Replace placeholder workspace content with Ant Design X conversation
  rendering.
- Use:
  - `Bubble.List` for messages
  - `XMarkdown` for markdown/code
  - `CodeHighlighter` for code blocks
  - `ThoughtChain` / `Think` only for safe thinking summaries, not hidden model
    chain-of-thought
  - custom `ToolCallCard` for tools
  - custom `PermissionGate` for approvals
- Add a right-side inspector later for selected tool/message/audit detail.

Important note:

- Do not expose hidden chain-of-thought. Display runtime-provided thinking
  summaries, reasoning labels, progress summaries, or tool planning metadata
  only when the runtime explicitly provides safe content.

Acceptance:

- User message, assistant response, code block, failed tool, successful tool,
  and permission request all render as distinct timeline items.

## Phase 4: Permissions

Status: completed for the current desktop workbench milestone.

Goal: permissions are runtime-enforced and recoverable.

Backend work:

- Use existing permission primitives as source of truth.
- Ensure pending permissions are persisted and recoverable after refresh.
- APIs:
  - `GET /v1/permissions`
  - `POST /v1/permissions/{id}/decision`
  - `GET /v1/policy`
  - `PUT /v1/policy`

Frontend work:

- Settings -> 权限 edits policy mode and rules.
- Conversation timeline shows permission gates.
- User can approve/deny from modal/drawer.
- Pending permissions reappear after refresh.

Acceptance:

- A tool call requiring approval pauses the turn.
- Approve continues the turn.
- Deny records the decision and shows the result in the timeline.

## Phase 5: Runtime Closure Gate Before Skills And MCP

Status: completed for the 2026-06-03 closure gate.

Goal: prove the completed runtime primitives across recovery, replay, policy,
hooks, MCP, AgentTask, sandbox/worktree, refs, and adapter contracts before
building broader management surfaces.

Execution plan:

- [`runtime-parity-closure-stabilization-plan.md`](./runtime-parity-closure-stabilization-plan.md)

Acceptance:

- Cross-boundary scenario tests fail on permission, tool, hook, MCP, AgentTask,
  sandbox/worktree, compact/replay, refs, or recovery regressions.
- Replay/recovery can explain visible runtime facts without React-owned state.
- Runtime DTOs are stable enough for later React diagnostics and management
  views.

Completed scope:

- Pending permission reload and deny flows now prove durable permission, tool
  call, turn, event, audit, and replay updates.
- Multi-session active recovery keeps running and waiting-permission turns
  separate by session.
- MCP pending elicitation recovery and denial are replayable and redacted.
- AgentTask follow-up rejection and mailbox order are durable and replayable.
- Existing closure tests continue to cover policy/headless, hooks, shell
  classification, sandbox/worktree, compact/refs/redaction, HTTP, Wails, and
  TypeScript build contracts.

## Phase 6: Skills And MCP

Status: completed for the 2026-06-03 management-surface milestone.

Goal: skills and MCP are runtime capabilities available to turns.

Backend work:

- Keep skills and MCP configuration in runtime/SQLite/config files as currently
  appropriate, but expose runtime APIs as the UI boundary.
- APIs already partially exist and should be hardened:
  - `GET /v1/skills`
  - `POST /v1/skills/refresh`
  - `POST /v1/skills`
  - `POST /v1/skills/{name}/enabled`
  - `GET /v1/mcp/servers`
  - `PUT /v1/mcp/servers/{name}`
  - `POST /v1/mcp/servers/{name}/enabled`
  - `POST /v1/mcp/servers/{name}/refresh`
  - `GET /v1/mcp/servers/{name}/tools`

Frontend work:

- Settings -> 技能 shows installed skills, enabled state, source path, and
  refresh status.
- Settings -> MCP shows servers, connection state, tools/resources/prompts
  counts, and enable toggles.
- Composer/session context can show active skills/MCP capabilities.

Acceptance:

- Enable a skill and see it included in runtime capability state.
- Add or enable an MCP server and see tool list refresh.
- A turn can use an enabled MCP tool subject to permissions.

Execution plan:

- [`workbench-skills-mcp-management-plan.md`](./workbench-skills-mcp-management-plan.md)

Completed scope:

- Settings -> Skills loads runtime skill DTOs, refreshes discovery, and toggles
  enabled state through the adapter.
- Settings -> MCP loads runtime server DTOs, supports baseline add/edit,
  refresh, server enable/disable, tool/resource/prompt detail loading, and
  tool enable/disable through the adapter.
- Composer shows a runtime-derived enabled skill/MCP/tool summary.
- Manual in-app browser verification covered Skills and MCP settings at
  `http://127.0.0.1:5174/`.

## Phase 7: Projects And Multi-Session Scope

Goal: project is the workspace boundary and owns sessions.

Backend work:

- Define project records:

```text
projects
  id
  name
  path
  data_dir
  is_git_repository
  branch
  created_at
  updated_at
```

- Associate sessions with projects.
- Load project-specific config, selected model, permissions, skills, MCP, and
  context sources.
- APIs:
  - `GET /v1/projects`
  - `POST /v1/projects`
  - `POST /v1/projects/{id}/select`
  - `PUT /v1/projects/{id}`
  - `DELETE /v1/projects/{id}`
  - `GET /v1/projects/{id}/sessions`

Frontend work:

- Sidebar Projects section uses runtime project data.
- Selecting a project updates current project, sessions, context, and model
  defaults.
- Project creation should choose a local path and then let runtime inspect git
  metadata.

Acceptance:

- Add two projects.
- Each project has independent session list.
- Switching project changes visible sessions and current context.

## Cross-Cutting Runtime Contracts

The frontend adapter should converge on an `AgentRuntime` interface with these
groups:

```text
status/recovery
projects
sessions
messages/turns
providers/models
permissions/policy
skills
MCP
events
audit/refs
```

Each group needs:

- DTO types in Go
- HTTP route
- Wails bridge method where desktop binding is used
- TypeScript DTO
- DTO-to-view-model mapper
- focused backend tests
- frontend build/lint verification
- browser verification for local UI changes

## Suggested Implementation Order

1. Completed: finish selected configured provider/model as the main chat model
   source.
2. Completed: make main chat create durable session turns and render messages.
3. Completed: add session model switching.
4. Completed: add timeline rendering for messages/tool calls/permissions.
5. Completed: add permission decision UI.
6. Completed: run runtime parity closure stabilization and scenario coverage.
7. Completed: add Skills settings.
8. Completed: add MCP settings and enabled tool visibility.
9. Next: add projects and project-scoped session lists.

This order avoids building management screens before the core chat loop is
usable.

## Known Risks

- Mixing legacy `model.json` with new provider settings can create two sources
  of truth. The product path should move to SQLite configured providers and
  selected model records.
- Some providers support chat but not model discovery. The UI must allow manual
  model entry.
- The in-app browser may not expose `fetch` or `XMLHttpRequest`; runtime adapter
  fallback must remain tested.
- Wails binding generation may lag Go method additions. HTTP/dev fallback should
  remain available during development.
- Hidden chain-of-thought must not be displayed. Only show safe summaries or
  runtime-provided structured thinking metadata.
