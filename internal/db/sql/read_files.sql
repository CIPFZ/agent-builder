-- name: RecordFileRead :exec
INSERT INTO read_files (
    session_id,
    path,
    read_at,
    turn_id,
    tool_call_id,
    size_bytes,
    content_hash,
    mtime_unix,
    offset,
    read_limit,
    partial,
    token_estimate,
    state,
    reason
) VALUES (
    ?,
    ?,
    strftime('%s', 'now'),
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
) ON CONFLICT(path, session_id) DO UPDATE SET
    read_at = excluded.read_at,
    turn_id = excluded.turn_id,
    tool_call_id = excluded.tool_call_id,
    size_bytes = excluded.size_bytes,
    content_hash = excluded.content_hash,
    mtime_unix = excluded.mtime_unix,
    offset = excluded.offset,
    read_limit = excluded.read_limit,
    partial = excluded.partial,
    token_estimate = excluded.token_estimate,
    state = excluded.state,
    reason = excluded.reason;

-- name: GetFileRead :one
SELECT * FROM read_files
WHERE session_id = ? AND path = ? LIMIT 1;

-- name: ListSessionReadFiles :many
SELECT * FROM read_files
WHERE session_id = ?
ORDER BY read_at DESC;
