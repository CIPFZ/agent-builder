package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/session"
)

//go:embed todos.md
var todosDescription string

const (
	TodosToolName = "todos"
)

type TodosParams struct {
	Todos []TodoItem `json:"todos" description:"The updated todo list"`
}

type TodoItem struct {
	Content    string `json:"content" description:"What needs to be done (imperative form)"`
	Status     string `json:"status" description:"Task status: pending, in_progress, or completed"`
	ActiveForm string `json:"active_form" description:"Present continuous form (e.g., 'Running tests')"`
}

type TodoUpdatedEvent struct {
	SessionID     string         `json:"session_id"`
	TurnID        string         `json:"turn_id,omitempty"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	Todos         []session.Todo `json:"todos"`
	Pending       int            `json:"pending"`
	InProgress    int            `json:"in_progress"`
	Completed     int            `json:"completed"`
	Total         int            `json:"total"`
	JustCompleted []string       `json:"just_completed,omitempty"`
	JustStarted   string         `json:"just_started,omitempty"`
	UpdatedAt     int64          `json:"updated_at"`
}

type TodosResponseMetadata struct {
	IsNew         bool           `json:"is_new"`
	Todos         []session.Todo `json:"todos"`
	JustCompleted []string       `json:"just_completed,omitempty"`
	JustStarted   string         `json:"just_started,omitempty"`
	Completed     int            `json:"completed"`
	Total         int            `json:"total"`
}

var todoUpdates = pubsub.NewBroker[TodoUpdatedEvent]()

func SubscribeTodoUpdates(ctx context.Context) <-chan pubsub.Event[TodoUpdatedEvent] {
	return todoUpdates.Subscribe(ctx)
}

func NewTodosTool(sessions session.Service) fantasy.AgentTool {
	return newTodosTool(TodosToolName, todosDescription, sessions)
}

func newTodosTool(name, description string, sessions session.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		name,
		description,
		func(ctx context.Context, params TodosParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for managing todos")
			}

			currentSession, err := sessions.Get(ctx, sessionID)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("failed to get session: %w", err)
			}

			isNew := len(currentSession.Todos) == 0
			oldStatusByContent := make(map[string]session.TodoStatus)
			for _, todo := range currentSession.Todos {
				oldStatusByContent[todo.Content] = todo.Status
			}

			normalized, err := normalizeTodoItems(params.Todos)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}

			todos := make([]session.Todo, len(normalized))
			var justCompleted []string
			var justStarted string
			completedCount := 0

			for i, item := range normalized {
				todos[i] = session.Todo{
					Content:    item.Content,
					Status:     session.TodoStatus(item.Status),
					ActiveForm: item.ActiveForm,
				}

				newStatus := session.TodoStatus(item.Status)
				oldStatus, existed := oldStatusByContent[item.Content]

				if newStatus == session.TodoStatusCompleted {
					completedCount++
					if existed && oldStatus != session.TodoStatusCompleted {
						justCompleted = append(justCompleted, item.Content)
					}
				}

				if newStatus == session.TodoStatusInProgress {
					if !existed || oldStatus != session.TodoStatusInProgress {
						if item.ActiveForm != "" {
							justStarted = item.ActiveForm
						} else {
							justStarted = item.Content
						}
					}
				}
			}

			updatedSession := currentSession
			updatedSession.Todos = todos
			savedSession, err := sessions.Save(ctx, updatedSession)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("failed to save todos: %w", err)
			}
			todos = savedSession.Todos

			response := "Todo list updated successfully.\n\n"

			pendingCount := 0
			inProgressCount := 0

			for _, todo := range todos {
				switch todo.Status {
				case session.TodoStatusPending:
					pendingCount++
				case session.TodoStatusInProgress:
					inProgressCount++
				}
			}

			todoUpdates.Publish(pubsub.UpdatedEvent, TodoUpdatedEvent{
				SessionID:     sessionID,
				TurnID:        GetTurnFromContext(ctx),
				ToolCallID:    call.ID,
				Todos:         todos,
				Pending:       pendingCount,
				InProgress:    inProgressCount,
				Completed:     completedCount,
				Total:         len(todos),
				JustCompleted: justCompleted,
				JustStarted:   justStarted,
				UpdatedAt:     firstNonZero(savedSession.UpdatedAt, time.Now().UnixMilli()),
			})

			response += fmt.Sprintf("Status: %d pending, %d in progress, %d completed\n",
				pendingCount, inProgressCount, completedCount)

			response += "Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress. Please proceed with the current tasks if applicable."

			metadata := TodosResponseMetadata{
				IsNew:         isNew,
				Todos:         todos,
				JustCompleted: justCompleted,
				JustStarted:   justStarted,
				Completed:     completedCount,
				Total:         len(todos),
			}

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
		})
}

func normalizeTodoItems(items []TodoItem) ([]TodoItem, error) {
	normalized := make([]TodoItem, len(items))
	seen := make(map[string]struct{}, len(items))
	inProgress := 0
	for i, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		item.ActiveForm = strings.TrimSpace(item.ActiveForm)
		if item.Content == "" {
			return nil, fmt.Errorf("todo content is required at index %d", i)
		}
		if _, exists := seen[item.Content]; exists {
			return nil, fmt.Errorf("duplicate todo content %q", item.Content)
		}
		seen[item.Content] = struct{}{}
		switch item.Status {
		case "pending", "completed":
		case "in_progress":
			inProgress++
			if inProgress > 1 {
				return nil, fmt.Errorf("only one todo may be in_progress")
			}
		default:
			return nil, fmt.Errorf("invalid status %q for todo %q", item.Status, item.Content)
		}
		normalized[i] = item
	}
	return normalized, nil
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
