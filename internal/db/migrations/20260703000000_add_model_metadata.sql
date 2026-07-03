-- +goose Up
-- +goose StatementBegin
ALTER TABLE configured_providers RENAME COLUMN model_ids_json TO models_json;
ALTER TABLE configured_providers ADD COLUMN default_context_window INTEGER;

UPDATE configured_providers
SET models_json = COALESCE((
    SELECT json_group_array(json_object('id', value))
    FROM json_each(configured_providers.models_json)
    WHERE type = 'text'
), '[]')
WHERE json_valid(models_json)
  AND json_type(models_json) = 'array'
  AND EXISTS (
      SELECT 1
      FROM json_each(configured_providers.models_json)
      WHERE type = 'text'
  );

CREATE TABLE IF NOT EXISTS model_metadata_cache (
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    context_window INTEGER,
    max_output_tokens INTEGER,
    source TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (provider_id, model_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS model_metadata_cache;
ALTER TABLE configured_providers DROP COLUMN default_context_window;
ALTER TABLE configured_providers RENAME COLUMN models_json TO model_ids_json;
-- +goose StatementEnd
