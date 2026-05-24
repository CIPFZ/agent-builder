package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type RuntimeAuditEvent struct {
	ID           string         `json:"id"`
	SessionID    string         `json:"session_id,omitempty"`
	TurnID       string         `json:"turn_id,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	PermissionID string         `json:"permission_id,omitempty"`
	Type         string         `json:"type"`
	CreatedAt    string         `json:"created_at"`
	Payload      map[string]any `json:"payload"`
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
	event.Payload = redactRuntimePayload(event.Payload)
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to encode runtime audit payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_audit_events (id, session_id, turn_id, tool_call_id, permission_id, type, created_at, payload_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.SessionID,
		event.TurnID,
		event.ToolCallID,
		event.PermissionID,
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
SELECT id, session_id, turn_id, tool_call_id, permission_id, type, created_at, payload_json
FROM runtime_audit_events
WHERE turn_id = ?
ORDER BY created_at ASC`, turnID)
	if err != nil {
		return RuntimeAuditResponse{}, fmt.Errorf("failed to list runtime audit events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanRuntimeAuditRows(rows)
}

func (s runtimeAuditStore) ListSession(ctx context.Context, sessionID string) (RuntimeAuditResponse, error) {
	if s.db == nil {
		return RuntimeAuditResponse{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, tool_call_id, permission_id, type, created_at, payload_json
FROM runtime_audit_events
WHERE session_id = ?
ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return RuntimeAuditResponse{}, fmt.Errorf("failed to list runtime audit events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanRuntimeAuditRows(rows)
}

func scanRuntimeAuditRows(rows *sql.Rows) (RuntimeAuditResponse, error) {
	var events []RuntimeAuditEvent
	for rows.Next() {
		var event RuntimeAuditEvent
		var payload string
		if err := rows.Scan(&event.ID, &event.SessionID, &event.TurnID, &event.ToolCallID, &event.PermissionID, &event.Type, &event.CreatedAt, &payload); err != nil {
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

func (r *runtimeService) AuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAuditResponse{}, err
	}
	resp, err := newRuntimeAuditStore(db).ListTurn(ctx, strings.TrimSpace(turnID))
	if err != nil {
		return RuntimeAuditResponse{}, err
	}
	resp.Summary = r.auditTurnSummary(ctx, strings.TrimSpace(turnID), resp.Events)
	return resp, nil
}

func (r *runtimeService) AuditSession(ctx context.Context, sessionID string) (RuntimeAuditResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeAuditResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAuditResponse{}, err
	}
	return newRuntimeAuditStore(db).ListSession(ctx, sessionID)
}

func (r *runtimeService) auditTurnSummary(ctx context.Context, turnID string, events []RuntimeAuditEvent) RuntimeAuditTurnSummary {
	summary := RuntimeAuditTurnSummary{TurnID: turnID}
	if turn, err := r.turns.Get(ctx, turnID); err == nil {
		summary.SessionID = turn.SessionID
		summary.Provider = turn.Provider
		summary.Model = turn.Model
		summary.PromptPreview = turn.PromptPreview
		summary.UsageBefore = turn.UsageBefore
		summary.UsageAfter = turn.UsageAfter
		summary.UsageDelta = turn.UsageDelta
		summary.FinalStatus = turn.Status
		summary.LatestAssistantMessageID = turn.LatestAssistantMessageID
		summary.StartedAt = turn.StartedAt
		summary.FinishedAt = turn.FinishedAt
	}
	if calls, err := r.TurnToolCalls(ctx, turnID); err == nil {
		summary.ToolCalls = calls.ToolCalls
	}
	if tasks, err := r.TurnAgentTasks(ctx, turnID); err == nil {
		summary.Tasks = tasks.Tasks
	}
	permissionIDs := make(map[string]struct{})
	for _, event := range events {
		if summary.SessionID == "" {
			summary.SessionID = event.SessionID
		}
		if summary.CreatedAt == "" || event.CreatedAt < summary.CreatedAt {
			summary.CreatedAt = event.CreatedAt
		}
		if event.CreatedAt > summary.UpdatedAt {
			summary.UpdatedAt = event.CreatedAt
		}
		mergeAuditSummaryPayload(&summary, event.Payload)
		permissionID := firstNonEmpty(event.PermissionID, stringFromMap(event.Payload, "permission_id"))
		if permissionID != "" {
			if _, ok := permissionIDs[permissionID]; !ok {
				permissionIDs[permissionID] = struct{}{}
				summary.Permissions = append(summary.Permissions, map[string]any{
					"permission_id": permissionID,
					"tool_call_id":  firstNonEmpty(event.ToolCallID, stringFromMap(event.Payload, "tool_call_id")),
					"tool_name":     stringFromMap(event.Payload, "permission_tool"),
					"action":        stringFromMap(event.Payload, "permission_action"),
					"decision":      stringFromMap(event.Payload, "permission_policy"),
					"path":          stringFromMap(event.Payload, "permission_path"),
				})
			}
		}
	}
	return summary
}

func mergeAuditSummaryPayload(summary *RuntimeAuditTurnSummary, payload map[string]any) {
	if summary.Provider == "" {
		summary.Provider = stringFromMap(payload, "provider")
	}
	if summary.Model == "" {
		summary.Model = stringFromMap(payload, "model")
	}
	if summary.PromptPreview == "" {
		summary.PromptPreview = stringFromMap(payload, "prompt_preview")
	}
	if summary.LatestAssistantMessageID == "" {
		summary.LatestAssistantMessageID = stringFromMap(payload, "latest_assistant_id")
	}
	if errText := stringFromMap(payload, "error"); errText != "" && !slices.Contains(summary.Errors, errText) {
		summary.Errors = append(summary.Errors, errText)
	}
	if skillSummary, ok := payload["skill_summary"].(map[string]any); ok && summary.Skills == nil {
		summary.Skills = runtimeTurnSkillSummaryFromPayload(skillSummary)
	}
	if contextSummary, ok := payload["context_summary"].(map[string]any); ok && summary.Context == nil {
		summary.Context = runtimeTurnContextSummaryFromPayload(contextSummary)
	}
	if summary.FinalStatus == "" {
		switch stringFromMap(payload, "event") {
		case "finished":
			summary.FinalStatus = turnStatusCompleted
		case "failed":
			summary.FinalStatus = turnStatusFailed
		case "cancelled", "cancel_requested":
			summary.FinalStatus = turnStatusCancelled
		}
	}
}

func runtimeTurnContextSummaryFromPayload(payload map[string]any) *RuntimeTurnContextSummary {
	summary := &RuntimeTurnContextSummary{
		AvailableCount: intFromMap(payload, "available_count"),
		TokenEstimate:  intFromMap(payload, "token_estimate"),
	}
	summary.Available = runtimeContextSourcesFromMap(payload, "available")
	summary.Loaded = runtimeContextSourcesFromMap(payload, "loaded")
	summary.Injected = runtimeContextSourcesFromMap(payload, "injected")
	summary.Skipped = runtimeContextSourcesFromMap(payload, "skipped")
	summary.Failed = runtimeContextSourcesFromMap(payload, "failed")
	return summary
}

func runtimeContextSourcesFromMap(payload map[string]any, key string) []RuntimeContextSource {
	raw, ok := payload[key].([]any)
	if !ok {
		return nil
	}
	items := make([]RuntimeContextSource, 0, len(raw))
	for _, value := range raw {
		itemMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, RuntimeContextSource{
			ID:             stringFromMap(itemMap, "id"),
			Kind:           stringFromMap(itemMap, "kind"),
			Name:           stringFromMap(itemMap, "name"),
			Path:           stringFromMap(itemMap, "path"),
			URI:            stringFromMap(itemMap, "uri"),
			Scope:          stringFromMap(itemMap, "scope"),
			Enabled:        boolFromMap(itemMap, "enabled"),
			State:          stringFromMap(itemMap, "state"),
			Reason:         stringFromMap(itemMap, "reason"),
			Diagnostics:    stringFromMap(itemMap, "diagnostics"),
			Error:          stringFromMap(itemMap, "error"),
			ContentSummary: stringFromMap(itemMap, "content_summary"),
			TokenEstimate:  intFromMap(itemMap, "token_estimate"),
			LoadedAt:       stringFromMap(itemMap, "loaded_at"),
		})
	}
	return items
}

func runtimeTurnSkillSummaryFromPayload(payload map[string]any) *RuntimeTurnSkillSummary {
	summary := &RuntimeTurnSkillSummary{
		AvailableCount: intFromMap(payload, "available_count"),
		PolicyMode:     stringFromMap(payload, "policy_mode"),
		PolicyRisk:     stringFromMap(payload, "policy_risk"),
		PolicyReason:   stringFromMap(payload, "policy_reason"),
		SourcePaths:    stringSliceFromMap(payload, "source_paths"),
	}
	summary.Available = runtimeSkillTurnItemsFromMap(payload, "available")
	summary.Activated = runtimeSkillTurnItemsFromMap(payload, "activated")
	summary.Excluded = runtimeSkillTurnItemsFromMap(payload, "excluded")
	summary.Failed = runtimeSkillTurnItemsFromMap(payload, "failed")
	return summary
}

func runtimeSkillTurnItemsFromMap(payload map[string]any, key string) []RuntimeSkillTurnItem {
	raw, ok := payload[key].([]any)
	if !ok {
		return nil
	}
	items := make([]RuntimeSkillTurnItem, 0, len(raw))
	for _, value := range raw {
		itemMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, RuntimeSkillTurnItem{
			Name:          stringFromMap(itemMap, "name"),
			CapabilityID:  stringFromMap(itemMap, "capability_id"),
			Builtin:       boolFromMap(itemMap, "builtin"),
			Path:          stringFromMap(itemMap, "path"),
			SkillFilePath: stringFromMap(itemMap, "skill_file_path"),
			State:         stringFromMap(itemMap, "state"),
			Reason:        stringFromMap(itemMap, "reason"),
			Error:         stringFromMap(itemMap, "error"),
			AllowedTools:  stringSliceFromMap(itemMap, "allowed_tools"),
		})
	}
	return items
}

func stringFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func stringSliceFromMap(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings
		}
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func intFromMap(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func boolFromMap(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	result, _ := value.(bool)
	return result
}
