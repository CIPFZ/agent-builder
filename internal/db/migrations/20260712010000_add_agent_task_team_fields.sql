-- +goose Up
ALTER TABLE runtime_agent_tasks ADD COLUMN parent_task_id TEXT;
ALTER TABLE runtime_agent_tasks ADD COLUMN team_id TEXT;
ALTER TABLE runtime_agent_tasks ADD COLUMN dependencies_json TEXT;
CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_parent_task_id ON runtime_agent_tasks (parent_task_id);
CREATE INDEX IF NOT EXISTS idx_runtime_agent_tasks_team_id ON runtime_agent_tasks (team_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_agent_tasks_team_id;
DROP INDEX IF EXISTS idx_runtime_agent_tasks_parent_task_id;
ALTER TABLE runtime_agent_tasks DROP COLUMN dependencies_json;
ALTER TABLE runtime_agent_tasks DROP COLUMN team_id;
ALTER TABLE runtime_agent_tasks DROP COLUMN parent_task_id;
