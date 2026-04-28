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

func TestManagerRestoresSubagentIsolationMetadata(t *testing.T) {
	manager := agent.NewManager()
	if err := manager.Restore(agent.Run{
		ID:                      "agent-000009",
		ParentSessionID:         "main-000001",
		ParentAgentID:           "main",
		ChildSessionID:          "child-000009",
		ChildSessionKey:         "agent:main:child:agent-000009",
		Label:                   "review",
		Prompt:                  "inspect state",
		Status:                  agent.StatusStopped,
		LastAction:              agent.ActionBackgrounded,
		Attempt:                 2,
		AllowedTools:            []string{"Read", "Grep"},
		RunInBackground:         true,
		Isolation:               "worktree",
		CWD:                     "C:/repo/.worktrees/child",
		RemoteIsolationBoundary: "remote:disabled",
		PermissionMode:          "ask",
		PermissionInherited:     true,
		ParentRunID:             "agent-000008",
		ContinuationMode:        "retry",
		OutputFile:              "C:/tmp/agent-000009.log",
	}); err != nil {
		t.Fatalf("restore: %v", err)
	}

	run, ok := manager.Get("agent-000009")
	if !ok {
		t.Fatal("restored run not found")
	}
	if !run.RunInBackground || run.Isolation != "worktree" || run.CWD != "C:/repo/.worktrees/child" {
		t.Fatalf("run = %#v, want restored isolation metadata", run)
	}
	if run.PermissionMode != "ask" || !run.PermissionInherited {
		t.Fatalf("run = %#v, want restored permission metadata", run)
	}
	if run.ParentRunID != "agent-000008" || run.ContinuationMode != "retry" {
		t.Fatalf("run = %#v, want restored retry metadata", run)
	}
}
