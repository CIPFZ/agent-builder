package runtime

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeSQLiteToolCallStoreIdempotentUpsert(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	sched := scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	if _, err := sched.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		MessageID:    "message-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		InputSummary: "pwd",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		MessageID:    "message-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		InputSummary: "pwd",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-1",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "C:/work",
	}); err != nil {
		t.Fatal(err)
	}

	calls, err := sched.ListCalls(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != "tool-1" || calls[0].Status != scheduler.ToolCallCompleted {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].OutputSummary != "C:/work" || calls[0].FinishedAt.IsZero() {
		t.Fatalf("completed call = %#v", calls[0])
	}
}

func TestRuntimeSQLiteToolCallStoreDoesNotDowngradeFinalState(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	sched := scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	if _, err := sched.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    scheduler.ToolSourceShell,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-1",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "done",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    scheduler.ToolSourceShell,
	}); err != nil {
		t.Fatal(err)
	}

	call, err := sched.GetCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallCompleted || call.OutputSummary != "done" || call.FinishedAt.IsZero() {
		t.Fatalf("call = %#v", call)
	}
}
