package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) UpsertRecord(ctx context.Context, record Record) (Record, error) {
	if s.db == nil {
		return Record{}, errors.New("memory database is not available")
	}
	tags, err := json.Marshal(record.Tags)
	if err != nil {
		return Record{}, fmt.Errorf("failed to encode memory tags: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO project_memory_records (
    id, project_id, relative_path, type, title, description, tags_json,
    content_hash, mtime_unix, size_bytes, token_estimate, enabled, deleted_at,
    created_at, updated_at, created_from_session_id, created_from_turn_id,
    last_indexed_at, last_injected_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, NULL, ?, ?, ?, ?, ?, NULL)
ON CONFLICT(id) DO UPDATE SET
    project_id = excluded.project_id,
    relative_path = excluded.relative_path,
    type = excluded.type,
    title = excluded.title,
    description = excluded.description,
    tags_json = excluded.tags_json,
    content_hash = excluded.content_hash,
    mtime_unix = excluded.mtime_unix,
    size_bytes = excluded.size_bytes,
    token_estimate = excluded.token_estimate,
    updated_at = excluded.updated_at,
    created_from_session_id = COALESCE(project_memory_records.created_from_session_id, excluded.created_from_session_id),
    created_from_turn_id = COALESCE(project_memory_records.created_from_turn_id, excluded.created_from_turn_id),
    last_indexed_at = excluded.last_indexed_at`,
		record.ID, record.ProjectID, record.RelativePath, record.Type, record.Title, record.Description, string(tags),
		record.ContentHash, record.MTimeUnix, record.SizeBytes, record.TokenEstimate,
		record.CreatedAt, record.UpdatedAt, nullableString(record.CreatedFromSessionID), nullableString(record.CreatedFromTurnID), record.LastIndexedAt,
	)
	if err != nil {
		return Record{}, fmt.Errorf("failed to upsert memory record: %w", err)
	}
	return s.Get(ctx, record.ID)
}

func (s Store) ListByProject(ctx context.Context, projectID string, includeDeleted bool) ([]Record, error) {
	if s.db == nil {
		return nil, errors.New("memory database is not available")
	}
	query := `SELECT id, project_id, relative_path, type, title, description, tags_json, content_hash, mtime_unix, size_bytes, token_estimate, enabled, deleted_at, created_at, updated_at, created_from_session_id, created_from_turn_id, last_indexed_at, last_injected_at FROM project_memory_records WHERE project_id = ?`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	query += ` ORDER BY updated_at DESC, title ASC`
	rows, err := s.db.QueryContext(ctx, query, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("failed to list memory records: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRecords(rows)
}

func (s Store) Get(ctx context.Context, id string) (Record, error) {
	if s.db == nil {
		return Record{}, errors.New("memory database is not available")
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, project_id, relative_path, type, title, description, tags_json, content_hash, mtime_unix, size_bytes, token_estimate, enabled, deleted_at, created_at, updated_at, created_from_session_id, created_from_turn_id, last_indexed_at, last_injected_at FROM project_memory_records WHERE id = ?`, strings.TrimSpace(id))
	return scanRecord(row)
}

func (s Store) MarkMissingDeleted(ctx context.Context, projectID string, seen map[string]struct{}, deletedAt string) (int, error) {
	records, err := s.ListByProject(ctx, projectID, false)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, record := range records {
		if _, ok := seen[record.ID]; ok {
			continue
		}
		if _, ok := seen[record.RelativePath]; ok {
			continue
		}
		if err := s.markDeleted(ctx, record.ID, deletedAt); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s Store) SetEnabled(ctx context.Context, id string, enabled bool, updatedAt string) (Record, error) {
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE project_memory_records SET enabled = ?, updated_at = ? WHERE id = ?`, value, updatedAt, strings.TrimSpace(id))
	if err != nil {
		return Record{}, fmt.Errorf("failed to update memory enabled state: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Record{}, sql.ErrNoRows
	}
	return s.Get(ctx, id)
}

func (s Store) Delete(ctx context.Context, id, deletedAt string) (Record, error) {
	if err := s.markDeleted(ctx, id, deletedAt); err != nil {
		return Record{}, err
	}
	return s.Get(ctx, id)
}

func (s Store) markDeleted(ctx context.Context, id, deletedAt string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE project_memory_records SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, deletedAt, deletedAt, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("failed to mark memory deleted: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s Store) InsertInjection(ctx context.Context, injection Injection) error {
	if s.db == nil {
		return errors.New("memory database is not available")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_memory_injections (
    id, project_id, session_id, turn_id, memory_id, prompt_assembly_id,
    injected_at, token_estimate, selection_reason
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		injection.ID, injection.ProjectID, injection.SessionID, injection.TurnID, injection.MemoryID,
		nullableString(injection.PromptAssemblyID), injection.InjectedAt, injection.TokenEstimate, injection.SelectionReason,
	)
	if err != nil {
		return fmt.Errorf("failed to insert memory injection: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE project_memory_records SET last_injected_at = ? WHERE id = ?`, injection.InjectedAt, injection.MemoryID)
	return nil
}

func (s Store) ListInjectionsByTurn(ctx context.Context, sessionID, turnID string) ([]Injection, error) {
	if s.db == nil {
		return nil, errors.New("memory database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, session_id, turn_id, memory_id, prompt_assembly_id, injected_at, token_estimate, selection_reason
FROM project_memory_injections
WHERE session_id = ? AND turn_id = ?
ORDER BY injected_at ASC`, strings.TrimSpace(sessionID), strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list memory injections: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Injection
	for rows.Next() {
		var injection Injection
		var promptAssemblyID sql.NullString
		if err := rows.Scan(&injection.ID, &injection.ProjectID, &injection.SessionID, &injection.TurnID, &injection.MemoryID, &promptAssemblyID, &injection.InjectedAt, &injection.TokenEstimate, &injection.SelectionReason); err != nil {
			return nil, fmt.Errorf("failed to scan memory injection: %w", err)
		}
		injection.PromptAssemblyID = promptAssemblyID.String
		out = append(out, injection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memory injections: %w", err)
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRecords(rows *sql.Rows) ([]Record, error) {
	var out []Record
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memory records: %w", err)
	}
	return out, nil
}

func scanRecord(row scanner) (Record, error) {
	var record Record
	var tagsJSON string
	var deletedAt, sourceSessionID, sourceTurnID, lastInjectedAt sql.NullString
	var enabled int
	if err := row.Scan(&record.ID, &record.ProjectID, &record.RelativePath, &record.Type, &record.Title, &record.Description, &tagsJSON, &record.ContentHash, &record.MTimeUnix, &record.SizeBytes, &record.TokenEstimate, &enabled, &deletedAt, &record.CreatedAt, &record.UpdatedAt, &sourceSessionID, &sourceTurnID, &record.LastIndexedAt, &lastInjectedAt); err != nil {
		return Record{}, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	record.Enabled = enabled != 0
	record.DeletedAt = deletedAt.String
	record.CreatedFromSessionID = sourceSessionID.String
	record.CreatedFromTurnID = sourceTurnID.String
	record.LastInjectedAt = lastInjectedAt.String
	return record, nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
