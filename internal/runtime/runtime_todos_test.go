package runtime

import (
	"testing"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/session"
)

func TestRuntimeTodoSummaryCountsStatuses(t *testing.T) {
	t.Parallel()

	summary := runtimeTodoSummary("session-1", "turn-1", []session.Todo{
		{Content: "Plan", Status: session.TodoStatusCompleted},
		{Content: "Build", Status: session.TodoStatusInProgress, ActiveForm: "Building"},
		{Content: "Test", Status: session.TodoStatusPending},
	}, 123)

	if summary.SessionID != "session-1" || summary.TurnID != "turn-1" || summary.Total != 3 {
		t.Fatalf("summary identity = %#v", summary)
	}
	if summary.Pending != 1 || summary.InProgress != 1 || summary.Completed != 1 {
		t.Fatalf("summary counts = %#v", summary)
	}
	if summary.Todos[1].ActiveForm != "Building" {
		t.Fatalf("todos = %#v", summary.Todos)
	}
}

func TestRuntimeTodoUpdatedEvent(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	event := service.recordTodoUpdate(tools.TodoUpdatedEvent{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
		Todos: []session.Todo{{
			Content: "Plan",
			Status:  session.TodoStatusInProgress,
		}},
		InProgress: 1,
		Total:      1,
		UpdatedAt:  123,
	})

	if event.Type != "todo.updated" || event.SessionID != "session-1" || event.TurnID != "turn-1" || event.ToolCallID != "tool-1" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["summary"] != "0 pending, 1 in progress, 0 completed" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}
