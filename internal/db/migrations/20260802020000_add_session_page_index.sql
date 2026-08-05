CREATE INDEX IF NOT EXISTS idx_sessions_root_active_page
ON sessions(parent_session_id, deleted_at, pinned DESC, updated_at DESC, id DESC);
