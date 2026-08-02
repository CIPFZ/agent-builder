package contextmgr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s SQLStore) NextSessionMemoryRevision(ctx context.Context, sessionID string) (int, error) {
	if s.db == nil {
		return 0, errors.New("context governance database is not available")
	}
	var revision int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) + 1 FROM runtime_session_memory_revisions WHERE session_id = ?`, strings.TrimSpace(sessionID)).Scan(&revision); err != nil {
		return 0, fmt.Errorf("next session memory revision: %w", err)
	}
	return revision, nil
}

func (s SQLStore) UpsertSessionMemoryRevision(ctx context.Context, revision SessionMemoryRevision) (SessionMemoryRevision, error) {
	if s.db == nil {
		return SessionMemoryRevision{}, errors.New("context governance database is not available")
	}
	if strings.TrimSpace(revision.ID) == "" || strings.TrimSpace(revision.SessionID) == "" || revision.Revision <= 0 {
		return SessionMemoryRevision{}, errors.New("session memory id, session id and positive revision are required")
	}
	if revision.Status == "" {
		revision.Status = SessionMemoryStatusStarted
	}
	if revision.CreatedAt == 0 {
		revision.CreatedAt = time.Now().UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO runtime_session_memory_revisions (
id, session_id, turn_id, revision, status, base_revision, content, content_hash,
last_summarized_message_id, source_message_count, source_token_estimate,
source_tool_call_count, provider, model, created_at, completed_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET status=excluded.status, content=excluded.content,
content_hash=excluded.content_hash, last_summarized_message_id=excluded.last_summarized_message_id,
source_message_count=excluded.source_message_count, source_token_estimate=excluded.source_token_estimate,
source_tool_call_count=excluded.source_tool_call_count, provider=excluded.provider, model=excluded.model,
completed_at=excluded.completed_at, error=excluded.error`,
		revision.ID, revision.SessionID, nullableString(revision.TurnID), revision.Revision,
		revision.Status, nullableInt(revision.BaseRevision), nullableString(revision.Content), nullableString(revision.ContentHash),
		nullableString(revision.LastSummarizedMessageID), revision.SourceMessageCount, revision.SourceTokenEstimate,
		revision.SourceToolCallCount, nullableString(revision.Provider), nullableString(revision.Model), revision.CreatedAt,
		nullableInt64(revision.CompletedAt), nullableString(revision.Error))
	if err != nil {
		return SessionMemoryRevision{}, fmt.Errorf("upsert session memory revision: %w", err)
	}
	return revision, nil
}

func (s SQLStore) LatestCompletedSessionMemory(ctx context.Context, sessionID string) (SessionMemoryRevision, error) {
	return s.latestSessionMemory(ctx, sessionID, SessionMemoryStatusCompleted)
}

func (s SQLStore) LatestSessionMemory(ctx context.Context, sessionID string) (SessionMemoryRevision, error) {
	return s.latestSessionMemory(ctx, sessionID, "")
}

func (s SQLStore) latestSessionMemory(ctx context.Context, sessionID, status string) (SessionMemoryRevision, error) {
	if s.db == nil {
		return SessionMemoryRevision{}, errors.New("context governance database is not available")
	}
	query := `SELECT id, session_id, turn_id, revision, status, base_revision, content, content_hash,
last_summarized_message_id, source_message_count, source_token_estimate, source_tool_call_count,
provider, model, created_at, completed_at, error FROM runtime_session_memory_revisions WHERE session_id = ?`
	args := []any{strings.TrimSpace(sessionID)}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY revision DESC LIMIT 1`
	return scanSessionMemoryRevision(s.db.QueryRowContext(ctx, query, args...))
}

func scanSessionMemoryRevision(row *sql.Row) (SessionMemoryRevision, error) {
	var out SessionMemoryRevision
	var turnID, content, hash, anchor, provider, model, errText sql.NullString
	var base sql.NullInt64
	var completed sql.NullInt64
	if err := row.Scan(&out.ID, &out.SessionID, &turnID, &out.Revision, &out.Status, &base,
		&content, &hash, &anchor, &out.SourceMessageCount, &out.SourceTokenEstimate,
		&out.SourceToolCallCount, &provider, &model, &out.CreatedAt, &completed, &errText); err != nil {
		return SessionMemoryRevision{}, err
	}
	out.TurnID, out.Content, out.ContentHash = turnID.String, content.String, hash.String
	out.LastSummarizedMessageID, out.Provider, out.Model, out.Error = anchor.String, provider.String, model.String, errText.String
	if base.Valid {
		out.BaseRevision = int(base.Int64)
	}
	if completed.Valid {
		out.CompletedAt = completed.Int64
	}
	return out, nil
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
