-- +goose Up
CREATE TABLE IF NOT EXISTS conversation_projector_retention_v2 (
    session_id TEXT PRIMARY KEY,
    floor_raw_sequence INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO conversation_projector_retention_v2(session_id, floor_raw_sequence, updated_at)
SELECT checkpoint.session_id,
       COALESCE(MIN(batch.previous_raw_sequence), checkpoint.last_raw_sequence),
       checkpoint.updated_at
FROM conversation_projector_checkpoints_v2 checkpoint
LEFT JOIN conversation_projector_batches_v2 batch ON batch.session_id = checkpoint.session_id
GROUP BY checkpoint.session_id, checkpoint.last_raw_sequence, checkpoint.updated_at;

-- +goose Down
DROP TABLE IF EXISTS conversation_projector_retention_v2;
