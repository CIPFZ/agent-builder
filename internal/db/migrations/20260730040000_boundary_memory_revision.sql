-- +goose Up
ALTER TABLE runtime_context_boundaries ADD COLUMN memory_revision INTEGER;

-- +goose Down
-- SQLite cannot safely drop individual columns on all supported versions.
