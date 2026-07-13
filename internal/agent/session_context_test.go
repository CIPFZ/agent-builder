package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/stretchr/testify/require"
)

func TestCreateUserMessagePersistsClientRequestID(t *testing.T) {
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "metadata")
	require.NoError(t, err)

	agent := &sessionAgent{messages: env.messages}
	created, err := agent.createUserMessage(t.Context(), SessionAgentCall{
		SessionID:       sess.ID,
		Prompt:          "hello",
		MessageMetadata: map[string]string{"clientRequestId": "prompt-123"},
	})
	require.NoError(t, err)
	require.Equal(t, "prompt-123", created.Metadata["clientRequestId"])

	persisted, err := env.messages.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, message.User, persisted.Role)
	require.Equal(t, "prompt-123", persisted.Metadata["clientRequestId"])
}

func TestWithPromptWorkingDirectoryReplacesWorkspaceDirectory(t *testing.T) {
	base := "Environment:\nWorking directory: C:\\Users\\ytq\\work\\ai\\agent-builder\nPlatform: windows"
	selected := "C:\\Users\\ytq\\work\\ai\\oh-my-claudecode"

	actual := withPromptWorkingDirectory(base, selected)

	require.Contains(t, actual, "Working directory: "+selected)
	require.NotContains(t, actual, "Working directory: C:\\Users\\ytq\\work\\ai\\agent-builder")
	require.Equal(t, 1, strings.Count(actual, "Working directory:"))
}
