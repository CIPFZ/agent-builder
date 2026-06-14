# Unified New Chat / Project Session Design

Status: implementation baseline.

Implementation note (2026-06-14):

- The visible new-conversation entries now open a draft composer and call
  `NewChat`, so repeated clicks do not create empty persisted sessions.
- Runtime session DTOs and storage now carry `projectId` and `scope`.
- `Chat` and explicit `CreateSession` can lazily create either project-scoped
  sessions or standalone conversations from runtime-owned ownership metadata.
- The sidebar filters project sessions and standalone conversations from those
  DTO fields instead of inferring ownership from UI placement.
- The composer exposes a first-version target selector for the current project
  and "Do not use project".

The full multi-project picker remains future work until runtime exposes a
project list or recent-project DTO.

## Problem

The current sidebar exposes three visually separate ways to start a
conversation:

- top-level "New chat";
- project-row "new session";
- conversation-section "new session".

These entries currently do not have a single product contract. In practice this
creates three user-visible problems:

- Repeated clicks can create many empty `New chat` sessions.
- Project sessions and the conversation list are rendered from the same
  `viewModel.sessions` collection, so the same sessions appear in both places.
- Sessions shown under a project do not have a clearly distinct product model,
  even though they should still support the same session features as ordinary
  conversations.

The product model should be:

```text
New conversation entry
  -> opens one draft composer surface
  -> draft has a target:
       project:<project_id>
       standalone
  -> first submitted prompt creates or selects the real runtime session
```

## Current Code Facts

### Runtime

Relevant files:

- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_sessions.go`
- `internal/runtime/runtime_turns.go`
- `desktop/runtime_bridge.go`

Current runtime session DTO:

```go
type RuntimeSession struct {
    ID               string
    Title            string
    MessageCount     int64
    PromptTokens     int64
    CompletionTokens int64
    Cost             float64
    CreatedAt        int64
    UpdatedAt        int64
    Active           bool
    Usage            RuntimeUsage
}
```

Current `RuntimeSessionCreateRequest` only accepts `title`.

Current `Sessions()` lists sessions for the current workspace only:

```text
runtimeService.Sessions
  -> ensureWorkspaceStarted
  -> workspace.ID
  -> runtime.ListSessions(ctx, wsID)
```

Current `NewChat(...)` only clears the active session:

```text
runtimeService.NewChat
  -> sessionID = ""
  -> Status()
```

Current `Chat(...)` already supports draft-first behavior. If no `sessionId`
is supplied and no active session exists, it creates a real session lazily on
first prompt submission.

This means the backend already has the right primitive for "top-level new chat
does not create an empty session". The current empty-session problem is caused
by frontend adapter behavior, not by `NewChat(...)`.

### Frontend

Relevant files:

- `client/src/app/shell/WorkbenchShell.tsx`
- `client/src/features/sidebar/Sidebar.tsx`
- `client/src/features/workspace/Workspace.tsx`
- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/runtime/staticWorkbenchAdapter.tsx`

Current `WorkbenchMode` is:

```ts
export type WorkbenchMode = 'project' | 'new-chat' | 'settings' | 'plugins'
```

Current `SessionViewModel` has no project/scope ownership fields:

```ts
export interface SessionViewModel {
  id: string
  title: string
  updatedLabel: string
  active?: boolean
  busy?: boolean
  activeTurnId?: string
}
```

Current sidebar project sessions are rendered with:

```tsx
viewModel.sessions.map(...)
```

The standalone conversation list also renders:

```tsx
viewModel.sessions.map(...)
```

So project sessions and conversation sessions cannot be separated in the UI
without adding ownership metadata to the runtime DTO and frontend view model.

Current frontend adapter `createSession(...)` calls runtime `CreateSession(...)`
when available:

```text
wailsWorkbenchAdapter.createSession
  -> bridge.CreateSession({ title: 'New chat' })
  -> hydrateWorkbench(...)
```

That path creates an empty persisted session before the user sends a prompt.
It should not be used for the unified "new conversation" entry.

## Feasibility Assessment

### Feasible Immediately

The empty-session bug can be fixed without backend schema changes:

- Change the top-level new conversation action to call `NewChat` / clear active
  session, not `CreateSession`.
- Keep one draft composer surface.
- Let `Chat` lazily create the session on first submitted prompt.

This is low risk because `runtimeService.NewChat` and draft submit behavior
already exist.

### Requires Contract Work

Separating project sessions from standalone conversations requires runtime
ownership metadata. The current runtime only has one workspace-scoped session
list for the current project/workspace. There is no `projectId`, `scope`, or
standalone workspace concept in `RuntimeSession`.

Required contract additions:

```go
type RuntimeSession struct {
    // existing fields...
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"` // "project" | "standalone"
}

type RuntimeSessionCreateRequest struct {
    Title     string `json:"title"`
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"` // optional; derived when possible
}
```

Recommended rule:

- `scope=project` requires a valid project/workspace id.
- `scope=standalone` creates a conversation without project file/review scope.
- If omitted for compatibility, the runtime treats the session as
  `scope=project` under the current workspace until standalone persistence is
  implemented.

### Requires Product Choice

The codebase currently models one current workspace/project at a time:

```text
hydrateWorkbench
  -> currentProject = mapProjectFromStatus(status, current.currentProject)
  -> projects = currentProject.path ? [currentProject] : []
```

The screenshot-style project picker can still be implemented for the current
project first, but a true multi-project picker requires a runtime project list
or persisted recent-project registry. Today the frontend only receives the
current project.

Recommended staged behavior:

1. First version picker shows:
   - current project;
   - "Do not use project";
   - "New blank project";
   - "Use existing folder".
2. Later version adds multiple recent projects after runtime exposes a project
   list DTO.

## Target UX

All three visible entry points call the same action:

```ts
startNewConversationDraft(target)
```

Where:

```ts
type NewConversationTarget =
  | { scope: 'project'; projectId: string }
  | { scope: 'standalone' }
```

Entry defaults:

- Top-level "New chat": default to current project when one exists; otherwise
  default to standalone.
- Project-row "new session": default to that project.
- Conversation-section "new session": default to standalone.

The composer shows a project target selector similar to:

```text
[agent-builder v] [mode] [branch]
```

The selector menu contains:

- current project;
- recent projects when available;
- New blank project;
- Use existing folder;
- Do not use project.

No runtime session is created until the user submits the first prompt.

## Target Sidebar Semantics

Project tree:

```ts
project.sessions = viewModel.sessions.filter(
  (session) => session.scope === 'project' && session.projectId === project.id,
)
```

Conversation section:

```ts
standaloneSessions = viewModel.sessions.filter(
  (session) => session.scope === 'standalone',
)
```

Do not render the same session in both sections.

If a session is project-scoped, it must still support the normal session
features:

- select;
- rename;
- delete;
- messages/timeline;
- permissions;
- diagnostics;
- RunProjection preview;
- scheduler actions where eligible;
- session-owned terminals through `SessionTerminals(sessionID)`.

Project-scoped sessions may also show project tools such as files/review,
because those tools are project-scoped and the session has a project owner.

Standalone sessions must not show project-only tools unless the user later
attaches/selects a project through an explicit runtime action.

## Runtime Source Of Truth Rules

Runtime remains authoritative for:

- project identity and path;
- session identity and ownership;
- active session;
- terminal ownership;
- session activity, diagnostics, permissions, RunProjection, and scheduler
  candidate reads.

React may own only:

- draft target selector state before first submit;
- popover/menu open state;
- local pending/error affordances for the create/open project picker.

React must not:

- infer session ownership from where a session appears in the sidebar;
- duplicate project sessions into standalone conversation state;
- create persisted empty sessions from repeated "new chat" clicks;
- infer project ownership from terminal text, assistant prose, runtime events,
  or action metadata.

## Proposed DTO / View Model Changes

Frontend:

```ts
export type SessionScopeViewModel = 'project' | 'standalone'

export interface SessionViewModel {
  id: string
  title: string
  updatedLabel: string
  projectId?: string
  scope: SessionScopeViewModel
  active?: boolean
  busy?: boolean
  activeTurnId?: string
}

export interface NewConversationDraftViewModel {
  active: boolean
  scope: SessionScopeViewModel
  projectId?: string
}

export interface WorkbenchViewModel {
  // existing fields...
  newConversationDraft?: NewConversationDraftViewModel
}
```

Backend:

```go
type RuntimeSession struct {
    // existing fields...
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"`
}

type RuntimeSessionCreateRequest struct {
    Title     string `json:"title"`
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"`
}

type RuntimeChatRequest struct {
    // existing fields...
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"`
}
```

`RuntimeChatRequest` ownership fields are needed because draft-first submit
creates the real session inside `Chat(...)`, not via `CreateSession(...)`.

## Implementation Plan

### Phase 1: Stop Empty Session Creation

Goal: repeated new-chat clicks must not create persisted sessions.

Frontend changes:

- Rename adapter-level `createSession` intent to a draft-oriented action in the
  call sites, or keep the interface temporarily but change the implementation
  to call `NewChat` rather than `CreateSession`.
- `WorkbenchShell.createSession()` should set draft UI state and clear active
  session.
- `wailsWorkbenchAdapter.createSession()` should call `bridge.NewChat('')`
  when available.
- `sendPrompt(...)` keeps omitting `sessionId` for draft submits so `Chat(...)`
  creates the session lazily.

Tests/smoke:

- Adapter smoke: calling new chat twice does not call `CreateSession`.
- Runtime smoke: `NewChat` preserves existing sessions and clears active
  session.
- Browser smoke: click top-level "New chat" repeatedly; no new sidebar rows
  appear until first prompt submit.

### Phase 2: Add Session Ownership DTOs

Goal: the UI can filter project sessions and standalone conversations from
runtime-owned fields.

Backend changes:

- Add `projectId` and `scope` to `RuntimeSession`.
- Add ownership fields to `RuntimeSessionCreateRequest`.
- Add ownership fields to `RuntimeChatRequest` for lazy creation.
- Make `toRuntimeSession(...)` populate ownership.

Storage decision:

- If existing session storage already has a stable workspace id per session,
  derive `projectId` from workspace id for project sessions.
- If standalone sessions need a separate durable home, add a design gate before
  introducing a new persistence location. Do not fake standalone ownership in
  React.

Transport changes:

- HTTP `POST /v1/sessions` forwards ownership fields.
- HTTP `POST /v1/chat` and `POST /v1/sessions/{id}/turns` preserve draft
  ownership where relevant.
- Wails `CreateSession` and `Chat` preserve ownership fields.

Tests:

- Creating a project session returns `scope=project` and matching `projectId`.
- Creating a standalone session returns `scope=standalone` and no project id.
- Draft `Chat` with `scope=project` creates a project session.
- Draft `Chat` with `scope=standalone` creates a standalone session.
- Session listing includes ownership fields.

### Phase 3: Sidebar Filtering

Goal: project and standalone lists no longer duplicate sessions.

Frontend changes:

- `mapSessions(...)` maps `projectId` and `scope`.
- Project tree filters sessions by project id.
- Conversation section filters `scope === 'standalone'`.
- Add empty states:
  - project: "No project conversations";
  - standalone: "No standalone conversations".
- Rename copy if needed:
  - "Conversations" can remain if only standalone sessions are listed;
  - or use "Standalone conversations" for clarity during refactor.

Tests/smoke:

- Source-level test or lightweight render smoke for filters.
- Browser smoke with seeded project and standalone sessions proves no duplicate
  rows.

### Phase 4: Unified Draft Target Picker

Goal: all new-conversation entry points share one draft composer and one target
selector.

Frontend changes:

- Add `newConversationDraft` to `WorkbenchViewModel`.
- Add `startNewConversationDraft(target)` in `WorkbenchShell`.
- Wire:
  - top-level new chat -> current project or standalone;
  - project-row new session -> target project;
  - conversation-section new session -> standalone.
- Add a compact target selector to the composer.
- Project menu actions reuse existing adapter methods:
  - `createProject`;
  - `openProject`;
  - `selectProjectDirectory`.

Runtime constraints:

- Selecting an existing project target must come from runtime project DTOs.
- In the first version, only current project can be listed unless a project
  list API exists.

Tests/smoke:

- Top-level new chat defaults to current project.
- Project-row new session selects that project.
- Conversation-section new session selects standalone.
- Switching target before submit changes the ownership passed to `Chat`.
- No persisted session is created until submit.

### Phase 5: Multi-project Picker Extension

Goal: match the full screenshot-style project picker with recent projects.

Required backend contract:

```go
type RuntimeProjectsResponse struct {
    Projects []RuntimeProject `json:"projects"`
}
```

Possible routes:

```text
GET /v1/projects
```

Wails:

```text
Projects() RuntimeProjectsResponse
```

The first implementation should not infer project recents from browser memory
or sidebar state. The runtime must provide the list.

## Validation Checklist

Required before accepting implementation:

- `git diff --check`
- `cd client && npm run build` for frontend changes.
- Relevant Go tests for runtime DTO/transport changes.
- Browser click smoke on `http://localhost:5173/`:
  - repeated top-level new-chat clicks do not add empty sessions;
  - project-row new session opens draft with that project selected;
  - conversation-section new session opens standalone draft;
  - first prompt submit creates exactly one session in the selected scope;
  - project sessions do not appear in standalone list;
  - standalone sessions do not appear under projects;
  - project sessions can still select, rename, delete, send messages, and open
    session terminals.

## Risks And Open Questions

- Current runtime appears to expose only the current project in the workbench
  view model. A full multi-project picker needs a runtime-owned project list.
- A true standalone session may need a durable workspace/storage decision.
  Until then, "standalone" should not be faked as a frontend-only flag.
- Existing sessions without ownership metadata need a compatibility mapping.
  Recommended default: treat legacy sessions from the current workspace as
  `scope=project` with the current project id.
- `CreateSession(...)` should remain available for explicit runtime/session
  creation contracts, but the visible "new conversation" UI should not call it
  before the first user prompt.
