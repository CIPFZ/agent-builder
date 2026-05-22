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
		InputSummary: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallRunning || call.StartedAt.IsZero() {
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
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != ToolCallCompleted || call.OutputSummary != "ok" || call.FinishedAt.IsZero() {
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
