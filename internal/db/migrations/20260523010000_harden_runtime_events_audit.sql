-- +goose Up
ALTER TABLE runtime_audit_events ADD COLUMN tool_call_id TEXT;
ALTER TABLE runtime_audit_events ADD COLUMN permission_id TEXT;

CREATE INDEX IF NOT EXISTS idx_runtime_audit_events_tool_call_id_created_at
    ON runtime_audit_events (tool_call_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_audit_events_permission_id_created_at
    ON runtime_audit_events (permission_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_audit_events_permission_id_created_at;
DROP INDEX IF EXISTS idx_runtime_audit_events_tool_call_id_created_at;
