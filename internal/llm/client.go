package llm

import (
	"context"
	"fmt"
	"strings"

	"myclaw/internal/prompt"
	"myclaw/internal/session"
)

type GenerateRequest struct {
	Session     session.Session
	UserMessage session.Message
	History     []session.Message
	Context     prompt.Context
	Model       string
}

type GenerateResponse struct {
	Content string
}

type Client interface {
	Stream(context.Context, GenerateRequest, StreamHandler) error
}

type StreamEvent struct {
	Type              string
	Delta             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
}

type StreamHandler interface {
	OnEvent(StreamEvent) error
}

type MockClient struct{}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (c *MockClient) Stream(ctx context.Context, req GenerateRequest, handler StreamHandler) error {
	if toolMessage := latestToolMessage(req.History); toolMessage != nil {
		return streamText(ctx, fmt.Sprintf("Using tool result: %s", toolMessage.Content), handler)
	}

	if name, input, ok := parseMockToolRequest(req.Context.UserInput); ok {
		if err := handler.OnEvent(StreamEvent{
			Type:      "tool.call",
			ToolName:  name,
			ToolInput: input,
		}); err != nil {
			return err
		}
		return handler.OnEvent(StreamEvent{Type: "message.end"})
	}

	content := fmt.Sprintf("Received: %s", req.Context.UserInput)
	if len(req.Context.WorkspaceLines) > 0 {
		content = fmt.Sprintf("%s [workspace:%d]", content, len(req.Context.WorkspaceLines))
	}
	return streamText(ctx, content, handler)
}

func streamText(ctx context.Context, content string, handler StreamHandler) error {
	parts := strings.Split(content, " ")

	for i, part := range parts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		delta := part
		if i > 0 {
			delta = " " + delta
		}
		if err := handler.OnEvent(StreamEvent{
			Type:  "text.delta",
			Delta: delta,
		}); err != nil {
			return err
		}
	}

	return handler.OnEvent(StreamEvent{
		Type: "message.end",
	})
}

func parseMockToolRequest(input string) (string, string, bool) {
	const prefix = "tool upper "
	if strings.HasPrefix(strings.ToLower(input), prefix) {
		return "text.upper", input[len(prefix):], true
	}
	const runPrefix = "tool run "
	if strings.HasPrefix(strings.ToLower(input), runPrefix) {
		return "system.run", input[len(runPrefix):], true
	}
	return "", "", false
}

func latestToolMessage(history []session.Message) *session.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "tool" {
			return &history[i]
		}
	}
	return nil
}
