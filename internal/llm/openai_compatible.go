package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"myclaw/internal/prompt"
)

type OpenAICompatibleConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type OpenAICompatibleClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *OpenAICompatibleClient) Stream(ctx context.Context, req GenerateRequest, handler StreamHandler) error {
	payload := openAIChatRequest{
		Model:    effectiveModel(req.Model, c.model),
		Messages: buildChatMessages(req),
		Stream:   true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("llm request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	return consumeSSE(resp.Body, handler)
}

func effectiveModel(requestModel, defaultModel string) string {
	if strings.TrimSpace(requestModel) != "" {
		return strings.TrimSpace(requestModel)
	}
	return defaultModel
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
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
		messages = append(messages, openAIChatMessage{
			Role:    normalizeOpenAIChatRole(msg.Role),
			Content: msg.Content,
		})
	}

	messages = append(messages, openAIChatMessage{
		Role:    "user",
		Content: req.Context.UserInput,
	})

	return messages
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
			if choice.Delta.Content != "" {
				if err := handler.OnEvent(StreamEvent{
					Type:  "text.delta",
					Delta: choice.Delta.Content,
				}); err != nil {
					return err
				}
			}
			if choice.FinishReason != "" {
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
