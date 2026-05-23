-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL,
    user_message_id TEXT,
    latest_assistant_message_id TEXT,
    provider TEXT,
    model TEXT,
    prompt_preview TEXT,
    usage_before_json TEXT,
    usage_after_json TEXT,
    usage_delta_json TEXT,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_turns_status_updated_at
    ON runtime_turns (status, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_turns_session_updated_at
    ON runtime_turns (session_id, updated_at);

CREATE TABLE IF NOT EXISTS runtime_tool_calls (
    id TEXT PRIMARY KEY,
    turn_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    message_id TEXT,
    name TEXT NOT NULL,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT,
    output_summary TEXT,
    stdout TEXT,
    stderr TEXT,
    is_error INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_tool_calls_session_started_at;
DROP INDEX IF EXISTS idx_runtime_tool_calls_turn_started_at;
DROP TABLE IF EXISTS runtime_tool_calls;
DROP INDEX IF EXISTS idx_runtime_turns_session_updated_at;
DROP INDEX IF EXISTS idx_runtime_turns_status_updated_at;
DROP TABLE IF EXISTS runtime_turns;
