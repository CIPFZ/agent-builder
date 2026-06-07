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
		ID:                  "tool-1",
		SessionID:           "session-1",
		TurnID:              "turn-1",
		MessageID:           "message-1",
		Name:                "bash",
		Source:              scheduler.ToolSourceShell,
		Risk:                "destructive",
		PolicyMode:          "ask",
		PolicyRuleID:        "deny-rm",
		PolicyScopeKind:     "shell_prefix",
		PolicyScopeValue:    "rm ",
		PolicyTargetSummary: "rm build",
		ShellRisk:           "destructive",
		ShellReason:         "Shell policy detected recursive delete.",
		InputSummary:        "pwd",
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
	if calls[0].PolicyRuleID != "deny-rm" || calls[0].PolicyScopeKind != "shell_prefix" || calls[0].ShellRisk != "destructive" {
		t.Fatalf("policy diagnostics not persisted: %#v", calls[0])
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

func TestRuntimeSQLiteToolCallStoreCancelsUnfinishedCallsOnRecovery(t *testing.T) {
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
		ID:        "tool-running",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "write",
		Source:    scheduler.ToolSourceBuiltin,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-complete",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "view",
		Source:    scheduler.ToolSourceBuiltin,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID: "tool-complete",
		Status:     scheduler.ToolCallCompleted,
	}); err != nil {
		t.Fatal(err)
	}

	cancelled, err := cancelUnfinishedRuntimeToolCalls(context.Background(), sched, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0].ID != "tool-running" || cancelled[0].Status != scheduler.ToolCallCancelled || cancelled[0].FinishedAt.IsZero() {
		t.Fatalf("cancelled calls = %#v", cancelled)
	}
	completed, err := sched.GetCall(context.Background(), "tool-complete")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != scheduler.ToolCallCompleted {
		t.Fatalf("completed call was changed: %#v", completed)
	}
}
