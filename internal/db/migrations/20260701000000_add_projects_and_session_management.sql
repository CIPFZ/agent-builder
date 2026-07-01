-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    canonical_path TEXT NOT NULL,
    data_dir TEXT NOT NULL,
    git_root TEXT,
    branch TEXT,
    is_git_repository INTEGER NOT NULL DEFAULT 0,
    exists_on_disk INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_opened_at INTEGER,
    deleted_at INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_active_canonical_path
ON projects(canonical_path)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_projects_last_opened
ON projects(deleted_at, last_opened_at DESC);

ALTER TABLE sessions ADD COLUMN deleted_at INTEGER;
ALTER TABLE sessions ADD COLUMN last_opened_at INTEGER;
ALTER TABLE sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN title_source TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE sessions ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

CREATE INDEX IF NOT EXISTS idx_sessions_project_active
ON sessions(project_id, deleted_at, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_scope_active
ON sessions(scope, deleted_at, updated_at DESC);

CREATE TABLE IF NOT EXISTS runtime_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO runtime_settings (key, value, updated_at)
VALUES ('delete_mode', 'hard', strftime('%s', 'now'))
ON CONFLICT(key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS runtime_settings;
DROP INDEX IF EXISTS idx_sessions_scope_active;
DROP INDEX IF EXISTS idx_sessions_project_active;
ALTER TABLE sessions DROP COLUMN status;
ALTER TABLE sessions DROP COLUMN title_source;
ALTER TABLE sessions DROP COLUMN pinned;
ALTER TABLE sessions DROP COLUMN last_opened_at;
ALTER TABLE sessions DROP COLUMN deleted_at;
DROP INDEX IF EXISTS idx_projects_last_opened;
DROP INDEX IF EXISTS idx_projects_active_canonical_path;
DROP TABLE IF EXISTS projects;
-- +goose StatementEnd
