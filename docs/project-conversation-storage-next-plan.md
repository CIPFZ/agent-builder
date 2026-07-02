# Project And Conversation Storage Next Plan

Status: discussion draft.

This document updates the project/conversation storage direction after reviewing
the current Agent Builder implementation and the `cc-haha` project. It is meant
to be an implementable plan, not a final code change. The main conclusion is:

- keep one global runtime SQLite database as the canonical structured store;
- do not use one canonical database per project;
- make projects and sessions first-class database entities;
- make sidebar/search use lightweight projections and indexes, never full
  conversation hydration;
- store large outputs and artifacts outside the main relational rows;
- optionally add cold archives later without breaking the global metadata/index
  model.

## Review Updates

This version includes the implementation review fixes:

- do not rely on `projects -> sessions ON DELETE CASCADE` for hard project
  delete, because hard delete must go through the centralized purge service;
- define session execution context separately from project selection;
- persist standalone working directories explicitly;
- enforce standalone `workdir` in schema/service rules;
- make destructive schema reset behavior explicit;
- make sidebar projection and FTS maintenance mandatory enough to implement
  consistently;
- list session-owned runtime tables for purge coverage;
- carry standalone `workdir` through runtime APIs;
- split path opening from DB project activation for missing projects;
- make schema reset and blob purge ordering safe;
- define missing-project draft behavior;
- keep draft-first behavior from being bypassed by session creation APIs;
- constrain project session workdirs to the owning project/worktree boundary.

## Problem Statement

The current code after the first refactor has several architectural tensions:

1. `projects.data_dir` and helper functions imply project-specific app data
   directories, but runtime workspace creation still uses the global desktop
   data directory.
2. Standalone sessions are represented by `scope = 'standalone'`, but
   `internal/workbench/session.go` still creates them through a workspace-bound
   API.
3. The schema is still assembled through many goose migrations even though this
   refactor does not need old-data compatibility.
4. `OpenProject` has large runtime side effects: it deletes the active
   workbench workspace, releases/rebuilds runtime stores, closes terminals, and
   resets in-memory state.
5. Future global search conflicts with one canonical database per project,
   because search would need to attach/scan many databases or maintain a second
   global index with hard consistency problems.
6. React must not infer projects from the current project; sidebar state must
   come from a runtime-owned projection.

## Lessons From cc-haha

`cc-haha` primarily stores conversations as JSONL files under a project-shaped
directory layout:

```text
~/.claude/projects/{sanitized_project_path}/{session_id}.jsonl
```

It does not solve our problem by using SQLite. Its useful ideas are query and
loading patterns:

- session list APIs are paginated;
- list views start from file metadata and only enrich the visible page;
- session list summaries are cached briefly;
- search is two-phase: use ripgrep to find candidate files/lines, then parse
  only the matching transcript lines;
- full transcript loading is reserved for opening a session, not for sidebar
  hydration.

Agent Builder should borrow those access patterns, not the JSONL canonical
store. SQLite remains a better fit for Agent Builder because runtime turns,
tool calls, permissions, refs, runs, hooks, worktrees, and recovery links are
structured and relational.

## Recommended Architecture

Use a single global Agent Builder data directory:

```text
{agent_builder_data_dir}/
  agent-builder.db
  blobs/
    runtime_refs/
    tool_outputs/
    attachments/
    artifacts/
  archives/
    optional-later/
```

The global SQLite database is the canonical store for structured state:

```text
projects
sessions
messages
message_search_fts
runtime_turns
runtime_tool_calls
runtime_permission_requests
runtime_refs
runtime_runs
runtime_events
runtime_audit_events
runtime_settings
```

`session_projection` is an optional materialized derived table. The MVP should
start with query projection over indexed `sessions` columns unless sidebar
previews require extra derived data.

Project directories on disk are execution context only. They are not database
boundaries.

## Execution Context Model

Project selection and agent execution are separate concepts.

```text
OpenProject:
  updates activeProjectID and project metadata only

Chat/Run:
  resolves session execution cwd from persisted session ownership/context
```

Runtime must stop treating the active workbench workspace as the business
owner of every session. A session needs enough persisted information to restart
execution without relying on the current UI project selection.

Recommended session context fields:

```sql
workdir TEXT,
canonical_workdir TEXT,
workdir_exists INTEGER NOT NULL DEFAULT 1
```

`workdir_exists` is cached projection metadata. It should be refreshed during
sidebar/status/open/run checks, but execution must stat the resolved working
directory again. Runtime must never treat a stale `workdir_exists = 1` value as
permission to run in a missing directory.

Rules:

- project sessions normally use `projects.path` as `workdir`;
- if a project session is launched from a subdirectory/worktree, persist that
  actual `workdir` on the session;
- project session `workdir` must be inside the owning project path, inside the
  owning project git root, or inside a recorded worktree linked to that
  project. If a requested workdir is outside those boundaries, runtime must
  reject the project session request or convert the draft to standalone by
  explicit product choice;
- standalone sessions must persist `workdir`;
- a standalone draft may carry a transient `workdir`, but the value is written
  only when the first prompt creates the real session;
- on resume, runtime uses `sessions.workdir` first, then project path for
  project sessions if `workdir` is empty;
- if the resolved workdir is missing, execution fails with a clear
  missing-workdir error and history remains readable.

Implementation options for the current workbench/app boundary:

1. Preferred: move agent execution toward a session-scoped execution context
   where each run receives cwd/config/session identity explicitly.
2. Transitional: keep a reusable workbench runtime, but before a run ensure
   the active app execution context matches the session cwd. This must be
   lazy on Chat/Run, not eager on OpenProject.

The transitional path may still restart an agent execution workspace when a
prompt is submitted for a different cwd, but project selection itself must stay
lightweight.

## Ownership Rules

Project session:

```text
sessions.scope = 'project'
sessions.project_id = projects.id
```

Standalone session:

```text
sessions.scope = 'standalone'
sessions.project_id IS NULL
```

Rules:

- `projects.id` is the only project identity.
- `projects.path` and `projects.canonical_path` are mutable location metadata.
- `workspaceID` is not business identity.
- `workspaceID` may remain only as a legacy execution adapter detail.
- new chat creates a runtime draft only.
- the first submitted prompt creates the real `sessions` row.
- project path missing/renamed on disk does not delete session history.

## Schema Direction

Because old data migration is out of scope, collapse the schema to a final
initial schema instead of layering more migrations.

Replace the goose migration chain with a schema initializer:

```text
internal/db/schema.sql
internal/db/connect.go -> ensureSchema(conn)
```

The final `sessions` table should directly include the target fields:

```sql
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  parent_session_id TEXT,
  title TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('project', 'standalone')),
  project_id TEXT REFERENCES projects(id) ON DELETE RESTRICT,
  workdir TEXT,
  canonical_workdir TEXT,
  workdir_exists INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'active',
  title_source TEXT NOT NULL DEFAULT 'auto',
  pinned INTEGER NOT NULL DEFAULT 0,
  message_count INTEGER NOT NULL DEFAULT 0 CHECK (message_count >= 0),
  prompt_tokens INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
  completion_tokens INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
  cost REAL NOT NULL DEFAULT 0.0 CHECK (cost >= 0.0),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_opened_at INTEGER,
  deleted_at INTEGER,
  CHECK (
    (
      scope = 'standalone' AND
      project_id IS NULL AND
      workdir IS NOT NULL AND
      length(trim(workdir)) > 0
    ) OR
    (scope = 'project' AND project_id IS NOT NULL)
  )
);
```

Do not use `ON DELETE CASCADE` from `projects` to `sessions`. Project hard
delete must first collect session ids and call the purge service so all runtime
rows, FTS rows, and Agent Builder-owned blobs are cleaned consistently.

The service layer must normalize `workdir` before insert/update:

- trim whitespace;
- resolve to an absolute path when possible;
- store `canonical_workdir` from symlink/realpath resolution when the path
  exists;
- set `workdir_exists` from a fresh stat;
- reject standalone session creation if `workdir` is empty after normalization.

Recommended project table:

```sql
CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  canonical_path TEXT NOT NULL,
  git_root TEXT,
  branch TEXT,
  is_git_repository INTEGER NOT NULL DEFAULT 0,
  exists_on_disk INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_opened_at INTEGER,
  deleted_at INTEGER
);
```

Runtime settings table:

```sql
CREATE TABLE runtime_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);
```

Required initial setting:

```text
schema_generation = 1
```

Important indexes:

```sql
CREATE UNIQUE INDEX idx_projects_active_canonical_path
ON projects(canonical_path)
WHERE deleted_at IS NULL;

CREATE INDEX idx_projects_active_recent
ON projects(deleted_at, last_opened_at DESC, updated_at DESC);

CREATE INDEX idx_sessions_project_active_recent
ON sessions(project_id, deleted_at, pinned DESC, updated_at DESC);

CREATE INDEX idx_sessions_standalone_active_recent
ON sessions(scope, deleted_at, pinned DESC, updated_at DESC)
WHERE scope = 'standalone' AND project_id IS NULL;

CREATE INDEX idx_sessions_active_recent
ON sessions(deleted_at, pinned DESC, updated_at DESC);
```

Optional projection table or view:

```sql
CREATE TABLE session_projection (
  session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  project_id TEXT,
  scope TEXT NOT NULL,
  title TEXT NOT NULL,
  subtitle TEXT,
  first_prompt TEXT,
  last_message_preview TEXT,
  message_count INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_opened_at INTEGER,
  deleted_at INTEGER
);
```

`ON DELETE CASCADE` is acceptable only for pure derived tables such as an
optional materialized `session_projection`, because they have no blob ownership,
audit meaning, or runtime side effects. Do not use cascade as the cleanup
mechanism for tables that own blobs, runtime lifecycle state, audit records, or
any data needed by the purge service.

Projection is required, but the MVP can choose one of two concrete forms:

1. Query projection: read directly from indexed `sessions` columns and avoid
   `messages` entirely.
2. Materialized `session_projection`: maintain the table on session/message
   writes.

If the materialized table is used, application code must update it on:

- session create;
- first prompt;
- message insert;
- title rename;
- AI title update;
- session soft/hard delete;
- project soft/hard delete;
- session pin/status changes.

Do not build sidebar data by scanning `messages`.

Recommended MVP: start with query projection over `sessions`, because it is
less state to keep in sync. Add materialized `session_projection` only when the
sidebar needs derived previews that are expensive to compute from session rows.

## Search Direction

Use database-backed search from the start:

```sql
CREATE VIRTUAL TABLE message_search_fts USING fts5(
  message_id UNINDEXED,
  session_id UNINDEXED,
  role UNINDEXED,
  content,
  created_at UNINDEXED
);
```

Search modes:

- global search: all non-deleted sessions;
- project search: `project_id = ?`;
- standalone search: `scope = 'standalone' AND project_id IS NULL`;
- current session search: `session_id = ?`;
- file/workspace search remains separate and should use ripgrep in the real
  project directory.

Borrow the `cc-haha` two-stage discipline:

1. search index returns candidate message/session ids;
2. runtime loads only the visible result page and snippets;
3. opening a result loads the session detail.

Never implement global session search as `LIKE '%query%'` over all messages.

Every search query must join or filter against active sessions:

```sql
SELECT ...
FROM message_search_fts f
JOIN sessions s ON s.id = f.session_id
LEFT JOIN projects p ON p.id = s.project_id
WHERE s.deleted_at IS NULL
  AND (p.id IS NULL OR p.deleted_at IS NULL)
  AND /* tested SQLite FTS MATCH predicate for message_search_fts */ 
LIMIT ?;
```

Use tested SQLite FTS syntax for the actual driver. Do not blindly rely on a
table alias in the `MATCH` predicate; SQLite FTS alias support varies by query
shape and driver behavior.

FTS should not duplicate session ownership fields such as `project_id` or
`scope` in the MVP. Those values must come from `sessions` and `projects` joins
so ownership changes cannot desynchronize search results. If performance later
requires denormalizing `project_id` or `scope` into FTS, that change must also
add explicit update hooks and tests for project relink, scope correction, soft
delete, and hard delete.

Soft-deleted sessions/projects must not appear in default search results. Hard
delete must remove FTS rows for deleted sessions in the purge transaction before
the session rows are removed.

## Large Data Strategy

Keep small/medium message parts in SQLite. Move large values to blob storage:

- long shell stdout/stderr;
- large tool results;
- binary attachments;
- screenshots/images;
- generated artifacts;
- huge file snapshots.

Store only metadata and references in SQLite:

```text
runtime_refs(id, session_id, kind, blob_key, size, mime_type, ...)
```

`runtime_refs` should not duplicate authoritative project ownership in the MVP.
Project ownership is derived from `runtime_refs.session_id -> sessions.project_id`.
If a future performance optimization adds `project_id` to refs, it must be
documented as a denormalized snapshot and must not become the source of truth
for ownership decisions.

Hard delete purges Agent Builder DB rows and owned blobs. It must never remove
the user's real project directory.

Blob deletion must happen after the database transaction commits:

1. inside the transaction, collect blob keys that should be deleted;
2. delete DB rows and commit;
3. after commit, delete blob files;
4. if file deletion fails, record retryable orphan cleanup work.

Do not delete files inside a DB transaction. File deletion cannot be rolled
back, and deleting blobs before a transaction rollback would leave broken DB
references.

Session-owned table families that purge must account for:

- `messages`;
- `files`;
- `runtime_turns`;
- `runtime_user_inputs`;
- `runtime_tool_calls`;
- `runtime_permission_requests`;
- `runtime_refs`;
- `runtime_events`;
- `runtime_audit_events`;
- `runtime_compact_boundaries`;
- `runtime_prompt_assemblies`;
- `runtime_agent_tasks`;
- `runtime_agent_task_messages/results` if present in final schema;
- `runtime_hook_executions`;
- `runtime_mcp_requests`;
- `runtime_worktrees`;
- `runtime_runs`;
- `runtime_run_transitions`;
- `runtime_recovery_links`;
- `message_search_fts`;
- any future table with `session_id`.

Before implementation, verify this list against the final schema with a simple
schema inspection test that finds all tables containing a `session_id` column.

## Runtime API Direction

Required runtime APIs:

```text
Projects(limit, offset)
Sessions(scope, project_id, limit, offset)
SidebarProjection(project_limit, session_limit_per_project, standalone_limit)
OpenProject(path, create_missing?)
ActivateProject(project_id)
RelinkProject(project_id, path)
RemoveProject(project_id)
Chat(prompt, session_id?, scope?, project_id?, workdir?)
DeleteSession(session_id)
SearchSessions(query, scope?, project_id?, limit, cursor)
```

`Projects` and `Sessions` may support `offset` for the first implementation,
but the runtime API should prefer cursor pagination for large or frequently
updated lists:

```text
Projects(limit, cursor?)
Sessions(scope, project_id, limit, cursor?)
```

The cursor should be based on stable ordering fields such as
`pinned DESC, updated_at DESC, id DESC`, not array positions in React state.

No public UI flow should call a generic `CreateSession` to start a draft. The
draft-first invariant is:

```text
new chat UI action:
  creates transient draft only

first prompt:
  Chat(...) validates ownership/workdir and creates the persisted session
```

If a lower-level persisted-session creation helper remains for tests, imports,
or adapter compatibility, it should be internal or explicitly named for its
side effect, for example `CreatePersistedSessionForPrompt`. It must not be used
by sidebar/new-chat hydration paths.

`SidebarProjection` should return only what the sidebar needs:

```text
projects:
  id, name, path, branch, exists_on_disk, current, updated_at

project_sessions:
  latest N sessions per project, by updated_at

standalone_sessions:
  latest N standalone sessions

active:
  current_project_id, active_session_id, draft target
```

It must not return full messages.

## OpenProject Behavior

`OpenProject` should be lightweight:

1. normalize the path;
2. require directory exists unless `CreateMissing`;
3. find active project by `canonical_path`;
4. create a new project if none exists;
5. refresh project metadata (`exists_on_disk`, git root, branch);
6. set `activeProjectID`;
7. return project DTO plus sidebar/status projection.

Split project APIs by intent:

- `OpenProject(path)`: path-based add/open. The path must exist unless
  `CreateMissing` is set. This is for adding or opening a real filesystem
  directory.
- `ActivateProject(project_id)`: DB-record activation. This may activate a
  project whose path is currently missing, refresh `exists_on_disk = false`,
  show missing state in the UI, and block execution until the path exists or is
  relinked.
- `RelinkProject(project_id, path)`: explicit user action to attach an existing
  project record to a new filesystem path. It must enforce active
  `canonical_path` uniqueness and must not silently merge projects.

`OpenProject` should not:

- delete the current workbench workspace;
- release or recreate the global DB;
- reset runtime stores;
- close unrelated terminals;
- create a session;
- navigate to an invalid/blank persisted session.

Missing project draft behavior:

- activating a missing project may set `activeProjectID` so the sidebar can
  show project context and previous conversations;
- a missing active project must not create a runnable project draft;
- top-level new chat while the active project is missing should create a
  standalone draft by default, unless the user explicitly selects "repair/relink
  project" first;
- project-section new chat for a missing project should create a disabled
  project draft state that explains the project path must be relinked before
  sending;
- first prompt for a project session must re-check the resolved workdir and fail
  before session creation if the project is missing.

Agent execution should resolve cwd at prompt/run time:

```text
if session.scope = project:
  cwd = sessions.workdir if present, otherwise projects.path
else:
  cwd = sessions.workdir
```

For standalone first-prompt creation, `Chat(..., scope='standalone')` must
receive a `workdir` or use a documented runtime standalone default such as the
user home directory. It must not silently use the current active project path.

## Standalone Conversation Behavior

Standalone is a real product scope, not a project fallback.

Rules:

- standalone sessions are stored in the global DB;
- `project_id` is `NULL`;
- standalone sessions appear only in the standalone sidebar section;
- they do not require an active project;
- if the UI starts a standalone draft, no DB row is created until first prompt.

`internal/workbench/session.go` should stop deciding that plain
`CreateSession` means standalone. Runtime ownership normalization should happen
above workbench.

## Delete Semantics

Deletion mode remains configurable:

```text
delete_mode = hard | soft
default = hard
```

Project hard delete:

- start a transaction;
- collect all active and deleted session ids for the project;
- call the centralized purge service for those session ids and collect blob
  keys for post-commit deletion;
- delete the project row only after dependent rows are gone;
- commit;
- delete collected Agent Builder-owned blob files after commit;
- enqueue retryable orphan cleanup for blob deletion failures;
- never delete `projects.path`.

Project soft delete:

- set `projects.deleted_at`;
- set `sessions.deleted_at` for project sessions;
- keep blobs until restore/purge policy handles them;
- same path added later creates a new `projects.id`.

Session hard delete:

- start a transaction;
- collect Agent Builder-owned blob refs for the session;
- delete all session-owned runtime rows and FTS rows;
- delete the session row;
- commit;
- purge only collected Agent Builder-owned blobs for that session after commit;
- enqueue retryable orphan cleanup for blob deletion failures.

Session soft delete:

- set `sessions.deleted_at`;
- hide from default lists and search.

## Project Path Changes

`projects.id` survives path problems.

If a real project directory is renamed or deleted outside Agent Builder:

- existing sessions still belong to the old project id;
- `exists_on_disk` should become false on projection refresh or open attempt;
- sidebar can show a missing project state;
- execution against that project should fail with a clear missing-workdir error;
- user may later relink the project path explicitly.

Do not silently infer that another path is the same project.

## Frontend Direction

Frontend hydration must use runtime projections:

```text
SidebarProjection()
```

or:

```text
Projects() + Sessions()
```

It must not build:

```ts
projects: [currentProject]
```

State ownership:

- Go/runtime/DB is the source of truth;
- Wails is only an adapter;
- React stores only UI state and active draft state;
- drafts can live in frontend/runtime memory because they are not persisted
  sessions until first prompt.

## Implementation Phases

### Phase 1: Freeze The Target Model

- Update docs to replace per-project canonical DB language.
- Mark global DB as the canonical project/session/runtime store.
- Mark per-project directories as optional blob/cache only.
- Decide whether `projects.data_dir` remains. Recommendation: remove it from
  project identity; if needed, replace it with blob namespace derived from
  `project.id`.

### Phase 2: Replace Migrations With Final Schema

- Delete or bypass `internal/db/migrations`.
- Add `internal/db/schema.sql`.
- Update `internal/db/connect.go` to initialize the final schema directly.
- Keep sqlc query files but regenerate them from the final schema.
- Remove compatibility assumptions around old `project_id = workspaceID`.
- Add explicit destructive reset behavior for non-compatible local DBs:
  - store `runtime_settings.schema_generation = 1`;
  - only reset the Agent Builder-owned DB path under the resolved desktop data
    directory;
  - preflight detection rules:
    - no DB file exists: create a fresh DB and initialize final schema;
    - DB exists but `runtime_settings` table is missing: treat as incompatible
      and back it up;
    - DB exists and `runtime_settings.schema_generation` is missing or not the
      expected value: treat as incompatible and back it up;
    - DB exists with expected generation: open normally;
  - before reset, release/close all pooled DB connections for that path;
  - if an existing DB has no matching generation, replace the active DB with a
    coordinated backup set:
    - backup `agent-builder.db`, `agent-builder.db-wal`, and
      `agent-builder.db-shm` when present using the same timestamp;
    - this is a coordinated best-effort backup set, not a multi-file atomic
      filesystem operation;
    - if any required backup step fails, abort fresh initialization and leave
      originals untouched where possible;
  - create and initialize a fresh DB after backups succeed;
  - if fresh initialization fails, leave backups in place and surface a clear
    startup error;
  - do not attempt data migration;
  - log the backup paths clearly.

### Phase 3: Normalize Session Ownership And Execution Context

- Add `workdir`, `canonical_workdir`, and `workdir_exists` to the final
  `sessions` schema.
- Runtime normalizes ownership:
  - project sessions require real active `projects.id`;
  - standalone sessions require `project_id NULL`;
  - standalone sessions require persisted `workdir`;
  - empty scope with no active project becomes standalone only where product
    behavior says so.
- Update runtime APIs to carry `workdir` for standalone draft/first prompt.
- Validate project session workdir containment against project path/git
  root/recorded worktree before creating the persisted session.
- Workbench no longer assigns business ownership by default.
- Session creation only happens on first prompt submit for draft chats.
- Chat/Run resolves cwd from persisted session context, not active project UI
  state.

### Phase 4: Normalize Project Store

- Keep `runtimeProjectStore`, but make it operate only on global DB project
  records.
- Remove `runtimeProjectDataDir` from identity decisions.
- Ensure soft-deleted same-path projects are not reused.
- Add active canonical path uniqueness.
- Refresh `exists_on_disk`, git metadata, and `last_opened_at` from the store.

### Phase 5: Make OpenProject Lightweight

- Remove workspace teardown/rebuild from `OpenProject`.
- `OpenProject` only mutates active project state and project metadata.
- Add `ActivateProject(project_id)` for DB-record activation, including missing
  project activation.
- Add `RelinkProject(project_id, path)` for explicit path repair.
- Keep active terminals/session execution independent unless the active project
  is the one being removed or explicitly closed.
- Opening a project should land in a clear project draft state, not create a
  session.
- Move cwd switching to Chat/Run execution, using the persisted session
  execution context.

### Phase 6: Add Sidebar Projection

- Implement DB-backed `SidebarProjection`.
- Return paginated/recent projects and recent sessions only.
- Do not load messages in sidebar projection.
- Add queries for:
  - active projects;
  - project sessions by project id;
  - standalone sessions;
  - active session.
- Avoid N+1 sidebar queries. Fetch project page first, then fetch recent
  sessions for those project ids in one batch using a window function or an
  equivalent indexed query.

Example shape:

```sql
WITH ranked AS (
  SELECT
    s.*,
    ROW_NUMBER() OVER (
      PARTITION BY s.project_id
      ORDER BY s.pinned DESC, s.updated_at DESC
    ) AS rn
  FROM sessions s
  WHERE s.deleted_at IS NULL
    AND s.project_id IN (...)
)
SELECT * FROM ranked WHERE rn <= ?;
```

### Phase 7: Add Search Foundation

- Add `message_search_fts` or a search index table.
- Update message writes to maintain search index.
- Implement global/project/standalone search APIs with limit/cursor.
- Return snippets and ids; load full session only when opened.
- Ensure soft-deleted sessions/projects are filtered out.
- Ensure hard delete removes FTS rows.
- Keep ownership filters from joined `sessions/projects`, not duplicated FTS
  ownership fields.

### Phase 8: Add Blob And Purge Boundaries

- Centralize blob ownership under `runtime_refs`.
- Make purge service delete only Agent Builder-owned blobs.
- Add tests that project hard delete never deletes `projects.path`.
- Add tests for soft delete filtering and same-path new project id.
- Add tests that blob files are deleted after DB commit and failures are
  recorded for retry without leaving broken DB references.

### Phase 9: Frontend Adapter Cleanup

- Hydrate sidebar from `SidebarProjection`.
- Remove `[currentProject]` project-list fallback.
- Ensure project sessions and standalone sessions do not duplicate.
- Ensure draft new chat does not call `CreateSession`.

## Code Areas To Change

Primary backend:

- `internal/db/connect.go`
- `internal/db/schema.sql`
- `internal/db/sql/*.sql`
- `internal/db/*.sql.go`
- `internal/session/session.go`
- `internal/workbench/session.go`
- `internal/runtime/runtime_project_store.go`
- `internal/runtime/runtime_projects.go`
- `internal/runtime/runtime_sessions.go`
- `internal/runtime/runtime_turns.go`
- `internal/runtime/runtime_lifecycle.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_service_types.go`
- `internal/runtime/runtime_http.go`
- `internal/runtime/runtime_purge.go`
- `desktop/runtime_bridge.go`

Primary frontend:

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/app/shell/WorkbenchShell.tsx`
- `client/src/features/sidebar/Sidebar.tsx`

Tests:

- project store tests;
- session ownership tests;
- sidebar projection tests;
- delete/purge tests;
- frontend adapter/static smoke tests.

## Verification Matrix

Backend tests should cover:

- creating a project record;
- opening the same active path reuses the active project id;
- soft deleting then re-adding the same path creates a new id;
- project session has `scope = project` and non-null `project_id`;
- standalone session has `scope = standalone`, null `project_id`, and persisted
  `workdir`;
- standalone first prompt passes or derives `workdir` without using the active
  project path;
- project session workdir outside the project/git-root/recorded-worktree
  boundary is rejected or explicitly converted to standalone before session
  creation;
- new draft does not create a session;
- first prompt creates the real session;
- hard project delete removes DB project/session/runtime rows;
- soft project delete only sets `deleted_at`;
- hard delete never removes the user's real project folder;
- `Projects` filters deleted rows;
- `SidebarProjection` returns projects, project sessions, and standalone
  sessions without duplicates;
- missing project path preserves history and reports missing execution context.
- schema generation mismatch renames the old DB and creates a fresh DB.
- schema reset backs up `agent-builder.db`, WAL, and SHM files after closing the
  DB pool.
- existing DB without `runtime_settings` or without expected
  `schema_generation` follows the backup-and-recreate path.
- purge coverage test detects all final-schema tables with `session_id`.
- blob purge happens only after DB commit, with retryable orphan cleanup on
  file deletion failure.

Frontend/static checks should cover:

- hydration does not use `[currentProject]`;
- sidebar consumes runtime projection;
- standalone sessions render only in standalone section;
- project sessions render only under their project;
- draft new chat does not call `CreateSession`.

Performance checks should cover:

- sidebar query uses `LIMIT`;
- project session query uses `project_id/deleted_at/updated_at` index;
- standalone query uses `scope/project_id/deleted_at/updated_at` index;
- search does not scan all messages with `LIKE`;
- search joins active sessions/projects or otherwise filters deleted rows;
- large tool outputs are referenced through blobs.

Commands:

```text
go test ./...
cd client && npm run build
```

## Risks And Tradeoffs

Single global DB risks:

- DB file growth over time;
- long-running writers can block readers if queries are poorly scoped;
- accidental full hydration can hurt UI responsiveness.

Mitigations:

- WAL mode;
- one shared connection with disciplined query limits;
- projection tables;
- FTS index;
- pagination/cursors everywhere;
- blobs for large data;
- optional archive for cold message bodies later;
- tests for query shape and API payload size.

Per-project DB risks:

- no natural home for standalone conversations;
- cross-project search becomes attach/scan or dual-write;
- cross-database foreign keys are not reliable;
- sidebar must aggregate many stores;
- project path moves/deletes complicate DB discovery;
- project switching has larger runtime side effects.

Decision:

Use the global DB model. It is the simpler consistency model and better matches
Agent Builder's product shape: one desktop runtime/workbench with projects and
conversations as first-class entities.
