-- +goose Up
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

ALTER TABLE runtime_tool_calls ADD COLUMN compacted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_tool_calls ADD COLUMN compact_ref TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN compact_boundary_id TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN compact_original_estimated_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_tool_calls ADD COLUMN compacted_at INTEGER;

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_compact_boundaries_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_compact_boundaries_session_created_at;
DROP TABLE IF EXISTS runtime_compact_boundaries;
