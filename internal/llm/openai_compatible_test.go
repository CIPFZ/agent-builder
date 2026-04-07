package llm

import (
	"testing"
	"time"

	"myclaw/internal/prompt"
	"myclaw/internal/session"
)

func TestBuildChatMessagesNormalizesSummaryRoleToAssistant(t *testing.T) {
	req := GenerateRequest{
		Session: session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{
			ID:        "user-2",
			SessionID: "sess-1",
			Role:      "user",
			Content:   "current question",
			CreatedAt: time.Now().UTC(),
		},
		History: []session.Message{
			{
				ID:        "summary-1",
				SessionID: "sess-1",
				Role:      "summary",
				Content:   "Summary: earlier work happened",
				CreatedAt: time.Now().UTC(),
			},
			{
				ID:        "user-2",
				SessionID: "sess-1",
				Role:      "user",
				Content:   "current question",
				CreatedAt: time.Now().UTC(),
			},
		},
		Context: prompt.Context{
			SystemPrompt: "You are myclaw.",
			UserInput:    "current question",
		},
	}

	messages := buildChatMessages(req)
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("history role = %q, want assistant for summary compatibility", messages[1].Role)
	}
	if messages[1].Content != "Summary: earlier work happened" {
		t.Fatalf("history content = %q, want summary content preserved", messages[1].Content)
	}
}
