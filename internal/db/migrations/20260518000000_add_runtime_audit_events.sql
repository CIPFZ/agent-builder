-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_audit_events (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    turn_id TEXT,
    type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    payload_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_audit_events_turn_id_created_at
    ON runtime_audit_events (turn_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_audit_events_turn_id_created_at;
DROP TABLE IF EXISTS runtime_audit_events;
