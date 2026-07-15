-- +goose Up
CREATE TABLE runtime_token_usage_daily (
    day TEXT PRIMARY KEY, timezone TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0, session_count INTEGER NOT NULL DEFAULT 0,
    turn_count INTEGER NOT NULL DEFAULT 0, model_call_count INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
CREATE TABLE runtime_token_usage_lifetime (
    id INTEGER PRIMARY KEY CHECK (id = 1), input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0, cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0, reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0, model_call_count INTEGER NOT NULL DEFAULT 0,
    peak_tokens INTEGER NOT NULL DEFAULT 0, peak_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
CREATE TABLE token_statistics_cursor (
    id INTEGER PRIMARY KEY CHECK (id = 1), sequence INTEGER NOT NULL DEFAULT 0,
    backfilled INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL
);
INSERT INTO runtime_token_usage_lifetime (id, updated_at) VALUES (1, strftime('%s','now') * 1000);
INSERT INTO token_statistics_cursor (id, updated_at) VALUES (1, strftime('%s','now') * 1000);
CREATE UNIQUE INDEX idx_runtime_events_message_completed_once ON runtime_events(message_id) WHERE type='message.completed' AND message_id IS NOT NULL;
-- +goose Down
DROP INDEX idx_runtime_events_message_completed_once;
DROP TABLE token_statistics_cursor;
DROP TABLE runtime_token_usage_lifetime;
DROP TABLE runtime_token_usage_daily;
