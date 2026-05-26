-- +goose Up
-- +goose StatementBegin
ALTER TABLE read_files ADD COLUMN turn_id TEXT NOT NULL DEFAULT '';
ALTER TABLE read_files ADD COLUMN tool_call_id TEXT NOT NULL DEFAULT '';
ALTER TABLE read_files ADD COLUMN size_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE read_files ADD COLUMN mtime_unix INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN read_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN partial INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN token_estimate INTEGER NOT NULL DEFAULT 0;
ALTER TABLE read_files ADD COLUMN state TEXT NOT NULL DEFAULT 'recorded';
ALTER TABLE read_files ADD COLUMN reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_read_files_session_turn ON read_files (session_id, turn_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_read_files_session_turn;
-- SQLite cannot drop columns without rebuilding the table; keep the widened
-- table on down migrations to avoid destructive read-file state loss.
-- +goose StatementEnd
