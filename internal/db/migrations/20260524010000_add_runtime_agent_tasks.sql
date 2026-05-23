-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_agent_tasks (
    id TEXT PRIMARY KEY,
    parent_turn_id TEXT,
    parent_session_id TEXT NOT NULL,
    parent_tool_call_id TEXT,
    child_session_id TEXT,
    title TEXT NOT NULL,
    kind TEXT NOT NULL,
    role TEXT,
    name TEXT,
    prompt_summary TEXT,
    model TEXT,
    provider TEXT,
    allowed_tools_json TEXT,
    capability_scope_json TEXT,
    cwd TEXT,
    worktree TEXT,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    result_summary TEXT,
    artifact_refs_json TEXT,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_parent_turn_updated_at
    ON runtime_agent_tasks (parent_turn_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_parent_session_updated_at
    ON runtime_agent_tasks (parent_session_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_status_updated_at
    ON runtime_agent_tasks (status, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_child_session_id
    ON runtime_agent_tasks (child_session_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_agent_tasks_child_session_id;
DROP INDEX IF EXISTS idx_runtime_agent_tasks_status_updated_at;
DROP INDEX IF EXISTS idx_runtime_agent_tasks_parent_session_updated_at;
DROP INDEX IF EXISTS idx_runtime_agent_tasks_parent_turn_updated_at;
DROP TABLE IF EXISTS runtime_agent_tasks;
