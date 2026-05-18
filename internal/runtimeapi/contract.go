package runtimeapi

import (
	"errors"
	"fmt"
	"time"
)

const Version = "v1"

const (
	MethodGet  = "GET"
	MethodPost = "POST"
	MethodPut  = "PUT"
)

type Endpoint struct {
	Method string
	Path   string
}

var Endpoints = []Endpoint{
	{Method: MethodGet, Path: "/v1/runtime/status"},
	{Method: MethodGet, Path: "/v1/config/model"},
	{Method: MethodPut, Path: "/v1/config/model"},
	{Method: MethodPost, Path: "/v1/config/model/verify"},
	{Method: MethodGet, Path: "/v1/sessions"},
	{Method: MethodPost, Path: "/v1/sessions"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}/messages"},
	{Method: MethodPost, Path: "/v1/sessions/{session_id}/turns"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}"},
	{Method: MethodPost, Path: "/v1/turns/{turn_id}/cancel"},
	{Method: MethodGet, Path: "/v1/permissions"},
	{Method: MethodPost, Path: "/v1/permissions/{permission_id}/decision"},
	{Method: MethodGet, Path: "/v1/capabilities"},
	{Method: MethodGet, Path: "/v1/skills"},
	{Method: MethodPost, Path: "/v1/skills/refresh"},
	{Method: MethodGet, Path: "/v1/mcp/servers"},
	{Method: MethodPut, Path: "/v1/mcp/servers/{server_name}"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/enabled"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/refresh"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/tools"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/tools/{tool_name}/enabled"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/resources"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/prompts"},
	{Method: MethodGet, Path: "/v1/audit/turns/{turn_id}"},
	{Method: MethodGet, Path: "/v1/events"},
}

const (
	EventRuntimeStarted          = "runtime.started"
	EventRuntimeFailed           = "runtime.failed"
	EventSessionCreated          = "session.created"
	EventSessionUpdated          = "session.updated"
	EventSessionDeleted          = "session.deleted"
	EventTurnStarted             = "turn.started"
	EventTurnProgress            = "turn.progress"
	EventTurnCompleted           = "turn.completed"
	EventTurnFailed              = "turn.failed"
	EventTurnCancelled           = "turn.cancelled"
	EventMessageCreated          = "message.created"
	EventMessageUpdated          = "message.updated"
	EventMessageCompleted        = "message.completed"
	EventToolCallStarted         = "tool.call.started"
	EventToolCallOutput          = "tool.call.output"
	EventToolCallCompleted       = "tool.call.completed"
	EventToolCallFailed          = "tool.call.failed"
	EventPermissionRequested     = "permission.requested"
	EventPermissionDecided       = "permission.decided"
	EventSkillDiscoveryStarted   = "skill.discovery.started"
	EventSkillDiscoveryCompleted = "skill.discovery.completed"
	EventSkillDiscoveryFailed    = "skill.discovery.failed"
	EventSkillEnabled            = "skill.enabled"
	EventSkillDisabled           = "skill.disabled"
	EventMCPServerStarting       = "mcp.server.starting"
	EventMCPServerConnected      = "mcp.server.connected"
	EventMCPServerFailed         = "mcp.server.failed"
	EventMCPServerDisabled       = "mcp.server.disabled"
	EventMCPToolsUpdated         = "mcp.tools.updated"
	EventMCPResourcesUpdated     = "mcp.resources.updated"
	EventMCPPromptsUpdated       = "mcp.prompts.updated"
	EventUsageUpdated            = "usage.updated"
	EventAuditRecorded           = "audit.recorded"
)

var EventTypes = []string{
	EventRuntimeStarted,
	EventRuntimeFailed,
	EventSessionCreated,
	EventSessionUpdated,
	EventSessionDeleted,
	EventTurnStarted,
	EventTurnProgress,
	EventTurnCompleted,
	EventTurnFailed,
	EventTurnCancelled,
	EventMessageCreated,
	EventMessageUpdated,
	EventMessageCompleted,
	EventToolCallStarted,
	EventToolCallOutput,
	EventToolCallCompleted,
	EventToolCallFailed,
	EventPermissionRequested,
	EventPermissionDecided,
	EventSkillDiscoveryStarted,
	EventSkillDiscoveryCompleted,
	EventSkillDiscoveryFailed,
	EventSkillEnabled,
	EventSkillDisabled,
	EventMCPServerStarting,
	EventMCPServerConnected,
	EventMCPServerFailed,
	EventMCPServerDisabled,
	EventMCPToolsUpdated,
	EventMCPResourcesUpdated,
	EventMCPPromptsUpdated,
	EventUsageUpdated,
	EventAuditRecorded,
}

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	CreatedAt  string         `json:"created_at"`
	SessionID  string         `json:"session_id,omitempty"`
	TurnID     string         `json:"turn_id,omitempty"`
	MessageID  string         `json:"message_id,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

func NewEvent(id, eventType string, createdAt time.Time) Event {
	return Event{
		ID:        id,
		Type:      eventType,
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
	}
}

func (e Event) Validate() error {
	if e.ID == "" {
		return errors.New("runtime event id is required")
	}
	if e.Type == "" {
		return errors.New("runtime event type is required")
	}
	if !IsEventType(e.Type) {
		return fmt.Errorf("unknown runtime event type %q", e.Type)
	}
	if e.CreatedAt == "" {
		return errors.New("runtime event created_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.CreatedAt); err != nil {
		return fmt.Errorf("runtime event created_at must be RFC3339: %w", err)
	}
	return nil
}

func IsEventType(eventType string) bool {
	for _, candidate := range EventTypes {
		if eventType == candidate {
			return true
		}
	}
	return false
}
