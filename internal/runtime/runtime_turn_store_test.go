package runtime

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeTurnStoreUpsertListAndInterrupt(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeTurnStore(conn)
	turn, err := store.Upsert(context.Background(), RuntimeTurn{
		ID:            "turn-1",
		SessionID:     "session-1",
		Status:        turnStatusRunning,
		Provider:      "provider",
		Model:         "model",
		PromptPreview: "hello",
		UsageBefore:   RuntimeUsage{TotalTokens: 3},
		StartedAt:     1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.ID != "turn-1" || turn.Status != turnStatusRunning || turn.UsageBefore.TotalTokens != 3 {
		t.Fatalf("turn = %#v", turn)
	}

	active, err := store.List(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "turn-1" {
		t.Fatalf("active turns = %#v", active)
	}

	interrupted, err := store.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 || interrupted[0].Status != turnStatusInterrupted {
		t.Fatalf("interrupted = %#v", interrupted)
	}
	turn, err = store.Get(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != turnStatusInterrupted || turn.FinishedAt == 0 {
		t.Fatalf("turn after recovery = %#v", turn)
	}
}

func TestRuntimeServiceTurnReadsDurableStore(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID}
	service.turns = newRuntimeTurnStore(conn)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-1",
		SessionID: "session-1",
		Status:    turnStatusCompleted,
		StartedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := service.Turn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Turn.ID != "turn-1" || resp.Turn.Status != turnStatusCompleted {
		t.Fatalf("turn = %#v", resp.Turn)
	}
}

func TestRuntimeStatusUsesDurableActiveTurnsWhenMemoryIsEmpty(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-1",
		SessionID: "session-1",
		Status:    turnStatusRunning,
		StartedAt: time.Now().Add(-time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Requests.Running != 1 || status.Requests.ActiveRequestID != "turn-1" {
		t.Fatalf("requests = %#v", status.Requests)
	}
}

func TestRuntimeStatusScopesSessionBusyToSelectedSession(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	otherSession, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "Other")
	if err != nil {
		t.Fatal(err)
	}
	currentSession, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "Current")
	if err != nil {
		t.Fatal(err)
	}
	idleSession, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "Idle")
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	now := time.Now()
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-other",
		SessionID: otherSession.ID,
		Status:    turnStatusRunning,
		StartedAt: now.Add(-2 * time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-current",
		SessionID: currentSession.ID,
		Status:    turnStatusWaitingPermission,
		StartedAt: now.Add(-time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	service.sessionID = idleSession.ID
	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Busy || status.Requests.SessionBusy || status.Requests.SessionRequestID != "" {
		t.Fatalf("idle selected session should not be busy: %#v", status.Requests)
	}
	if status.Requests.Running != 2 || status.Requests.ActiveRequestID != "turn-other" {
		t.Fatalf("global running requests = %#v", status.Requests)
	}

	service.sessionID = currentSession.ID
	status, err = service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Busy || !status.Requests.SessionBusy || status.Requests.SessionRequestID != "turn-current" {
		t.Fatalf("selected active session should be busy: %#v", status.Requests)
	}
	if status.Requests.Running != 2 {
		t.Fatalf("running = %d, want 2", status.Requests.Running)
	}
}

func TestRuntimeServiceMarkInterruptedDoneCancelsInterruptedTurn(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-interrupted",
		SessionID:  "session-1",
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
	if resp.Action == nil {
		t.Fatal("mark interrupted done action metadata missing")
	}
	if !resp.Action.Accepted || resp.Action.Reason != runtimeTurnActionReasonInterruptedMarkedDone {
		t.Fatalf("action metadata = %#v", resp.Action)
	}
	if resp.Action.Source.Kind != runtimeTurnActionSourceKind || resp.Action.Source.Action != runtimeTurnActionMarkInterruptedDone {
		t.Fatalf("action source = %#v", resp.Action.Source)
	}
	if !resp.Action.Source.WorkbenchOnly || resp.Action.Source.StartsWorker || resp.Action.Source.IdempotentBy != "turn_id" || !resp.Action.Source.SessionActivityParity {
		t.Fatalf("action source semantics = %#v", resp.Action.Source)
	}
	if len(resp.Action.RefreshTargets) == 0 {
		t.Fatal("action refresh targets missing")
	}
	plain, err := service.Turn(context.Background(), "turn-interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Action != nil {
		t.Fatalf("ordinary turn read returned action metadata: %#v", plain.Action)
	}
	stored, err := service.turns.Get(context.Background(), "turn-interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != turnStatusCancelled {
		t.Fatalf("stored turn = %#v", stored)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == "turn.cancelled" && event.TurnID == "turn-interrupted"
	}) {
		t.Fatalf("mark-done event missing: %#v", events.Events)
	}
}
