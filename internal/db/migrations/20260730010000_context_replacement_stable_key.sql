-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_context_replacements_stable_key
    ON runtime_context_content_replacements (session_id, tool_call_id, kind);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_context_replacements_stable_key;
