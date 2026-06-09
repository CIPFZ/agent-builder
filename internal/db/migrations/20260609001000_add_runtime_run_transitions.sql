-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_run_transitions (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    session_id TEXT,
    turn_id TEXT,
    task_id TEXT,
    from_status TEXT,
    to_status TEXT NOT NULL,
    reason TEXT,
    source TEXT NOT NULL,
    event_id TEXT,
    created_at INTEGER NOT NULL,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_run_transitions_run_created_at
    ON runtime_run_transitions (run_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_run_transitions_session_created_at
    ON runtime_run_transitions (session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_run_transitions_turn_created_at
    ON runtime_run_transitions (turn_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_run_transitions_turn_created_at;
DROP INDEX IF EXISTS idx_runtime_run_transitions_session_created_at;
DROP INDEX IF EXISTS idx_runtime_run_transitions_run_created_at;
DROP TABLE IF EXISTS runtime_run_transitions;
