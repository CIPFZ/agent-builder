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
