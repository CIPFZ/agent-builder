-- +goose Up
CREATE TABLE IF NOT EXISTS objects (
    id TEXT PRIMARY KEY,
    uri TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    kind TEXT NOT NULL,
    media_type TEXT,
    content_type TEXT,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    estimated_tokens INTEGER NOT NULL DEFAULT 0,
    preview TEXT,
    summary TEXT,
    storage_kind TEXT NOT NULL,
    storage_path TEXT,
    inline_payload TEXT,
    redaction_status TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_objects_session_created_at
    ON objects (session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_objects_project_created_at
    ON objects (project_id, created_at);

CREATE INDEX IF NOT EXISTS idx_objects_turn_created_at
    ON objects (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_objects_tool_call_created_at
    ON objects (tool_call_id, created_at);

CREATE INDEX IF NOT EXISTS idx_objects_task_created_at
    ON objects (task_id, created_at);

CREATE INDEX IF NOT EXISTS idx_objects_kind_created_at
    ON objects (kind, created_at);

ALTER TABLE runtime_tool_calls ADD COLUMN output_refs_json TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN artifact_refs_json TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN diff_refs_json TEXT;

-- +goose Down
DROP INDEX IF EXISTS idx_objects_kind_created_at;
DROP INDEX IF EXISTS idx_objects_task_created_at;
DROP INDEX IF EXISTS idx_objects_tool_call_created_at;
DROP INDEX IF EXISTS idx_objects_turn_created_at;
DROP INDEX IF EXISTS idx_objects_session_created_at;
DROP INDEX IF EXISTS idx_objects_project_created_at;
DROP TABLE IF EXISTS objects;

CREATE TABLE runtime_tool_calls_old AS SELECT
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value,
    policy_target_summary, shell_risk, shell_reason, exit_code, job_status,
    job_started_at, job_finished_at, input_summary, output_summary,
    model_content, structured_output, stdout, stderr, is_error, compacted,
    compact_ref, compact_boundary_id, compact_original_estimated_tokens,
    compacted_at, started_at, finished_at, error
FROM runtime_tool_calls;

DROP TABLE runtime_tool_calls;

ALTER TABLE runtime_tool_calls_old RENAME TO runtime_tool_calls;

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);
