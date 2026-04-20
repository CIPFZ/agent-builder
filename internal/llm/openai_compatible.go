package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/prompt"
	"myclaw/internal/session"
)

type OpenAICompatibleConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	GlobalProxy  ProxySettings
	Proxy        ProxySettings
	Headers      map[string]string
	Timeout      time.Duration
	MaxRetries   int
	AuthScheme   string
	APIKeyHeader string
	Organization string
}

type OpenAICompatibleClient struct {
	baseURL      string
	apiKey       string
	model        string
	headers      map[string]string
	maxRetries   int
	authScheme   string
	apiKeyHeader string
	organization string
	httpClient   *http.Client
}

func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) *OpenAICompatibleClient {
	authScheme := strings.TrimSpace(cfg.AuthScheme)
	if authScheme == "" {
		authScheme = "Bearer"
	}
	apiKeyHeader := strings.TrimSpace(cfg.APIKeyHeader)
	if apiKeyHeader == "" {
		apiKeyHeader = "Authorization"
	}
	return &OpenAICompatibleClient{
		baseURL:      cfg.BaseURL,
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		headers:      copyHeaders(cfg.Headers),
		maxRetries:   cfg.MaxRetries,
		authScheme:   authScheme,
		apiKeyHeader: apiKeyHeader,
		organization: strings.TrimSpace(cfg.Organization),
		httpClient:   mustNewHTTPClient(cfg.Timeout, cfg.GlobalProxy, cfg.Proxy),
	}
}

func (c *OpenAICompatibleClient) Stream(ctx context.Context, req GenerateRequest, handler StreamHandler) error {
	payload := openAIChatRequest{
		Model:      effectiveModel(req.Model, c.model),
		Messages:   buildChatMessages(req),
		Tools:      buildChatTools(req.Tools),
		ToolChoice: toolChoice(req.Tools),
		Stream:     true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	headers := copyHeaders(c.headers)
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "text/event-stream"
	if c.apiKey != "" {
		if strings.EqualFold(c.apiKeyHeader, "Authorization") {
			headers[c.apiKeyHeader] = c.authScheme + " " + c.apiKey
		} else {
			headers[c.apiKeyHeader] = c.apiKey
		}
	}
	if c.organization != "" {
		headers["OpenAI-Organization"] = c.organization
	}
	resp, err := doStreamingRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, body, headers, c.maxRetries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return consumeSSE(resp.Body, handler)
}

func effectiveModel(requestModel, defaultModel string) string {
	if strings.TrimSpace(requestModel) != "" {
		return strings.TrimSpace(requestModel)
	}
	return defaultModel
}

type openAIChatRequest struct {
	Model      string              `json:"model"`
	Messages   []openAIChatMessage `json:"messages"`
	Tools      []openAIChatTool    `json:"tools,omitempty"`
	ToolChoice string              `json:"tool_choice,omitempty"`
	Stream     bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIChatTool struct {
	Type     string             `json:"type"`
	Function openAIFunctionTool `json:"function"`
}

type openAIFunctionTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIToolCallFunction `json:"function"`
}

type openAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content   string                `json:"content"`
			ToolCalls []openAIToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func buildChatMessages(req GenerateRequest) []openAIChatMessage {
	messages := []openAIChatMessage{
		{
			Role:    "system",
			Content: buildSystemContent(req),
		},
	}

	for _, msg := range req.History {
		if msg.ID == req.UserMessage.ID {
			continue
		}
		messages = append(messages, buildHistoryChatMessage(msg))
	}

	messages = append(messages, openAIChatMessage{
		Role:    "user",
		Content: req.Context.UserInput,
	})

	return ensureOpenAIToolResultPairing(messages)
}

func buildHistoryChatMessage(msg session.Message) openAIChatMessage {
	if msg.Role == "assistant" {
		if toolCalls := openAIToolCallsFromBlocks(msg.Blocks); len(toolCalls) > 0 {
			return openAIChatMessage{
				Role:      "assistant",
				Content:   textContentFromBlocks(msg.Content, msg.Blocks),
				ToolCalls: toolCalls,
			}
		}
	}
	if msg.Role == "tool" {
		toolUseID, content := openAIToolResultFromBlocks(msg)
		return openAIChatMessage{
			Role:       "tool",
			ToolCallID: toolUseID,
			Content:    content,
		}
	}
	return openAIChatMessage{
		Role:    normalizeOpenAIChatRole(msg.Role),
		Content: msg.Content,
	}
}

const syntheticToolResultPlaceholder = "[Tool result missing due to internal error]"
const orphanedToolResultPlaceholder = "[Orphaned tool result removed due to conversation resume]"

func ensureOpenAIToolResultPairing(messages []openAIChatMessage) []openAIChatMessage {
	if len(messages) == 0 {
		return nil
	}
	result := make([]openAIChatMessage, 0, len(messages))
	seenToolUseIDs := make(map[string]struct{})
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != "assistant" {
			if msg.Role == "tool" {
				if len(result) == 0 || result[len(result)-1].Role != "assistant" {
					if len(result) == 1 && result[0].Role == "system" {
						result = append(result, openAIChatMessage{Role: "user", Content: orphanedToolResultPlaceholder})
					}
					continue
				}
			}
			result = append(result, msg)
			continue
		}

		toolCalls := dedupeOpenAIToolCalls(msg.ToolCalls, seenToolUseIDs)
		msg.ToolCalls = toolCalls
		result = append(result, msg)
		if len(toolCalls) == 0 {
			continue
		}

		expectedIDs := make(map[string]struct{}, len(toolCalls))
		for _, call := range toolCalls {
			if call.ID != "" {
				expectedIDs[call.ID] = struct{}{}
			}
		}
		seenResults := make(map[string]struct{}, len(expectedIDs))
		insertAt := len(result)
		for i+1 < len(messages) && messages[i+1].Role == "tool" {
			next := messages[i+1]
			i++
			if _, ok := expectedIDs[next.ToolCallID]; !ok {
				continue
			}
			if _, ok := seenResults[next.ToolCallID]; ok {
				continue
			}
			seenResults[next.ToolCallID] = struct{}{}
			result = append(result, next)
		}
		for _, call := range toolCalls {
			if _, ok := seenResults[call.ID]; ok {
				continue
			}
			synthetic := openAIChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    syntheticToolResultPlaceholder,
			}
			if insertAt == len(result) {
				result = append(result, synthetic)
			} else {
				result = append(result[:insertAt+1], result[insertAt:]...)
				result[insertAt] = synthetic
				insertAt++
			}
		}
	}
	return result
}

func dedupeOpenAIToolCalls(calls []openAIToolCall, seen map[string]struct{}) []openAIToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openAIToolCall, 0, len(calls))
	local := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.ID == "" {
			out = append(out, call)
			continue
		}
		if _, ok := seen[call.ID]; ok {
			continue
		}
		if _, ok := local[call.ID]; ok {
			continue
		}
		local[call.ID] = struct{}{}
		seen[call.ID] = struct{}{}
		out = append(out, call)
	}
	return out
}

func buildChatTools(defs []ToolDefinition) []openAIChatTool {
	if len(defs) == 0 {
		return nil
	}
	tools := make([]openAIChatTool, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		tools = append(tools, openAIChatTool{
			Type: "function",
			Function: openAIFunctionTool{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.InputSchema,
			},
		})
	}
	return tools
}

func toolChoice(defs []ToolDefinition) string {
	if len(defs) == 0 {
		return ""
	}
	return "auto"
}

func openAIToolCallsFromBlocks(blocks []model.MessageBlock) []openAIToolCall {
	var calls []openAIToolCall
	for _, block := range blocks {
		if block.Type != model.MessageBlockToolUse {
			continue
		}
		calls = append(calls, openAIToolCall{
			ID:   block.ID,
			Type: "function",
			Function: openAIToolCallFunction{
				Name:      block.Name,
				Arguments: toolArguments(block),
			},
		})
	}
	return calls
}

func toolArguments(block model.MessageBlock) string {
	if block.InputObject != nil {
		data, err := json.Marshal(block.InputObject)
		if err == nil {
			return string(data)
		}
	}
	return block.Input
}

func openAIToolResultFromBlocks(msg session.Message) (string, string) {
	for _, block := range msg.Blocks {
		if block.Type == model.MessageBlockToolResult {
			return block.ToolUseID, block.Content
		}
	}
	return "", msg.Content
}

func textContentFromBlocks(fallback string, blocks []model.MessageBlock) string {
	var parts []string
	for _, block := range blocks {
		if block.Type == model.MessageBlockText && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	return fallback
}

func normalizeOpenAIChatRole(role string) string {
	switch strings.TrimSpace(role) {
	case "system", "user", "assistant", "tool":
		return role
	case "summary":
		return "assistant"
	default:
		return "assistant"
	}
}

func buildSystemContent(req GenerateRequest) string {
	return prompt.ComposeSystemContent(req.Context)
}

func consumeSSE(r io.Reader, handler StreamHandler) error {
	scanner := bufio.NewScanner(r)
	pendingToolCalls := make(map[int]*streamingToolCall)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return handler.OnEvent(StreamEvent{Type: "message.end"})
		}

		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}

		for _, choice := range chunk.Choices {
			for _, delta := range choice.Delta.ToolCalls {
				call := pendingToolCalls[delta.Index]
				if call == nil {
					call = &streamingToolCall{}
					pendingToolCalls[delta.Index] = call
				}
				if delta.ID != "" {
					call.id = delta.ID
				}
				if delta.Function.Name != "" {
					call.name = delta.Function.Name
				}
				if delta.Function.Arguments != "" {
					call.arguments.WriteString(delta.Function.Arguments)
				}
				if chunk.ID != "" {
					call.providerMessageID = chunk.ID
				}
			}
			if choice.Delta.Content != "" {
				if err := handler.OnEvent(StreamEvent{
					Type:  "text.delta",
					Delta: choice.Delta.Content,
				}); err != nil {
					return err
				}
			}
			if choice.FinishReason != "" {
				if choice.FinishReason == "tool_calls" {
					if err := emitStreamingToolCalls(handler, pendingToolCalls); err != nil {
						return err
					}
				}
				if err := handler.OnEvent(StreamEvent{Type: "message.end"}); err != nil {
					return err
				}
				return nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return handler.OnEvent(StreamEvent{Type: "message.end"})
}

type streamingToolCall struct {
	id                string
	name              string
	arguments         strings.Builder
	providerMessageID string
}

func emitStreamingToolCalls(handler StreamHandler, calls map[int]*streamingToolCall) error {
	for i := 0; i < len(calls); i++ {
		call := calls[i]
		if call == nil || strings.TrimSpace(call.name) == "" {
			continue
		}
		input := call.arguments.String()
		return handler.OnEvent(StreamEvent{
			Type:              "tool.call",
			ToolName:          call.name,
			ToolInput:         input,
			ToolInputObject:   decodeToolArguments(input),
			ToolUseID:         call.id,
			ProviderMessageID: call.providerMessageID,
		})
	}
	return nil
}

func decodeToolArguments(input string) map[string]any {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(input), &object); err != nil {
		return nil
	}
	return object
}
