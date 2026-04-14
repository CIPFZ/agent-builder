package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestOpenAICompatibleClientPrefersRequestModelOverride(t *testing.T) {
	var payload openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "default-model",
	})

	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "hello", CreatedAt: time.Now().UTC()},
		Context: prompt.Context{
			SystemPrompt: "system",
			UserInput:    "hello",
		},
		Model: "override-model",
	}, discardStreamHandler{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if payload.Model != "override-model" {
		t.Fatalf("payload model = %q, want request override", payload.Model)
	}
}

type discardStreamHandler struct{}

func (discardStreamHandler) OnEvent(StreamEvent) error {
	return nil
}
