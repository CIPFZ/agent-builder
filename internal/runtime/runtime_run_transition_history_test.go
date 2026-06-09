package runtime

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/proto"
)

func TestRuntimeRunTransitionHistoryReturnsReadOnlyCursorWindow(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	attachRuntimeTransitionHistoryBackend(t, service)
	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	for _, transition := range []RuntimeRunTransition{
		{RunID: run.ID, SessionID: "session-1", TurnID: "turn-1", ToStatus: runtimeRunStatusActive, Source: runtimeRunTransitionSourceTurnStarted, CreatedAt: 1000},
		{RunID: run.ID, SessionID: "session-1", TurnID: "turn-1", FromStatus: runtimeRunStatusActive, ToStatus: runtimeRunStatusCompleted, Source: runtimeRunTransitionSourceTurnFinished, CreatedAt: 2000},
		{RunID: run.ID, SessionID: "session-1", TurnID: "turn-2", ToStatus: runtimeRunStatusActive, Source: runtimeRunTransitionSourceTurnStarted, CreatedAt: 3000},
	} {
		if _, err := service.transitions.Upsert(context.Background(), transition); err != nil {
			t.Fatal(err)
		}
	}

	first, err := service.RunTransitionHistory(context.Background(), RuntimeRunTransitionHistoryRequest{RunID: run.ID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Transitions) != 2 || first.Transitions[0].CreatedAt != 2000 || first.Transitions[1].CreatedAt != 3000 {
		t.Fatalf("first window = %#v", first)
	}
	if first.Source.Kind != runtimeRunTransitionHistorySourceKind || !first.Source.ReadOnly || !first.Source.AuditOnly || !first.Source.SessionActivityParity {
		t.Fatalf("source = %#v", first.Source)
	}
	if first.Window.FirstCursor == "" || first.Window.LastCursor == "" || !first.Window.HasMoreBefore || !first.Window.ToEnd {
		t.Fatalf("window = %#v", first.Window)
	}
	beforeLast, err := service.RunTransitionHistory(context.Background(), RuntimeRunTransitionHistoryRequest{RunID: run.ID, Cursor: first.Window.FirstCursor, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeLast.Transitions) != 1 || beforeLast.Transitions[0].CreatedAt != 3000 {
		t.Fatalf("cursor window = %#v", beforeLast)
	}
}

func TestRuntimeRunTransitionHistoryDoesNotReplaceRunProjectionOrSessionActivity(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "transition parity")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(context.Background(), workspace.ID, sess.ID, "transition parity", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-cancelled",
		SessionID:  sess.ID,
		Status:     turnStatusCancelled,
		StartedAt:  1000,
		FinishedAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.transitions.Upsert(context.Background(), RuntimeRunTransition{
		RunID:      run.ID,
		SessionID:  sess.ID,
		TurnID:     "turn-cancelled",
		FromStatus: runtimeRunStatusActive,
		ToStatus:   runtimeRunStatusCancelled,
		Source:     runtimeRunTransitionSourceTurnCancelled,
		CreatedAt:  2000,
	}); err != nil {
		t.Fatal(err)
	}

	history, err := service.RunTransitionHistory(context.Background(), RuntimeRunTransitionHistoryRequest{RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(history.Transitions) != 1 || history.Transitions[0].ToStatus != runtimeRunStatusCancelled {
		t.Fatalf("history = %#v", history)
	}
	if projection.Run.Status != runtimeRunStatusCancelled || projection.Run.Diagnostics.CancelledTurnCount != 1 {
		t.Fatalf("projection should derive state from turn evidence, got %#v", projection.Run)
	}
	if len(activity.Turns) != 1 || activity.Turns[0].Status != turnStatusCancelled {
		t.Fatalf("activity should retain turn evidence, got %#v", activity.Turns)
	}
	if len(history.Transitions) != 1 && len(activity.Turns) == 1 {
		t.Fatal("transition history must not synthesize activity evidence")
	}
}

func attachRuntimeTransitionHistoryBackend(t *testing.T, service *runtimeService) {
	t.Helper()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
}
