package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errRuntimeUserInputNotFound = errors.New("runtime user input not found")

type runtimeUserInputStore struct {
	db *sql.DB
}

func newRuntimeUserInputStore(db *sql.DB) runtimeUserInputStore {
	return runtimeUserInputStore{db: db}
}

func (s runtimeUserInputStore) Upsert(ctx context.Context, input RuntimeNormalizedInput, items []RuntimeUserInputItem, turnID string) (RuntimeNormalizedInput, error) {
	if s.db == nil {
		return RuntimeNormalizedInput{}, errors.New("runtime user input database is not available")
	}
	input.ID = strings.TrimSpace(input.ID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	if input.ID == "" {
		return RuntimeNormalizedInput{}, errors.New("input id is required")
	}
	itemsJSON, err := json.Marshal(sanitizeRuntimeUserInputItems(items))
	if err != nil {
		return RuntimeNormalizedInput{}, fmt.Errorf("failed to encode user input items: %w", err)
	}
	normalizedJSON, err := json.Marshal(input)
	if err != nil {
		return RuntimeNormalizedInput{}, fmt.Errorf("failed to encode normalized input: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_user_inputs (
    id, session_id, turn_id, project_id, scope, mode, prompt_preview,
    items_json, normalized_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_user_inputs.turn_id),
    project_id = COALESCE(NULLIF(excluded.project_id, ''), runtime_user_inputs.project_id),
    scope = COALESCE(NULLIF(excluded.scope, ''), runtime_user_inputs.scope),
    mode = excluded.mode,
    prompt_preview = excluded.prompt_preview,
    items_json = excluded.items_json,
    normalized_json = excluded.normalized_json`,
		input.ID,
		input.SessionID,
		nullableString(strings.TrimSpace(turnID)),
		nullableString(input.ProjectID),
		nullableString(input.Scope),
		input.Mode,
		preview(input.Prompt, auditPreviewLimit),
		string(itemsJSON),
		string(normalizedJSON),
		input.CreatedAt,
	)
	if err != nil {
		return RuntimeNormalizedInput{}, fmt.Errorf("failed to upsert runtime user input: %w", err)
	}
	return s.Get(ctx, input.ID)
}

func (s runtimeUserInputStore) Get(ctx context.Context, id string) (RuntimeNormalizedInput, error) {
	if s.db == nil {
		return RuntimeNormalizedInput{}, errors.New("runtime user input database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT normalized_json
FROM runtime_user_inputs
WHERE id = ?`, strings.TrimSpace(id))
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuntimeNormalizedInput{}, errRuntimeUserInputNotFound
		}
		return RuntimeNormalizedInput{}, err
	}
	var input RuntimeNormalizedInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return RuntimeNormalizedInput{}, fmt.Errorf("failed to decode runtime user input: %w", err)
	}
	return input, nil
}

func sanitizeRuntimeUserInputItems(items []RuntimeUserInputItem) []RuntimeUserInputItem {
	sanitized := make([]RuntimeUserInputItem, 0, len(items))
	for _, item := range items {
		next := item
		if strings.TrimSpace(next.Data) != "" {
			next.Data = fmt.Sprintf("[redacted:%d bytes]", len(next.Data))
		}
		sanitized = append(sanitized, next)
	}
	return sanitized
}
