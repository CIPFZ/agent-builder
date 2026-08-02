-- +goose Up
ALTER TABLE runtime_context_boundaries ADD COLUMN preserved_message_refs_json TEXT;
ALTER TABLE runtime_context_boundaries ADD COLUMN boundary_cutoff_message_id TEXT;
ALTER TABLE runtime_context_boundaries ADD COLUMN summary_mode TEXT;

-- +goose Down
ALTER TABLE runtime_context_boundaries DROP COLUMN summary_mode;
ALTER TABLE runtime_context_boundaries DROP COLUMN boundary_cutoff_message_id;
ALTER TABLE runtime_context_boundaries DROP COLUMN preserved_message_refs_json;
