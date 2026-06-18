-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_prompt_assemblies (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    step INTEGER NOT NULL,
    provider TEXT,
    model TEXT,
    system_json TEXT,
    messages_json TEXT,
    tools_json TEXT,
    skills_json TEXT,
    mcp_json TEXT,
    context_sources_json TEXT,
    compact_json TEXT,
    budget_json TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_prompt_assemblies_turn_step
    ON runtime_prompt_assemblies (turn_id, step);

CREATE INDEX IF NOT EXISTS idx_runtime_prompt_assemblies_session_created_at
    ON runtime_prompt_assemblies (session_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_prompt_assemblies_session_created_at;
DROP INDEX IF EXISTS idx_runtime_prompt_assemblies_turn_step;
DROP TABLE IF EXISTS runtime_prompt_assemblies;
