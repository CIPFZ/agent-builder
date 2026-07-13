-- +goose Up
CREATE TABLE application_settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE policy_settings (
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL,
    profile TEXT NOT NULL,
    rules_json TEXT NOT NULL DEFAULT '[]',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (scope, project_id)
);

CREATE TABLE skill_registrations (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    path TEXT,
    name TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE mcp_servers (
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (name, scope, project_id)
);

-- +goose Down
DROP TABLE mcp_servers;
DROP TABLE skill_registrations;
DROP TABLE policy_settings;
DROP TABLE application_settings;
