-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_user_inputs (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    turn_id TEXT,
    project_id TEXT,
    scope TEXT,
    mode TEXT NOT NULL,
    prompt_preview TEXT,
    items_json TEXT NOT NULL,
    normalized_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_user_inputs_session_created_at
    ON runtime_user_inputs (session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_user_inputs_turn_id
    ON runtime_user_inputs (turn_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_user_inputs_turn_id;
DROP INDEX IF EXISTS idx_runtime_user_inputs_session_created_at;
DROP TABLE IF EXISTS runtime_user_inputs;
