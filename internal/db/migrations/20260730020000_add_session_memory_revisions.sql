-- +goose Up
CREATE TABLE runtime_session_memory_revisions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    revision INTEGER NOT NULL,
    status TEXT NOT NULL,
    base_revision INTEGER,
    content TEXT,
    content_hash TEXT,
    last_summarized_message_id TEXT,
    source_message_count INTEGER NOT NULL DEFAULT 0,
    source_token_estimate INTEGER NOT NULL DEFAULT 0,
    source_tool_call_count INTEGER NOT NULL DEFAULT 0,
    provider TEXT,
    model TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT,
    UNIQUE (session_id, revision)
);
CREATE INDEX idx_runtime_session_memory_status_revision
    ON runtime_session_memory_revisions (session_id, status, revision);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_session_memory_status_revision;
DROP TABLE IF EXISTS runtime_session_memory_revisions;
