package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/approval"
	"myclaw/internal/model"
	protocolws "myclaw/internal/protocol/ws"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func runtimeEventFromWSMessage(msg protocolws.Message) (runtime.RuntimeEvent, bool) {
	if msg.Type != protocolws.TypeEvent {
		return runtime.RuntimeEvent{}, false
	}

	payload := msg.Payload
	switch msg.Event {
	case protocolws.EventMessageCreated:
		message, ok := parseSessionMessage(payload["message"])
		if !ok {
			return runtime.RuntimeEvent{}, false
		}
		return runtime.RuntimeEvent{
			Type:    "message.created",
			Session: parseRuntimeSession(payload),
			Message: &message,
		}, true
	case "tool.progress":
		progress := parseToolProgress(payload)
		return runtime.RuntimeEvent{
			Type:      "tool.progress",
			Session:   parseRuntimeSession(payload),
			RunID:     stringValue(payload, "run_id"),
			ToolName:  stringValue(payload, "tool_name"),
			ToolUseID: stringValue(payload, "tool_use_id"),
			Progress:  progress,
		}, true
	case "tool.result":
		message, _ := parseSessionMessage(payload["message"])
		return runtime.RuntimeEvent{
			Type:            "tool.result",
			Session:         parseRuntimeSession(payload),
			RunID:           stringValue(payload, "run_id"),
			ToolName:        stringValue(payload, "tool_name"),
			ToolUseID:       stringValue(payload, "tool_use_id"),
			ToolInput:       stringValue(payload, "tool_input"),
			ToolInputObject: mapValue(payload, "tool_input_object"),
			Message:         &message,
		}, true
	case "permission.required":
		return runtime.RuntimeEvent{
			Type:            "permission.required",
			Session:         parseRuntimeSession(payload),
			RunID:           stringValue(payload, "run_id"),
			ToolName:        stringValue(payload, "tool_name"),
			ToolInput:       stringValue(payload, "tool_input"),
			ToolInputObject: mapValue(payload, "tool_input_object"),
			Approval:        parseApprovalRequest(payload),
		}, true
	case "approval.updated":
		return runtime.RuntimeEvent{
			Type:     "approval.updated",
			Session:  parseRuntimeSession(payload),
			RunID:    stringValue(payload, "run_id"),
			Approval: parseApprovalRequest(payload),
		}, true
	case protocolws.EventSubagentUpdated:
		content := summarizeSubagentEvent(payload)
		return runtime.RuntimeEvent{
			Type:    "message.created",
			Session: parseRuntimeSession(payload),
			Message: &session.Message{
				ID:        "subagent-" + stringValue(payload, "run_id"),
				Role:      "system",
				Content:   content,
				CreatedAt: time.Now().UTC(),
			},
		}, true
	case protocolws.EventSubagentCompleted:
		content := summarizeSubagentEvent(payload)
		return runtime.RuntimeEvent{
			Type:    "message.created",
			Session: parseRuntimeSession(payload),
			Message: &session.Message{
				ID:        "subagent-complete-" + stringValue(payload, "run_id"),
				Role:      "system",
				Content:   content,
				CreatedAt: time.Now().UTC(),
			},
		}, true
	case protocolws.EventOrchestrationUpdated, protocolws.EventOrchestrationPlanStepUpdated:
		return runtime.RuntimeEvent{
			Type:    "message.created",
			Session: parseRuntimeSession(payload),
			Message: &session.Message{
				ID:        "orchestration-" + msg.Event + "-" + stringValue(payload, "run_id"),
				Role:      "system",
				Content:   summarizeOrchestrationEvent(msg.Event, payload),
				CreatedAt: time.Now().UTC(),
			},
		}, true
	case "run.error":
		return runtime.RuntimeEvent{
			Type:    "run.error",
			Session: parseRuntimeSession(payload),
			RunID:   stringValue(payload, "run_id"),
			Error:   stringValue(payload, "message"),
		}, true
	case "agent.lifecycle.start", "agent.lifecycle.end", "tool.called", "assistant.delta":
		return runtime.RuntimeEvent{
			Type:      msg.Event,
			Session:   parseRuntimeSession(payload),
			RunID:     stringValue(payload, "run_id"),
			ToolName:  stringValue(payload, "tool_name"),
			ToolUseID: stringValue(payload, "tool_use_id"),
			Delta:     stringValue(payload, "delta"),
		}, true
	default:
		return runtime.RuntimeEvent{}, false
	}
}

func parseRuntimeSession(payload map[string]any) session.Session {
	return session.Session{
		ID:      stringValue(payload, "session_id"),
		Key:     stringValue(payload, "session_key"),
		AgentID: stringValue(payload, "agent_id"),
		IsMain:  boolValue(payload, "is_main"),
	}
}

func parseSessionMessage(raw any) (session.Message, bool) {
	item, ok := raw.(map[string]any)
	if !ok {
		return session.Message{}, false
	}
	message := session.Message{
		ID:      stringValue(item, "id"),
		Role:    stringValue(item, "role"),
		Content: stringValue(item, "content"),
		Blocks:  parseMessageBlocks(item["blocks"]),
	}
	if createdAt := stringValue(item, "created_at"); strings.TrimSpace(createdAt) != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			message.CreatedAt = parsed
		}
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	return message, true
}

func parseMessageBlocks(raw any) []model.MessageBlock {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	blocks := make([]model.MessageBlock, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := model.MessageBlock{
			Type:        model.MessageBlockType(stringValue(entry, "type")),
			ID:          stringValue(entry, "id"),
			Text:        stringValue(entry, "text"),
			ToolUseID:   stringValue(entry, "tool_use_id"),
			Name:        stringValue(entry, "name"),
			Input:       stringValue(entry, "input"),
			InputObject: mapValue(entry, "input_object"),
			Content:     stringValue(entry, "content"),
			IsError:     boolValue(entry, "is_error"),
			Raw:         mapValue(entry, "raw"),
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func parseToolProgress(payload map[string]any) *tools.ToolProgress {
	progress := &tools.ToolProgress{
		ToolUseID: stringValue(payload, "tool_use_id"),
		Type:      stringValue(payload, "type"),
		Message:   stringValue(payload, "message"),
		Data:      mapValue(payload, "data"),
	}
	if progress.ToolUseID == "" && progress.Type == "" && progress.Message == "" && len(progress.Data) == 0 {
		return nil
	}
	return progress
}

func parseApprovalRequest(payload map[string]any) *approval.Request {
	id := stringValue(payload, "approval_id")
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &approval.Request{
		ID:              id,
		SessionID:       stringValue(payload, "session_id"),
		RunID:           stringValue(payload, "run_id"),
		ToolName:        stringValue(payload, "tool_name"),
		ToolInput:       stringValue(payload, "tool_input"),
		ToolInputObject: mapValue(payload, "tool_input_object"),
		Reason:          stringValue(payload, "reason"),
		DecisionReason:  stringValue(payload, "decision_reason"),
		AcceptFeedback:  stringValue(payload, "accept_feedback"),
		Status:          approval.Status(stringValue(payload, "status")),
	}
}

func summarizeSubagentEvent(payload map[string]any) string {
	label := stringValue(payload, "label")
	if label == "" {
		label = stringValue(payload, "run_id")
	}
	status := stringValue(payload, "status")
	message := stringValue(payload, "message")
	if message == "" {
		message = stringValue(payload, "output")
	}
	parts := []string{"Subagent", label}
	if status != "" {
		parts = append(parts, "["+status+"]")
	}
	if message != "" {
		parts = append(parts, "-", message)
	}
	return strings.Join(parts, " ")
}

func summarizeOrchestrationEvent(event string, payload map[string]any) string {
	parts := []string{"Orchestration", event}
	if runID := stringValue(payload, "run_id"); runID != "" {
		parts = append(parts, runID)
	}
	if status := stringValue(payload, "status"); status != "" {
		parts = append(parts, "["+status+"]")
	}
	if msg := stringValue(payload, "message"); msg != "" {
		parts = append(parts, "-", msg)
	}
	return strings.Join(parts, " ")
}

func stringValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func boolValue(payload map[string]any, key string) bool {
	value, _ := payload[key].(bool)
	return value
}

func mapValue(payload map[string]any, key string) map[string]any {
	value, _ := payload[key].(map[string]any)
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for k, v := range value {
		cloned[k] = v
	}
	return cloned
}

func decodePayloadMap(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed, nil
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return out, nil
}
