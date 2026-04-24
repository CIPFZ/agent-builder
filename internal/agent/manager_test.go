package agent_test

import (
	"context"
	"testing"
	"time"

	"myclaw/internal/agent"
)

func TestManagerSpawnAndWait(t *testing.T) {
	manager := agent.NewManager()

	run, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "research",
		Prompt:          "Investigate the failing test",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "investigation complete", nil
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.ChildSessionID == "" {
		t.Fatal("expected child session id to be assigned")
	}
	if run.ChildSessionKey == "" {
		t.Fatal("expected child session key to be assigned")
	}

	result, err := manager.Wait(context.Background(), run.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.Status != agent.StatusCompleted {
		t.Fatalf("result status = %q, want %q", result.Status, agent.StatusCompleted)
	}
	if result.LastAction != agent.ActionSpawned {
		t.Fatalf("last action = %q, want %q", result.LastAction, agent.ActionSpawned)
	}
	if result.CreatedAt.IsZero() || result.StartedAt.IsZero() || result.CompletedAt.IsZero() {
		t.Fatalf("result timestamps = %#v, want populated lifecycle timestamps", result)
	}
	if result.Output != "investigation complete" {
		t.Fatalf("result output = %q, want %q", result.Output, "investigation complete")
	}
}

func TestManagerListRunsIncludesActiveAndCompleted(t *testing.T) {
	manager := agent.NewManager()
	block := make(chan struct{})

	running, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "running",
		Prompt:          "keep running",
		Run: func(context.Context, agent.RunContext) (string, error) {
			<-block
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("spawn running: %v", err)
	}

	completed, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "completed",
		Prompt:          "finish fast",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("spawn completed: %v", err)
	}
	if _, err := manager.Wait(context.Background(), completed.ID, 2*time.Second); err != nil {
		t.Fatalf("wait completed: %v", err)
	}

	runs := manager.List()
	if len(runs) != 2 {
		t.Fatalf("run count = %d, want 2", len(runs))
	}
	active := manager.Active()
	if len(active) != 1 || active[0].ID != running.ID {
		t.Fatalf("active runs = %#v, want only running run", active)
	}

	close(block)
	if _, err := manager.Wait(context.Background(), running.ID, 2*time.Second); err != nil {
		t.Fatalf("wait running: %v", err)
	}
}

func TestManagerStopRunningAgent(t *testing.T) {
	manager := agent.NewManager()
	block := make(chan struct{})

	run, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "blocking",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-block:
				return "done", nil
			}
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if err := manager.Stop(run.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	result, err := manager.Wait(context.Background(), run.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.Status != agent.StatusStopped {
		t.Fatalf("status = %q, want %q", result.Status, agent.StatusStopped)
	}
	if result.LastAction != agent.ActionStopped {
		t.Fatalf("last action = %q, want %q", result.LastAction, agent.ActionStopped)
	}
	close(block)
}

func TestManagerSteerAppendsControlMessage(t *testing.T) {
	manager := agent.NewManager()
	block := make(chan struct{})

	run, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "blocking",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			<-block
			controls := manager.ControlMessages(runCtx.RunID)
			if len(controls) == 0 {
				return "missing controls", nil
			}
			return controls[len(controls)-1], nil
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if err := manager.Steer(run.ID, "switch to safer plan"); err != nil {
		t.Fatalf("steer: %v", err)
	}
	close(block)

	result, err := manager.Wait(context.Background(), run.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.Output != "switch to safer plan" {
		t.Fatalf("result output = %q, want steer message", result.Output)
	}
	if result.LastAction != agent.ActionSteered {
		t.Fatalf("last action = %q, want %q", result.LastAction, agent.ActionSteered)
	}
}

func TestManagerGetRunByID(t *testing.T) {
	manager := agent.NewManager()
	run, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "quick",
		Prompt:          "finish",
		Run:             func(context.Context, agent.RunContext) (string, error) { return "ok", nil },
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	got, ok := manager.Get(run.ID)
	if !ok {
		t.Fatal("expected run to be retrievable")
	}
	if got.ChildSessionKey != run.ChildSessionKey {
		t.Fatalf("child session key = %q, want %q", got.ChildSessionKey, run.ChildSessionKey)
	}
}

func TestManagerResumeReusesStableRunID(t *testing.T) {
	manager := agent.NewManager()

	run, err := manager.Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		ChildSessionID:  "child-1",
		ChildSessionKey: "agent:main:child:1",
		Label:           "research",
		Prompt:          "first pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "first result", nil
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := manager.Wait(context.Background(), run.ID, 2*time.Second); err != nil {
		t.Fatalf("wait first: %v", err)
	}

	resumed, err := manager.Resume(context.Background(), run.ID, agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		ChildSessionID:  "child-1",
		ChildSessionKey: "agent:main:child:1",
		Label:           "research",
		Prompt:          "second pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "second result", nil
		},
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.ID != run.ID {
		t.Fatalf("resume id = %q, want stable id %q", resumed.ID, run.ID)
	}

	result, err := manager.Wait(context.Background(), run.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait resumed: %v", err)
	}
	if result.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", result.Attempt)
	}
	if result.LastAction != agent.ActionResumed {
		t.Fatalf("last action = %q, want %q", result.LastAction, agent.ActionResumed)
	}
	if result.Output != "second result" {
		t.Fatalf("output = %q, want resumed output", result.Output)
	}
}
