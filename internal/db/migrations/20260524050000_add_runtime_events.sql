-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_events (
    sequence INTEGER PRIMARY KEY,
    id TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    message_id TEXT,
    tool_call_id TEXT,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_events_session_sequence
    ON runtime_events (session_id, sequence);

CREATE INDEX IF NOT EXISTS idx_runtime_events_turn_sequence
    ON runtime_events (turn_id, sequence);

CREATE INDEX IF NOT EXISTS idx_runtime_events_tool_call_sequence
    ON runtime_events (tool_call_id, sequence);

CREATE INDEX IF NOT EXISTS idx_runtime_events_type_sequence
    ON runtime_events (type, sequence);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_events_type_sequence;
DROP INDEX IF EXISTS idx_runtime_events_tool_call_sequence;
DROP INDEX IF EXISTS idx_runtime_events_turn_sequence;
DROP INDEX IF EXISTS idx_runtime_events_session_sequence;
DROP TABLE IF EXISTS runtime_events;
