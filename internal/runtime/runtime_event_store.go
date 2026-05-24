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

type runtimeEventStore struct {
	db *sql.DB
}

func newRuntimeEventStore(db *sql.DB) runtimeEventStore {
	return runtimeEventStore{db: db}
}

func (s runtimeEventStore) Append(ctx context.Context, event RuntimeEvent) error {
	if s.db == nil {
		return nil
	}
	if event.ID == "" {
		event.ID = newRuntimeEventID()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Sequence <= 0 {
		return errors.New("runtime event sequence is required")
	}
	event.Payload = redactRuntimePayload(event.Payload)
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to encode runtime event payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_events (
    sequence, id, type, session_id, turn_id, message_id, tool_call_id, payload_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sequence) DO UPDATE SET
    id = excluded.id,
    type = excluded.type,
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    message_id = excluded.message_id,
    tool_call_id = excluded.tool_call_id,
    payload_json = excluded.payload_json,
    created_at = excluded.created_at`,
		event.Sequence,
		event.ID,
		event.Type,
		nullableString(event.SessionID),
		nullableString(event.TurnID),
		nullableString(event.MessageID),
		nullableString(event.ToolCallID),
		string(payload),
		event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to append runtime event: %w", err)
	}
	return nil
}

func (s runtimeEventStore) List(ctx context.Context, after int64) (RuntimeEventsResponse, error) {
	if s.db == nil {
		return RuntimeEventsResponse{}, nil
	}
	query := `
SELECT sequence, id, type, session_id, turn_id, message_id, tool_call_id, payload_json, created_at
FROM runtime_events`
	var args []any
	if after > 0 {
		query += ` WHERE sequence > ?`
		args = append(args, after)
	}
	query += ` ORDER BY sequence ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return RuntimeEventsResponse{}, fmt.Errorf("failed to list runtime events: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeEventRows(rows)
}

func (s runtimeEventStore) ListTurn(ctx context.Context, turnID string, after int64) (RuntimeEventsResponse, error) {
	if s.db == nil {
		return RuntimeEventsResponse{}, nil
	}
	turnID = strings.TrimSpace(turnID)
	query := `
SELECT sequence, id, type, session_id, turn_id, message_id, tool_call_id, payload_json, created_at
FROM runtime_events
WHERE turn_id = ?`
	args := []any{turnID}
	if after > 0 {
		query += ` AND sequence > ?`
		args = append(args, after)
	}
	query += ` ORDER BY sequence ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return RuntimeEventsResponse{}, fmt.Errorf("failed to list runtime turn events: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeEventRows(rows)
}

func (s runtimeEventStore) ListSession(ctx context.Context, sessionID string, after int64) (RuntimeEventsResponse, error) {
	if s.db == nil {
		return RuntimeEventsResponse{}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	query := `
SELECT sequence, id, type, session_id, turn_id, message_id, tool_call_id, payload_json, created_at
FROM runtime_events
WHERE session_id = ?`
	args := []any{sessionID}
	if after > 0 {
		query += ` AND sequence > ?`
		args = append(args, after)
	}
	query += ` ORDER BY sequence ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return RuntimeEventsResponse{}, fmt.Errorf("failed to list runtime session events: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeEventRows(rows)
}

func (s runtimeEventStore) MaxSequence(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	var sequence sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(sequence) FROM runtime_events`).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("failed to read runtime event max sequence: %w", err)
	}
	if !sequence.Valid {
		return 0, nil
	}
	return sequence.Int64, nil
}

func scanRuntimeEventRows(rows *sql.Rows) (RuntimeEventsResponse, error) {
	events := make([]RuntimeEvent, 0)
	var firstSequence, lastSequence int64
	for rows.Next() {
		var event RuntimeEvent
		var sessionID, turnID, messageID, toolCallID sql.NullString
		var payload string
		if err := rows.Scan(
			&event.Sequence,
			&event.ID,
			&event.Type,
			&sessionID,
			&turnID,
			&messageID,
			&toolCallID,
			&payload,
			&event.CreatedAt,
		); err != nil {
			return RuntimeEventsResponse{}, fmt.Errorf("failed to scan runtime event: %w", err)
		}
		event.SessionID = sessionID.String
		event.TurnID = turnID.String
		event.MessageID = messageID.String
		event.ToolCallID = toolCallID.String
		if strings.TrimSpace(payload) == "" {
			event.Payload = map[string]any{}
		} else if err := json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return RuntimeEventsResponse{}, fmt.Errorf("failed to decode runtime event payload: %w", err)
		}
		event.Payload = redactRuntimePayload(event.Payload)
		if firstSequence == 0 {
			firstSequence = event.Sequence
		}
		lastSequence = event.Sequence
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return RuntimeEventsResponse{}, fmt.Errorf("failed to iterate runtime events: %w", err)
	}
	return RuntimeEventsResponse{
		Events:        events,
		FirstSequence: firstSequence,
		LastSequence:  lastSequence,
	}, nil
}
