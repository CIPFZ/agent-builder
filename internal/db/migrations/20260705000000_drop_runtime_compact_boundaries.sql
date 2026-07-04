-- +goose Up
DROP INDEX IF EXISTS idx_runtime_compact_boundaries_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_compact_boundaries_session_created_at;
DROP TABLE IF EXISTS runtime_compact_boundaries;

-- +goose Down
CREATE TABLE IF NOT EXISTS runtime_compact_boundaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    kind TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    budget_before_json TEXT,
    budget_after_json TEXT,
    summary_ref TEXT,
    message_refs_json TEXT,
    tool_call_refs_json TEXT,
    reinjected_refs_json TEXT,
    error TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runtime_compact_boundaries_session_created_at
    ON runtime_compact_boundaries (session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_compact_boundaries_turn_created_at
    ON runtime_compact_boundaries (turn_id, created_at);
