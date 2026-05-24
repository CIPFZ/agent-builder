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

type runtimeCompactBoundaryStore struct {
	db *sql.DB
}

func newRuntimeCompactBoundaryStore(db *sql.DB) runtimeCompactBoundaryStore {
	return runtimeCompactBoundaryStore{db: db}
}

func (s runtimeCompactBoundaryStore) Upsert(ctx context.Context, boundary RuntimeCompactBoundary) (RuntimeCompactBoundary, error) {
	if s.db == nil {
		return RuntimeCompactBoundary{}, errors.New("runtime compact database is not available")
	}
	boundary.ID = strings.TrimSpace(boundary.ID)
	if boundary.ID == "" {
		return RuntimeCompactBoundary{}, errors.New("compact boundary id is required")
	}
	if boundary.SessionID == "" {
		return RuntimeCompactBoundary{}, errors.New("compact boundary session id is required")
	}
	if boundary.Kind == "" {
		boundary.Kind = "boundary"
	}
	if boundary.Trigger == "" {
		boundary.Trigger = "runtime"
	}
	if boundary.Status == "" {
		boundary.Status = "recorded"
	}
	if boundary.CreatedAt == 0 {
		boundary.CreatedAt = time.Now().UTC().UnixMilli()
	}
	budgetBefore, err := marshalOptionalJSON(boundary.BudgetBefore)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	budgetAfter, err := marshalOptionalJSON(boundary.BudgetAfter)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	messageRefs, err := marshalJSON(boundary.MessageRefs)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	toolCallRefs, err := marshalJSON(boundary.ToolCallRefs)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	reinjectedRefs, err := marshalJSON(boundary.ReinjectedRefs)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_compact_boundaries (
    id, session_id, turn_id, kind, trigger, status,
    budget_before_json, budget_after_json, summary_ref,
    message_refs_json, tool_call_refs_json, reinjected_refs_json,
    error, created_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_compact_boundaries.turn_id),
    kind = excluded.kind,
    trigger = excluded.trigger,
    status = excluded.status,
    budget_before_json = COALESCE(excluded.budget_before_json, runtime_compact_boundaries.budget_before_json),
    budget_after_json = COALESCE(excluded.budget_after_json, runtime_compact_boundaries.budget_after_json),
    summary_ref = COALESCE(NULLIF(excluded.summary_ref, ''), runtime_compact_boundaries.summary_ref),
    message_refs_json = excluded.message_refs_json,
    tool_call_refs_json = excluded.tool_call_refs_json,
    reinjected_refs_json = excluded.reinjected_refs_json,
    error = COALESCE(NULLIF(excluded.error, ''), runtime_compact_boundaries.error),
    completed_at = COALESCE(excluded.completed_at, runtime_compact_boundaries.completed_at)`,
		boundary.ID,
		boundary.SessionID,
		nullableString(boundary.TurnID),
		boundary.Kind,
		boundary.Trigger,
		boundary.Status,
		budgetBefore,
		budgetAfter,
		nullableString(boundary.SummaryRef),
		messageRefs,
		toolCallRefs,
		reinjectedRefs,
		nullableString(boundary.Error),
		boundary.CreatedAt,
		nullableCompactInt64(boundary.CompletedAt),
	)
	if err != nil {
		return RuntimeCompactBoundary{}, fmt.Errorf("failed to upsert compact boundary: %w", err)
	}
	return s.Get(ctx, boundary.ID)
}

func (s runtimeCompactBoundaryStore) Get(ctx context.Context, id string) (RuntimeCompactBoundary, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, kind, trigger, status,
    budget_before_json, budget_after_json, summary_ref,
    message_refs_json, tool_call_refs_json, reinjected_refs_json,
    error, created_at, completed_at
FROM runtime_compact_boundaries
WHERE id = ?`, strings.TrimSpace(id))
	return scanRuntimeCompactBoundary(row)
}

func (s runtimeCompactBoundaryStore) ListByTurn(ctx context.Context, turnID string) ([]RuntimeCompactBoundary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, kind, trigger, status,
    budget_before_json, budget_after_json, summary_ref,
    message_refs_json, tool_call_refs_json, reinjected_refs_json,
    error, created_at, completed_at
FROM runtime_compact_boundaries
WHERE turn_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list turn compact boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeCompactBoundaryRows(rows)
}

func (s runtimeCompactBoundaryStore) ListBySession(ctx context.Context, sessionID string) ([]RuntimeCompactBoundary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, kind, trigger, status,
    budget_before_json, budget_after_json, summary_ref,
    message_refs_json, tool_call_refs_json, reinjected_refs_json,
    error, created_at, completed_at
FROM runtime_compact_boundaries
WHERE session_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list session compact boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeCompactBoundaryRows(rows)
}

type runtimeCompactBoundaryScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeCompactBoundaryRows(rows *sql.Rows) ([]RuntimeCompactBoundary, error) {
	var out []RuntimeCompactBoundary
	for rows.Next() {
		boundary, err := scanRuntimeCompactBoundary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, boundary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate compact boundaries: %w", err)
	}
	return out, nil
}

func scanRuntimeCompactBoundary(scanner runtimeCompactBoundaryScanner) (RuntimeCompactBoundary, error) {
	var boundary RuntimeCompactBoundary
	var turnID, budgetBefore, budgetAfter, summaryRef, messageRefs, toolCallRefs, reinjectedRefs, errText sql.NullString
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&boundary.ID,
		&boundary.SessionID,
		&turnID,
		&boundary.Kind,
		&boundary.Trigger,
		&boundary.Status,
		&budgetBefore,
		&budgetAfter,
		&summaryRef,
		&messageRefs,
		&toolCallRefs,
		&reinjectedRefs,
		&errText,
		&boundary.CreatedAt,
		&completedAt,
	); err != nil {
		return RuntimeCompactBoundary{}, err
	}
	boundary.TurnID = turnID.String
	boundary.SummaryRef = summaryRef.String
	boundary.Error = errText.String
	if completedAt.Valid {
		boundary.CompletedAt = completedAt.Int64
	}
	if budgetBefore.Valid && budgetBefore.String != "" {
		var report RuntimeBudgetReport
		if err := json.Unmarshal([]byte(budgetBefore.String), &report); err != nil {
			return RuntimeCompactBoundary{}, fmt.Errorf("failed to decode compact budget_before: %w", err)
		}
		boundary.BudgetBefore = &report
	}
	if budgetAfter.Valid && budgetAfter.String != "" {
		var report RuntimeBudgetReport
		if err := json.Unmarshal([]byte(budgetAfter.String), &report); err != nil {
			return RuntimeCompactBoundary{}, fmt.Errorf("failed to decode compact budget_after: %w", err)
		}
		boundary.BudgetAfter = &report
	}
	if err := unmarshalJSON(messageRefs.String, &boundary.MessageRefs); err != nil {
		return RuntimeCompactBoundary{}, err
	}
	if err := unmarshalJSON(toolCallRefs.String, &boundary.ToolCallRefs); err != nil {
		return RuntimeCompactBoundary{}, err
	}
	if err := unmarshalJSON(reinjectedRefs.String, &boundary.ReinjectedRefs); err != nil {
		return RuntimeCompactBoundary{}, err
	}
	return boundary, nil
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to encode compact JSON: %w", err)
	}
	return string(data), nil
}

func marshalOptionalJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return marshalJSON(value)
}

func unmarshalJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if refs, ok := target.(*[]RuntimeReinjectedRef); ok {
		if err := json.Unmarshal([]byte(raw), refs); err == nil {
			return nil
		}
		var legacy []string
		if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
			return fmt.Errorf("failed to decode compact JSON: %w", err)
		}
		for _, ref := range legacy {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			*refs = append(*refs, RuntimeReinjectedRef{
				ID:     ref,
				Kind:   "legacy",
				Ref:    ref,
				Status: compactStatusCompleted,
				Reason: "legacy_reinjected_ref",
			})
		}
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("failed to decode compact JSON: %w", err)
	}
	return nil
}

func nullableCompactInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
