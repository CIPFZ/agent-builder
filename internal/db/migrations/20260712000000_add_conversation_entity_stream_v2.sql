-- +goose Up
CREATE TABLE IF NOT EXISTS conversation_entity_events_v2 (
    session_id TEXT NOT NULL,
    raw_sequence INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    event_id TEXT NOT NULL UNIQUE,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    revision TEXT NOT NULL,
    event_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, raw_sequence, ordinal),
    UNIQUE (session_id, raw_sequence, entity_type, entity_id, operation)
);

CREATE TABLE IF NOT EXISTS conversation_projector_checkpoints_v2 (
    session_id TEXT PRIMARY KEY,
    last_raw_sequence INTEGER NOT NULL,
    failure_reason TEXT,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS conversation_projector_batches_v2 (
    session_id TEXT NOT NULL,
    raw_sequence INTEGER NOT NULL,
    previous_raw_sequence INTEGER NOT NULL,
    entity_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, raw_sequence)
);

CREATE TABLE IF NOT EXISTS conversation_entities_v2 (
    session_id TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    turn_id TEXT,
    activity_sequence TEXT NOT NULL,
    revision TEXT NOT NULL,
    entity_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, entity_type, entity_id)
);

-- +goose Down
DROP TABLE IF EXISTS conversation_entities_v2;
DROP TABLE IF EXISTS conversation_projector_batches_v2;
DROP TABLE IF EXISTS conversation_projector_checkpoints_v2;
DROP TABLE IF EXISTS conversation_entity_events_v2;
