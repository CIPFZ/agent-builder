package runtime

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeRunTransitionWriterRecordsTurnLifecycleIdempotently(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-1",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", started.ID, started.StartedAt); err != nil {
		t.Fatal(err)
	}
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnStarted, started, "", runtimeRunStatusActive, "turn started")
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnStarted, started, "", runtimeRunStatusActive, "turn started")

	finished, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-1",
		SessionID:  "session-1",
		Status:     turnStatusCompleted,
		StartedAt:  1000,
		FinishedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusCompleted,
		TurnIDs:          []string{"turn-1"},
		Diagnostics:      RuntimeRunDiagnostics{TurnCount: 1},
		CreatedAt:        1000,
		UpdatedAt:        2000,
		FinishedAt:       2000,
	}, runtimeRunSourceBackfill); err != nil {
		t.Fatal(err)
	}
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnFinished, finished, runtimeRunStatusActive, runtimeRunStatusCompleted, "turn finished")
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnFinished, finished, runtimeRunStatusActive, runtimeRunStatusCompleted, "turn finished")

	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 {
		t.Fatalf("transitions = %#v", transitions)
	}
	if transitions[0].Source != runtimeRunTransitionSourceTurnStarted || transitions[0].ToStatus != runtimeRunStatusActive || transitions[0].CreatedAt != 1000 {
		t.Fatalf("start transition = %#v", transitions[0])
	}
	if transitions[1].Source != runtimeRunTransitionSourceTurnFinished || transitions[1].ToStatus != runtimeRunStatusCompleted || transitions[1].CreatedAt != 2000 {
		t.Fatalf("finish transition = %#v", transitions[1])
	}
}

func TestRuntimeRunTransitionWriterMarkInterruptedDonePreservesCancelledSemantics(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "interrupted")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(context.Background(), workspace.ID, sess.ID, "resume work", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-interrupted",
		SessionID:  sess.ID,
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2000,
		Error:      "runtime restarted before turn completed",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := service.MarkInterruptedDone(context.Background(), "turn-interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Turn.Status != turnStatusCancelled || resp.Turn.Interrupted != nil {
		t.Fatalf("turn response = %#v", resp.Turn)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transitions = %#v", transitions)
	}
	if transitions[0].Source != runtimeRunTransitionSourceInterruptedMarkedDone || transitions[0].ToStatus != runtimeRunStatusCancelled {
		t.Fatalf("transition = %#v", transitions[0])
	}
}

func TestRuntimeRunTransitionWriterRecordsStartupRecoveryForTurnAndTask(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "recover work", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{
		ID:         "turn-recovered",
		SessionID:  "session-1",
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2000,
	}
	task := RuntimeAgentTask{
		ID:              "task-recovered",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		Status:          agentTaskStatusInterrupted,
		StartedAt:       1100,
		FinishedAt:      2100,
	}

	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceStartupRecovery, turn, "", runtimeRunStatusInterrupted, "startup recovery")
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceStartupRecovery, turn, "", runtimeRunStatusInterrupted, "startup recovery")
	service.recordRunTaskTransition(context.Background(), runtimeRunTransitionSourceStartupRecovery, task, "", runtimeRunStatusInterrupted, "startup recovery")
	service.recordRunTaskTransition(context.Background(), runtimeRunTransitionSourceStartupRecovery, task, "", runtimeRunStatusInterrupted, "startup recovery")

	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 {
		t.Fatalf("transitions = %#v", transitions)
	}
	if transitions[0].TurnID != turn.ID || transitions[0].TaskID != "" || transitions[0].CreatedAt != 2000 {
		t.Fatalf("turn recovery transition = %#v", transitions[0])
	}
	if transitions[1].TaskID != task.ID || transitions[1].CreatedAt != 2100 {
		t.Fatalf("task recovery transition = %#v", transitions[1])
	}
}

func TestRuntimeRunTransitionWriterRecordsCheckpointResumeFromNewTurn(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	run, err := service.runs.Upsert(context.Background(), RuntimeRun{
		ID:               "run-1",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:             "turn:turn-source:interrupted",
			TurnID:         "turn-source",
			Status:         turnStatusInterrupted,
			CreatedAt:      2000,
			ResumeEligible: true,
		}},
		CreatedAt: 1000,
		UpdatedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-resumed",
		SessionID: "session-1",
		Status:    turnStatusRunning,
		StartedAt: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}

	service.recordCheckpointResumeTransition(context.Background(), run, run.Checkpoints[0], resumed.ID)
	service.recordCheckpointResumeTransition(context.Background(), run, run.Checkpoints[0], resumed.ID)

	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transitions = %#v", transitions)
	}
	if transitions[0].Source != runtimeRunTransitionSourceCheckpointResume || transitions[0].FromStatus != runtimeRunStatusInterrupted || transitions[0].ToStatus != runtimeRunStatusActive {
		t.Fatalf("resume transition = %#v", transitions[0])
	}
	if transitions[0].TurnID != resumed.ID || transitions[0].CreatedAt != 3000 {
		t.Fatalf("resume transition evidence = %#v", transitions[0])
	}
	refreshed, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.Checkpoints) != 1 || refreshed.Checkpoints[0].ID != run.Checkpoints[0].ID || refreshed.Checkpoints[0].AcknowledgedAt != 0 || refreshed.Checkpoints[0].DiscardedAt != 0 {
		t.Fatalf("checkpoint mutated = %#v", refreshed.Checkpoints)
	}
}

func runtimeRunTransitionWriterTestService(t *testing.T) (*runtimeService, func()) {
	t.Helper()
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.transitions = newRuntimeRunTransitionStore(conn)
	return service, func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	}
}
