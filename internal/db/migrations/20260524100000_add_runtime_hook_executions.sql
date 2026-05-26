-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_hook_executions (
    id TEXT PRIMARY KEY,
    hook_id TEXT NOT NULL,
    hook_name TEXT,
    hook_source TEXT,
    event TEXT NOT NULL,
    status TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    tool_call_id TEXT,
    task_id TEXT,
    capability_id TEXT,
    mcp_server TEXT,
    skill TEXT,
    context_ref TEXT,
    policy_mode TEXT,
    policy_profile TEXT,
    policy_rule TEXT,
    policy_decision TEXT,
    policy_reason TEXT,
    headless INTEGER NOT NULL DEFAULT 0,
    headless_reason TEXT,
    sandbox_decision_id TEXT,
    sandbox_status TEXT,
    scope_kind TEXT,
    scope_value TEXT,
    reason TEXT,
    error TEXT,
    input_summary TEXT,
    output_summary TEXT,
    context_summary TEXT,
    input_rewritten INTEGER NOT NULL DEFAULT 0,
    context_injected INTEGER NOT NULL DEFAULT 0,
    redacted INTEGER NOT NULL DEFAULT 1,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_runtime_hook_executions_session_started_at
    ON runtime_hook_executions (session_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_hook_executions_turn_started_at
    ON runtime_hook_executions (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_hook_executions_tool_call_started_at
    ON runtime_hook_executions (tool_call_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_hook_executions_task_started_at
    ON runtime_hook_executions (task_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_hook_executions_event_started_at
    ON runtime_hook_executions (event, started_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_hook_executions_event_started_at;
DROP INDEX IF EXISTS idx_runtime_hook_executions_task_started_at;
DROP INDEX IF EXISTS idx_runtime_hook_executions_tool_call_started_at;
DROP INDEX IF EXISTS idx_runtime_hook_executions_turn_started_at;
DROP INDEX IF EXISTS idx_runtime_hook_executions_session_started_at;
DROP TABLE IF EXISTS runtime_hook_executions;
