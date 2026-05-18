package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type RuntimeAuditEvent struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id,omitempty"`
	TurnID    string         `json:"turn_id,omitempty"`
	Type      string         `json:"type"`
	CreatedAt string         `json:"created_at"`
	Payload   map[string]any `json:"payload"`
}

type RuntimeAuditResponse struct {
	Events []RuntimeAuditEvent `json:"events"`
}

type runtimeAuditStore struct {
	db *sql.DB
}

func newRuntimeAuditStore(db *sql.DB) runtimeAuditStore {
	return runtimeAuditStore{db: db}
}

func (s runtimeAuditStore) Append(ctx context.Context, event RuntimeAuditEvent) error {
	if s.db == nil {
		return nil
	}
	if event.ID == "" {
		event.ID = newRuntimeEventID()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to encode runtime audit payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_audit_events (id, session_id, turn_id, type, created_at, payload_json)
VALUES (?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.SessionID,
		event.TurnID,
		event.Type,
		event.CreatedAt,
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to append runtime audit event: %w", err)
	}
	return nil
}

func (s runtimeAuditStore) ListTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {
	if s.db == nil {
		return RuntimeAuditResponse{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, type, created_at, payload_json
FROM runtime_audit_events
WHERE turn_id = ?
ORDER BY created_at ASC`, turnID)
	if err != nil {
		return RuntimeAuditResponse{}, fmt.Errorf("failed to list runtime audit events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var events []RuntimeAuditEvent
	for rows.Next() {
		var event RuntimeAuditEvent
		var payload string
		if err := rows.Scan(&event.ID, &event.SessionID, &event.TurnID, &event.Type, &event.CreatedAt, &payload); err != nil {
			return RuntimeAuditResponse{}, fmt.Errorf("failed to scan runtime audit event: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return RuntimeAuditResponse{}, fmt.Errorf("failed to decode runtime audit payload: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return RuntimeAuditResponse{}, fmt.Errorf("failed to iterate runtime audit events: %w", err)
	}
	return RuntimeAuditResponse{Events: events}, nil
}
