package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

type mockTodoSessionService struct {
	session session.Session
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
	m.session = sess
	return sess, nil
}

func (m *mockTodoSessionService) UpdateTitleAndUsage(context.Context, string, string, int64, int64, float64) error {
	return nil
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

func TestTodosSpanToolUpdatesTodos(t *testing.T) {
	t.Parallel()

	sessions := &mockTodoSessionService{session: session.Session{ID: "session-1"}}
	tool := NewTodosSpanTool(sessions)
	input, err := json.Marshal(TodosParams{Todos: []TodoItem{{
		Content:    "Write report",
		Status:     "in_progress",
		ActiveForm: "Writing report",
	}}})
	require.NoError(t, err)

	resp, err := tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "session-1"), fantasy.ToolCall{
		ID:    "todo-call",
		Name:  TodosSpanToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Len(t, sessions.session.Todos, 1)
	require.Equal(t, "Write report", sessions.session.Todos[0].Content)
	require.Equal(t, session.TodoStatusInProgress, sessions.session.Todos[0].Status)
}
