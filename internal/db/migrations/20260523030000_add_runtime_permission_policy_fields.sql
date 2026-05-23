-- +goose Up
ALTER TABLE runtime_permission_requests ADD COLUMN policy_mode TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN policy_reason TEXT;
ALTER TABLE runtime_permission_requests ADD COLUMN decision TEXT;

-- +goose Down
CREATE TABLE runtime_permission_requests_old AS SELECT
    id, session_id, turn_id, tool_call_id, tool_name, description, action,
    params_json, path, target, risk, status, created_at, decided_at
FROM runtime_permission_requests;

DROP TABLE runtime_permission_requests;

ALTER TABLE runtime_permission_requests_old RENAME TO runtime_permission_requests;

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_status_created_at
    ON runtime_permission_requests (status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_turn_created_at
    ON runtime_permission_requests (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_tool_call_id
    ON runtime_permission_requests (tool_call_id);
