package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/session"
)

type AnthropicConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	GlobalProxy  ProxySettings
	Proxy        ProxySettings
	APIVersion   string
	Headers      map[string]string
	Timeout      time.Duration
	MaxRetries   int
	APIKeyHeader string
}

type AnthropicClient struct {
	baseURL      string
	apiKey       string
	model        string
	apiVersion   string
	headers      map[string]string
	maxRetries   int
	httpClient   *http.Client
	apiKeyHeader string
}

type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

func NewAnthropicClient(cfg AnthropicConfig) *AnthropicClient {
	headers := copyHeaders(cfg.Headers)
	apiKeyHeader := strings.TrimSpace(cfg.APIKeyHeader)
	if apiKeyHeader == "" {
		apiKeyHeader = "x-api-key"
	}
	apiVersion := strings.TrimSpace(cfg.APIVersion)
	if apiVersion == "" {
		apiVersion = "2023-06-01"
	}
	return &AnthropicClient{
		baseURL:      resolveAnthropicMessagesURL(cfg.BaseURL),
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		apiVersion:   apiVersion,
		headers:      headers,
		maxRetries:   cfg.MaxRetries,
		httpClient:   mustNewHTTPClient(cfg.Timeout, cfg.GlobalProxy, cfg.Proxy),
		apiKeyHeader: apiKeyHeader,
	}
}

func (c *AnthropicClient) Stream(ctx context.Context, req GenerateRequest, handler StreamHandler) error {
	payload := anthropicMessagesRequest{
		Model:     effectiveModel(req.Model, c.model),
		System:    buildSystemContent(req),
		Messages:  buildAnthropicMessages(req),
		Tools:     buildAnthropicTools(req.Tools),
		Stream:    true,
		MaxTokens: 8192,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	headers := copyHeaders(c.headers)
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "text/event-stream"
	headers[c.apiKeyHeader] = c.apiKey
	headers["anthropic-version"] = c.apiVersion

	resp, err := doStreamingRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, body, headers, c.maxRetries)
	if err != nil {
		return wrapAnthropicError(c.baseURL, err)
	}
	defer resp.Body.Close()

	if err := consumeAnthropicSSE(resp.Body, handler); err != nil {
		return wrapAnthropicError(c.baseURL, err)
	}
	return nil
}

func buildAnthropicMessages(req GenerateRequest) []anthropicMessage {
	messages := make([]anthropicMessage, 0, len(req.History)+1)
	for _, msg := range req.History {
		if msg.ID == req.UserMessage.ID {
			continue
		}
		if mapped, ok := buildAnthropicHistoryMessage(msg); ok {
			messages = append(messages, mapped)
		}
	}
	messages = append(messages, anthropicMessage{
		Role: "user",
		Content: []anthropicContentBlock{{
			Type: "text",
			Text: req.Context.UserInput,
		}},
	})
	return messages
}

func buildAnthropicHistoryMessage(msg session.Message) (anthropicMessage, bool) {
	switch normalizeOpenAIChatRole(msg.Role) {
	case "assistant":
		content := anthropicBlocksFromMessage(msg)
		if len(content) == 0 {
			content = []anthropicContentBlock{{Type: "text", Text: msg.Content}}
		}
		return anthropicMessage{Role: "assistant", Content: content}, true
	case "tool":
		toolUseID, content := openAIToolResultFromBlocks(msg)
		return anthropicMessage{
			Role: "user",
			Content: []anthropicContentBlock{{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   content,
			}},
		}, true
	case "user":
		return anthropicMessage{
			Role: "user",
			Content: []anthropicContentBlock{{
				Type: "text",
				Text: msg.Content,
			}},
		}, true
	default:
		return anthropicMessage{}, false
	}
}

func anthropicBlocksFromMessage(msg session.Message) []anthropicContentBlock {
	if len(msg.Blocks) == 0 {
		if strings.TrimSpace(msg.Content) == "" {
			return nil
		}
		return []anthropicContentBlock{{Type: "text", Text: msg.Content}}
	}
	blocks := make([]anthropicContentBlock, 0, len(msg.Blocks))
	for _, block := range msg.Blocks {
		switch block.Type {
		case model.MessageBlockText:
			if block.Text != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: block.Text})
			}
		case model.MessageBlockToolUse:
			blocks = append(blocks, anthropicContentBlock{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: decodeToolArguments(toolArguments(block)),
			})
		}
	}
	return blocks
}

func buildAnthropicTools(defs []ToolDefinition) []anthropicTool {
	if len(defs) == 0 {
		return nil
	}
	tools := make([]anthropicTool, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == "" || !hasAnthropicToolSchema(def.InputSchema) {
			continue
		}
		tools = append(tools, anthropicTool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
	}
	return tools
}

func hasAnthropicToolSchema(schema map[string]any) bool {
	if len(schema) == 0 {
		return false
	}
	schemaType, _ := schema["type"].(string)
	return strings.EqualFold(strings.TrimSpace(schemaType), "object")
}

func resolveAnthropicMessagesURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	cleanPath := strings.TrimSpace(parsed.Path)
	if strings.HasSuffix(cleanPath, "/v1/messages") {
		return parsed.String()
	}
	parsed.Path = path.Join(cleanPath, "/v1/messages")
	if strings.HasSuffix(raw, "/") && !strings.HasSuffix(parsed.Path, "/messages") {
		parsed.Path += "/"
	}
	return parsed.String()
}

func wrapAnthropicError(endpoint string, err error) error {
	if err == nil {
		return nil
	}
	message := "llm provider connection failed"
	if strings.Contains(strings.ToLower(err.Error()), "unexpected eof") {
		message += ": upstream closed the stream unexpectedly"
	}
	if strings.TrimSpace(endpoint) != "" {
		message += " (" + endpoint + ")"
	}
	return fmt.Errorf("%s: %w", message, err)
}

type anthropicStreamMessageStart struct {
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
}

type anthropicStreamContentStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input any    `json:"input"`
	} `json:"content_block"`
}

type anthropicStreamContentDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type anthropicStreamContentStop struct {
	Index int `json:"index"`
}

type anthropicPendingTool struct {
	id      string
	name    string
	input   bytes.Buffer
	emitted bool
}

func consumeAnthropicSSE(r io.Reader, handler StreamHandler) error {
	scanner := bufio.NewScanner(r)
	var currentEvent string
	providerMessageID := ""
	pendingTools := make(map[int]*anthropicPendingTool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch currentEvent {
		case "message_start":
			var start anthropicStreamMessageStart
			if err := json.Unmarshal([]byte(data), &start); err != nil {
				return err
			}
			providerMessageID = start.Message.ID
		case "content_block_start":
			var start anthropicStreamContentStart
			if err := json.Unmarshal([]byte(data), &start); err != nil {
				return err
			}
			if start.ContentBlock.Type == "tool_use" {
				pendingTools[start.Index] = &anthropicPendingTool{
					id:   start.ContentBlock.ID,
					name: start.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			var delta anthropicStreamContentDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return err
			}
			switch delta.Delta.Type {
			case "text_delta":
				if err := handler.OnEvent(StreamEvent{Type: "text.delta", Delta: delta.Delta.Text}); err != nil {
					return err
				}
			case "input_json_delta":
				if pending := pendingTools[delta.Index]; pending != nil {
					pending.input.WriteString(delta.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			var stop anthropicStreamContentStop
			if err := json.Unmarshal([]byte(data), &stop); err != nil {
				return err
			}
			if err := emitAnthropicToolCall(handler, pendingTools[stop.Index], providerMessageID); err != nil {
				return err
			}
		case "message_stop":
			if err := emitAllAnthropicToolCalls(handler, pendingTools, providerMessageID); err != nil {
				return err
			}
			return handler.OnEvent(StreamEvent{Type: "message.end"})
		case "error":
			return fmt.Errorf("anthropic stream error: %s", data)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := emitAllAnthropicToolCalls(handler, pendingTools, providerMessageID); err != nil {
		return err
	}
	return handler.OnEvent(StreamEvent{Type: "message.end"})
}

func emitAllAnthropicToolCalls(handler StreamHandler, pending map[int]*anthropicPendingTool, providerMessageID string) error {
	for i := 0; i < len(pending); i++ {
		if err := emitAnthropicToolCall(handler, pending[i], providerMessageID); err != nil {
			return err
		}
	}
	for _, tool := range pending {
		if err := emitAnthropicToolCall(handler, tool, providerMessageID); err != nil {
			return err
		}
	}
	return nil
}

func emitAnthropicToolCall(handler StreamHandler, pending *anthropicPendingTool, providerMessageID string) error {
	if pending == nil || pending.emitted || strings.TrimSpace(pending.name) == "" {
		return nil
	}
	pending.emitted = true
	input := pending.input.String()
	return handler.OnEvent(StreamEvent{
		Type:              "tool.call",
		ToolName:          pending.name,
		ToolInput:         input,
		ToolInputObject:   decodeToolArguments(input),
		ToolUseID:         pending.id,
		ProviderMessageID: providerMessageID,
	})
}
