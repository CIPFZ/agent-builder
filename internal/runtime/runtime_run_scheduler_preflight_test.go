package runtime

import (
	"context"
	"testing"
)

func TestRuntimeRunSchedulerPreflightRequiresDurableRunTurnLink(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-scheduler-preflight",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	missingRun, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{SessionID: "session-1", TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if missingRun.CanSchedule || missingRun.Reason != runtimeRunSchedulerPreflightReasonMissingRun {
		t.Fatalf("missing run preflight = %#v", missingRun)
	}

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	missingLink, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{RunID: run.ID, SessionID: "session-1", TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if missingLink.CanSchedule || missingLink.Reason != runtimeRunSchedulerPreflightReasonMissingTurnLink {
		t.Fatalf("missing turn link preflight = %#v", missingLink)
	}

	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	allowed, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{RunID: run.ID, SessionID: "session-1", TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.CanSchedule || allowed.Reason != "" || allowed.RunID != run.ID || allowed.SessionID != "session-1" || allowed.TurnID != turn.ID {
		t.Fatalf("allowed preflight = %#v", allowed)
	}
	if allowed.Source.Kind != runtimeRunSchedulerPreflightSourceKind || !allowed.Source.ReadOnly || allowed.Source.StartsWorker {
		t.Fatalf("preflight source = %#v", allowed.Source)
	}
}

func TestRuntimeRunSchedulerPreflightRejectsMismatchedOrTerminalEvidence(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-terminal",
		SessionID:  "session-1",
		Status:     turnStatusCompleted,
		StartedAt:  1000,
		FinishedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}

	mismatch, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{RunID: run.ID, SessionID: "session-2", TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.CanSchedule || mismatch.Reason != runtimeRunSchedulerPreflightReasonSessionMismatch {
		t.Fatalf("session mismatch preflight = %#v", mismatch)
	}

	otherRun, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-other", "other report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	runMismatch, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{RunID: otherRun.ID, TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if runMismatch.CanSchedule || runMismatch.Reason != runtimeRunSchedulerPreflightReasonRunSessionMismatch {
		t.Fatalf("run session mismatch preflight = %#v", runMismatch)
	}

	terminal, err := service.runtimeRunSchedulerPreflight(context.Background(), RuntimeRunSchedulerPreflightRequest{RunID: run.ID, TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.CanSchedule || terminal.Reason != runtimeRunSchedulerPreflightReasonTerminalTurn {
		t.Fatalf("terminal turn preflight = %#v", terminal)
	}
}
