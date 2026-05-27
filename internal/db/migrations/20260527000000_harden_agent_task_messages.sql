-- +goose Up
ALTER TABLE runtime_agent_task_messages ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_agent_task_messages ADD COLUMN processed_at INTEGER;
ALTER TABLE runtime_agent_task_messages ADD COLUMN error TEXT;

CREATE INDEX IF NOT EXISTS idx_runtime_agent_task_messages_task_sequence
    ON runtime_agent_task_messages (task_id, sequence);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_agent_task_messages_task_sequence;
