package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	turnStatusQueued            = "queued"
	turnStatusRunning           = "running"
	turnStatusWaitingPermission = "waiting_permission"
	turnStatusCancelling        = "cancelling"
	turnStatusCompleted         = "completed"
	turnStatusFailed            = "failed"
	turnStatusCancelled         = "cancelled"
	turnStatusInterrupted       = "interrupted"
)

var errRuntimeTurnNotFound = errors.New("runtime turn not found")

type runtimeTurnStore struct {
	db *sql.DB
}

func newRuntimeTurnStore(db *sql.DB) runtimeTurnStore {
	return runtimeTurnStore{db: db}
}

func (s runtimeTurnStore) Upsert(ctx context.Context, turn RuntimeTurn) (RuntimeTurn, error) {
	if s.db == nil {
		return RuntimeTurn{}, errors.New("runtime turn database is not available")
	}
	turn.ID = strings.TrimSpace(turn.ID)
	turn.SessionID = strings.TrimSpace(turn.SessionID)
	if turn.ID == "" {
		return RuntimeTurn{}, errors.New("turn id is required")
	}
	if turn.SessionID == "" {
		return RuntimeTurn{}, errors.New("turn session id is required")
	}
	if turn.Status == "" {
		turn.Status = turnStatusQueued
	}
	now := time.Now().UnixMilli()
	if turn.StartedAt == 0 {
		turn.StartedAt = now
	}
	updatedAt := now
	if turn.FinishedAt > 0 && isFinalTurnStatus(turn.Status) {
		updatedAt = turn.FinishedAt
	}
	usageBefore, err := encodeRuntimeUsage(turn.UsageBefore)
	if err != nil {
		return RuntimeTurn{}, err
	}
	usageAfter, err := encodeRuntimeUsage(turn.UsageAfter)
	if err != nil {
		return RuntimeTurn{}, err
	}
	usageDelta, err := encodeRuntimeUsage(turn.UsageDelta)
	if err != nil {
		return RuntimeTurn{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_turns (
    id, session_id, status, user_message_id, latest_assistant_message_id,
    provider, model, prompt_preview, usage_before_json, usage_after_json,
    usage_delta_json, started_at, updated_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_turns.session_id),
    status = excluded.status,
    user_message_id = COALESCE(NULLIF(excluded.user_message_id, ''), runtime_turns.user_message_id),
    latest_assistant_message_id = COALESCE(NULLIF(excluded.latest_assistant_message_id, ''), runtime_turns.latest_assistant_message_id),
    provider = COALESCE(NULLIF(excluded.provider, ''), runtime_turns.provider),
    model = COALESCE(NULLIF(excluded.model, ''), runtime_turns.model),
    prompt_preview = COALESCE(NULLIF(excluded.prompt_preview, ''), runtime_turns.prompt_preview),
    usage_before_json = COALESCE(excluded.usage_before_json, runtime_turns.usage_before_json),
    usage_after_json = COALESCE(excluded.usage_after_json, runtime_turns.usage_after_json),
    usage_delta_json = COALESCE(excluded.usage_delta_json, runtime_turns.usage_delta_json),
    started_at = runtime_turns.started_at,
    updated_at = excluded.updated_at,
    finished_at = COALESCE(excluded.finished_at, runtime_turns.finished_at),
    error = COALESCE(NULLIF(excluded.error, ''), runtime_turns.error)`,
		turn.ID,
		turn.SessionID,
		turn.Status,
		nullableString(turn.UserMessageID),
		nullableString(firstNonEmpty(turn.LatestAssistantMessageID, turn.LatestMessageID)),
		nullableString(turn.Provider),
		nullableString(turn.Model),
		nullableString(turn.PromptPreview),
		usageBefore,
		usageAfter,
		usageDelta,
		turn.StartedAt,
		updatedAt,
		nullableInt64(turn.FinishedAt),
		nullableString(turn.Error),
	)
	if err != nil {
		return RuntimeTurn{}, fmt.Errorf("failed to upsert runtime turn: %w", err)
	}
	return s.Get(ctx, turn.ID)
}

func (s runtimeTurnStore) Get(ctx context.Context, id string) (RuntimeTurn, error) {
	if s.db == nil {
		return RuntimeTurn{}, errors.New("runtime turn database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, status, user_message_id, latest_assistant_message_id,
    provider, model, prompt_preview, usage_before_json, usage_after_json,
    usage_delta_json, started_at, updated_at, finished_at, error
FROM runtime_turns
WHERE id = ?`, strings.TrimSpace(id))
	turn, err := scanRuntimeTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeTurn{}, errRuntimeTurnNotFound
	}
	return turn, err
}

func (s runtimeTurnStore) List(ctx context.Context, status string) ([]RuntimeTurn, error) {
	if s.db == nil {
		return nil, errors.New("runtime turn database is not available")
	}
	status = strings.TrimSpace(status)
	query := `
SELECT id, session_id, status, user_message_id, latest_assistant_message_id,
    provider, model, prompt_preview, usage_before_json, usage_after_json,
    usage_delta_json, started_at, updated_at, finished_at, error
FROM runtime_turns`
	var args []any
	if status == "active" {
		query += ` WHERE status IN (?, ?, ?, ?)`
		args = append(args, turnStatusQueued, turnStatusRunning, turnStatusWaitingPermission, turnStatusCancelling)
	} else if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime turns: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var turns []RuntimeTurn
	for rows.Next() {
		turn, err := scanRuntimeTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime turns: %w", err)
	}
	return turns, nil
}

func (s runtimeTurnStore) InterruptUnfinished(ctx context.Context) ([]RuntimeTurn, error) {
	if s.db == nil {
		return nil, errors.New("runtime turn database is not available")
	}
	active, err := s.List(ctx, "active")
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for i := range active {
		active[i].Status = turnStatusInterrupted
		active[i].FinishedAt = now
		active[i].Error = firstNonEmpty(active[i].Error, "runtime restarted before turn completed")
		if _, err := s.Upsert(ctx, active[i]); err != nil {
			return nil, err
		}
	}
	return active, nil
}

type runtimeTurnScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeTurn(scanner runtimeTurnScanner) (RuntimeTurn, error) {
	var turn RuntimeTurn
	var userMessageID, assistantMessageID, provider, model, promptPreview sql.NullString
	var usageBefore, usageAfter, usageDelta sql.NullString
	var finishedAt sql.NullInt64
	var errText sql.NullString
	var updatedAt int64
	if err := scanner.Scan(
		&turn.ID,
		&turn.SessionID,
		&turn.Status,
		&userMessageID,
		&assistantMessageID,
		&provider,
		&model,
		&promptPreview,
		&usageBefore,
		&usageAfter,
		&usageDelta,
		&turn.StartedAt,
		&updatedAt,
		&finishedAt,
		&errText,
	); err != nil {
		return RuntimeTurn{}, err
	}
	turn.UserMessageID = userMessageID.String
	turn.LatestAssistantMessageID = assistantMessageID.String
	turn.LatestMessageID = assistantMessageID.String
	turn.Provider = provider.String
	turn.Model = model.String
	turn.PromptPreview = promptPreview.String
	turn.UsageBefore = decodeRuntimeUsage(usageBefore.String)
	turn.UsageAfter = decodeRuntimeUsage(usageAfter.String)
	turn.UsageDelta = decodeRuntimeUsage(usageDelta.String)
	if finishedAt.Valid {
		turn.FinishedAt = finishedAt.Int64
	}
	if turn.FinishedAt > 0 && turn.StartedAt > 0 {
		turn.DurationMS = turn.FinishedAt - turn.StartedAt
	}
	turn.Error = errText.String
	return turn, nil
}

func encodeRuntimeUsage(usage RuntimeUsage) (any, error) {
	if usage == (RuntimeUsage{}) {
		return nil, nil
	}
	data, err := json.Marshal(usage)
	if err != nil {
		return nil, fmt.Errorf("failed to encode runtime usage: %w", err)
	}
	return string(data), nil
}

func decodeRuntimeUsage(data string) RuntimeUsage {
	var usage RuntimeUsage
	if strings.TrimSpace(data) == "" {
		return usage
	}
	_ = json.Unmarshal([]byte(data), &usage)
	return usage
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func isFinalTurnStatus(status string) bool {
	switch status {
	case turnStatusCompleted, turnStatusFailed, turnStatusCancelled, turnStatusInterrupted:
		return true
	default:
		return false
	}
}
