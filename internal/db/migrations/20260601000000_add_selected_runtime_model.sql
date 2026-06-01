-- +goose Up
CREATE TABLE IF NOT EXISTS selected_models (
    id TEXT PRIMARY KEY,
    configured_provider_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'global',
    project_id TEXT,
    session_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (configured_provider_id) REFERENCES configured_providers(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_selected_models_scope
    ON selected_models (
        scope,
        COALESCE(project_id, ''),
        COALESCE(session_id, '')
    );

CREATE INDEX IF NOT EXISTS idx_selected_models_configured_provider
    ON selected_models (configured_provider_id);

-- +goose Down
DROP INDEX IF EXISTS idx_selected_models_configured_provider;
DROP INDEX IF EXISTS idx_selected_models_scope;
DROP TABLE IF EXISTS selected_models;
