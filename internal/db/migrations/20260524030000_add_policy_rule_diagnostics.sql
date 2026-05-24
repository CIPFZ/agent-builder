-- +goose Up
ALTER TABLE runtime_tool_calls ADD COLUMN policy_mode TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_profile TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_rule_id TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_rule_source TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_scope_kind TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_scope_value TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN policy_target_summary TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN shell_risk TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN shell_reason TEXT;

ALTER TABLE runtime_permission_requests ADD COLUMN policy_profile TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_rule_id TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_rule_source TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_scope_kind TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_scope_value TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_target_summary TEXT;

-- +goose Down
CREATE TABLE runtime_tool_calls_old AS SELECT
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, exit_code, job_status, job_started_at, job_finished_at,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
    compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens, compacted_at,
    started_at, finished_at, error
FROM runtime_tool_calls;

DROP TABLE runtime_tool_calls;

ALTER TABLE runtime_tool_calls_old RENAME TO runtime_tool_calls;

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);

CREATE TABLE runtime_permission_requests_old AS SELECT
    id, session_id, turn_id, tool_call_id, tool_name, description, action,
    params_json, path, target, risk, policy_mode, policy_reason, decision,
    status, created_at, decided_at
FROM runtime_permission_requests;

DROP TABLE runtime_permission_requests;

ALTER TABLE runtime_permission_requests_old RENAME TO runtime_permission_requests;

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_status_created_at
    ON runtime_permission_requests (status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_turn_created_at
    ON runtime_permission_requests (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_tool_call_id
    ON runtime_permission_requests (tool_call_id);
