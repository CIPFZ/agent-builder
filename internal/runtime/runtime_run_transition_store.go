package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RuntimeRunTransition struct {
	ID         string         `json:"id"`
	RunID      string         `json:"runId"`
	SessionID  string         `json:"sessionId,omitempty"`
	TurnID     string         `json:"turnId,omitempty"`
	TaskID     string         `json:"taskId,omitempty"`
	FromStatus string         `json:"fromStatus,omitempty"`
	ToStatus   string         `json:"toStatus"`
	Reason     string         `json:"reason,omitempty"`
	Source     string         `json:"source"`
	EventID    string         `json:"eventId,omitempty"`
	CreatedAt  int64          `json:"createdAt"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type runtimeRunTransitionStore struct {
	db *sql.DB
}

func newRuntimeRunTransitionStore(db *sql.DB) runtimeRunTransitionStore {
	return runtimeRunTransitionStore{db: db}
}

func (s runtimeRunTransitionStore) Upsert(ctx context.Context, transition RuntimeRunTransition) (RuntimeRunTransition, error) {
	if s.db == nil {
		return RuntimeRunTransition{}, errors.New("runtime run transition database is not available")
	}
	transition.RunID = strings.TrimSpace(transition.RunID)
	transition.SessionID = strings.TrimSpace(transition.SessionID)
	transition.TurnID = strings.TrimSpace(transition.TurnID)
	transition.TaskID = strings.TrimSpace(transition.TaskID)
	transition.FromStatus = strings.TrimSpace(transition.FromStatus)
	transition.ToStatus = strings.TrimSpace(transition.ToStatus)
	transition.Source = strings.TrimSpace(transition.Source)
	transition.EventID = strings.TrimSpace(transition.EventID)
	if transition.RunID == "" {
		return RuntimeRunTransition{}, errors.New("run transition run id is required")
	}
	if transition.ToStatus == "" {
		return RuntimeRunTransition{}, errors.New("run transition target status is required")
	}
	if transition.Source == "" {
		return RuntimeRunTransition{}, errors.New("run transition source is required")
	}
	if transition.CreatedAt == 0 {
		transition.CreatedAt = time.Now().UnixMilli()
	}
	transition.ID = strings.TrimSpace(transition.ID)
	if transition.ID == "" {
		transition.ID = runtimeRunTransitionID(transition)
	}
	metadata, err := encodeJSONMap(transition.Metadata)
	if err != nil {
		return RuntimeRunTransition{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_run_transitions (
    id, run_id, session_id, turn_id, task_id, from_status, to_status,
    reason, source, event_id, created_at, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    run_id = runtime_run_transitions.run_id,
    session_id = COALESCE(runtime_run_transitions.session_id, excluded.session_id),
    turn_id = COALESCE(runtime_run_transitions.turn_id, excluded.turn_id),
    task_id = COALESCE(runtime_run_transitions.task_id, excluded.task_id),
    from_status = COALESCE(runtime_run_transitions.from_status, excluded.from_status),
    to_status = runtime_run_transitions.to_status,
    reason = COALESCE(runtime_run_transitions.reason, excluded.reason),
    source = runtime_run_transitions.source,
    event_id = COALESCE(runtime_run_transitions.event_id, excluded.event_id),
    created_at = runtime_run_transitions.created_at,
    metadata_json = COALESCE(runtime_run_transitions.metadata_json, excluded.metadata_json)`,
		transition.ID,
		transition.RunID,
		nullableString(transition.SessionID),
		nullableString(transition.TurnID),
		nullableString(transition.TaskID),
		nullableString(transition.FromStatus),
		transition.ToStatus,
		nullableString(transition.Reason),
		transition.Source,
		nullableString(transition.EventID),
		transition.CreatedAt,
		metadata,
	)
	if err != nil {
		return RuntimeRunTransition{}, fmt.Errorf("failed to upsert runtime run transition: %w", err)
	}
	return s.Get(ctx, transition.ID)
}

func (s runtimeRunTransitionStore) Get(ctx context.Context, id string) (RuntimeRunTransition, error) {
	if s.db == nil {
		return RuntimeRunTransition{}, errors.New("runtime run transition database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeRunTransitionSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	transition, err := scanRuntimeRunTransition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRunTransition{}, errors.New("runtime run transition not found")
	}
	return transition, err
}

func (s runtimeRunTransitionStore) ListByRun(ctx context.Context, runID string) ([]RuntimeRunTransition, error) {
	return s.list(ctx, `run_id = ?`, strings.TrimSpace(runID))
}

func (s runtimeRunTransitionStore) ListBySession(ctx context.Context, sessionID string) ([]RuntimeRunTransition, error) {
	return s.list(ctx, `session_id = ?`, strings.TrimSpace(sessionID))
}

func (s runtimeRunTransitionStore) ListByTurn(ctx context.Context, turnID string) ([]RuntimeRunTransition, error) {
	return s.list(ctx, `turn_id = ?`, strings.TrimSpace(turnID))
}

func (s runtimeRunTransitionStore) list(ctx context.Context, where string, arg string) ([]RuntimeRunTransition, error) {
	if s.db == nil {
		return nil, errors.New("runtime run transition database is not available")
	}
	if strings.TrimSpace(arg) == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, runtimeRunTransitionSelectSQL()+` WHERE `+where+` ORDER BY created_at ASC, id ASC`, arg)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime run transitions: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []RuntimeRunTransition
	for rows.Next() {
		transition, err := scanRuntimeRunTransition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, transition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime run transitions: %w", err)
	}
	return out, nil
}

func runtimeRunTransitionSelectSQL() string {
	return `
SELECT id, run_id, session_id, turn_id, task_id, from_status, to_status,
    reason, source, event_id, created_at, metadata_json
FROM runtime_run_transitions`
}

type runtimeRunTransitionScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeRunTransition(scanner runtimeRunTransitionScanner) (RuntimeRunTransition, error) {
	var transition RuntimeRunTransition
	var sessionID, turnID, taskID, fromStatus, reason, eventID, metadata sql.NullString
	if err := scanner.Scan(
		&transition.ID,
		&transition.RunID,
		&sessionID,
		&turnID,
		&taskID,
		&fromStatus,
		&transition.ToStatus,
		&reason,
		&transition.Source,
		&eventID,
		&transition.CreatedAt,
		&metadata,
	); err != nil {
		return RuntimeRunTransition{}, err
	}
	transition.SessionID = sessionID.String
	transition.TurnID = turnID.String
	transition.TaskID = taskID.String
	transition.FromStatus = fromStatus.String
	transition.Reason = reason.String
	transition.EventID = eventID.String
	transition.Metadata = decodeJSONMap(metadata.String)
	return transition, nil
}

func runtimeRunTransitionID(transition RuntimeRunTransition) string {
	parts := strings.Join([]string{
		transition.RunID,
		transition.SessionID,
		transition.TurnID,
		transition.TaskID,
		transition.FromStatus,
		transition.ToStatus,
		transition.Source,
		transition.EventID,
		fmt.Sprintf("%d", transition.CreatedAt),
	}, "\x00")
	sum := sha256.Sum256([]byte(parts))
	return "run_transition_" + hex.EncodeToString(sum[:12])
}
