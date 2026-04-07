package engine

import (
	"context"
	"strings"

	"myclaw/internal/model"
)

type EventType string

const (
	EventTextDelta EventType = "text.delta"
	EventToolCall  EventType = "tool.call"
)

type ModelEvent struct {
	Type      EventType
	Delta     string
	ToolName  string
	ToolInput string
}

type GenerateRequest struct {
	Session        model.Session
	History        []model.Message
	UserMessage    model.Message
	LastToolResult string
}

type Model interface {
	Generate(context.Context, GenerateRequest) ([]ModelEvent, error)
}

type StaticModel func(GenerateRequest) []ModelEvent

func (m StaticModel) Generate(_ context.Context, req GenerateRequest) ([]ModelEvent, error) {
	return m(req), nil
}

type ToolCall struct {
	Name    string
	Input   string
	Session model.Session
}

type ToolExecutor interface {
	Execute(context.Context, ToolCall) (string, error)
}

type ToolExecutorFunc func(context.Context, ToolCall) (string, error)

func (f ToolExecutorFunc) Execute(ctx context.Context, call ToolCall) (string, error) {
	return f(ctx, call)
}

type TurnRequest struct {
	Session     model.Session
	UserMessage model.Message
	History     []model.Message
}

type TurnResult struct {
	History          []model.Message
	AssistantMessage model.Message
	ToolCalls        []ToolCall
	Compacted        bool
}

func collectAssistantContent(events []ModelEvent) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Type == EventTextDelta {
			builder.WriteString(event.Delta)
		}
	}
	return builder.String()
}
