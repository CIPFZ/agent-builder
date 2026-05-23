-- +goose Up
ALTER TABLE runtime_tool_calls
    ADD COLUMN job_id TEXT;

ALTER TABLE runtime_tool_calls
    ADD COLUMN command TEXT;

ALTER TABLE runtime_tool_calls
    ADD COLUMN risk TEXT;

ALTER TABLE runtime_tool_calls
    ADD COLUMN policy_reason TEXT;

ALTER TABLE runtime_tool_calls
    ADD COLUMN exit_code INTEGER NOT NULL DEFAULT 0;

-- +goose Down
CREATE TABLE runtime_tool_calls_old AS SELECT
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
    started_at, finished_at, error
FROM runtime_tool_calls;

DROP TABLE runtime_tool_calls;

ALTER TABLE runtime_tool_calls_old RENAME TO runtime_tool_calls;

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_turn_started_at
    ON runtime_tool_calls (turn_id, started_at);

CREATE INDEX IF NOT EXISTS idx_runtime_tool_calls_session_started_at
    ON runtime_tool_calls (session_id, started_at);
