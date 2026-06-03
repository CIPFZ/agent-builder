package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/proto"
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
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID}
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
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
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
	runtimeBackend, workspace := backendForSkillTest(t)
	otherSession, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "Other")
	if err != nil {
		t.Fatal(err)
	}
	currentSession, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "Current")
	if err != nil {
		t.Fatal(err)
	}
	idleSession, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "Idle")
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
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
