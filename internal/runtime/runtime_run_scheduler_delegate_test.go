package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestRuntimeRunSchedulerDelegateAllowsLinkedUserTurn(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	plan, err := service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, turn)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Source.StartsWorker || len(plan.Plan.Items) != 1 || !plan.Plan.Items[0].CanSchedule {
		t.Fatalf("delegate plan = %#v", plan)
	}
}

func TestRuntimeRunSchedulerDelegateRejectsAndTerminalizesBeforeStartedTransition(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-rejected",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, turn)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("delegate error = %v", err)
	}
	failed, err := service.failRuntimeRunScheduledTurn(context.Background(), turn, err.Error())
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != turnStatusFailed || failed.FinishedAt == 0 || !strings.Contains(failed.Error, runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("failed turn = %#v", failed)
	}
	service.mu.Lock()
	state := service.requests[failed.ID]
	service.mu.Unlock()
	if !state.Finished || state.Status != "failed" || !strings.Contains(state.Error, runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("request state = %#v", state)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("turn_started transition recorded for rejected delegate: %#v", transitions)
	}
}

func TestRuntimeRunSchedulerDelegateAcceptsExplicitCheckpointResumeTurnOnly(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.Upsert(context.Background(), RuntimeRun{
		ID:               "run-resume-delegate",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Objective:        "resume work",
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-source:interrupted",
			TurnID:       "turn-source",
			Status:       turnStatusInterrupted,
			Summary:      "runtime restarted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    2000,
		}},
		CreatedAt: 1000,
		UpdatedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpointPlan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:        run.ID,
		CheckpointID: "turn:turn-source:interrupted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpointPlan.Plan.Items) != 1 || checkpointPlan.Plan.Items[0].CanSchedule || checkpointPlan.Plan.Items[0].PreflightReason != runtimeRunSchedulerPlanReasonCheckpointRequiresTurn {
		t.Fatalf("checkpoint plan = %#v", checkpointPlan.Plan.Items)
	}

	resumed, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-resumed-explicit",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", resumed.ID, resumed.StartedAt); err != nil {
		t.Fatal(err)
	}
	delegatePlan, err := service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, resumed)
	if err != nil {
		t.Fatal(err)
	}
	if len(delegatePlan.Plan.Items) != 1 || !delegatePlan.Plan.Items[0].CanSchedule {
		t.Fatalf("delegate plan = %#v", delegatePlan.Plan.Items)
	}
	if _, err := service.runs.LinkCheckpointResume(context.Background(), run.ID, "turn:turn-source:interrupted", resumed.ID); err != nil {
		t.Fatal(err)
	}
	service.recordCheckpointResumeTransition(context.Background(), run, run.Checkpoints[0], resumed.ID)
	refreshed, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := refreshed.Checkpoints[0]
	if checkpoint.AcknowledgedAt != 0 || checkpoint.DiscardedAt != 0 || len(checkpoint.ResumedTurnIDs) != 1 || checkpoint.ResumedTurnIDs[0] != resumed.ID || checkpoint.ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("checkpoint evidence = %#v", checkpoint)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceCheckpointResume || transitions[0].TurnID != resumed.ID {
		t.Fatalf("checkpoint resume transitions = %#v", transitions)
	}
}
