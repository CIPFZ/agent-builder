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
	taskMessageDirectionParentToChild = "parent_to_child"
	taskMessageDirectionChildToParent = "child_to_parent"

	taskMessageKindInstruction = "instruction"
	taskMessageKindControl     = "control"
	taskMessageKindProgress    = "progress"
	taskMessageKindResult      = "result"
	taskMessageKindArtifact    = "artifact"

	taskMessageStatusCreated   = "created"
	taskMessageStatusDelivered = "delivered"
)

type runtimeAgentTaskMessageStore struct {
	db *sql.DB
}

type runtimeAgentTaskResultStore struct {
	db *sql.DB
}

func newRuntimeAgentTaskMessageStore(db *sql.DB) runtimeAgentTaskMessageStore {
	return runtimeAgentTaskMessageStore{db: db}
}

func newRuntimeAgentTaskResultStore(db *sql.DB) runtimeAgentTaskResultStore {
	return runtimeAgentTaskResultStore{db: db}
}

func (s runtimeAgentTaskMessageStore) Create(ctx context.Context, msg RuntimeAgentTaskMessage) (RuntimeAgentTaskMessage, error) {
	if s.db == nil {
		return RuntimeAgentTaskMessage{}, errors.New("runtime agent task message database is not available")
	}
	msg.ID = strings.TrimSpace(msg.ID)
	msg.TaskID = strings.TrimSpace(msg.TaskID)
	if msg.ID == "" {
		msg.ID = newRuntimeEventID()
	}
	if msg.TaskID == "" {
		return RuntimeAgentTaskMessage{}, errors.New("agent task message task id is required")
	}
	msg.Direction = normalizeTaskMessageDirection(msg.Direction)
	msg.Kind = normalizeTaskMessageKind(msg.Kind)
	if msg.Status == "" {
		msg.Status = taskMessageStatusCreated
	}
	if msg.CreatedAt == 0 {
		msg.CreatedAt = time.Now().UnixMilli()
	}
	payload, err := encodeJSONMap(msg.Payload)
	if err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	artifactRefs, err := encodeStringSlice(msg.ArtifactRefs)
	if err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_agent_task_messages (
    id, task_id, parent_task_id, parent_turn_id, parent_session_id, child_session_id,
    direction, kind, status, content_summary, payload_json, related_tool_call_id,
    related_message_id, artifact_refs_json, created_at, delivered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.TaskID,
		nullableString(msg.ParentTaskID),
		nullableString(msg.ParentTurnID),
		nullableString(msg.ParentSessionID),
		nullableString(msg.ChildSessionID),
		msg.Direction,
		msg.Kind,
		msg.Status,
		nullableString(preview(msg.ContentSummary, runtimePartPreviewLimit)),
		payload,
		nullableString(msg.RelatedToolCallID),
		nullableString(msg.RelatedMessageID),
		artifactRefs,
		msg.CreatedAt,
		nullableInt64(msg.DeliveredAt),
	)
	if err != nil {
		return RuntimeAgentTaskMessage{}, fmt.Errorf("failed to create runtime agent task message: %w", err)
	}
	return s.Get(ctx, msg.ID)
}

func (s runtimeAgentTaskMessageStore) Get(ctx context.Context, id string) (RuntimeAgentTaskMessage, error) {
	if s.db == nil {
		return RuntimeAgentTaskMessage{}, errors.New("runtime agent task message database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeAgentTaskMessageSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	msg, err := scanRuntimeAgentTaskMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeAgentTaskMessage{}, errRuntimeAgentTaskNotFound
	}
	return msg, err
}

func (s runtimeAgentTaskMessageStore) ListByTask(ctx context.Context, taskID string) ([]RuntimeAgentTaskMessage, error) {
	if s.db == nil {
		return nil, errors.New("runtime agent task message database is not available")
	}
	rows, err := s.db.QueryContext(ctx, runtimeAgentTaskMessageSelectSQL()+` WHERE task_id = ? ORDER BY created_at ASC`, strings.TrimSpace(taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime agent task messages: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeAgentTaskMessages(rows)
}

func runtimeAgentTaskMessageSelectSQL() string {
	return `
SELECT id, task_id, parent_task_id, parent_turn_id, parent_session_id, child_session_id,
    direction, kind, status, content_summary, payload_json, related_tool_call_id,
    related_message_id, artifact_refs_json, created_at, delivered_at
FROM runtime_agent_task_messages`
}

func scanRuntimeAgentTaskMessages(rows *sql.Rows) ([]RuntimeAgentTaskMessage, error) {
	var messages []RuntimeAgentTaskMessage
	for rows.Next() {
		msg, err := scanRuntimeAgentTaskMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

type runtimeAgentTaskMessageScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeAgentTaskMessage(scanner runtimeAgentTaskMessageScanner) (RuntimeAgentTaskMessage, error) {
	var msg RuntimeAgentTaskMessage
	var parentTaskID, parentTurnID, parentSessionID, childSessionID, contentSummary, payload, toolCallID, messageID, artifactRefs sql.NullString
	var deliveredAt sql.NullInt64
	if err := scanner.Scan(
		&msg.ID,
		&msg.TaskID,
		&parentTaskID,
		&parentTurnID,
		&parentSessionID,
		&childSessionID,
		&msg.Direction,
		&msg.Kind,
		&msg.Status,
		&contentSummary,
		&payload,
		&toolCallID,
		&messageID,
		&artifactRefs,
		&msg.CreatedAt,
		&deliveredAt,
	); err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	msg.ParentTaskID = parentTaskID.String
	msg.ParentTurnID = parentTurnID.String
	msg.ParentSessionID = parentSessionID.String
	msg.ChildSessionID = childSessionID.String
	msg.ContentSummary = contentSummary.String
	msg.Payload = decodeJSONMap(payload.String)
	msg.RelatedToolCallID = toolCallID.String
	msg.RelatedMessageID = messageID.String
	msg.ArtifactRefs = decodeStringSlice(artifactRefs.String)
	if deliveredAt.Valid {
		msg.DeliveredAt = deliveredAt.Int64
	}
	return msg, nil
}

func (s runtimeAgentTaskResultStore) Upsert(ctx context.Context, result RuntimeAgentTaskResult) (RuntimeAgentTaskResult, error) {
	if s.db == nil {
		return RuntimeAgentTaskResult{}, errors.New("runtime agent task result database is not available")
	}
	result.TaskID = strings.TrimSpace(result.TaskID)
	if result.TaskID == "" {
		return RuntimeAgentTaskResult{}, errors.New("agent task result task id is required")
	}
	if result.Status == "" {
		result.Status = agentTaskStatusRunning
	}
	now := time.Now().UnixMilli()
	if result.CreatedAt == 0 {
		result.CreatedAt = now
	}
	result.UpdatedAt = now
	artifactRefs, err := encodeStringSlice(result.ArtifactRefs)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	messageRefs, err := encodeStringSlice(result.RelatedMessageRefs)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	toolRefs, err := encodeStringSlice(result.RelatedToolCallRefs)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	compactRefs, err := encodeStringSlice(result.CompactBoundaryRefs)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_agent_task_results (
    task_id, status, summary, error_detail, cancellation_detail, artifact_refs_json,
    related_message_refs_json, related_tool_call_refs_json, compact_boundary_refs_json,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
    status = excluded.status,
    summary = COALESCE(NULLIF(excluded.summary, ''), runtime_agent_task_results.summary),
    error_detail = COALESCE(NULLIF(excluded.error_detail, ''), runtime_agent_task_results.error_detail),
    cancellation_detail = COALESCE(NULLIF(excluded.cancellation_detail, ''), runtime_agent_task_results.cancellation_detail),
    artifact_refs_json = COALESCE(excluded.artifact_refs_json, runtime_agent_task_results.artifact_refs_json),
    related_message_refs_json = COALESCE(excluded.related_message_refs_json, runtime_agent_task_results.related_message_refs_json),
    related_tool_call_refs_json = COALESCE(excluded.related_tool_call_refs_json, runtime_agent_task_results.related_tool_call_refs_json),
    compact_boundary_refs_json = COALESCE(excluded.compact_boundary_refs_json, runtime_agent_task_results.compact_boundary_refs_json),
    updated_at = excluded.updated_at`,
		result.TaskID,
		result.Status,
		nullableString(preview(result.Summary, runtimePartPreviewLimit)),
		nullableString(preview(result.ErrorDetail, runtimePartPreviewLimit)),
		nullableString(preview(result.CancellationDetail, runtimePartPreviewLimit)),
		artifactRefs,
		messageRefs,
		toolRefs,
		compactRefs,
		result.CreatedAt,
		result.UpdatedAt,
	)
	if err != nil {
		return RuntimeAgentTaskResult{}, fmt.Errorf("failed to upsert runtime agent task result: %w", err)
	}
	return s.Get(ctx, result.TaskID)
}

func (s runtimeAgentTaskResultStore) Get(ctx context.Context, taskID string) (RuntimeAgentTaskResult, error) {
	if s.db == nil {
		return RuntimeAgentTaskResult{}, errors.New("runtime agent task result database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT task_id, status, summary, error_detail, cancellation_detail, artifact_refs_json,
    related_message_refs_json, related_tool_call_refs_json, compact_boundary_refs_json,
    created_at, updated_at
FROM runtime_agent_task_results
WHERE task_id = ?`, strings.TrimSpace(taskID))
	result, err := scanRuntimeAgentTaskResult(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeAgentTaskResult{}, errRuntimeAgentTaskNotFound
	}
	return result, err
}

type runtimeAgentTaskResultScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeAgentTaskResult(scanner runtimeAgentTaskResultScanner) (RuntimeAgentTaskResult, error) {
	var result RuntimeAgentTaskResult
	var summary, errText, cancellationDetail, artifactRefs, messageRefs, toolRefs, compactRefs sql.NullString
	if err := scanner.Scan(
		&result.TaskID,
		&result.Status,
		&summary,
		&errText,
		&cancellationDetail,
		&artifactRefs,
		&messageRefs,
		&toolRefs,
		&compactRefs,
		&result.CreatedAt,
		&result.UpdatedAt,
	); err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	result.Summary = summary.String
	result.ErrorDetail = errText.String
	result.CancellationDetail = cancellationDetail.String
	result.ArtifactRefs = decodeStringSlice(artifactRefs.String)
	result.RelatedMessageRefs = decodeStringSlice(messageRefs.String)
	result.RelatedToolCallRefs = decodeStringSlice(toolRefs.String)
	result.CompactBoundaryRefs = decodeStringSlice(compactRefs.String)
	return result, nil
}

func normalizeTaskMessageDirection(direction string) string {
	switch strings.TrimSpace(direction) {
	case taskMessageDirectionChildToParent:
		return taskMessageDirectionChildToParent
	default:
		return taskMessageDirectionParentToChild
	}
}

func normalizeTaskMessageKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case taskMessageKindControl, taskMessageKindProgress, taskMessageKindResult, taskMessageKindArtifact:
		return strings.TrimSpace(kind)
	default:
		return taskMessageKindInstruction
	}
}

func encodeJSONMap(values map[string]any) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to encode json map: %w", err)
	}
	return string(data), nil
}

func decodeJSONMap(data string) map[string]any {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	values := map[string]any{}
	_ = json.Unmarshal([]byte(data), &values)
	return values
}
