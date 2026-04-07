package engine_test

import (
	"context"
	"testing"

	"myclaw/internal/agent"
	"myclaw/internal/compaction"
	"myclaw/internal/engine"
	"myclaw/internal/model"
	"myclaw/internal/permissions"
)

func TestEngineRunTurnHandlesToolLoopAndCompaction(t *testing.T) {
	llm := engine.StaticModel(func(req engine.GenerateRequest) []engine.ModelEvent {
		if req.LastToolResult == "" {
			return []engine.ModelEvent{
				{Type: engine.EventToolCall, ToolName: "text.upper", ToolInput: "hello claw"},
			}
		}
		return []engine.ModelEvent{
			{Type: engine.EventTextDelta, Delta: "done"},
			{Type: engine.EventTextDelta, Delta: " after tool"},
		}
	})

	toolExecutor := engine.ToolExecutorFunc(func(_ context.Context, call engine.ToolCall) (string, error) {
		if call.Name != "text.upper" {
			t.Fatalf("unexpected tool name %q", call.Name)
		}
		if call.Input != "hello claw" {
			t.Fatalf("unexpected tool input %q", call.Input)
		}
		return "HELLO CLAW", nil
	})

	eng := engine.New(engine.Config{
		Model: llm,
		Tools: toolExecutor,
		Permissions: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
		},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:        2,
			PreserveRecentTurns: 1,
			SummaryPrefix:      "Summary:",
		}),
		Subagents: agent.NewManager(),
	})

	history := []model.Message{
		{ID: "msg-1", SessionID: "main-000001", Role: "user", Content: "old question"},
		{ID: "msg-2", SessionID: "main-000001", Role: "assistant", Content: "old answer"},
	}

	result, err := eng.RunTurn(context.Background(), engine.TurnRequest{
		Session: model.Session{
			ID:      "main-000001",
			Key:     "agent:main:main",
			AgentID: "main",
			IsMain:  true,
		},
		UserMessage: model.Message{
			ID:        "msg-3",
			SessionID: "main-000001",
			Role:      "user",
			Content:   "please use a tool",
		},
		History: history,
	})
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if result.AssistantMessage.Content != "done after tool" {
		t.Fatalf("assistant content = %q, want %q", result.AssistantMessage.Content, "done after tool")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(result.ToolCalls))
	}
	if !result.Compacted {
		t.Fatal("expected compaction to trigger")
	}
	if len(result.History) < 2 || result.History[0].Role != "summary" {
		t.Fatalf("history = %#v, want summary boundary at the front", result.History)
	}
}

func TestEngineRunTurnBlocksDeniedTool(t *testing.T) {
	llm := engine.StaticModel(func(req engine.GenerateRequest) []engine.ModelEvent {
		return []engine.ModelEvent{
			{Type: engine.EventToolCall, ToolName: "system.run", ToolInput: "pwd"},
		}
	})

	eng := engine.New(engine.Config{
		Model: llm,
		Tools: engine.ToolExecutorFunc(func(_ context.Context, call engine.ToolCall) (string, error) {
			t.Fatalf("tool should not execute when denied: %#v", call)
			return "", nil
		}),
		Permissions: permissions.Policy{
			Mode: permissions.ModeAsk,
		},
		Compactor: compaction.NewService(compaction.Config{MaxMessages: 99, PreserveRecentTurns: 2}),
		Subagents: agent.NewManager(),
	})

	_, err := eng.RunTurn(context.Background(), engine.TurnRequest{
		Session: model.Session{
			ID:      "main-000001",
			Key:     "agent:main:main",
			AgentID: "main",
			IsMain:  true,
		},
		UserMessage: model.Message{
			ID:        "msg-1",
			SessionID: "main-000001",
			Role:      "user",
			Content:   "run pwd",
		},
	})
	if err == nil {
		t.Fatal("expected denied tool request to return an error")
	}
}
