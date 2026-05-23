package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/session"
)

func (r *runtimeService) SessionTodos(ctx context.Context, sessionID string) (RuntimeTodosResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTodosResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	if sessionID == "" {
		return RuntimeTodosResponse{}, fmt.Errorf("session id is required")
	}
	sess, err := r.runtime.GetSession(ctx, r.workspace.ID, sessionID)
	if err != nil {
		return RuntimeTodosResponse{}, err
	}
	return RuntimeTodosResponse{Summary: runtimeTodoSummary(sessionID, "", sess.Todos, sess.UpdatedAt)}, nil
}

func (r *runtimeService) TurnTodos(ctx context.Context, turnID string) (RuntimeTodosResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTodosResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTodosResponse{}, fmt.Errorf("turn id is required")
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTodosResponse{}, err
	}
	resp, err := r.SessionTodos(ctx, turn.SessionID)
	if err != nil {
		return RuntimeTodosResponse{}, err
	}
	resp.Summary.TurnID = turnID
	return resp, nil
}

func runtimeTodoSummary(sessionID, turnID string, todos []session.Todo, updatedAt int64) RuntimeTodoSummary {
	summary := RuntimeTodoSummary{
		SessionID: sessionID,
		TurnID:    turnID,
		Todos:     make([]RuntimeTodo, 0, len(todos)),
		Total:     len(todos),
		UpdatedAt: updatedAt,
	}
	for _, todo := range todos {
		summary.Todos = append(summary.Todos, RuntimeTodo{
			Content:    todo.Content,
			Status:     string(todo.Status),
			ActiveForm: todo.ActiveForm,
		})
		switch todo.Status {
		case session.TodoStatusPending:
			summary.Pending++
		case session.TodoStatusInProgress:
			summary.InProgress++
		case session.TodoStatusCompleted:
			summary.Completed++
		}
	}
	return summary
}

func (r *runtimeService) consumeTodoUpdates(ctx context.Context) {
	events := tools.SubscribeTodoUpdates(ctx)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			r.recordTodoUpdate(event.Payload)
		case <-ctx.Done():
			return
		}
	}
}

func (r *runtimeService) recordTodoUpdate(update tools.TodoUpdatedEvent) RuntimeEvent {
	summary := runtimeTodoSummary(update.SessionID, update.TurnID, update.Todos, update.UpdatedAt)
	if summary.UpdatedAt == 0 {
		summary.UpdatedAt = time.Now().UnixMilli()
	}
	summaryText := fmt.Sprintf("%d pending, %d in progress, %d completed", summary.Pending, summary.InProgress, summary.Completed)
	event := r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventTodoUpdated,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  update.SessionID,
		TurnID:     update.TurnID,
		ToolCallID: update.ToolCallID,
		Payload: map[string]any{
			"session_id":     update.SessionID,
			"turn_id":        update.TurnID,
			"pending":        summary.Pending,
			"in_progress":    summary.InProgress,
			"completed":      summary.Completed,
			"total":          summary.Total,
			"just_completed": update.JustCompleted,
			"just_started":   update.JustStarted,
			"summary":        summaryText,
		},
	})
	r.writeAudit(auditEntry{
		RequestID:  update.TurnID,
		Event:      "todo_updated",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  update.SessionID,
		ToolCallID: update.ToolCallID,
		ToolCalls: []auditToolCall{{
			ID:     update.ToolCallID,
			Name:   tools.TodosToolName,
			Output: summaryText,
		}},
	})
	return event
}
