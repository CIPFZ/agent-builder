-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_permission_requests (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    tool_call_id TEXT,
    tool_name TEXT NOT NULL,
    description TEXT,
    action TEXT NOT NULL,
    params_json TEXT,
    path TEXT,
    target TEXT,
    risk TEXT,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    decided_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_status_created_at
    ON runtime_permission_requests (status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_turn_created_at
    ON runtime_permission_requests (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_permission_requests_tool_call_id
    ON runtime_permission_requests (tool_call_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_permission_requests_tool_call_id;
DROP INDEX IF EXISTS idx_runtime_permission_requests_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_permission_requests_status_created_at;
DROP TABLE IF EXISTS runtime_permission_requests;
