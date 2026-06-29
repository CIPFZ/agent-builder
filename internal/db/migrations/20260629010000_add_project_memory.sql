-- +goose Up
CREATE TABLE IF NOT EXISTS project_memory_records (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    content_hash TEXT NOT NULL,
    mtime_unix INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_from_session_id TEXT,
    created_from_turn_id TEXT,
    last_indexed_at TEXT NOT NULL,
    last_injected_at TEXT,
    UNIQUE(project_id, relative_path)
);

CREATE INDEX IF NOT EXISTS idx_project_memory_records_project
    ON project_memory_records(project_id, enabled, deleted_at, updated_at);

CREATE INDEX IF NOT EXISTS idx_project_memory_records_type
    ON project_memory_records(project_id, type);

CREATE TABLE IF NOT EXISTS project_memory_injections (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    prompt_assembly_id TEXT,
    injected_at TEXT NOT NULL,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    selection_reason TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(memory_id) REFERENCES project_memory_records(id)
);

CREATE INDEX IF NOT EXISTS idx_project_memory_injections_turn
    ON project_memory_injections(session_id, turn_id);

-- +goose Down
DROP INDEX IF EXISTS idx_project_memory_injections_turn;
DROP TABLE IF EXISTS project_memory_injections;
DROP INDEX IF EXISTS idx_project_memory_records_type;
DROP INDEX IF EXISTS idx_project_memory_records_project;
DROP TABLE IF EXISTS project_memory_records;
