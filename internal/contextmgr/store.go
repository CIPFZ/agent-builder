package contextmgr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) SQLStore {
	return SQLStore{db: db}
}

func (s SQLStore) UpsertProjection(ctx context.Context, projection Projection) (Projection, error) {
	if s.db == nil {
		return Projection{}, errors.New("context governance database is not available")
	}
	projection.ID = strings.TrimSpace(projection.ID)
	projection.SessionID = strings.TrimSpace(projection.SessionID)
	projection.TurnID = strings.TrimSpace(projection.TurnID)
	if projection.ID == "" {
		return Projection{}, errors.New("context projection id is required")
	}
	if projection.SessionID == "" {
		return Projection{}, errors.New("context projection session id is required")
	}
	if projection.TurnID == "" {
		return Projection{}, errors.New("context projection turn id is required")
	}
	if projection.Step <= 0 {
		projection.Step = 1
	}
	if projection.Source == "" {
		projection.Source = ProjectionSourceMainTurn
	}
	if projection.Status == "" {
		projection.Status = ProjectionStatusCompleted
	}
	if projection.CreatedAt == 0 {
		projection.CreatedAt = time.Now().UTC().UnixMilli()
	}
	before, err := marshalOptionalJSON(projection.BudgetBefore)
	if err != nil {
		return Projection{}, err
	}
	after, err := marshalOptionalJSON(projection.BudgetAfter)
	if err != nil {
		return Projection{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_context_projections (
    id, session_id, turn_id, step, provider, model, source, status,
    canonical_message_count, projected_message_count, budget_before_json,
    budget_after_json, created_at, completed_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    step = excluded.step,
    provider = excluded.provider,
    model = excluded.model,
    source = excluded.source,
    status = excluded.status,
    canonical_message_count = excluded.canonical_message_count,
    projected_message_count = excluded.projected_message_count,
    budget_before_json = excluded.budget_before_json,
    budget_after_json = excluded.budget_after_json,
    completed_at = excluded.completed_at,
    error = excluded.error`,
		projection.ID, projection.SessionID, projection.TurnID, projection.Step,
		nullableString(projection.Provider), nullableString(projection.Model),
		projection.Source, projection.Status, projection.CanonicalMessageCount,
		projection.ProjectedMessageCount, before, after, projection.CreatedAt,
		nullableInt64(projection.CompletedAt), nullableString(projection.Error),
	)
	if err != nil {
		return Projection{}, fmt.Errorf("failed to upsert context projection: %w", err)
	}
	return s.GetProjection(ctx, projection.ID)
}

func (s SQLStore) GetProjection(ctx context.Context, id string) (Projection, error) {
	if s.db == nil {
		return Projection{}, errors.New("context governance database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, step, provider, model, source, status,
    canonical_message_count, projected_message_count, budget_before_json,
    budget_after_json, created_at, completed_at, error
FROM runtime_context_projections
WHERE id = ?`, strings.TrimSpace(id))
	return scanProjection(row)
}

func (s SQLStore) ListProjectionsByTurn(ctx context.Context, turnID string) ([]Projection, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, step, provider, model, source, status,
    canonical_message_count, projected_message_count, budget_before_json,
    budget_after_json, created_at, completed_at, error
FROM runtime_context_projections
WHERE turn_id = ?
ORDER BY step ASC, created_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context projections: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Projection
	for rows.Next() {
		projection, err := scanProjection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, projection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context projections: %w", err)
	}
	return out, nil
}

func (s SQLStore) UpsertProjectionMessage(ctx context.Context, msg ProjectionMessage) (ProjectionMessage, error) {
	if s.db == nil {
		return ProjectionMessage{}, errors.New("context governance database is not available")
	}
	if msg.ID == "" {
		return ProjectionMessage{}, errors.New("context projection message id is required")
	}
	if msg.ProjectionID == "" {
		return ProjectionMessage{}, errors.New("context projection message projection id is required")
	}
	if msg.Status == "" {
		msg.Status = "selected"
	}
	if msg.CreatedAt == 0 {
		msg.CreatedAt = time.Now().UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_context_projection_messages (
    id, projection_id, session_id, turn_id, sequence, role,
    canonical_message_id, status, replacement_id, token_estimate,
    content_ref, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    projection_id = excluded.projection_id,
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    sequence = excluded.sequence,
    role = excluded.role,
    canonical_message_id = excluded.canonical_message_id,
    status = excluded.status,
    replacement_id = excluded.replacement_id,
    token_estimate = excluded.token_estimate,
    content_ref = excluded.content_ref,
    summary = excluded.summary`,
		msg.ID, msg.ProjectionID, msg.SessionID, msg.TurnID, msg.Sequence, msg.Role,
		nullableString(msg.CanonicalMessageID), msg.Status, nullableString(msg.ReplacementID),
		msg.TokenEstimate, nullableString(msg.ContentRef), nullableString(msg.Summary), msg.CreatedAt,
	)
	if err != nil {
		return ProjectionMessage{}, fmt.Errorf("failed to upsert context projection message: %w", err)
	}
	messages, err := s.ListProjectionMessages(ctx, msg.ProjectionID)
	if err != nil {
		return ProjectionMessage{}, err
	}
	for _, stored := range messages {
		if stored.ID == msg.ID {
			return stored, nil
		}
	}
	return ProjectionMessage{}, sql.ErrNoRows
}

func (s SQLStore) ListProjectionMessages(ctx context.Context, projectionID string) ([]ProjectionMessage, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, projection_id, session_id, turn_id, sequence, role,
    canonical_message_id, status, replacement_id, token_estimate,
    content_ref, summary, created_at
FROM runtime_context_projection_messages
WHERE projection_id = ?
ORDER BY sequence ASC`, strings.TrimSpace(projectionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context projection messages: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []ProjectionMessage
	for rows.Next() {
		var msg ProjectionMessage
		var canonicalID, replacementID, contentRef, summary sql.NullString
		if err := rows.Scan(
			&msg.ID, &msg.ProjectionID, &msg.SessionID, &msg.TurnID, &msg.Sequence, &msg.Role,
			&canonicalID, &msg.Status, &replacementID, &msg.TokenEstimate,
			&contentRef, &summary, &msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		msg.CanonicalMessageID = canonicalID.String
		msg.ReplacementID = replacementID.String
		msg.ContentRef = contentRef.String
		msg.Summary = summary.String
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context projection messages: %w", err)
	}
	return out, nil
}

func (s SQLStore) UpsertBoundary(ctx context.Context, boundary Boundary) (Boundary, error) {
	if s.db == nil {
		return Boundary{}, errors.New("context governance database is not available")
	}
	if boundary.ID == "" {
		return Boundary{}, errors.New("context boundary id is required")
	}
	if boundary.SessionID == "" {
		return Boundary{}, errors.New("context boundary session id is required")
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
	messageRefs, err := marshalJSON(boundary.MessageRefs)
	if err != nil {
		return Boundary{}, err
	}
	toolRefs, err := marshalJSON(boundary.ToolCallRefs)
	if err != nil {
		return Boundary{}, err
	}
	reinjectedRefs, err := marshalJSON(boundary.ReinjectedRefs)
	if err != nil {
		return Boundary{}, err
	}
	before, err := marshalOptionalJSON(boundary.BudgetBefore)
	if err != nil {
		return Boundary{}, err
	}
	after, err := marshalOptionalJSON(boundary.BudgetAfter)
	if err != nil {
		return Boundary{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_context_boundaries (
    id, session_id, turn_id, projection_id, kind, trigger, status,
    summary_message_id, summary_ref, message_refs_json, tool_call_refs_json,
    reinjected_refs_json, budget_before_json, budget_after_json,
    created_at, completed_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    kind = excluded.kind,
    trigger = excluded.trigger,
    status = excluded.status,
    summary_message_id = excluded.summary_message_id,
    summary_ref = excluded.summary_ref,
    message_refs_json = excluded.message_refs_json,
    tool_call_refs_json = excluded.tool_call_refs_json,
    reinjected_refs_json = excluded.reinjected_refs_json,
    budget_before_json = excluded.budget_before_json,
    budget_after_json = excluded.budget_after_json,
    completed_at = excluded.completed_at,
    error = excluded.error`,
		boundary.ID, boundary.SessionID, nullableString(boundary.TurnID), nullableString(boundary.ProjectionID),
		boundary.Kind, boundary.Trigger, boundary.Status, nullableString(boundary.SummaryMessageID),
		nullableString(boundary.SummaryRef), messageRefs, toolRefs, reinjectedRefs,
		before, after, boundary.CreatedAt, nullableInt64(boundary.CompletedAt), nullableString(boundary.Error),
	)
	if err != nil {
		return Boundary{}, fmt.Errorf("failed to upsert context boundary: %w", err)
	}
	return boundary, nil
}

func (s SQLStore) ListBoundariesByProjection(ctx context.Context, projectionID string) ([]Boundary, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, kind, trigger, status,
    summary_message_id, summary_ref, message_refs_json, tool_call_refs_json,
    reinjected_refs_json, budget_before_json, budget_after_json,
    created_at, completed_at, error
FROM runtime_context_boundaries
WHERE projection_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(projectionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Boundary
	for rows.Next() {
		boundary, err := scanBoundary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, boundary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context boundaries: %w", err)
	}
	return out, nil
}

func (s SQLStore) ListBoundariesByTurn(ctx context.Context, turnID string) ([]Boundary, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, kind, trigger, status,
    summary_message_id, summary_ref, message_refs_json, tool_call_refs_json,
    reinjected_refs_json, budget_before_json, budget_after_json,
    created_at, completed_at, error
FROM runtime_context_boundaries
WHERE turn_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list turn context boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Boundary
	for rows.Next() {
		boundary, err := scanBoundary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, boundary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate turn context boundaries: %w", err)
	}
	return out, nil
}

func (s SQLStore) ListBoundariesBySession(ctx context.Context, sessionID string) ([]Boundary, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, kind, trigger, status,
    summary_message_id, summary_ref, message_refs_json, tool_call_refs_json,
    reinjected_refs_json, budget_before_json, budget_after_json,
    created_at, completed_at, error
FROM runtime_context_boundaries
WHERE session_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list session context boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Boundary
	for rows.Next() {
		boundary, err := scanBoundary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, boundary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate session context boundaries: %w", err)
	}
	return out, nil
}

func (s SQLStore) UpsertContentReplacement(ctx context.Context, replacement ContentReplacement) (ContentReplacement, error) {
	if s.db == nil {
		return ContentReplacement{}, errors.New("context governance database is not available")
	}
	if replacement.ID == "" {
		return ContentReplacement{}, errors.New("context replacement id is required")
	}
	if replacement.SessionID == "" {
		return ContentReplacement{}, errors.New("context replacement session id is required")
	}
	if replacement.Kind == "" {
		replacement.Kind = "tool_result"
	}
	if replacement.CreatedAt == 0 {
		replacement.CreatedAt = time.Now().UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_context_content_replacements (
    id, session_id, turn_id, projection_id, tool_call_id, tool_name, kind,
    original_ref, replacement_text, original_size_bytes,
    original_estimated_tokens, replacement_estimated_tokens, reason, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    tool_call_id = excluded.tool_call_id,
    tool_name = excluded.tool_name,
    kind = excluded.kind,
    original_ref = excluded.original_ref,
    replacement_text = excluded.replacement_text,
    original_size_bytes = excluded.original_size_bytes,
    original_estimated_tokens = excluded.original_estimated_tokens,
    replacement_estimated_tokens = excluded.replacement_estimated_tokens,
    reason = excluded.reason`,
		replacement.ID, replacement.SessionID, nullableString(replacement.TurnID),
		nullableString(replacement.ProjectionID), nullableString(replacement.ToolCallID),
		nullableString(replacement.ToolName), replacement.Kind, nullableString(replacement.OriginalRef),
		replacement.ReplacementText, replacement.OriginalSizeBytes, replacement.OriginalEstimatedTokens,
		replacement.ReplacementEstimatedTokens, nullableString(replacement.Reason), replacement.CreatedAt,
	)
	if err != nil {
		return ContentReplacement{}, fmt.Errorf("failed to upsert context replacement: %w", err)
	}
	return replacement, nil
}

func (s SQLStore) ListContentReplacementsByProjection(ctx context.Context, projectionID string) ([]ContentReplacement, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, tool_call_id, tool_name, kind,
    original_ref, replacement_text, original_size_bytes,
    original_estimated_tokens, replacement_estimated_tokens, reason, created_at
FROM runtime_context_content_replacements
WHERE projection_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(projectionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context replacements: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []ContentReplacement
	for rows.Next() {
		replacement, err := scanContentReplacement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, replacement)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context replacements: %w", err)
	}
	return out, nil
}

func (s SQLStore) GetContentReplacementByToolCall(ctx context.Context, toolCallID string) (ContentReplacement, error) {
	if s.db == nil {
		return ContentReplacement{}, errors.New("context governance database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, projection_id, tool_call_id, tool_name, kind,
    original_ref, replacement_text, original_size_bytes,
    original_estimated_tokens, replacement_estimated_tokens, reason, created_at
FROM runtime_context_content_replacements
WHERE tool_call_id = ?
ORDER BY created_at ASC
LIMIT 1`, strings.TrimSpace(toolCallID))
	replacement, err := scanContentReplacement(row)
	if err != nil {
		return ContentReplacement{}, err
	}
	return replacement, nil
}

func (s SQLStore) UpsertSnipBoundary(ctx context.Context, snip SnipBoundary) (SnipBoundary, error) {
	if s.db == nil {
		return SnipBoundary{}, errors.New("context governance database is not available")
	}
	if snip.ID == "" {
		return SnipBoundary{}, errors.New("context snip boundary id is required")
	}
	if snip.SessionID == "" {
		return SnipBoundary{}, errors.New("context snip boundary session id is required")
	}
	if snip.CreatedAt == 0 {
		snip.CreatedAt = time.Now().UTC().UnixMilli()
	}
	removedRefs, err := marshalJSON(snip.RemovedMessageRefs)
	if err != nil {
		return SnipBoundary{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_context_snip_boundaries (
    id, session_id, turn_id, projection_id, removed_message_refs_json,
    preserved_head_ref, preserved_tail_ref, summary_ref, reason, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    removed_message_refs_json = excluded.removed_message_refs_json,
    preserved_head_ref = excluded.preserved_head_ref,
    preserved_tail_ref = excluded.preserved_tail_ref,
    summary_ref = excluded.summary_ref,
    reason = excluded.reason`,
		snip.ID, snip.SessionID, nullableString(snip.TurnID), nullableString(snip.ProjectionID),
		removedRefs, nullableString(snip.PreservedHeadRef), nullableString(snip.PreservedTailRef),
		nullableString(snip.SummaryRef), nullableString(snip.Reason), snip.CreatedAt,
	)
	if err != nil {
		return SnipBoundary{}, fmt.Errorf("failed to upsert context snip boundary: %w", err)
	}
	return snip, nil
}

func (s SQLStore) ListSnipBoundariesByProjection(ctx context.Context, projectionID string) ([]SnipBoundary, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, removed_message_refs_json,
    preserved_head_ref, preserved_tail_ref, summary_ref, reason, created_at
FROM runtime_context_snip_boundaries
WHERE projection_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(projectionID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context snip boundaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []SnipBoundary
	for rows.Next() {
		snip, err := scanSnipBoundary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context snip boundaries: %w", err)
	}
	return out, nil
}

func (s SQLStore) UpsertWarning(ctx context.Context, warning Warning) (Warning, error) {
	if s.db == nil {
		return Warning{}, errors.New("context governance database is not available")
	}
	if warning.ID == "" {
		return Warning{}, errors.New("context warning id is required")
	}
	if warning.SessionID == "" {
		return Warning{}, errors.New("context warning session id is required")
	}
	if warning.Severity == "" {
		warning.Severity = "info"
	}
	if warning.CreatedAt == 0 {
		warning.CreatedAt = time.Now().UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_context_warnings (
    id, session_id, turn_id, projection_id, code, message, severity, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    code = excluded.code,
    message = excluded.message,
    severity = excluded.severity`,
		warning.ID, warning.SessionID, nullableString(warning.TurnID), nullableString(warning.ProjectionID),
		warning.Code, warning.Message, warning.Severity, warning.CreatedAt,
	)
	if err != nil {
		return Warning{}, fmt.Errorf("failed to upsert context warning: %w", err)
	}
	return warning, nil
}

func (s SQLStore) UpsertReactiveAttempt(ctx context.Context, attempt ReactiveAttempt) (ReactiveAttempt, error) {
	if s.db == nil {
		return ReactiveAttempt{}, errors.New("context governance database is not available")
	}
	if attempt.ID == "" {
		return ReactiveAttempt{}, errors.New("context reactive attempt id is required")
	}
	if attempt.SessionID == "" {
		return ReactiveAttempt{}, errors.New("context reactive attempt session id is required")
	}
	if attempt.TurnID == "" {
		return ReactiveAttempt{}, errors.New("context reactive attempt turn id is required")
	}
	if attempt.Attempt <= 0 {
		attempt.Attempt = 1
	}
	if attempt.Action == "" {
		attempt.Action = "projection_reduction"
	}
	if attempt.Status == "" {
		attempt.Status = "recorded"
	}
	if attempt.CreatedAt == 0 {
		attempt.CreatedAt = time.Now().UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_context_reactive_attempts (
    id, session_id, turn_id, projection_id, attempt, action, status,
    error, created_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    attempt = excluded.attempt,
    action = excluded.action,
    status = excluded.status,
    error = excluded.error,
    completed_at = excluded.completed_at`,
		attempt.ID, attempt.SessionID, attempt.TurnID, nullableString(attempt.ProjectionID),
		attempt.Attempt, attempt.Action, attempt.Status, nullableString(attempt.Error),
		attempt.CreatedAt, nullableInt64(attempt.CompletedAt),
	)
	if err != nil {
		return ReactiveAttempt{}, fmt.Errorf("failed to upsert context reactive attempt: %w", err)
	}
	return attempt, nil
}

func (s SQLStore) ListReactiveAttemptsByTurn(ctx context.Context, turnID string) ([]ReactiveAttempt, error) {
	if s.db == nil {
		return nil, errors.New("context governance database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, attempt, action, status,
    error, created_at, completed_at
FROM runtime_context_reactive_attempts
WHERE turn_id = ?
ORDER BY attempt ASC, created_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list context reactive attempts: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []ReactiveAttempt
	for rows.Next() {
		attempt, err := scanReactiveAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate context reactive attempts: %w", err)
	}
	return out, nil
}

type projectionScanner interface {
	Scan(dest ...any) error
}

type boundaryScanner interface {
	Scan(dest ...any) error
}

func scanBoundary(scanner boundaryScanner) (Boundary, error) {
	var boundary Boundary
	var turnID, projectionID, summaryMessageID, summaryRef, messageRefs, toolRefs, reinjectedRefs, before, after, errText sql.NullString
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&boundary.ID, &boundary.SessionID, &turnID, &projectionID, &boundary.Kind,
		&boundary.Trigger, &boundary.Status, &summaryMessageID, &summaryRef,
		&messageRefs, &toolRefs, &reinjectedRefs, &before, &after,
		&boundary.CreatedAt, &completedAt, &errText,
	); err != nil {
		return Boundary{}, err
	}
	boundary.TurnID = turnID.String
	boundary.ProjectionID = projectionID.String
	boundary.SummaryMessageID = summaryMessageID.String
	boundary.SummaryRef = summaryRef.String
	boundary.Error = errText.String
	if completedAt.Valid {
		boundary.CompletedAt = completedAt.Int64
	}
	if err := decodeContextJSON(messageRefs.String, &boundary.MessageRefs); err != nil {
		return Boundary{}, err
	}
	if err := decodeContextJSON(toolRefs.String, &boundary.ToolCallRefs); err != nil {
		return Boundary{}, err
	}
	if err := decodeContextJSON(reinjectedRefs.String, &boundary.ReinjectedRefs); err != nil {
		return Boundary{}, err
	}
	if before.Valid && strings.TrimSpace(before.String) != "" {
		var report BudgetReport
		if err := json.Unmarshal([]byte(before.String), &report); err != nil {
			return Boundary{}, fmt.Errorf("failed to decode boundary budget_before: %w", err)
		}
		boundary.BudgetBefore = &report
	}
	if after.Valid && strings.TrimSpace(after.String) != "" {
		var report BudgetReport
		if err := json.Unmarshal([]byte(after.String), &report); err != nil {
			return Boundary{}, fmt.Errorf("failed to decode boundary budget_after: %w", err)
		}
		boundary.BudgetAfter = &report
	}
	return boundary, nil
}

type contentReplacementScanner interface {
	Scan(dest ...any) error
}

func scanContentReplacement(scanner contentReplacementScanner) (ContentReplacement, error) {
	var replacement ContentReplacement
	var turnID, projectionID, toolCall, toolName, originalRef, reason sql.NullString
	if err := scanner.Scan(
		&replacement.ID, &replacement.SessionID, &turnID, &projectionID, &toolCall, &toolName,
		&replacement.Kind, &originalRef, &replacement.ReplacementText, &replacement.OriginalSizeBytes,
		&replacement.OriginalEstimatedTokens, &replacement.ReplacementEstimatedTokens, &reason,
		&replacement.CreatedAt,
	); err != nil {
		return ContentReplacement{}, err
	}
	replacement.TurnID = turnID.String
	replacement.ProjectionID = projectionID.String
	replacement.ToolCallID = toolCall.String
	replacement.ToolName = toolName.String
	replacement.OriginalRef = originalRef.String
	replacement.Reason = reason.String
	return replacement, nil
}

type snipBoundaryScanner interface {
	Scan(dest ...any) error
}

func scanSnipBoundary(scanner snipBoundaryScanner) (SnipBoundary, error) {
	var snip SnipBoundary
	var turnID, projectionID, removedRefs, headRef, tailRef, summaryRef, reason sql.NullString
	if err := scanner.Scan(
		&snip.ID, &snip.SessionID, &turnID, &projectionID, &removedRefs,
		&headRef, &tailRef, &summaryRef, &reason, &snip.CreatedAt,
	); err != nil {
		return SnipBoundary{}, err
	}
	snip.TurnID = turnID.String
	snip.ProjectionID = projectionID.String
	snip.PreservedHeadRef = headRef.String
	snip.PreservedTailRef = tailRef.String
	snip.SummaryRef = summaryRef.String
	snip.Reason = reason.String
	if err := decodeContextJSON(removedRefs.String, &snip.RemovedMessageRefs); err != nil {
		return SnipBoundary{}, err
	}
	return snip, nil
}

type reactiveAttemptScanner interface {
	Scan(dest ...any) error
}

func scanReactiveAttempt(scanner reactiveAttemptScanner) (ReactiveAttempt, error) {
	var attempt ReactiveAttempt
	var projectionID, errText sql.NullString
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&attempt.ID, &attempt.SessionID, &attempt.TurnID, &projectionID,
		&attempt.Attempt, &attempt.Action, &attempt.Status, &errText,
		&attempt.CreatedAt, &completedAt,
	); err != nil {
		return ReactiveAttempt{}, err
	}
	attempt.ProjectionID = projectionID.String
	attempt.Error = errText.String
	if completedAt.Valid {
		attempt.CompletedAt = completedAt.Int64
	}
	return attempt, nil
}

func scanProjection(scanner projectionScanner) (Projection, error) {
	var projection Projection
	var provider, model, before, after, errText sql.NullString
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&projection.ID, &projection.SessionID, &projection.TurnID, &projection.Step,
		&provider, &model, &projection.Source, &projection.Status,
		&projection.CanonicalMessageCount, &projection.ProjectedMessageCount,
		&before, &after, &projection.CreatedAt, &completedAt, &errText,
	); err != nil {
		return Projection{}, err
	}
	projection.Provider = provider.String
	projection.Model = model.String
	projection.Error = errText.String
	if completedAt.Valid {
		projection.CompletedAt = completedAt.Int64
	}
	if before.Valid && strings.TrimSpace(before.String) != "" {
		var report BudgetReport
		if err := json.Unmarshal([]byte(before.String), &report); err != nil {
			return Projection{}, fmt.Errorf("failed to decode projection budget_before: %w", err)
		}
		projection.BudgetBefore = &report
	}
	if after.Valid && strings.TrimSpace(after.String) != "" {
		var report BudgetReport
		if err := json.Unmarshal([]byte(after.String), &report); err != nil {
			return Projection{}, fmt.Errorf("failed to decode projection budget_after: %w", err)
		}
		projection.BudgetAfter = &report
	}
	return projection, nil
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to encode context governance JSON: %w", err)
	}
	return string(data), nil
}

func marshalOptionalJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return marshalJSON(value)
}

func decodeContextJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("failed to decode context governance JSON: %w", err)
	}
	return nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
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
