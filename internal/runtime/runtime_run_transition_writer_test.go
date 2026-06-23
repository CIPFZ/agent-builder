package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
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

func TestRuntimeRunTransitionWriterRequiresRunTurnLinkBeforeStartedTransition(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-preflight",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnStarted, started, "", runtimeRunStatusActive, "turn started before link")
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("turn_started transition recorded before run turn link: %#v", transitions)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", started.ID, started.StartedAt); err != nil {
		t.Fatal(err)
	}
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnStarted, started, "", runtimeRunStatusActive, "turn started after link")
	transitions, err = service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].TurnID != started.ID || transitions[0].Source != runtimeRunTransitionSourceTurnStarted {
		t.Fatalf("linked turn_started transition missing: %#v", transitions)
	}
	if !runtimeRunSessionLinkedToTurn(context.Background(), service.runs, run.ID, "session-1", started.ID) {
		t.Fatalf("run session link missing for started transition: run=%s turn=%s", run.ID, started.ID)
	}
}

func TestRuntimeRunTransitionWriterRejectsUnknownSources(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-unknown-source",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}

	service.recordRunTurnTransition(context.Background(), "runtime_event_payload", turn, "", runtimeRunStatusActive, "event payload attempted status write")
	service.recordRunTransition(context.Background(), RuntimeRunTransition{
		RunID:     run.ID,
		SessionID: "session-1",
		TurnID:    turn.ID,
		ToStatus:  runtimeRunStatusCancelled,
		Source:    "action_metadata",
		CreatedAt: 2000,
	})

	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("unknown transition sources should be rejected: %#v", transitions)
	}
	persisted, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != runtimeRunStatusActive || persisted.FinishedAt != 0 {
		t.Fatalf("unknown transition source mutated run status: %#v", persisted)
	}
}

func TestRuntimeRunTransitionWriterMarkInterruptedDonePreservesCancelledSemantics(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	sess, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "interrupted")
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
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, sess.ID, "turn-interrupted", 1000); err != nil {
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
	if !runtimeRunSessionLinkedToTurn(context.Background(), service.runs, run.ID, sess.ID, "turn-interrupted") {
		t.Fatalf("interrupted acknowledgement broke run turn link: run=%s turn=%s", run.ID, "turn-interrupted")
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
	if _, err := service.runs.LinkCheckpointResume(context.Background(), run.ID, run.Checkpoints[0].ID, resumed.ID); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.Checkpoints) != 1 || refreshed.Checkpoints[0].ID != run.Checkpoints[0].ID || refreshed.Checkpoints[0].AcknowledgedAt != 0 || refreshed.Checkpoints[0].DiscardedAt != 0 {
		t.Fatalf("checkpoint mutated = %#v", refreshed.Checkpoints)
	}
	if len(refreshed.Checkpoints[0].ResumedTurnIDs) != 1 || refreshed.Checkpoints[0].ResumedTurnIDs[0] != resumed.ID {
		t.Fatalf("checkpoint resumed turn link missing = %#v", refreshed.Checkpoints[0])
	}
}

func TestRuntimeRunTransitionWriterRequiresResumedTurnBeforeCheckpointResume(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()
	run, err := service.runs.Upsert(context.Background(), RuntimeRun{
		ID:               "run-resume-preflight",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:        "turn:turn-source:interrupted",
			TurnID:    "turn-source",
			Status:    turnStatusInterrupted,
			CreatedAt: 2000,
		}},
		CreatedAt: 1000,
		UpdatedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := run.Checkpoints[0]
	service.recordCheckpointResumeTransition(context.Background(), run, checkpoint, "turn-resumed-missing")
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("checkpoint resume transition recorded before resumed turn exists: %#v", transitions)
	}
	before, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Checkpoints) != 1 || len(before.Checkpoints[0].ResumedTurnIDs) != 0 || before.Checkpoints[0].AcknowledgedAt != 0 || before.Checkpoints[0].DiscardedAt != 0 {
		t.Fatalf("missing resumed turn mutated checkpoint evidence: %#v", before.Checkpoints)
	}
	resumed, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-resumed-created",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkCheckpointResume(context.Background(), run.ID, checkpoint.ID, resumed.ID); err != nil {
		t.Fatal(err)
	}
	service.recordCheckpointResumeTransition(context.Background(), run, checkpoint, resumed.ID)
	transitions, err = service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceCheckpointResume || transitions[0].TurnID != resumed.ID {
		t.Fatalf("checkpoint resume transition missing after resumed turn exists: %#v", transitions)
	}
	after, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Checkpoints[0].ResumedTurnIDs) != 1 || after.Checkpoints[0].ResumedTurnIDs[0] != resumed.ID || after.Checkpoints[0].AcknowledgedAt != 0 || after.Checkpoints[0].DiscardedAt != 0 {
		t.Fatalf("resumed turn link mutated source checkpoint evidence: %#v", after.Checkpoints[0])
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
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	return service, func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	}
}
