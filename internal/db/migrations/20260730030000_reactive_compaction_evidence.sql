-- +goose Up
ALTER TABLE runtime_context_reactive_attempts ADD COLUMN budget_before_json TEXT;
ALTER TABLE runtime_context_reactive_attempts ADD COLUMN budget_after_json TEXT;
ALTER TABLE runtime_context_reactive_attempts ADD COLUMN will_retry INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_context_reactive_attempts ADD COLUMN circuit_open INTEGER NOT NULL DEFAULT 0;

CREATE TABLE runtime_context_circuit_states (
    session_id TEXT PRIMARY KEY,
    failure_count INTEGER NOT NULL DEFAULT 0,
    circuit_open INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS runtime_context_circuit_states;
-- SQLite cannot safely drop individual columns on all supported versions.
