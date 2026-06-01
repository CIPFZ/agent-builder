-- +goose Up
CREATE TABLE IF NOT EXISTS provider_catalog (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    api_endpoint TEXT,
    api_key_template TEXT,
    model_count INTEGER NOT NULL DEFAULT 0,
    default_large_model TEXT,
    default_small_model TEXT,
    required_fields_json TEXT NOT NULL DEFAULT '[]',
    notes_json TEXT NOT NULL DEFAULT '[]',
    configurable INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS configured_providers (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    name TEXT NOT NULL,
    remark TEXT,
    protocol TEXT NOT NULL,
    api_endpoint TEXT NOT NULL,
    api_key TEXT,
    api_key_secret_ref TEXT,
    proxy TEXT,
    default_model TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES provider_catalog(id)
);

CREATE INDEX IF NOT EXISTS idx_configured_providers_provider_id
    ON configured_providers (provider_id);

CREATE INDEX IF NOT EXISTS idx_configured_providers_enabled
    ON configured_providers (enabled);

-- +goose Down
DROP INDEX IF EXISTS idx_configured_providers_enabled;
DROP INDEX IF EXISTS idx_configured_providers_provider_id;
DROP TABLE IF EXISTS configured_providers;
DROP TABLE IF EXISTS provider_catalog;
