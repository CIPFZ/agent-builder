package runtime

import (
	"context"
	"testing"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/llm"
	"myclaw/internal/session"
	"myclaw/internal/store/memory"
	"myclaw/internal/workspace"
)

func TestRunnerContinuationSnapshotRecoversPendingApprovalAfterRestart(t *testing.T) {
	store := memory.NewSessionStore()
	sessions := session.NewManager(store)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000001"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "system.run"
		metadata.PendingApprovalToolInput = "pwd"
		metadata.PendingApprovalToolUseID = "toolu-1"
		metadata.PendingApprovalRunID = "run-1"
		metadata.PendingApprovalUserMessageID = "msg-1"
		metadata.PendingApprovalReason = "requires approval"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	reloadedSessions := session.NewManager(store)
	runner := NewRunnerWithOptions(reloadedSessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{})

	snapshot, err := runner.ContinuationSnapshot(sess.ID)
	if err != nil {
		t.Fatalf("ContinuationSnapshot: %v", err)
	}
	if snapshot.ReadyForPrompt {
		t.Fatalf("ReadyForPrompt = true, want false while approval is pending")
	}
	if snapshot.Status != session.ContinuationStatusAwaitingApproval {
		t.Fatalf("status = %q, want awaiting approval", snapshot.Status)
	}
	if snapshot.PendingApproval == nil || snapshot.PendingApproval.ID != "approval-000001" {
		t.Fatalf("pending approval = %#v, want recovered approval", snapshot.PendingApproval)
	}
}

func TestRunnerPersistsFastCompletedSubagentInsteadOfStaleRunningState(t *testing.T) {
	store := memory.NewSessionStore()
	sessions := session.NewManager(store)
	parent := sessions.GetOrCreateMain("main")
	agentManager := agent.NewManager()
	NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		AgentManager: agentManager,
	})

	run, err := agentManager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		Label:           "fast",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "fast done", nil
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := agentManager.Wait(context.Background(), run.ID, time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	var reloaded session.Session
	var recovered session.AgentRunMetadata
	for time.Now().Before(deadline) {
		var ok bool
		reloaded, ok = sessions.GetByID(parent.ID)
		if !ok {
			t.Fatalf("parent session %q not found", parent.ID)
		}
		recovered = session.AgentRunMetadata{}
		for _, item := range reloaded.Metadata.AgentRuns {
			if item.ID == run.ID {
				recovered = item
				break
			}
		}
		if recovered.Status == string(agent.StatusCompleted) && recovered.Output == "fast done" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if recovered.ID == "" {
		t.Fatalf("agent runs = %#v, want persisted run %q", reloaded.Metadata.AgentRuns, run.ID)
	}
	t.Fatalf("persisted run = %#v, want completed output", recovered)
}

func TestRunnerContinuationSnapshotRecoversVisibleSubagentStateAfterRestart(t *testing.T) {
	store := memory.NewSessionStore()
	sessions := session.NewManager(store)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild("main", "agent:main:child:agent-000001")
	if err := sessions.UpdateMetadata(parent.ID, func(metadata *session.SessionMetadata) {
		metadata.AgentRuns = []session.AgentRunMetadata{{
			ID:              "agent-000001",
			ParentSessionID: parent.ID,
			ParentAgentID:   "main",
			ChildSessionID:  child.ID,
			ChildSessionKey: child.Key,
			Label:           "research",
			Prompt:          "inspect runtime state",
			Status:          string(agent.StatusCompleted),
			LastAction:      string(agent.ActionSpawned),
			Attempt:         1,
			Output:          "done",
		}}
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	reloadedSessions := session.NewManager(store)
	runner := NewRunnerWithOptions(reloadedSessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{})

	snapshot, err := runner.ContinuationSnapshot(parent.ID)
	if err != nil {
		t.Fatalf("ContinuationSnapshot: %v", err)
	}
	if len(snapshot.Tasks) != 1 {
		t.Fatalf("tasks = %#v, want one recovered task", snapshot.Tasks)
	}
	if snapshot.Tasks[0].RunID != "agent-000001" || snapshot.Tasks[0].Status != string(agent.StatusCompleted) || snapshot.Tasks[0].Output != "done" {
		t.Fatalf("task = %#v, want recovered visible task state", snapshot.Tasks[0])
	}
}

func TestRunnerContinuationSnapshotConservativeWhenApprovalMetadataIsCorrupt(t *testing.T) {
	store := memory.NewSessionStore()
	sessions := session.NewManager(store)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-corrupt"
		metadata.PendingApprovalStatus = "lost"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{})

	snapshot, err := runner.ContinuationSnapshot(sess.ID)
	if err != nil {
		t.Fatalf("ContinuationSnapshot should return conservative state, got error: %v", err)
	}
	if snapshot.ReadyForPrompt || snapshot.RecoveryError == "" {
		t.Fatalf("snapshot = %#v, want conservative not-ready recovery error", snapshot)
	}
}
