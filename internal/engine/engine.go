package engine

import (
	"context"
	"fmt"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/compaction"
	"myclaw/internal/model"
	"myclaw/internal/permissions"
)

type Config struct {
	Model       Model
	Tools       ToolExecutor
	Permissions permissions.Policy
	Compactor   *compaction.Service
	Subagents   *agent.Manager
}

type Engine struct {
	model       Model
	tools       ToolExecutor
	permissions permissions.Policy
	compactor   *compaction.Service
	subagents   *agent.Manager
}

func New(cfg Config) *Engine {
	return &Engine{
		model:       cfg.Model,
		tools:       cfg.Tools,
		permissions: cfg.Permissions,
		compactor:   cfg.Compactor,
		subagents:   cfg.Subagents,
	}
}

func (e *Engine) RunTurn(ctx context.Context, req TurnRequest) (TurnResult, error) {
	if e.model == nil {
		return TurnResult{}, fmt.Errorf("engine model is required")
	}

	history := append(cloneMessages(req.History), req.UserMessage)
	compactedHistory := history
	compacted := false
	if e.compactor != nil {
		compactedHistory, compacted = e.compactor.CompactIfNeeded(history)
	}

	modelReq := GenerateRequest{
		Session:     req.Session,
		History:     compactedHistory,
		UserMessage: req.UserMessage,
	}
	events, err := e.model.Generate(ctx, modelReq)
	if err != nil {
		return TurnResult{}, err
	}

	toolCalls := make([]ToolCall, 0, 1)
	lastToolResult := ""
	for {
		call, ok := firstToolCall(req.Session, events)
		if !ok {
			break
		}

		decision := e.permissions.Evaluate(permissions.Request{
			ToolName: call.Name,
			Command:  call.Input,
			WorkDir:  req.Session.Key,
		})
		if !decision.Allowed {
			return TurnResult{}, fmt.Errorf("tool %s requires approval: %s", call.Name, decision.Reason)
		}
		if e.tools == nil {
			return TurnResult{}, fmt.Errorf("tool executor is required")
		}

		toolCalls = append(toolCalls, call)
		lastToolResult, err = e.tools.Execute(ctx, call)
		if err != nil {
			return TurnResult{}, err
		}

		modelReq.LastToolResult = lastToolResult
		events, err = e.model.Generate(ctx, modelReq)
		if err != nil {
			return TurnResult{}, err
		}
	}

	assistant := model.Message{
		ID:        "assistant-1",
		SessionID: req.Session.ID,
		Role:      "assistant",
		Content:   collectAssistantContent(events),
		CreatedAt: time.Now().UTC(),
	}
	finalHistory := append(cloneMessages(compactedHistory), assistant)

	return TurnResult{
		History:          finalHistory,
		AssistantMessage: assistant,
		ToolCalls:        toolCalls,
		Compacted:        compacted,
	}, nil
}

func firstToolCall(sess model.Session, events []ModelEvent) (ToolCall, bool) {
	for _, event := range events {
		if event.Type == EventToolCall {
			return ToolCall{
				Name:    event.ToolName,
				Input:   event.ToolInput,
				Session: sess,
			}, true
		}
	}
	return ToolCall{}, false
}

func cloneMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	copy(out, messages)
	return out
}
