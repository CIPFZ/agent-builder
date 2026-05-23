package scheduler

import (
	"context"
	"testing"
)

func TestMemoryStoreLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := New(NewMemoryStore())

	call, err := s.CreateCall(ctx, ToolCallRequest{
		ID:           "call-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		MessageID:    "message-1",
		Name:         "bash",
		Source:       ToolSourceShell,
		JobID:        "ABC",
		Command:      "pwd",
		Risk:         "execute",
		PolicyReason: "allowed",
		InputSummary: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallRunning || call.StartedAt.IsZero() || call.JobID != "ABC" || call.Command != "pwd" || call.Risk != "execute" {
		t.Fatalf("created call = %#v", call)
	}

	call, err = s.MarkWaitingPermission(ctx, "call-1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallWaitingPermission {
		t.Fatalf("status = %s", call.Status)
	}

	call, err = s.CompleteCall(ctx, ToolCallResult{
		ToolCallID:    "call-1",
		Status:        ToolCallCompleted,
		OutputSummary: "ok",
		ExitCode:      7,
		JobStatus:     "completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallCompleted || call.OutputSummary != "ok" || call.ExitCode != 7 || call.JobStatus != "completed" || call.FinishedAt.IsZero() {
		t.Fatalf("completed call = %#v", call)
	}

	calls, err := s.ListCalls(ctx, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != "call-1" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestCompleteCallRunningOutputDoesNotFinishCall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := New(NewMemoryStore())

	if _, err := s.CreateCall(ctx, ToolCallRequest{
		ID:        "call-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    ToolSourceShell,
	}); err != nil {
		t.Fatal(err)
	}
	call, err := s.CompleteCall(ctx, ToolCallResult{
		ToolCallID:    "call-1",
		Status:        ToolCallRunning,
		OutputSummary: "partial output",
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallRunning || !call.FinishedAt.IsZero() {
		t.Fatalf("call = %#v", call)
	}
}

func TestMemoryStoreDoesNotDowngradeFinalState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := New(NewMemoryStore())

	if _, err := s.CreateCall(ctx, ToolCallRequest{
		ID:        "call-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    ToolSourceShell,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteCall(ctx, ToolCallResult{
		ToolCallID:    "call-1",
		Status:        ToolCallFailed,
		OutputSummary: "failed",
		Error:         "boom",
		IsError:       true,
	}); err != nil {
		t.Fatal(err)
	}
	call, err := s.CreateCall(ctx, ToolCallRequest{
		ID:        "call-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    ToolSourceShell,
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallFailed || call.OutputSummary != "failed" || call.Error != "boom" {
		t.Fatalf("call = %#v", call)
	}
}
