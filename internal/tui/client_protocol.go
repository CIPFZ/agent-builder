package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

type clientEvent struct {
	Type    string
	Session clientSessionRef
	Message *clientMessage
	Tool    *clientToolEvent
	Error   string
}

type clientSessionRef struct {
	ID      string
	Key     string
	AgentID string
	IsMain  bool
}

type clientMessage struct {
	ID        string
	Role      string
	Content   string
	Blocks    []clientMessageBlock
	CreatedAt string
}

type clientMessageBlock struct {
	Type        string
	ID          string
	ToolUseID   string
	Text        string
	Name        string
	Input       string
	InputObject map[string]any
	Content     string
	IsError     bool
}

type clientToolEvent struct {
	RunID            string
	ToolName         string
	ToolUseID        string
	ToolInput        string
	ToolInputObject  map[string]any
	ProgressType     string
	ProgressMessage  string
	ProgressData     map[string]any
	Approval         *clientApproval
	SubagentUpdate   *clientTaskUpdate
	OrchestrationRun *clientTaskUpdate
}

type clientApproval struct {
	ID              string
	SessionID       string
	RunID           string
	ToolName        string
	ToolInput       string
	ToolInputObject map[string]any
	Category        string
	RuleSource      string
	Reason          string
	DecisionReason  string
	AcceptFeedback  string
	Status          string
}

type clientTaskUpdate struct {
	RunID             string
	Label             string
	Status            string
	ParentSessionID   string
	ChildSessionID    string
	ChildSessionKey   string
	Output            string
	Error             string
	LastAction        string
	LastEvent         string
	Message           string
	NextAction        string
	RecommendedRole   string
	RecommendedAction string
	DecisionPriority  string
	DecisionReason    string
	AutoExecutable    bool
}

func parseClientEventMessage(msg wsMessageLike) (clientEvent, bool) {
	if msg.Type != "event" {
		return clientEvent{}, false
	}
	payload := msg.Payload
	event := clientEvent{
		Type:    msg.Event,
		Session: parseClientSession(payload),
	}
	switch msg.Event {
	case "assistant.delta":
		delta := stringValue(payload, "delta")
		event.Message = &clientMessage{Role: "assistant", Content: delta}
		event.Tool = &clientToolEvent{ProgressMessage: delta}
	case "message.created":
		message, ok := parseClientMessage(payload["message"])
		if !ok {
			return clientEvent{}, false
		}
		event.Message = &message
	case "tool.called":
		event.Tool = &clientToolEvent{
			RunID:           stringValue(payload, "run_id"),
			ToolName:        stringValue(payload, "tool_name"),
			ToolUseID:       stringValue(payload, "tool_use_id"),
			ToolInput:       stringValue(payload, "tool_input"),
			ToolInputObject: mapValue(payload, "tool_input_object"),
		}
	case "tool.progress":
		event.Tool = &clientToolEvent{
			RunID:           stringValue(payload, "run_id"),
			ToolName:        stringValue(payload, "tool_name"),
			ToolUseID:       stringValue(payload, "tool_use_id"),
			ProgressType:    stringValue(payload, "type"),
			ProgressMessage: stringValue(payload, "message"),
			ProgressData:    mapValue(payload, "data"),
		}
	case "tool.result":
		message, _ := parseClientMessage(payload["message"])
		event.Message = &message
		event.Tool = &clientToolEvent{
			RunID:           stringValue(payload, "run_id"),
			ToolName:        stringValue(payload, "tool_name"),
			ToolUseID:       stringValue(payload, "tool_use_id"),
			ToolInput:       stringValue(payload, "tool_input"),
			ToolInputObject: mapValue(payload, "tool_input_object"),
		}
	case "permission.required", "approval.updated":
		event.Tool = &clientToolEvent{
			RunID:    stringValue(payload, "run_id"),
			ToolName: stringValue(payload, "tool_name"),
			Approval: parseClientApproval(payload),
		}
	case "subagent.updated", "subagent.completed":
		event.Tool = &clientToolEvent{
			SubagentUpdate: parseClientTaskUpdate(payload),
		}
	case "orchestration.updated", "orchestration.plan_step.updated":
		event.Tool = &clientToolEvent{
			OrchestrationRun: parseClientTaskUpdate(payload),
		}
	case "run.error":
		event.Error = stringValue(payload, "message")
	default:
	}
	return event, true
}

type wsMessageLike struct {
	Type    string
	Event   string
	Payload map[string]any
}

func parseClientSession(payload map[string]any) clientSessionRef {
	return clientSessionRef{
		ID:      stringValue(payload, "session_id"),
		Key:     stringValue(payload, "session_key"),
		AgentID: stringValue(payload, "agent_id"),
		IsMain:  boolValue(payload, "is_main"),
	}
}

func parseClientMessage(raw any) (clientMessage, bool) {
	item, ok := raw.(map[string]any)
	if !ok {
		return clientMessage{}, false
	}
	return clientMessage{
		ID:        stringValue(item, "id"),
		Role:      stringValue(item, "role"),
		Content:   stringValue(item, "content"),
		Blocks:    parseClientMessageBlocks(item["blocks"]),
		CreatedAt: stringValue(item, "created_at"),
	}, true
}

func parseClientMessageBlocks(raw any) []clientMessageBlock {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	blocks := make([]clientMessageBlock, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blocks = append(blocks, clientMessageBlock{
			Type:        stringValue(entry, "type"),
			ID:          stringValue(entry, "id"),
			ToolUseID:   stringValue(entry, "tool_use_id"),
			Text:        stringValue(entry, "text"),
			Name:        stringValue(entry, "name"),
			Input:       stringValue(entry, "input"),
			InputObject: mapValue(entry, "input_object"),
			Content:     stringValue(entry, "content"),
			IsError:     boolValue(entry, "is_error"),
		})
	}
	return blocks
}

func parseClientApproval(payload map[string]any) *clientApproval {
	id := stringValue(payload, "approval_id")
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &clientApproval{
		ID:              id,
		SessionID:       stringValue(payload, "session_id"),
		RunID:           stringValue(payload, "run_id"),
		ToolName:        stringValue(payload, "tool_name"),
		ToolInput:       stringValue(payload, "tool_input"),
		ToolInputObject: mapValue(payload, "tool_input_object"),
		Category:        stringValue(payload, "category"),
		RuleSource:      stringValue(payload, "rule_source"),
		Reason:          stringValue(payload, "reason"),
		DecisionReason:  stringValue(payload, "decision_reason"),
		AcceptFeedback:  stringValue(payload, "accept_feedback"),
		Status:          stringValue(payload, "status"),
	}
}

func parseClientTaskUpdate(payload map[string]any) *clientTaskUpdate {
	runID := stringValue(payload, "run_id")
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	return &clientTaskUpdate{
		RunID:             runID,
		Label:             stringValue(payload, "label"),
		Status:            stringValue(payload, "status"),
		ParentSessionID:   stringValue(payload, "parent_session_id"),
		ChildSessionID:    stringValue(payload, "child_session_id"),
		ChildSessionKey:   stringValue(payload, "child_session_key"),
		Output:            stringValue(payload, "output"),
		Error:             stringValue(payload, "error"),
		LastAction:        stringValue(payload, "last_action"),
		LastEvent:         stringValue(payload, "last_event"),
		Message:           stringValue(payload, "message"),
		NextAction:        stringValue(payload, "next_action"),
		RecommendedRole:   stringValue(payload, "recommended_role"),
		RecommendedAction: stringValue(payload, "recommended_action"),
		DecisionPriority:  stringValue(payload, "decision_priority"),
		DecisionReason:    stringValue(payload, "decision_reason"),
		AutoExecutable:    boolValue(payload, "auto_executable"),
	}
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
