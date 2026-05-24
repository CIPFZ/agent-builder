-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_mcp_requests (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    server TEXT NOT NULL,
    capability_id TEXT,
    session_id TEXT,
    turn_id TEXT,
    status TEXT NOT NULL,
    prompt TEXT,
    description TEXT,
    response_summary TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_decision TEXT,
    policy_reason TEXT,
    policy_risk TEXT,
    policy_rule_id TEXT,
    policy_rule_source TEXT,
    policy_scope_kind TEXT,
    policy_scope_value TEXT,
    policy_target_summary TEXT,
    policy_headless INTEGER DEFAULT 0,
    policy_headless_reason TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    expires_at INTEGER,
    completed_at INTEGER,
    error TEXT,
    redacted INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_runtime_mcp_requests_status_created_at
    ON runtime_mcp_requests (status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_mcp_requests_kind_status_created_at
    ON runtime_mcp_requests (kind, status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_mcp_requests_server_status_created_at
    ON runtime_mcp_requests (server, status, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_mcp_requests_turn_created_at
    ON runtime_mcp_requests (turn_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_mcp_requests_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_mcp_requests_server_status_created_at;
DROP INDEX IF EXISTS idx_runtime_mcp_requests_kind_status_created_at;
DROP INDEX IF EXISTS idx_runtime_mcp_requests_status_created_at;
DROP TABLE IF EXISTS runtime_mcp_requests;
