# Project And Conversation Storage Refactor Plan

Status: planning.

This document records the target storage architecture and implementation plan
for project, conversation, and sidebar management. The goal is to make project
and conversation state runtime-owned, database-backed, and recoverable, while
keeping the frontend as a projection of runtime state.

## Decisions

- Project and conversation management should be database-backed.
- Conversation content should use SQLite as the canonical data source.
- JSONL should not be the canonical session store. It can be added later as an
  export, compatibility, audit, or debugging format.
- Projects should be first-class database records.
- Sessions should bind to projects by `project_id`.
- Standalone conversations should use `scope = 'standalone'` and no
  `project_id`.
- A new conversation entry should create a draft only. A persisted session is
  created on first user submit.
- Deletion should be configurable.
- The default deletion mode should be hard delete for database size and disk
  use.
- Users may opt into soft delete.
- Hard delete must only delete Agent Builder database/blob data. It must never
  delete the user's real project files.
- If a project was soft-deleted and the same filesystem path is added again,
  Agent Builder must create a new project record with a new ID. It must not
  silently revive or reuse the soft-deleted project.

## Current Implementation Summary

Current code already stores most session data in SQLite:

- `sessions`, `messages`, and `files` are created by
  `internal/db/migrations/20250424200609_initial.sql`.
- `sessions.project_id` and `sessions.scope` are added by
  `internal/db/migrations/20260614000000_add_session_scope.sql`.
- runtime state such as turns, tool calls, permission requests, MCP requests,
  refs, runs, hooks, worktrees, and agent tasks also has SQLite-backed stores.
- `NewChat` and `SubmitUserInput` already support draft-first creation, where a
  real session is created lazily on first prompt submit.
- frontend session view models already carry `scope` and `projectId`.
- the sidebar already filters project sessions and standalone sessions from
  those fields.

The main gaps are:

- there is no `projects` table;
- `sessions.project_id` is not a foreign key to a stable project entity;
- project identity is currently derived from runtime workspace state;
- `OpenProject` switches an in-memory runtime path, but does not register a
  durable project record;
- `internal/projects/projects.go` has a `projects.json` registry, but the
  desktop runtime path does not use it as the source of truth;
- frontend hydration builds `projects` from the current runtime status, so the
  project list is effectively `[currentProject]`;
- project removal currently behaves like application data removal, whereas the
  target product semantics are database cleanup only.

## Target Data Model

Long term, the runtime should expose project and session state from a single
database-backed model:

```text
projects.id -> sessions.project_id
sessions.id -> messages.session_id
sessions.id -> runtime_turns.session_id
sessions.id -> runtime_tool_calls.session_id
sessions.id -> runtime_permission_requests.session_id
sessions.id -> runtime_refs.session_id
```

Recommended project table:

```sql
CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  canonical_path TEXT NOT NULL,
  data_dir TEXT NOT NULL,
  git_root TEXT,
  branch TEXT,
  is_git_repository INTEGER NOT NULL DEFAULT 0,
  exists_on_disk INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_opened_at INTEGER,
  deleted_at INTEGER
);

CREATE UNIQUE INDEX idx_projects_active_canonical_path
ON projects(canonical_path)
WHERE deleted_at IS NULL;

CREATE INDEX idx_projects_last_opened
ON projects(deleted_at, last_opened_at DESC);
```

The partial unique index is important. It allows only one active project for a
filesystem path while allowing a new project record for the same path after a
previous record was soft-deleted.

Recommended session additions:

```sql
ALTER TABLE sessions ADD COLUMN deleted_at INTEGER;
ALTER TABLE sessions ADD COLUMN last_opened_at INTEGER;
ALTER TABLE sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN title_source TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE sessions ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

CREATE INDEX idx_sessions_project_active
ON sessions(project_id, deleted_at, updated_at DESC);

CREATE INDEX idx_sessions_scope_active
ON sessions(scope, deleted_at, updated_at DESC);
```

Session ownership rules:

```text
project conversation:
  sessions.scope = 'project'
  sessions.project_id = projects.id

standalone conversation:
  sessions.scope = 'standalone'
  sessions.project_id is NULL or ''
```

Prefer `NULL` for standalone `project_id` in new schema work. If the existing
code path keeps empty strings for compatibility, normalize at runtime API
boundaries and keep queries consistent.

## Canonical Conversation Storage

SQLite should remain the canonical store for conversation content and runtime
state:

```text
sessions:
  conversation metadata

messages:
  user, assistant, tool, and system messages

runtime_turns:
  request/response lifecycle state

runtime_tool_calls:
  structured tool calls and statuses

runtime_permission_requests:
  pending/allowed/denied permission records

runtime_refs and blob files:
  large outputs, artifacts, and file references
```

JSONL is not recommended as the primary store because Agent Builder is a
desktop runtime with structured sidebar, recovery, tool, permission, and
cross-project management needs. JSONL can be added as:

- export transcript;
- Claude-compatible import/export;
- append-only audit mirror;
- debugging backup.

For large content, use a hybrid approach:

```text
DB:
  structured records, summaries, small and medium message parts

blob files:
  large stdout/stderr, artifacts, images, large file snapshots

DB refs:
  stable references to blob files
```

## Delete Semantics

Deletion mode should be configurable.

Default:

```text
delete_mode = hard
```

Optional:

```text
delete_mode = soft
```

Hard delete behavior:

- delete Agent Builder database records for the selected project/session;
- delete Agent Builder-owned blob/artifact data when it is only referenced by
  deleted records;
- never delete the user's real project directory or project files.

Soft delete behavior:

- set `deleted_at`;
- hide records from normal project/session lists;
- allow restore or later purge;
- do not reuse soft-deleted project IDs.

Project delete:

```text
if delete_mode == hard:
  purge project-scoped sessions and associated runtime records;
  purge the project record;

if delete_mode == soft:
  projects.deleted_at = now;
  sessions.deleted_at = now for sessions.project_id = project.id;
```

Session delete:

```text
if delete_mode == hard:
  purge session and associated runtime records;

if delete_mode == soft:
  sessions.deleted_at = now;
```

Important same-path rule:

```text
If project A for path P was soft-deleted and the user later adds path P again,
insert project B with a new project ID. Do not reuse project A.
```

Restore rule:

```text
If a soft-deleted project is restored but another active project already has
the same canonical_path, reject restore and require the user to remove or purge
the active project first.
```

## Runtime API Target

Add project and sidebar projection contracts:

```go
type RuntimeProject struct {
  ID              string `json:"id"`
  Name            string `json:"name"`
  Path            string `json:"path"`
  CanonicalPath   string `json:"canonicalPath,omitempty"`
  IsGitRepository bool   `json:"isGitRepository"`
  Branch          string `json:"branch,omitempty"`
  Current         bool   `json:"current"`
  ExistsOnDisk    bool   `json:"existsOnDisk"`
  CreatedAt       int64  `json:"createdAt"`
  UpdatedAt       int64  `json:"updatedAt"`
  LastOpenedAt    int64  `json:"lastOpenedAt,omitempty"`
  DeletedAt       int64  `json:"deletedAt,omitempty"`
}

type RuntimeProjectsResponse struct {
  Projects []RuntimeProject `json:"projects"`
}

type RuntimeSidebarProjectionResponse struct {
  Projects         []RuntimeProject `json:"projects"`
  Sessions         []RuntimeSession `json:"sessions"`
  CurrentProjectID string           `json:"currentProjectId,omitempty"`
  ActiveSessionID  string           `json:"activeSessionId,omitempty"`
}
```

Recommended operations:

```text
Projects()
SidebarProjection()
OpenProject(projectId | path)
DeleteProject(projectId)
RestoreProject(projectId)
PurgeProject(projectId)
DeleteSession(sessionId)
RestoreSession(sessionId)
PurgeSession(sessionId)
```

`OpenProject` should no longer be only a runtime path switch. It should:

1. canonicalize the path;
2. find or create an active project record;
3. update `last_opened_at`;
4. set `activeProjectID`;
5. switch runtime workspace;
6. return project and sidebar projection state.

## Backend Implementation Plan

### Phase 1: Add Project Store

Create a DB-backed project repository, for example:

```text
internal/runtime/runtime_project_store.go
```

Responsibilities:

```go
UpsertActiveProjectByPath(ctx, path string) (RuntimeProjectRecord, error)
ListProjects(ctx, filter ProjectListFilter) ([]RuntimeProjectRecord, error)
GetProject(ctx, id string) (RuntimeProjectRecord, error)
MarkOpened(ctx, id string) error
SoftDeleteProject(ctx, id string) error
HardDeleteProject(ctx, id string) error
```

Use UUIDs for project IDs. Do not use path hashes as business IDs. Path hashes
may still be used for data directory naming.

### Phase 2: Make OpenProject Project-Aware

Add runtime service state:

```go
activeProjectID string
```

Update `OpenProject` so the active runtime project is a DB project record.

Compatibility behavior:

- if called with a path, create a new active project if one does not exist;
- if an active record exists for the path, reuse it;
- if only a soft-deleted record exists, create a new project record;
- if called with a project ID, require it to exist and not be deleted.

### Phase 3: Bind Sessions To Stable Project IDs

Change ownership normalization:

```text
scope = project:
  project_id must be a real active projects.id

scope = standalone:
  project_id must be empty
```

Stop using workspace ID as the durable project ID. Workspace ID is runtime
state; project ID is product data.

### Phase 4: Add Sidebar Projection

Add a runtime-owned projection used by the frontend:

```text
projects: all active projects
sessions: active session summaries
currentProjectId
activeSessionId
```

Normal list queries must exclude `deleted_at IS NOT NULL`.

### Phase 5: Implement Delete Mode And Purge Service

Add setting:

```text
delete_mode = hard | soft
```

Default to `hard`.

Add a centralized purge implementation:

```text
internal/runtime/runtime_purge.go
```

It should hard-delete session/project data in a transaction. Do not scatter
hard delete SQL across handlers.

Purge service should clean all session-owned/runtime-owned data, including:

- messages;
- files;
- runtime turns;
- runtime tool calls;
- runtime permission requests;
- runtime MCP requests;
- runtime refs;
- runtime runs and transitions;
- runtime agent tasks and task messages/results;
- runtime hook executions where session-owned;
- project memory records where project-owned;
- session records;
- project records.

The exact table list should be verified against all migrations before
implementation.

### Phase 6: Migrate Existing Data

Migration steps:

1. create `projects`;
2. add session management columns;
3. import current active project path into `projects`;
4. import `internal/projects/projects.json` as active project records when
   present;
5. migrate existing project-scoped sessions from workspace IDs to real
   `projects.id`;
6. clear standalone session `project_id`;
7. mark migration completion in settings to avoid repeated imports.

Migration markers:

```text
projects_json_imported = true
session_project_id_migrated = true
```

## Frontend Implementation Plan

Main files:

```text
client/src/runtime/workbenchTypes.ts
client/src/runtime/wailsWorkbenchAdapter.ts
client/src/app/shell/WorkbenchShell.tsx
client/src/features/sidebar/Sidebar.tsx
```

### Hydration

Replace project hydration based on current status:

```ts
projects: currentProject.path ? [currentProject] : []
```

with runtime projection:

```text
projects = response.projects
sessions = response.sessions
currentProject = projects.find(project.current)
```

### Sidebar Behavior

Project section:

- render all active projects from runtime projection;
- project row opens or activates that project;
- chevron expands/collapses sessions;
- project `+` starts a project-scoped draft;
- show missing-folder state if `existsOnDisk` is false.

Conversation section:

- render only `scope === 'standalone'`;
- conversation `+` starts a standalone draft.

New conversation:

```text
top-level new conversation:
  current project if available, otherwise standalone

project row new conversation:
  that project

conversation section new conversation:
  standalone
```

No new conversation entry should create a persisted session before first
submit.

### Delete UI Copy

Hard delete copy:

```text
This deletes Agent Builder's stored project/conversation data. It does not
delete files from the project folder.
```

Soft delete copy:

```text
This moves the project/conversation out of the active list. It can be restored
or permanently deleted later.
```

## Testing Plan

Backend tests:

- project creation stores a durable project ID;
- opening the same active path reuses the active project;
- soft-deleting a project and adding the same path creates a new project ID;
- restoring a soft-deleted project fails if an active project has the same path;
- project sessions require active project IDs;
- standalone sessions have no project ID;
- list APIs exclude deleted records by default;
- hard delete removes Agent Builder DB records for sessions and projects;
- hard delete does not touch user project files;
- soft delete sets `deleted_at` only;
- `OpenProject` survives restart through DB-backed project records.

Frontend/runtime tests:

- project list is not derived from `[currentProject]`;
- restart preserves project list;
- project sessions render only under their project;
- standalone conversations render only in the conversation section;
- repeated new-conversation clicks do not create DB sessions;
- first prompt creates exactly one session with correct scope/project ID;
- delete mode changes UI copy and backend behavior.

Required validation:

```text
go test ./...
cd client && npm run build
```

## Recommended Implementation Order

1. Add DB migrations for projects, session management columns, and delete
   settings.
2. Add project store and project DTO mapping.
3. Make `OpenProject` create or activate project DB records.
4. Add `Projects()` and `SidebarProjection()` runtime APIs.
5. Change frontend hydration to consume runtime project/session projection.
6. Replace workspace-derived project IDs with stable `projects.id`.
7. Add hard/soft delete setting and purge service.
8. Update project/session delete flows.
9. Add migration/import from `projects.json`.
10. Add empty states and missing-folder UI states.
11. Clean up legacy project-list fallback logic.
12. Fix sidebar text encoding issues separately.

## Acceptance Criteria

- Projects are first-class DB records.
- Sessions bind to projects by stable `projects.id`.
- Standalone conversations have no project ID.
- The sidebar project list is restored after app restart.
- Adding the same path after soft delete creates a new project record.
- Default delete mode hard-deletes Agent Builder DB/blob data only.
- Optional soft delete hides records through `deleted_at`.
- User project files are never deleted by project/session cleanup.
- New conversation entries create drafts only.
- First prompt submit creates the real session.
- Project sessions and standalone conversations never duplicate across sidebar
  sections.
