-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_context_projections (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    step INTEGER NOT NULL,
    provider TEXT,
    model TEXT,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    canonical_message_count INTEGER NOT NULL DEFAULT 0,
    projected_message_count INTEGER NOT NULL DEFAULT 0,
    budget_before_json TEXT,
    budget_after_json TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_projections_turn_step
    ON runtime_context_projections (turn_id, step);

CREATE INDEX IF NOT EXISTS idx_runtime_context_projections_session_created_at
    ON runtime_context_projections (session_id, created_at);

CREATE TABLE IF NOT EXISTS runtime_context_projection_messages (
    id TEXT PRIMARY KEY,
    projection_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    role TEXT NOT NULL,
    canonical_message_id TEXT,
    status TEXT NOT NULL,
    replacement_id TEXT,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    content_ref TEXT,
    summary TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_projection_messages_projection_sequence
    ON runtime_context_projection_messages (projection_id, sequence);

CREATE TABLE IF NOT EXISTS runtime_context_boundaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    kind TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    summary_message_id TEXT,
    summary_ref TEXT,
    message_refs_json TEXT,
    tool_call_refs_json TEXT,
    reinjected_refs_json TEXT,
    budget_before_json TEXT,
    budget_after_json TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_boundaries_turn_created_at
    ON runtime_context_boundaries (turn_id, created_at);

CREATE INDEX IF NOT EXISTS idx_runtime_context_boundaries_projection_created_at
    ON runtime_context_boundaries (projection_id, created_at);

CREATE TABLE IF NOT EXISTS runtime_context_content_replacements (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    tool_call_id TEXT,
    tool_name TEXT,
    kind TEXT NOT NULL,
    original_ref TEXT,
    replacement_text TEXT NOT NULL,
    original_size_bytes INTEGER NOT NULL DEFAULT 0,
    original_estimated_tokens INTEGER NOT NULL DEFAULT 0,
    replacement_estimated_tokens INTEGER NOT NULL DEFAULT 0,
    reason TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_content_replacements_projection
    ON runtime_context_content_replacements (projection_id);

CREATE INDEX IF NOT EXISTS idx_runtime_context_content_replacements_tool_call
    ON runtime_context_content_replacements (tool_call_id);

CREATE TABLE IF NOT EXISTS runtime_context_snip_boundaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    removed_message_refs_json TEXT,
    preserved_head_ref TEXT,
    preserved_tail_ref TEXT,
    summary_ref TEXT,
    reason TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_snip_boundaries_projection
    ON runtime_context_snip_boundaries (projection_id);

CREATE TABLE IF NOT EXISTS runtime_context_read_state_snapshots (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    state_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_read_state_snapshots_projection
    ON runtime_context_read_state_snapshots (projection_id);

CREATE TABLE IF NOT EXISTS runtime_context_reinjections (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    kind TEXT NOT NULL,
    ref TEXT,
    status TEXT NOT NULL,
    reason TEXT,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_reinjections_projection
    ON runtime_context_reinjections (projection_id);

CREATE TABLE IF NOT EXISTS runtime_context_warnings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT,
    projection_id TEXT,
    code TEXT NOT NULL,
    message TEXT NOT NULL,
    severity TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_warnings_projection
    ON runtime_context_warnings (projection_id);

CREATE TABLE IF NOT EXISTS runtime_context_reactive_attempts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    projection_id TEXT,
    attempt INTEGER NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runtime_context_reactive_attempts_turn_attempt
    ON runtime_context_reactive_attempts (turn_id, attempt);

ALTER TABLE runtime_prompt_assemblies
    ADD COLUMN projection_id TEXT;

CREATE INDEX IF NOT EXISTS idx_runtime_prompt_assemblies_projection
    ON runtime_prompt_assemblies (projection_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_prompt_assemblies_projection;
ALTER TABLE runtime_prompt_assemblies DROP COLUMN projection_id;

DROP INDEX IF EXISTS idx_runtime_context_reactive_attempts_turn_attempt;
DROP TABLE IF EXISTS runtime_context_reactive_attempts;

DROP INDEX IF EXISTS idx_runtime_context_warnings_projection;
DROP TABLE IF EXISTS runtime_context_warnings;

DROP INDEX IF EXISTS idx_runtime_context_reinjections_projection;
DROP TABLE IF EXISTS runtime_context_reinjections;

DROP INDEX IF EXISTS idx_runtime_context_read_state_snapshots_projection;
DROP TABLE IF EXISTS runtime_context_read_state_snapshots;

DROP INDEX IF EXISTS idx_runtime_context_snip_boundaries_projection;
DROP TABLE IF EXISTS runtime_context_snip_boundaries;

DROP INDEX IF EXISTS idx_runtime_context_content_replacements_tool_call;
DROP INDEX IF EXISTS idx_runtime_context_content_replacements_projection;
DROP TABLE IF EXISTS runtime_context_content_replacements;

DROP INDEX IF EXISTS idx_runtime_context_boundaries_projection_created_at;
DROP INDEX IF EXISTS idx_runtime_context_boundaries_turn_created_at;
DROP TABLE IF EXISTS runtime_context_boundaries;

DROP INDEX IF EXISTS idx_runtime_context_projection_messages_projection_sequence;
DROP TABLE IF EXISTS runtime_context_projection_messages;

DROP INDEX IF EXISTS idx_runtime_context_projections_session_created_at;
DROP INDEX IF EXISTS idx_runtime_context_projections_turn_step;
DROP TABLE IF EXISTS runtime_context_projections;
