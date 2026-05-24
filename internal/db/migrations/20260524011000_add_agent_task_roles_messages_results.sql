-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_agent_roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    title TEXT,
    description TEXT,
    prompt_summary TEXT,
    allowed_tools_json TEXT,
    capability_scope_json TEXT,
    model TEXT,
    provider TEXT,
    cwd TEXT,
    worktree TEXT,
    risk TEXT,
    policy_metadata_json TEXT,
    source TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_agent_task_messages (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    parent_task_id TEXT,
    parent_turn_id TEXT,
    parent_session_id TEXT,
    child_session_id TEXT,
    direction TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    content_summary TEXT,
    payload_json TEXT,
    related_tool_call_id TEXT,
    related_message_id TEXT,
    artifact_refs_json TEXT,
    created_at INTEGER NOT NULL,
    delivered_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_task_messages_task_created_at
    ON runtime_agent_task_messages (task_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_agent_task_messages_parent_turn_created_at
    ON runtime_agent_task_messages (parent_turn_id, created_at);

CREATE TABLE IF NOT EXISTS runtime_agent_task_results (
    task_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    summary TEXT,
    error_detail TEXT,
    cancellation_detail TEXT,
    artifact_refs_json TEXT,
    related_message_refs_json TEXT,
    related_tool_call_refs_json TEXT,
    compact_boundary_refs_json TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS runtime_agent_task_results;
DROP INDEX IF EXISTS idx_runtime_agent_task_messages_parent_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_agent_task_messages_task_created_at;
DROP TABLE IF EXISTS runtime_agent_task_messages;
DROP TABLE IF EXISTS runtime_agent_roles;
