package runtimeapi

import (
	"slices"
	"testing"
	"time"
)

func TestEndpointsFreezePhase2MinimalAPI(t *testing.T) {
	t.Parallel()

	expected := []Endpoint{
		{Method: MethodGet, Path: "/v1/runtime/status"},
		{Method: MethodGet, Path: "/v1/recovery/status"},
		{Method: MethodGet, Path: "/v1/config/model"},
		{Method: MethodPut, Path: "/v1/config/model"},
		{Method: MethodPost, Path: "/v1/config/model/verify"},
		{Method: MethodGet, Path: "/v1/config/models"},
		{Method: MethodGet, Path: "/v1/sessions"},
		{Method: MethodPost, Path: "/v1/sessions"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/messages"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/todos"},
		{Method: MethodPost, Path: "/v1/sessions/{session_id}/turns"},
		{Method: MethodGet, Path: "/v1/turns"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/todos"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/tool-calls"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/tasks"},
		{Method: MethodGet, Path: "/v1/tool-calls/{tool_call_id}"},
		{Method: MethodGet, Path: "/v1/tasks/{task_id}"},
		{Method: MethodPost, Path: "/v1/tasks/{task_id}/cancel"},
		{Method: MethodPost, Path: "/v1/turns/{turn_id}/cancel"},
		{Method: MethodGet, Path: "/v1/permissions"},
		{Method: MethodPost, Path: "/v1/permissions/{permission_id}/decision"},
		{Method: MethodGet, Path: "/v1/policy"},
		{Method: MethodPut, Path: "/v1/policy"},
		{Method: MethodGet, Path: "/v1/capabilities"},
		{Method: MethodPost, Path: "/v1/capabilities/{capability_id}/refresh"},
		{Method: MethodGet, Path: "/v1/context/sources"},
		{Method: MethodGet, Path: "/v1/skills"},
		{Method: MethodPost, Path: "/v1/skills"},
		{Method: MethodPost, Path: "/v1/skills/refresh"},
		{Method: MethodPost, Path: "/v1/skills/paths"},
		{Method: MethodGet, Path: "/v1/mcp/servers"},
		{Method: MethodPut, Path: "/v1/mcp/servers/{server_name}"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/enabled"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/refresh"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/tools"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/tools/{tool_name}/enabled"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/resources"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/prompts"},
		{Method: MethodGet, Path: "/v1/audit/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/audit/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/events"},
	}

	if len(Endpoints) != len(expected) {
		t.Fatalf("Endpoints len = %d, want %d", len(Endpoints), len(expected))
	}
	for _, endpoint := range expected {
		if !slices.Contains(Endpoints, endpoint) {
			t.Fatalf("Endpoints missing %#v", endpoint)
		}
	}
}

func TestEventTypesFreezePhase2Schema(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		EventRuntimeStarted,
		EventSessionCreated,
		EventTurnStarted,
		EventMessageCreated,
		EventToolCallStarted,
		EventTaskProgress,
		EventPermissionRequested,
		EventPermissionPolicyApplied,
		EventTodoUpdated,
		EventCapabilityLoading,
		EventCapabilityLoaded,
		EventCapabilityFailed,
		EventSkillDiscoveryCompleted,
		EventMCPServerConnected,
		EventUsageUpdated,
		EventAuditRecorded,
	} {
		if !IsEventType(eventType) {
			t.Fatalf("IsEventType(%q) = false", eventType)
		}
	}
	if IsEventType("message") {
		t.Fatal("legacy event type should not be part of the Phase 2 schema")
	}
}

func TestEventValidateRequiresStableEnvelope(t *testing.T) {
	t.Parallel()

	event := NewEvent("event-1", EventMessageCreated, time.Date(2026, 5, 18, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	event.SessionID = "session-1"
	event.MessageID = "message-1"
	event.Payload = map[string]any{"role": "assistant"}

	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}

	event.Type = "message"
	if err := event.Validate(); err == nil {
		t.Fatal("Validate accepted legacy event type")
	}
}
