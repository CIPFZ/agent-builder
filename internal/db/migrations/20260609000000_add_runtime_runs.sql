-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_runs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    primary_session_id TEXT NOT NULL,
    objective TEXT,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    discarded_at INTEGER,
    source TEXT NOT NULL,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_runs_workspace_updated_at
    ON runtime_runs (workspace_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_runtime_runs_primary_session_id
    ON runtime_runs (primary_session_id);

CREATE TABLE IF NOT EXISTS runtime_run_sessions (
    run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    task_id TEXT,
    turn_id TEXT,
    worktree_id TEXT,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (run_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_runtime_run_sessions_session_id
    ON runtime_run_sessions (session_id);

CREATE TABLE IF NOT EXISTS runtime_run_checkpoints (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    task_id TEXT,
    status TEXT NOT NULL,
    summary TEXT,
    artifact_refs_json TEXT,
    diagnostic_refs_json TEXT,
    created_at INTEGER NOT NULL,
    acknowledged_at INTEGER,
    discarded_at INTEGER,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_run_checkpoints_run_created_at
    ON runtime_run_checkpoints (run_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_run_checkpoints_session_id
    ON runtime_run_checkpoints (session_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_run_checkpoints_session_id;
DROP INDEX IF EXISTS idx_runtime_run_checkpoints_run_created_at;
DROP TABLE IF EXISTS runtime_run_checkpoints;
DROP INDEX IF EXISTS idx_runtime_run_sessions_session_id;
DROP TABLE IF EXISTS runtime_run_sessions;
DROP INDEX IF EXISTS idx_runtime_runs_primary_session_id;
DROP INDEX IF EXISTS idx_runtime_runs_workspace_updated_at;
DROP TABLE IF EXISTS runtime_runs;
