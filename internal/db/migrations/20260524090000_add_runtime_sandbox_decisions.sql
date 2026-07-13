-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_sandbox_decisions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    executor TEXT,
    cwd TEXT,
    worktree_id TEXT,
    worktree_path TEXT,
    command_summary TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_rule TEXT,
    reason TEXT,
    error TEXT,
    allowed_paths_json TEXT,
    denied_paths_json TEXT,
    network_allowed INTEGER NOT NULL DEFAULT 0,
    network_reason TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runtime_sandbox_decisions_session_created_at
    ON runtime_sandbox_decisions (session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_sandbox_decisions_turn_created_at
    ON runtime_sandbox_decisions (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_sandbox_decisions_tool_call_created_at
    ON runtime_sandbox_decisions (tool_call_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_sandbox_decisions_task_created_at
    ON runtime_sandbox_decisions (task_id, created_at);

ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_decision_id TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_mode TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_status TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_executor TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_reason TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN sandbox_error TEXT;

ALTER TABLE objects ADD COLUMN sandbox_decision_id TEXT;
ALTER TABLE objects ADD COLUMN sandbox_mode TEXT;
ALTER TABLE objects ADD COLUMN sandbox_status TEXT;

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_sandbox_decisions_task_created_at;
DROP INDEX IF EXISTS idx_runtime_sandbox_decisions_tool_call_created_at;
DROP INDEX IF EXISTS idx_runtime_sandbox_decisions_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_sandbox_decisions_session_created_at;
DROP TABLE IF EXISTS runtime_sandbox_decisions;

CREATE TABLE runtime_tool_calls_old AS SELECT
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value,
    policy_target_summary, shell_risk, shell_reason, exit_code, job_status,
    job_started_at, job_finished_at, input_summary, output_summary,
    model_content, structured_output, stdout, stderr, is_error,
    output_refs_json, artifact_refs_json, diff_refs_json,
    compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens,
    compacted_at, started_at, finished_at, error
FROM runtime_tool_calls;

DROP TABLE runtime_tool_calls;
ALTER TABLE runtime_tool_calls_old RENAME TO runtime_tool_calls;

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);

CREATE TABLE objects_old AS SELECT
    id, uri, project_id, session_id, turn_id, tool_call_id, task_id, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM objects;

DROP TABLE objects;
ALTER TABLE objects_old RENAME TO objects;

CREATE UNIQUE INDEX IF NOT EXISTS idx_objects_uri
    ON objects (uri);

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
