-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_recovery_links (
  id TEXT PRIMARY KEY,
  source_turn_id TEXT NOT NULL,
  resumed_turn_id TEXT NOT NULL,
  action TEXT NOT NULL,
  mode TEXT NOT NULL,
  interruption_kind TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_recovery_links_source_turn ON runtime_recovery_links(source_turn_id);
CREATE INDEX IF NOT EXISTS idx_runtime_recovery_links_resumed_turn ON runtime_recovery_links(resumed_turn_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runtime_recovery_links_resumed_turn;
DROP INDEX IF EXISTS idx_runtime_recovery_links_source_turn;
DROP TABLE IF EXISTS runtime_recovery_links;
