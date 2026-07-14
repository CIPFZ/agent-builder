package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/session"
	"github.com/stretchr/testify/require"
)

type mockTodoSessionService struct {
	session session.Session
	saveErr error
}

func (m *mockTodoSessionService) Subscribe(context.Context) <-chan pubsub.Event[session.Session] {
	return make(chan pubsub.Event[session.Session])
}

func (m *mockTodoSessionService) Create(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (m *mockTodoSessionService) CreateWithScope(context.Context, string, string, string) (session.Session, error) {
	return session.Session{}, nil
}

func (m *mockTodoSessionService) CreateWithScopeAndWorkdir(context.Context, string, string, string, string, string, bool) (session.Session, error) {
	return session.Session{}, nil
}

func (m *mockTodoSessionService) CreateTitleSession(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (m *mockTodoSessionService) CreateTaskSession(context.Context, string, string, string) (session.Session, error) {
	return session.Session{}, nil
}

func (m *mockTodoSessionService) Get(_ context.Context, id string) (session.Session, error) {
	if m.session.ID != id {
		return session.Session{}, errors.New("session not found")
	}
	return m.session, nil
}

func (m *mockTodoSessionService) GetLast(context.Context) (session.Session, error) {
	return m.session, nil
}

func (m *mockTodoSessionService) List(context.Context) ([]session.Session, error) {
	return []session.Session{m.session}, nil
}

func (m *mockTodoSessionService) Save(_ context.Context, sess session.Session) (session.Session, error) {
	if m.saveErr != nil {
		return session.Session{}, m.saveErr
	}
	m.session = sess
	return sess, nil
}

func (m *mockTodoSessionService) UpdateTitleAndUsage(context.Context, string, string, int64, int64, float64) error {
	return nil
}

func (m *mockTodoSessionService) FinalizeGeneratedTitle(context.Context, string, string, string, string, int64, int64, float64) (bool, error) {
	return true, nil
}

func (m *mockTodoSessionService) Rename(context.Context, string, string) error {
	return nil
}

func (m *mockTodoSessionService) Delete(context.Context, string) error {
	return nil
}

func (m *mockTodoSessionService) CreateAgentToolSessionID(messageID, toolCallID string) string {
	return messageID + ":" + toolCallID
}

func (m *mockTodoSessionService) ParseAgentToolSessionID(string) (string, string, bool) {
	return "", "", false
}

func (m *mockTodoSessionService) IsAgentToolSession(string) bool {
	return false
}

func TestTodosToolUpdatesTodos(t *testing.T) {
	t.Parallel()

	sessions := &mockTodoSessionService{session: session.Session{ID: "session-1"}}
	tool := NewTodosTool(sessions)
	input, err := json.Marshal(TodosParams{Todos: []TodoItem{{
		Content:    "Write report",
		Status:     "in_progress",
		ActiveForm: "Writing report",
	}}})
	require.NoError(t, err)

	resp, err := tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "session-1"), fantasy.ToolCall{
		ID:    "todo-call",
		Name:  TodosToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Len(t, sessions.session.Todos, 1)
	require.Equal(t, "Write report", sessions.session.Todos[0].Content)
	require.Equal(t, session.TodoStatusInProgress, sessions.session.Todos[0].Status)
	require.NotEmpty(t, sessions.session.Todos[0].ID)
	stableID := sessions.session.Todos[0].ID

	input, err = json.Marshal(TodosParams{Todos: []TodoItem{{Content: "Write report", Status: "completed", ActiveForm: "Writing report"}}})
	require.NoError(t, err)
	_, err = tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "session-1"), fantasy.ToolCall{ID: "todo-call-2", Name: TodosToolName, Input: string(input)})
	require.NoError(t, err)
	require.Equal(t, stableID, sessions.session.Todos[0].ID, "Todo identity must survive status updates")
}

func TestTodosToolRejectsInvalidListsWithoutChangingSession(t *testing.T) {
	t.Parallel()

	original := []session.Todo{{Content: "Existing", Status: session.TodoStatusPending}}
	sessions := &mockTodoSessionService{session: session.Session{ID: "session-1", Todos: original}}
	tool := NewTodosTool(sessions)
	input, err := json.Marshal(TodosParams{Todos: []TodoItem{
		{Content: "First", Status: "in_progress"},
		{Content: "Second", Status: "in_progress"},
	}})
	require.NoError(t, err)

	_, err = tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "session-1"), fantasy.ToolCall{
		ID: "todo-call", Name: TodosToolName, Input: string(input),
	})
	require.ErrorContains(t, err, "only one todo may be in_progress")
	require.Equal(t, original, sessions.session.Todos)
}

func TestTodosToolSaveFailureKeepsPreviousState(t *testing.T) {
	t.Parallel()

	original := []session.Todo{{Content: "Existing", Status: session.TodoStatusPending}}
	sessions := &mockTodoSessionService{
		session: session.Session{ID: "session-1", Todos: original},
		saveErr: errors.New("database unavailable"),
	}
	tool := NewTodosTool(sessions)
	input, err := json.Marshal(TodosParams{Todos: []TodoItem{{Content: "Replacement", Status: "completed"}}})
	require.NoError(t, err)

	_, err = tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "session-1"), fantasy.ToolCall{
		ID: "todo-call", Name: TodosToolName, Input: string(input),
	})
	require.ErrorContains(t, err, "failed to save todos")
	require.Equal(t, original, sessions.session.Todos)
}
