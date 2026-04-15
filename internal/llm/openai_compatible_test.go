package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myclaw/internal/model"
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

func TestOpenAICompatibleClientSendsNativeToolSchemasAndBlocks(t *testing.T) {
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

	client := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: server.URL, APIKey: "test-key", Model: "model"})
	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-2", SessionID: "sess-1", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		History: []session.Message{
			{
				ID:        "assistant-1",
				SessionID: "sess-1",
				Role:      "assistant",
				Content:   "calling",
				Blocks: []model.MessageBlock{{
					Type:        model.MessageBlockToolUse,
					ID:          "toolu-1",
					Name:        "system.run",
					InputObject: map[string]any{"command": "pwd"},
				}},
			},
			{
				ID:        "tool-1",
				SessionID: "sess-1",
				Role:      "tool",
				Content:   "system.run: C:/repo",
				Blocks:    []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-1", Content: "C:/repo"}},
			},
		},
		Context: prompt.Context{SystemPrompt: "system", UserInput: "continue"},
		Tools: []ToolDefinition{{
			Name:        "system.run",
			Description: "Run a command",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"command": map[string]any{"type": "string"}},
				"required":   []string{"command"},
			},
		}},
	}, discardStreamHandler{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "system.run" {
		t.Fatalf("tools = %#v, want native system.run function schema", payload.Tools)
	}
	if payload.ToolChoice != "auto" {
		t.Fatalf("tool_choice = %q, want auto", payload.ToolChoice)
	}
	if _, ok := payload.Tools[0].Function.Parameters["additionalProperties"]; ok {
		t.Fatalf("tool schema = %#v, did not want provider adapter to inject generic additionalProperties", payload.Tools[0].Function.Parameters)
	}
	if len(payload.Messages) < 3 {
		t.Fatalf("messages = %#v, want system/history/current messages", payload.Messages)
	}
	assistant := payload.Messages[1]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "toolu-1" {
		t.Fatalf("assistant message = %#v, want native tool_calls", assistant)
	}
	tool := payload.Messages[2]
	if tool.Role != "tool" || tool.ToolCallID != "toolu-1" || tool.Content != "C:/repo" {
		t.Fatalf("tool message = %#v, want OpenAI tool result message", tool)
	}
}

func TestOpenAICompatibleClientDoesNotSendSchemaForSchemaLessTools(t *testing.T) {
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

	client := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: server.URL, APIKey: "test-key", Model: "model"})
	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "hello", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system", UserInput: "hello"},
		Tools:       []ToolDefinition{{Name: "schema.less", Description: "No schema"}},
	}, discardStreamHandler{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", payload.Tools)
	}
	if payload.Tools[0].Function.Parameters != nil {
		t.Fatalf("schema-less parameters = %#v, want nil rather than generic object fallback", payload.Tools[0].Function.Parameters)
	}
}

func TestOpenAICompatibleClientEmitsNativeToolCallFromStreamingDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"id":"provider-msg","choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu-1","type":"function","function":{"name":"system.run","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: server.URL, APIKey: "test-key", Model: "model"})
	handler := &captureStreamHandler{}
	err := client.Stream(t.Context(), GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "pwd", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system", UserInput: "pwd"},
		Tools:       []ToolDefinition{{Name: "system.run", Description: "Run command"}},
	}, handler)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(handler.events) != 2 {
		t.Fatalf("events = %#v, want tool.call and message.end", handler.events)
	}
	call := handler.events[0]
	if call.Type != "tool.call" || call.ToolName != "system.run" || call.ToolUseID != "toolu-1" || call.ProviderMessageID != "provider-msg" {
		t.Fatalf("tool call = %#v, want native tool call event", call)
	}
	if call.ToolInputObject["command"] != "pwd" {
		t.Fatalf("tool input object = %#v, want decoded JSON arguments", call.ToolInputObject)
	}
}

func TestBuildChatMessagesRepairsOrphanToolResultAtStart(t *testing.T) {
	req := GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		History: []session.Message{
			{
				ID:      "tool-1",
				Role:    "tool",
				Content: "orphan output",
				Blocks:  []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-orphan", Content: "orphan output"}},
			},
			{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		},
		Context: prompt.Context{SystemPrompt: "system", UserInput: "continue"},
	}

	messages := buildChatMessages(req)
	for _, message := range messages {
		if message.Role == "tool" {
			t.Fatalf("messages = %#v, did not want orphan tool result sent to provider", messages)
		}
	}
	if messages[1].Role != "user" {
		t.Fatalf("messages = %#v, want orphan placeholder user message after system", messages)
	}
}

func TestBuildChatMessagesAddsSyntheticResultForMissingToolResult(t *testing.T) {
	req := GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		History: []session.Message{
			{
				ID:      "assistant-1",
				Role:    "assistant",
				Content: "call",
				Blocks:  []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-missing", Name: "Bash", InputObject: map[string]any{"command": "pwd"}}},
			},
			{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		},
		Context: prompt.Context{SystemPrompt: "system", UserInput: "continue"},
	}

	messages := buildChatMessages(req)
	if len(messages) < 3 || messages[2].Role != "tool" || messages[2].ToolCallID != "toolu-missing" {
		t.Fatalf("messages = %#v, want synthetic tool result after assistant tool call", messages)
	}
	if messages[2].Content != "[Tool result missing due to internal error]" {
		t.Fatalf("synthetic tool content = %#v", messages[2].Content)
	}
}

func TestBuildChatMessagesDeduplicatesToolUseAndToolResultIDs(t *testing.T) {
	req := GenerateRequest{
		Session:     session.Session{ID: "sess-1", Key: "main", AgentID: "main"},
		UserMessage: session.Message{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		History: []session.Message{
			{
				ID:      "assistant-1",
				Role:    "assistant",
				Content: "call",
				Blocks: []model.MessageBlock{
					{Type: model.MessageBlockToolUse, ID: "toolu-dup", Name: "Bash", InputObject: map[string]any{"command": "pwd"}},
					{Type: model.MessageBlockToolUse, ID: "toolu-dup", Name: "Bash", InputObject: map[string]any{"command": "pwd"}},
				},
			},
			{
				ID:      "tool-1",
				Role:    "tool",
				Content: "result",
				Blocks: []model.MessageBlock{
					{Type: model.MessageBlockToolResult, ToolUseID: "toolu-dup", Content: "first"},
					{Type: model.MessageBlockToolResult, ToolUseID: "toolu-dup", Content: "second"},
				},
			},
			{ID: "user-2", Role: "user", Content: "continue", CreatedAt: time.Now().UTC()},
		},
		Context: prompt.Context{SystemPrompt: "system", UserInput: "continue"},
	}

	messages := buildChatMessages(req)
	if len(messages[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool calls = %#v, want one deduped tool call", messages[1].ToolCalls)
	}
	toolResults := 0
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == "toolu-dup" {
			toolResults++
		}
	}
	if toolResults != 1 {
		t.Fatalf("messages = %#v, want one deduped tool result", messages)
	}
}

type discardStreamHandler struct{}

func (discardStreamHandler) OnEvent(StreamEvent) error {
	return nil
}

type captureStreamHandler struct {
	events []StreamEvent
}

func (h *captureStreamHandler) OnEvent(event StreamEvent) error {
	h.events = append(h.events, event)
	return nil
}
