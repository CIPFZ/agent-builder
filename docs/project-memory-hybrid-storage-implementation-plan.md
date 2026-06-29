# Project Memory Hybrid Storage Implementation Plan

Status: proposed.

## Goal

Introduce a project-scoped memory system for Agent Builder that stores memory
content as editable Markdown files while using SQLite as a rebuildable index,
metadata, audit, and UI acceleration layer.

The memory system must stay aligned with the product architecture:

- Go runtime is the source of truth for memory state, indexing, injection,
  permissions, and audit.
- React renders and edits memory through runtime DTOs; browser state is not the
  authority.
- Wails is only an adapter. HTTP/dev transport must expose the same runtime
  contract.
- Memory files are project-related user knowledge, not hidden model state.
- Prompt injection must be auditable through context source and prompt assembly
  diagnostics.

## Decision

Use a hybrid storage model:

```text
memory content:     Markdown files under a project memory directory
runtime index:      SQLite tables that mirror file metadata and injection state
runtime events:     memory lifecycle, indexing, injection, and tool activity
prompt evidence:    context sources and prompt assembly snapshots
```

The Markdown files are authoritative for memory content. The database index is
derived and recoverable. If the DB index is lost or stale, the runtime can
rescan files and rebuild it.

## Non-Goals

- Do not implement automatic memory extraction in the first phase.
- Do not implement AutoDream-style background consolidation in the first phase.
- Do not store raw full prompts in the memory tables.
- Do not make React parse or inject memory directly.
- Do not require a vector database for the initial implementation.
- Do not replace existing AGENTS.md / CLAUDE.md context loading. Memory is a new
  project-scoped capability layered next to instruction files.

## Reference Comparison

### Claude Code / cc-haha

Claude Code style memory is file-first:

- `MEMORY.md` is an index.
- Topic Markdown files store actual memories.
- Frontmatter describes name, description, and type.
- Related memories can be selected and injected into the turn.
- Background agents can extract, consolidate, and prune memories.

This model is strong for transparency, manual editing, portability, and team
sharing.

### DeepSeek-GUI Kun

Kun uses structured memory records:

- Each memory is a typed JSON record.
- Runtime exposes `/v1/memory/*`.
- Tools create, update, and delete memory through on-request policy.
- Turns record `injectedMemoryIds`.

This model is strong for typed APIs, UI management, auditability, and a runtime
source of truth.

### Agent Builder Direction

Agent Builder should combine both:

- Use files for content and user-visible project knowledge.
- Use DB records for IDs, metadata, state, indexing, provenance, and injection
  history.
- Reuse existing context source and prompt assembly diagnostics.

## Terminology

### Instruction Files

Files such as `AGENTS.md`, `CLAUDE.md`, `.claude/CLAUDE.md`, and
`.claude/rules/*.md`.

These are project instructions. They should remain in the context source
system and should not be treated as long-term memory records.

### Memory Files

Project-scoped Markdown files under the runtime-managed memory directory.

They represent user-approved or user-editable knowledge that should be reused
across sessions for the same project.

### Memory Index

SQLite rows derived from memory files. The index includes stable IDs,
frontmatter metadata, file hashes, enabled/deleted state, and provenance.

### Memory Injection

The process of selecting memory records for a turn, creating runtime context
sources for them, and including their content or summary in prompt assembly.

## Project Memory Directory

### Default Location

Memory should live under Agent Builder's existing project data root, not inside
the user's Git working tree by default.

Suggested layout:

```text
{agent-builder-data-dir}/projects/{project-id}/memory/
  MEMORY.md
  user/
  feedback/
  project/
  reference/
```

`project-id` should reuse the existing project identity used by runtime/project
storage. If the current project registry does not yet expose a durable project
data directory helper, add one instead of recomputing paths in multiple places.

### Optional Project-Local Location

Later, allow opt-in project-local memory:

```text
{workspace}/.agent-builder/memory/
```

This should be disabled by default because it affects version control and team
sharing semantics. If enabled, the UI must clearly label whether memory files
are private runtime data or project-local files.

### File Layout

```text
memory/
  MEMORY.md
  user/
    role.md
  feedback/
    response-style.md
  project/
    release-freeze.md
  reference/
    dashboards.md
```

`MEMORY.md` is an index. Topic files contain the durable content.

## Memory Types

Start with the same high-level taxonomy as Claude Code:

| Type | Purpose | Example |
| --- | --- | --- |
| `user` | User profile, preferences, role, collaboration style | User prefers concise diffs over long summaries |
| `feedback` | Behavioral corrections or repeated preferences for the agent | Do not mock the database in integration tests |
| `project` | Project facts not derivable from source code | Release branch freezes after a specific date |
| `reference` | Pointers to external systems or docs | Grafana dashboard URL for on-call latency |

Do not use memory for facts that are directly recoverable from the codebase,
Git history, package metadata, or current files.

## Markdown Format

Each topic file should use YAML frontmatter:

```markdown
---
id: mem_01JABCDEF1234567890
title: Testing policy
type: feedback
description: Integration tests should use the real database
tags:
  - testing
  - database
created_at: 2026-06-29T00:00:00Z
updated_at: 2026-06-29T00:00:00Z
---

Integration tests should use the real database instead of mocks.

Why: mock-only tests previously missed migration behavior.

How to apply: when adding or reviewing integration tests, prefer a real
database connection unless the user explicitly asks for a narrow unit test.
```

Required frontmatter:

- `id`
- `title`
- `type`
- `description`

Optional frontmatter:

- `tags`
- `created_at`
- `updated_at`
- `source_session_id`
- `source_turn_id`
- `confidence`

The DB should tolerate missing optional fields and repair missing generated
fields where safe.

## MEMORY.md

`MEMORY.md` is an index, not the canonical body store.

Example:

```markdown
- [Testing policy](feedback/testing-policy.md) - Integration tests should use the real database.
- [Release freeze](project/release-freeze.md) - Non-critical merges freeze after 2026-07-02.
```

Initial behavior:

- `MEMORY.md` is maintained by the runtime when memory files are created,
  renamed, disabled, or deleted through Agent Builder.
- If a user manually edits `MEMORY.md`, runtime should preserve valid lines and
  repair broken lines only during explicit index rebuild.
- The index should stay small. Recommended limits:
  - 200 lines.
  - 25 KB.
  - one line per memory.

## SQLite Schema

Add a migration such as:

```text
internal/db/migrations/YYYYMMDDHHMMSS_add_project_memory.sql
```

Suggested tables:

```sql
CREATE TABLE IF NOT EXISTS project_memory_records (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  tags_json TEXT NOT NULL DEFAULT '[]',
  content_hash TEXT NOT NULL,
  mtime_unix INTEGER NOT NULL DEFAULT 0,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  deleted_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  created_from_session_id TEXT,
  created_from_turn_id TEXT,
  last_indexed_at TEXT NOT NULL,
  last_injected_at TEXT,
  UNIQUE(project_id, relative_path)
);

CREATE INDEX IF NOT EXISTS idx_project_memory_records_project
  ON project_memory_records(project_id, enabled, deleted_at, updated_at);

CREATE INDEX IF NOT EXISTS idx_project_memory_records_type
  ON project_memory_records(project_id, type);

CREATE TABLE IF NOT EXISTS project_memory_injections (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  memory_id TEXT NOT NULL,
  prompt_assembly_id TEXT,
  injected_at TEXT NOT NULL,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  selection_reason TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(memory_id) REFERENCES project_memory_records(id)
);

CREATE INDEX IF NOT EXISTS idx_project_memory_injections_turn
  ON project_memory_injections(session_id, turn_id);
```

Optional later table:

```sql
CREATE TABLE IF NOT EXISTS project_memory_tombstones (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  memory_id TEXT NOT NULL,
  deleted_at TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT ''
);
```

Use tombstones if we need to preserve delete history after a file is removed.

## Source of Truth Rules

### Content

The Markdown file is authoritative for:

- body content;
- frontmatter title/type/description/tags when manually edited;
- file-level update time.

### DB

The DB is authoritative for:

- enabled/disabled state;
- deleted/tombstoned state;
- source session/turn provenance;
- last indexed time;
- last injected time;
- injection history;
- runtime audit references.

### Reconciliation

At runtime startup, project open, manual refresh, and before injection:

1. Scan memory directory.
2. Parse Markdown/frontmatter.
3. Hash file content.
4. Upsert DB rows when `mtime` or `content_hash` changed.
5. Mark DB rows missing from disk as `deleted_at` only if they were previously
   indexed and no file exists.
6. Never silently discard manual file content.

## Runtime Package Shape

Add a focused package:

```text
internal/memory/
  types.go
  paths.go
  markdown.go
  scanner.go
  store.go
  service.go
  retrieval.go
  index.go
```

Responsibilities:

- Resolve memory directories from project/runtime state.
- Parse and render Markdown frontmatter.
- Scan memory files safely.
- Rebuild and update DB index.
- Create, update, disable, delete memory records.
- Select records for injection.
- Produce runtime DTOs.

Keep this package independent from Wails and React.

## Runtime Service API

Extend `internal/runtime.RuntimeService` with transport-neutral methods:

```go
ProjectMemories(ctx context.Context, projectID string) (RuntimeMemoryListResponse, error)
ProjectMemory(ctx context.Context, memoryID string) (RuntimeMemoryDetailResponse, error)
CreateProjectMemory(ctx context.Context, req RuntimeMemoryCreateRequest) (RuntimeMemoryRecord, error)
UpdateProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryUpdateRequest) (RuntimeMemoryRecord, error)
DisableProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDisableRequest) (RuntimeMemoryRecord, error)
DeleteProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDeleteRequest) (RuntimeMemoryRecord, error)
RefreshProjectMemoryIndex(ctx context.Context, projectID string) (RuntimeMemoryIndexResponse, error)
ProjectMemoryDiagnostics(ctx context.Context, projectID string) (RuntimeMemoryDiagnostics, error)
```

DTOs should include:

```go
type RuntimeMemoryRecord struct {
  ID          string   `json:"id"`
  ProjectID   string   `json:"projectId"`
  RelativePath string  `json:"relativePath"`
  AbsolutePath string  `json:"absolutePath,omitempty"`
  Type        string   `json:"type"`
  Title       string   `json:"title"`
  Description string   `json:"description"`
  Tags        []string `json:"tags"`
  Enabled     bool     `json:"enabled"`
  DeletedAt   string   `json:"deletedAt,omitempty"`
  ContentHash string   `json:"contentHash"`
  TokenEstimate int    `json:"tokenEstimate"`
  CreatedAt   string   `json:"createdAt"`
  UpdatedAt   string   `json:"updatedAt"`
  LastIndexedAt string `json:"lastIndexedAt"`
  LastInjectedAt string `json:"lastInjectedAt,omitempty"`
}
```

Detail responses may include raw Markdown content only for explicit read/edit
operations. List responses should use metadata and short previews.

## HTTP and Wails Adapters

Expose the same service methods through:

- Wails `RuntimeBridge`.
- `internal/runtime/runtime_http.go`.
- frontend adapter mapping in `client/src/runtime/wailsWorkbenchAdapter.ts`.

Suggested HTTP routes:

```text
GET    /api/runtime/projects/{projectID}/memory
GET    /api/runtime/memory/{memoryID}
POST   /api/runtime/projects/{projectID}/memory
PATCH  /api/runtime/memory/{memoryID}
POST   /api/runtime/memory/{memoryID}/disable
DELETE /api/runtime/memory/{memoryID}
POST   /api/runtime/projects/{projectID}/memory/refresh
GET    /api/runtime/projects/{projectID}/memory/diagnostics
```

Follow existing runtime HTTP route conventions if names differ.

## Tool Integration

Add memory tools after the runtime CRUD path is stable:

```text
memory_create
memory_update
memory_disable
memory_delete
memory_search
```

Initial tool policy:

- `memory_search`: read risk.
- `memory_create/update/disable/delete`: ask/on-request by default.
- Plan mode should not allow write tools.
- Headless/recovery paths should fail closed for memory writes unless policy
  explicitly grants them.

Tool outputs should include:

- memory IDs;
- relative paths;
- actions performed;
- warnings if index repair happened.

Timeline should render memory writes as a high-signal memory activity row or
tool card badge.

## Injection Pipeline

### Phase 1 Selection

Use deterministic selection first:

1. Filter by project ID.
2. Exclude disabled/deleted records.
3. Score by keyword overlap over title, description, tags, and body preview.
4. Prefer recently updated records.
5. Limit to a small number, such as 5.
6. Apply token budget.

Do not use an LLM selector in the first implementation. It can be added later.

### Phase 2 Selection

Add an optional model-based selector:

- Input: user prompt, memory metadata manifest, recent tools, active files.
- Output: selected memory IDs with reasons.
- Limit: at most 5 records.
- Fail closed to deterministic selector or no model-selected records.

### Prompt Integration

Selected memories should become context sources:

```text
Kind: project_memory
Scope: project
State: loaded / skipped / failed
Reason: memory_selected / disabled / token_budget / stale_index / read_failed
```

Prompt assembly should record:

- selected memory IDs;
- omitted count;
- token estimates;
- content hashes;
- selection reasons;
- no raw memory body in DB event payloads.

Turn metadata should include:

```json
{
  "injectedMemoryIds": ["mem_..."]
}
```

This mirrors the Kun concept and fits Agent Builder diagnostics.

## Events and Audit

Add runtime event types:

```text
memory.index.started
memory.index.completed
memory.index.failed
memory.record.created
memory.record.updated
memory.record.disabled
memory.record.deleted
memory.record.injected
memory.record.skipped
```

Event payloads should include IDs, paths, hashes, and summaries. Do not include
full raw memory content in event payloads.

Audit records should link:

- memory action;
- project ID;
- session ID;
- turn ID;
- tool call ID when applicable;
- actor: user, assistant tool, runtime, import.

## Frontend UI

### Settings Memory Panel

Implement the existing `memory` settings nav key as a real panel.

Core layout:

- Left side: project memory source tree.
- Center: memory list with type, title, tags, updated time, enabled state.
- Right side or drawer: Markdown preview/editor.
- Top controls: refresh index, create memory, search/filter, type filter.

Controls:

- Create memory.
- Edit memory.
- Disable/enable memory.
- Delete/tombstone memory.
- Open file location.
- Rebuild index.

### Diagnostics

Extend context diagnostics:

- loaded memory sources;
- skipped memory sources;
- failed memory reads;
- last injected memory IDs;
- stale index warnings.

Timeline:

- Show memory save/update cards for memory tools.
- Show compact memory reference chips on assistant/model-step details.
- Show injected memory count in prompt assembly diagnostics.

## File Safety

Memory file path handling must reject:

- absolute paths from user input;
- `..` path traversal;
- empty path segments;
- non-Markdown extension for memory topic files;
- NUL bytes;
- symlink escapes outside the memory directory.

When writing files:

- resolve target against the project memory root;
- verify target remains inside memory root;
- create parent directories with private permissions where supported;
- prefer atomic write for topic files and `MEMORY.md`;
- update DB only after file write succeeds.

When reading files:

- enforce max file size, initially 512 KB per topic file;
- treat binary or invalid UTF-8 as failed memory source;
- record failed state without crashing the turn.

## Permissions

Memory writes are durable user-visible state changes. Default policy should ask.

Suggested risk classification:

| Operation | Risk |
| --- | --- |
| list/read/search memory | read |
| create/update memory | write |
| disable/delete memory | destructive or write-high |
| rebuild index | read plus metadata write |

The permission prompt should show:

- memory title;
- memory type;
- relative path;
- summary of changed fields;
- source turn/session if available.

## Migration and Compatibility

### Existing Projects

There is no existing Agent Builder memory directory to migrate.

First run behavior:

1. Ensure memory directory exists when memory panel opens or memory is first
   used.
2. Create empty `MEMORY.md` only when needed.
3. Do not inject empty memory.

### Existing Context Files

Do not import `AGENTS.md` or `CLAUDE.md` into memory automatically.

Offer later explicit actions:

- "Create memory from selected context source".
- "Promote memory to AGENTS.md".
- "Move memory to project-local shared file".

### Import From Claude Code / cc-haha

Later import path:

```text
~/.claude/projects/{sanitized-project}/memory/
```

Import should:

- copy or reference topic files;
- preserve frontmatter where possible;
- generate missing IDs;
- rebuild `MEMORY.md`;
- mark imported records with provenance.

## Implementation Phases

### Phase 0: Naming and Boundaries

Deliverables:

- Document that `AGENTS.md` / `CLAUDE.md` are instruction/context sources, not
  memory records.
- Add runtime type names for `project_memory` context source kind.
- Decide project memory directory helper.

Acceptance:

- No behavior change.
- Terminology is consistent across docs and code comments.

### Phase 1: File Scanner and DB Index

Deliverables:

- `internal/memory` package.
- Markdown frontmatter parser/renderer.
- Safe path resolver.
- DB migration for memory records and injections.
- Index rebuild service.
- Unit tests for parsing, path safety, index rebuild, stale/missing files.

Acceptance:

- Runtime can scan `memory/` and rebuild DB.
- DB can be deleted and rebuilt from files.
- Invalid files show failed diagnostics instead of crashing.

### Phase 2: Runtime API and Adapters

Deliverables:

- RuntimeService memory methods.
- HTTP routes.
- Wails bridge forwarding.
- Contract DTO tests.
- Runtime event emission for create/update/delete/index.

Acceptance:

- Memory records can be listed, read, created, updated, disabled, and deleted
  through both HTTP/dev and Wails paths.
- Raw content appears only in explicit detail/edit responses.

### Phase 3: Settings UI

Deliverables:

- Real Settings `memory` panel.
- Memory list, filters, detail preview, Markdown editor.
- Refresh/rebuild index action.
- Create/edit/disable/delete flows.

Acceptance:

- User can manage project memory without leaving Agent Builder.
- UI uses adapter methods only, not direct filesystem or direct fetch.
- Unsaved edit handling is explicit.

### Phase 4: Injection and Diagnostics

Deliverables:

- Deterministic retrieval.
- Memory context sources.
- Prompt assembly memory summaries.
- `project_memory_injections` rows.
- Context diagnostics and timeline chips/cards.

Acceptance:

- A turn records which memory IDs were injected.
- Prompt assembly shows memory counts and hashes without raw body leaks.
- Disabled/deleted memories are not injected.

### Phase 5: Memory Tools

Deliverables:

- `memory_create`, `memory_update`, `memory_disable`, `memory_delete`,
  `memory_search`.
- Permission classification.
- Tool result cards.
- Tests for plan mode and ask policy.

Acceptance:

- Agent can create/update memory only through policy-controlled tools.
- Memory writes are visible in timeline and audit.

### Phase 6: Advanced Recall

Deliverables:

- Optional LLM selector.
- Recency/freshness warnings.
- Duplicate detection.
- Import from Claude Code memory directory.

Acceptance:

- Deterministic path remains the fallback.
- LLM selector failures do not block turns.

### Phase 7: Automatic Extraction and Consolidation

Deliverables:

- User-approved or setting-gated automatic extraction.
- Background consolidation job.
- Memory conflict/duplicate review UI.

Acceptance:

- Automatic writes are opt-in or clearly policy-controlled.
- Background jobs are cancelable and auditable.

## Testing Plan

### Unit Tests

- Frontmatter parse/render.
- Missing frontmatter repair.
- Path traversal rejection.
- Symlink escape rejection.
- Markdown file size limit.
- Index rebuild after manual file edit.
- DB stale row marking when file disappears.
- Deterministic retrieval scoring.

### Runtime Tests

- CRUD through RuntimeService.
- HTTP route parity.
- Wails bridge forwarding.
- Memory events emitted.
- Injection rows recorded.
- Prompt assembly redacts memory body.
- Disabled/deleted records skipped.

### Frontend Tests

- Settings memory nav opens the memory panel.
- List renders records from adapter DTOs.
- Edit/save/disable/delete call adapter methods.
- Unsaved changes prompt before switching files.
- Diagnostics show injected memory IDs.

### Browser Smoke

1. Open project.
2. Create memory from Settings.
3. Start a new turn mentioning matching terms.
4. Confirm context diagnostics show memory injected.
5. Disable memory.
6. Start another turn.
7. Confirm memory is skipped.

## Open Questions

- Should default memory directory live under the same data root as SQLite, or
  under a separate project artifact root?
- Should memory file writes use one topic file per record forever, or allow
  multiple records per Markdown file later?
- Should deleting through UI remove the file immediately or first disable and
  tombstone it?
- Should `MEMORY.md` be injected directly, or should only selected topic files
  be injected in phase 4?
- Should memory be project-only initially, or include user/global memory from
  the first release?

Recommended initial answers:

- Use the same project data root as SQLite.
- One record per Markdown topic file.
- Disable first, delete/tombstone through explicit destructive action.
- Inject selected topic files, not all of `MEMORY.md`.
- Project-only first. Add user/global memory later after product semantics are
  clear.

## Initial Milestone Checklist

- [ ] Add memory terminology to runtime docs.
- [ ] Add DB migration.
- [ ] Implement `internal/memory` scanner and store.
- [ ] Add runtime list/detail/create/update/disable/delete methods.
- [ ] Add HTTP and Wails adapter coverage.
- [ ] Build Settings memory panel.
- [ ] Add deterministic retrieval and prompt assembly evidence.
- [ ] Add memory tools with permission policy.
- [ ] Add import/advanced recall later.

