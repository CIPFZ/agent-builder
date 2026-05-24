-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_worktrees (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    task_id TEXT,
    base_repo_path TEXT NOT NULL,
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    ref TEXT,
    status TEXT NOT NULL,
    preserve_policy TEXT NOT NULL,
    cleanup_policy TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    entered_at INTEGER,
    exited_at INTEGER,
    cleaned_at INTEGER,
    updated_at INTEGER NOT NULL,
    error TEXT,
    owner TEXT,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_worktrees_session_updated_at
    ON runtime_worktrees (session_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_worktrees_turn_updated_at
    ON runtime_worktrees (turn_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_worktrees_task_updated_at
    ON runtime_worktrees (task_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_worktrees_status_updated_at
    ON runtime_worktrees (status, updated_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_worktrees_worktree_path
    ON runtime_worktrees (worktree_path);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_worktrees_worktree_path;
DROP INDEX IF EXISTS idx_runtime_worktrees_status_updated_at;
DROP INDEX IF EXISTS idx_runtime_worktrees_task_updated_at;
DROP INDEX IF EXISTS idx_runtime_worktrees_turn_updated_at;
DROP INDEX IF EXISTS idx_runtime_worktrees_session_updated_at;
DROP TABLE IF EXISTS runtime_worktrees;
