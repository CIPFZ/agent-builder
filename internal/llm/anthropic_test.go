package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/prompt"
	"myclaw/internal/session"
)

func TestAnthropicClientUsesMessagesAPIHeadersAndStreamsToolUse(t *testing.T) {
	var payload anthropicMessagesRequest
	var apiKeyHeader string
	var versionHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		apiKeyHeader = r.Header.Get("x-api-key")
		versionHeader = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_start\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu-1\",\"name\":\"system.run\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"command\\\":\\\"pwd\\\"}\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewAnthropicClient(AnthropicConfig{
		BaseURL:    server.URL,
		APIKey:     "anthropic-key",
		Model:      "claude-sonnet-4-5",
		APIVersion: "2023-06-01",
	})
	handler := &captureStreamHandler{}

	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "run pwd", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system prompt", UserInput: "run pwd"},
		Tools:       []ToolDefinition{{Name: "system.run", Description: "Run command", InputSchema: map[string]any{"type": "object"}}},
	}, handler)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if apiKeyHeader != "anthropic-key" {
		t.Fatalf("x-api-key = %q, want anthropic-key", apiKeyHeader)
	}
	if versionHeader != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", versionHeader)
	}
	if payload.Model != "claude-sonnet-4-5" {
		t.Fatalf("payload model = %q, want claude-sonnet-4-5", payload.Model)
	}
	if !strings.Contains(payload.System, "system prompt") {
		t.Fatalf("payload system = %q, want composed system content to include system prompt", payload.System)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "system.run" {
		t.Fatalf("payload tools = %#v, want native anthropic tool schema", payload.Tools)
	}
	if len(handler.events) != 3 {
		t.Fatalf("events = %#v, want text.delta, tool.call, message.end", handler.events)
	}
	if handler.events[0].Type != "text.delta" || handler.events[0].Delta != "hello" {
		t.Fatalf("first event = %#v, want text.delta hello", handler.events[0])
	}
	if handler.events[1].Type != "tool.call" || handler.events[1].ToolName != "system.run" || handler.events[1].ToolUseID != "toolu-1" {
		t.Fatalf("tool event = %#v, want anthropic tool call", handler.events[1])
	}
	if handler.events[1].ToolInputObject["command"] != "pwd" {
		t.Fatalf("tool input object = %#v, want decoded anthropic input json", handler.events[1].ToolInputObject)
	}
	if handler.events[1].ProviderMessageID != "msg_123" {
		t.Fatalf("provider message id = %q, want msg_123", handler.events[1].ProviderMessageID)
	}
}

func TestAnthropicClientAppendsMessagesPathForBaseRoot(t *testing.T) {
	var requestPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewAnthropicClient(AnthropicConfig{
		BaseURL: server.URL + "/anthropic",
		APIKey:  "anthropic-key",
		Model:   "claude-sonnet-4-5",
	})

	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "hello", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system prompt", UserInput: "hello"},
	}, discardStreamHandler{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if requestPath != "/anthropic/v1/messages" {
		t.Fatalf("request path = %q, want /anthropic/v1/messages", requestPath)
	}
}

func TestAnthropicClientOmitsSchemaLessToolsFromPayload(t *testing.T) {
	var payload anthropicMessagesRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewAnthropicClient(AnthropicConfig{
		BaseURL: server.URL + "/v1/messages",
		APIKey:  "anthropic-key",
		Model:   "claude-sonnet-4-5",
	})

	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "hello", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system prompt", UserInput: "hello"},
		Tools: []ToolDefinition{
			{Name: "text.upper", Description: "Uppercase text"},
			{Name: "system.run", Description: "Run command", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}},
		},
	}, discardStreamHandler{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(payload.Tools) != 1 || payload.Tools[0].Name != "system.run" {
		t.Fatalf("payload tools = %#v, want only schema-backed tools", payload.Tools)
	}
}

func TestBuildAnthropicMessagesConvertsAssistantToolUseAndToolResultHistory(t *testing.T) {
	req := GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-2", SessionID: "sess-1", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		History: []session.Message{
			{
				ID:        "assistant-1",
				SessionID: "sess-1",
				Role:      "assistant",
				Content:   "calling",
				Blocks: []model.MessageBlock{
					{Type: model.MessageBlockText, Text: "calling"},
					{Type: model.MessageBlockToolUse, ID: "toolu-1", Name: "system.run", InputObject: map[string]any{"command": "pwd"}},
				},
			},
			{
				ID:        "tool-1",
				SessionID: "sess-1",
				Role:      "tool",
				Content:   "C:/repo",
				Blocks:    []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-1", Content: "C:/repo"}},
			},
		},
		Context: prompt.Context{SystemPrompt: "system", UserInput: "continue"},
	}

	messages := buildAnthropicMessages(req)
	if len(messages) != 3 {
		t.Fatalf("messages = %#v, want assistant history, user tool result, current user", messages)
	}
	if messages[0].Role != "assistant" {
		t.Fatalf("assistant role = %q, want assistant", messages[0].Role)
	}
	if len(messages[0].Content) != 2 || messages[0].Content[1].Type != "tool_use" || messages[0].Content[1].ID != "toolu-1" {
		t.Fatalf("assistant content = %#v, want text+tool_use blocks", messages[0].Content)
	}
	if messages[1].Role != "user" || len(messages[1].Content) != 1 || messages[1].Content[0].Type != "tool_result" {
		t.Fatalf("tool result message = %#v, want user tool_result block", messages[1])
	}
	if messages[1].Content[0].ToolUseID != "toolu-1" || messages[1].Content[0].Content != "C:/repo" {
		t.Fatalf("tool result content = %#v, want linked tool result", messages[1].Content[0])
	}
}
