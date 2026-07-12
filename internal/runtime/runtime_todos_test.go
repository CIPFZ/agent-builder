package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/session"
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
		PlanID:     "todo-plan-1",
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
	if event.Payload["plan_id"] != "todo-plan-1" || event.Payload["todos"] == nil {
		t.Fatalf("canonical todo evidence missing: %#v", event.Payload)
	}

	if event.Type != "todo.updated" || event.SessionID != "session-1" || event.TurnID != "turn-1" || event.ToolCallID != "tool-1" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["summary"] != "0 pending, 1 in progress, 0 completed" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestRuntimeTurnTodosUsesSessionStateForRecovery(t *testing.T) {
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

	sess, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "Todo session")
	if err != nil {
		t.Fatal(err)
	}
	sess.Todos = []session.Todo{{
		Content:    "Inspect plan mode",
		Status:     session.TodoStatusInProgress,
		ActiveForm: "Inspecting plan mode",
	}}
	if _, err := runtimeWorkbench.SaveSession(context.Background(), workspace.ID, sess); err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-1",
		SessionID: sess.ID,
		Status:    turnStatusCompleted,
		StartedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	bySession, err := service.SessionTodos(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bySession.Summary.Total != 1 || bySession.Summary.InProgress != 1 {
		t.Fatalf("session todos = %#v", bySession.Summary)
	}

	byTurn, err := service.TurnTodos(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if byTurn.Summary.TurnID != "turn-1" || byTurn.Summary.SessionID != sess.ID || byTurn.Summary.Todos[0].ActiveForm != "Inspecting plan mode" {
		t.Fatalf("turn todos = %#v", byTurn.Summary)
	}
}
