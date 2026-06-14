-- +goose Up
-- +goose StatementBegin
ALTER TABLE sessions ADD COLUMN project_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN scope TEXT NOT NULL DEFAULT 'project';
CREATE INDEX IF NOT EXISTS idx_sessions_scope_project_id ON sessions (scope, project_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_sessions_scope_project_id;
ALTER TABLE sessions DROP COLUMN scope;
ALTER TABLE sessions DROP COLUMN project_id;
-- +goose StatementEnd
