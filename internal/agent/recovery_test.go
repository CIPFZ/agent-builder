package agent_test

import (
	"testing"

	"myclaw/internal/agent"
)

func TestManagerRestoresVisibleRunsForRecoveredRuntimeState(t *testing.T) {
	manager := agent.NewManager()
	if err := manager.Restore(agent.Run{
		ID:              "agent-000007",
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		ChildSessionID:  "child-000001",
		ChildSessionKey: "agent:main:child:agent-000007",
		Label:           "review",
		Prompt:          "inspect state",
		Status:          agent.StatusCompleted,
		LastAction:      agent.ActionSpawned,
		Attempt:         1,
		Output:          "done",
	}); err != nil {
		t.Fatalf("restore: %v", err)
	}

	run, ok := manager.Get("agent-000007")
	if !ok {
		t.Fatal("restored run not found")
	}
	if run.Status != agent.StatusCompleted || run.Output != "done" {
		t.Fatalf("run = %#v, want restored visible state", run)
	}
}
