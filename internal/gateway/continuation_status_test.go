package gateway

import (
	"testing"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/llm"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/store/memory"
	"myclaw/internal/workspace"
)

func TestSessionStatusPayloadIncludesRecoveredContinuationState(t *testing.T) {
	store := memory.NewSessionStore()
	sessions := session.NewManager(store)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild("main", "agent:main:child:agent-000001")
	if err := sessions.UpdateMetadata(parent.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000001"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "system.run"
		metadata.PendingApprovalToolInput = "pwd"
		metadata.AgentRuns = []session.AgentRunMetadata{{
			ID:              "agent-000001",
			ParentSessionID: parent.ID,
			ParentAgentID:   "main",
			ChildSessionID:  child.ID,
			ChildSessionKey: child.Key,
			Label:           "research",
			Status:          string(agent.StatusRunning),
			LastAction:      string(agent.ActionSpawned),
			Attempt:         1,
		}}
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	reloaded := session.NewManager(store)
	runner := runtime.NewRunnerWithOptions(reloaded, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{})
	server := NewServerWithOptions(nil, reloaded, llm.NewMockClient(), Options{Runner: runner})
	payload := server.sessionStatusPayload(parent)

	continuation, _ := payload["continuation"].(map[string]any)
	if continuation["ready_for_prompt"] != false || continuation["status"] != string(session.ContinuationStatusAwaitingApproval) {
		t.Fatalf("continuation = %#v, want awaiting approval", continuation)
	}
	approvalPayload, _ := continuation["pending_approval"].(map[string]any)
	if approvalPayload["id"] != "approval-000001" || approvalPayload["tool_name"] != "system.run" {
		t.Fatalf("pending approval = %#v, want recovered approval", approvalPayload)
	}
	tasks, _ := continuation["tasks"].([]map[string]any)
	if len(tasks) != 1 || tasks[0]["run_id"] != "agent-000001" {
		t.Fatalf("tasks = %#v, want recovered task projection", tasks)
	}
}
