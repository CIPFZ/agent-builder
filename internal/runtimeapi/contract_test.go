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
		{Method: MethodPost, Path: "/v1/recovery/turns/{turn_id}/resume"},
		{Method: MethodPost, Path: "/v1/recovery/turns/{turn_id}/discard"},
		{Method: MethodPost, Path: "/v1/recovery/errors/{error_id}/retry"},
		{Method: MethodGet, Path: "/v1/config/model"},
		{Method: MethodPut, Path: "/v1/config/model"},
		{Method: MethodPost, Path: "/v1/config/model/verify"},
		{Method: MethodGet, Path: "/v1/config/models"},
		{Method: MethodGet, Path: "/v1/projects/{project_id}/memory"},
		{Method: MethodPost, Path: "/v1/projects/{project_id}/memory"},
		{Method: MethodPost, Path: "/v1/projects/{project_id}/memory/refresh"},
		{Method: MethodGet, Path: "/v1/projects/{project_id}/memory/diagnostics"},
		{Method: MethodGet, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPut, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPatch, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPost, Path: "/v1/memory/{memory_id}/disable"},
		{Method: MethodDelete, Path: "/v1/memory/{memory_id}"},
		{Method: MethodGet, Path: "/v1/sessions"},
		{Method: MethodPost, Path: "/v1/sessions"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/messages"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/activity-window"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/run-projection"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/todos"},
		{Method: MethodPost, Path: "/v1/sessions/{session_id}/turns"},
		{Method: MethodGet, Path: "/v1/turns"},
		{Method: MethodGet, Path: "/v1/runs"},
		{Method: MethodGet, Path: "/v1/runs/{run_id}"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/acknowledge"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/discard"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/activity"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/todos"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/tool-calls"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/compact"},
		{Method: MethodGet, Path: "/v1/hooks"},
		{Method: MethodGet, Path: "/v1/hook-executions"},
		{Method: MethodGet, Path: "/v1/hook-executions/{execution_id}"},
		{Method: MethodGet, Path: "/v1/sandbox/decisions"},
		{Method: MethodGet, Path: "/v1/sandbox/decisions/{decision_id}"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/agent-tasks"},
		{Method: MethodGet, Path: "/v1/tool-calls/{tool_call_id}"},
		{Method: MethodGet, Path: "/v1/refs"},
		{Method: MethodGet, Path: "/v1/refs/{ref_id}"},
		{Method: MethodGet, Path: "/v1/refs/{ref_id}/content"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/agent-tasks"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/messages"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/messages"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/result"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/output"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/cancel"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/follow-up"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/effective-scope"},
		{Method: MethodPost, Path: "/v1/turns/{turn_id}/cancel"},
		{Method: MethodPost, Path: "/v1/turns/{turn_id}/interrupted/done"},
		{Method: MethodGet, Path: "/v1/worktrees"},
		{Method: MethodPost, Path: "/v1/worktrees"},
		{Method: MethodGet, Path: "/v1/worktrees/{worktree_id}"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/enter"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/exit"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/cleanup"},
		{Method: MethodGet, Path: "/v1/permissions"},
		{Method: MethodPost, Path: "/v1/permissions/{permission_id}/decision"},
		{Method: MethodGet, Path: "/v1/policy"},
		{Method: MethodPut, Path: "/v1/policy"},
		{Method: MethodGet, Path: "/v1/capabilities"},
		{Method: MethodPost, Path: "/v1/capabilities/{capability_id}/refresh"},
		{Method: MethodPost, Path: "/v1/tools/search"},
		{Method: MethodGet, Path: "/v1/context/sources"},
		{Method: MethodGet, Path: "/v1/read-files"},
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
		{Method: MethodGet, Path: "/v1/mcp/requests"},
		{Method: MethodGet, Path: "/v1/mcp/requests/{request_id}"},
		{Method: MethodPost, Path: "/v1/mcp/requests/{request_id}/decision"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/retry"},
		{Method: MethodGet, Path: "/v1/audit/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/audit/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/compact"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output/events"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output/stream"},
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
		EventRecoveryStatusChanged,
		EventRecoveryTurnResumed,
		EventRecoveryTurnDiscarded,
		EventRecoveryErrorClassified,
		EventRecoveryRetryStarted,
		EventRecoveryRetryCompleted,
		EventRecoveryRetryFailed,
		EventRecoveryHistoryHygieneApplied,
		EventRecoveryCompactRetryStarted,
		EventRecoveryCompactRetryCompleted,
		EventRecoveryCompactRetryFailed,
		EventSessionCreated,
		EventTurnStarted,
		EventMessageCreated,
		EventToolCallStarted,
		EventOutputRefCreated,
		EventArtifactRefCreated,
		EventToolOutputRefCreated,
		EventToolSearchPerformed,
		EventToolDiscoverySelected,
		EventToolDiscoveryOmitted,
		EventSchedulerDeadlockPrevented,
		EventTaskProgress,
		EventWorktreeCreated,
		EventWorktreeEntered,
		EventWorktreeExited,
		EventWorktreeCleaned,
		EventWorktreeCleanupFailed,
		EventWorktreePolicyDenied,
		EventPermissionRequested,
		EventPermissionPolicyApplied,
		EventPolicyRuleMatched,
		EventPolicyRuleDenied,
		EventPolicyRuleAsk,
		EventShellPolicyClassified,
		EventHookDiscovered,
		EventHookConfigured,
		EventHookExecutionStarted,
		EventHookExecutionCompleted,
		EventHookExecutionSkipped,
		EventHookExecutionBlocked,
		EventHookExecutionFailed,
		EventHookContextInjected,
		EventHookInputRewritten,
		EventTodoUpdated,
		EventCapabilityLoading,
		EventCapabilityLoaded,
		EventCapabilityFailed,
		EventBudgetUpdated,
		EventCompactBoundaryRecorded,
		EventCompactMicroCompleted,
		EventCompactOutputPreserved,
		EventSkillDiscoveryCompleted,
		EventMCPServerConnected,
		EventMCPServerBlocked,
		EventMCPAuthRequested,
		EventMCPAuthCompleted,
		EventMCPAuthDenied,
		EventMCPAuthFailed,
		EventMCPElicitationRequested,
		EventMCPElicitationCompleted,
		EventMCPElicitationDenied,
		EventMCPElicitationFailed,
		EventUsageUpdated,
		EventAuditRecorded,
		EventMemoryIndexStarted,
		EventMemoryIndexCompleted,
		EventMemoryIndexFailed,
		EventMemoryRecordCreated,
		EventMemoryRecordUpdated,
		EventMemoryRecordDisabled,
		EventMemoryRecordDeleted,
		EventMemoryRecordInjected,
		EventMemoryRecordSkipped,
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

func TestOutputTextDeltaIsEphemeral(t *testing.T) {
	t.Parallel()

	if !IsEventType(EventOutputTextDelta) {
		t.Fatal("EventOutputTextDelta must be registered as an event type")
	}
	if !IsEphemeralEventType(EventOutputTextDelta) {
		t.Fatal("output.text.delta must be classified as ephemeral")
	}
	// Sanity: none of the load-bearing persisted types leak into ephemeral.
	for _, persisted := range []string{EventMessageCreated, EventMessageUpdated, EventMessageCompleted, EventToolCallStarted, EventToolCallCompleted, EventTurnStarted, EventTurnCompleted} {
		if IsEphemeralEventType(persisted) {
			t.Fatalf("%q must not be classified ephemeral", persisted)
		}
	}
}
