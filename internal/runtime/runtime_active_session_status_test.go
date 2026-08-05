package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestActiveSessionStatusIndexBoundsAtAmplificationScale(t *testing.T) {
	service := newRuntimeService()
	service.mu.Lock()
	for i := 0; i < 500; i++ {
		service.upsertActiveSessionStatusLocked(RuntimeActiveSessionStatus{
			SessionID:     fmt.Sprintf("session-%03d-", i) + strings.Repeat("long-", 80),
			ProjectID:     strings.Repeat("project-long-", 80),
			Status:        "running",
			Phase:         strings.Repeat("phase-", 100),
			ProgressLabel: strings.Repeat("progress-label-", 200),
			ActiveTurnID:  fmt.Sprintf("turn-%03d-", i) + strings.Repeat("long-", 100),
			UpdatedAt:     int64(i + 1),
		})
	}
	statuses := service.activeSessionStatusesLocked()
	service.mu.Unlock()

	if len(statuses) != 500 {
		t.Fatalf("expected 500 bounded statuses, got %d", len(statuses))
	}
	for i, status := range statuses {
		encoded, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) > 2*1024 {
			t.Fatalf("status %d encoded to %d bytes; want <= 2048", i, len(encoded))
		}
	}
	encoded, err := json.Marshal(statuses)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 1024*1024 {
		t.Fatalf("aggregate status index encoded to %d bytes; want <= 1 MiB", len(encoded))
	}
}

func TestActiveSessionStatusIndexEvictsToFiveHundred(t *testing.T) {
	service := newRuntimeService()
	service.mu.Lock()
	for i := 0; i < 650; i++ {
		service.upsertActiveSessionStatusLocked(RuntimeActiveSessionStatus{
			SessionID: fmt.Sprintf("session-%03d", i), Status: "running", UpdatedAt: int64(i + 1),
		})
	}
	statuses := service.activeSessionStatusesLocked()
	service.mu.Unlock()
	if len(statuses) != runtimeActiveSessionStatusLimit {
		t.Fatalf("expected %d statuses, got %d", runtimeActiveSessionStatusLimit, len(statuses))
	}
	if statuses[len(statuses)-1].SessionID != "session-150" {
		t.Fatalf("expected oldest retained session-150, got %q", statuses[len(statuses)-1].SessionID)
	}
}

func TestActiveSessionStatusReducerLifecycle(t *testing.T) {
	service := newRuntimeService()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	service.mu.Lock()
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventTurnStarted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: now})
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventPermissionRequested, SessionID: "session-1", TurnID: "turn-1", CreatedAt: now})
	waiting := service.activeSessionStatuses["session-1"]
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventToolCallStarted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: now, Payload: map[string]any{"name": strings.Repeat("tool", 200)}})
	runningTool := service.activeSessionStatuses["session-1"]
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventTurnCompleted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: now})
	_, completedRetained := service.activeSessionStatuses["session-1"]
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventTurnFailed, SessionID: "session-2", TurnID: "turn-2", CreatedAt: now, Payload: map[string]any{"error": strings.Repeat("failure", 500)}})
	attention := service.activeSessionStatuses["session-2"]
	service.mu.Unlock()

	if waiting.Status != turnStatusWaitingPermission || waiting.Phase != "permission" {
		t.Fatalf("unexpected permission status: %#v", waiting)
	}
	if runningTool.Status != turnStatusRunning || runningTool.Phase != "tool" || len(runningTool.ProgressLabel) > runtimeActiveSessionStatusFieldBytes {
		t.Fatalf("unexpected tool status: %#v", runningTool)
	}
	if completedRetained {
		t.Fatal("completed session remained in active status index")
	}
	if attention.Status != "attention" || !attention.Unread || len(attention.ProgressLabel) > runtimeActiveSessionStatusFieldBytes {
		t.Fatalf("unexpected attention status: %#v", attention)
	}
}

func TestActiveSessionStatusReducerIgnoresLateOlderTurnTerminal(t *testing.T) {
	service := newRuntimeService()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	service.mu.Lock()
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventTurnStarted, SessionID: "session-1", TurnID: "turn-new", CreatedAt: now})
	service.reduceActiveSessionStatusLocked(RuntimeEvent{Type: runtimeapi.EventTurnCompleted, SessionID: "session-1", TurnID: "turn-old", CreatedAt: now})
	status, retained := service.activeSessionStatuses["session-1"]
	service.mu.Unlock()
	if !retained || status.ActiveTurnID != "turn-new" || status.Status != turnStatusRunning {
		t.Fatalf("late older terminal replaced newer status: %#v", status)
	}
}
