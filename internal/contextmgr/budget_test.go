package contextmgr

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestToolResultBudgetReplacesHugeResultAndPersistsDecision(t *testing.T) {
	t.Parallel()

	store, manager := testContextManager(t)
	huge := strings.Repeat("a", 200)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Step:      1,
		ModelMessages: []fantasy.Message{
			{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{ToolCallID: "tool-1", ToolName: "bash", Input: "{}"},
				},
			},
			{
				Role: fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{
					fantasy.ToolResultPart{
						ToolCallID: "tool-1",
						Output:     fantasy.ToolResultOutputContentText{Text: huge},
					},
				},
			},
		},
		ToolResultBudget: ToolResultBudgetConfig{MaxSingleResultChars: 50, MessageBudgetChars: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replacements) != 1 {
		t.Fatalf("replacements = %#v", result.Replacements)
	}
	if result.Replacements[0].OriginalRef == "" || result.Replacements[0].ReplacementText == "" {
		t.Fatalf("replacement missing ref/text: %#v", result.Replacements[0])
	}
	replaced := mustToolResultText(t, result.ModelMessages[1])
	if !strings.Contains(replaced, "<persisted-output>") || !strings.Contains(replaced, result.Replacements[0].OriginalRef) {
		t.Fatalf("model replacement = %q; replacement = %#v", replaced, result.Replacements[0])
	}
	if call := result.ModelMessages[0].Content[0].(fantasy.ToolCallPart); call.ToolCallID != "tool-1" {
		t.Fatalf("tool call id changed: %#v", call)
	}
	if part := result.ModelMessages[1].Content[0].(fantasy.ToolResultPart); part.ToolCallID != "tool-1" {
		t.Fatalf("tool result id changed: %#v", part)
	}
	stored, err := store.GetContentReplacementByToolCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementText != replaced {
		t.Fatalf("stored replacement differs from model replacement")
	}
}

func TestToolResultBudgetReusesReplacementByteIdentical(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	first, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		Step:              1,
		ModelMessages:     toolResultMessages("tool-1", strings.Repeat("a", 200)),
		ToolResultBudget:  ToolResultBudgetConfig{MaxSingleResultChars: 50, MessageBudgetChars: 1000},
		CanonicalMessages: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:         "session-1",
		TurnID:            "turn-2",
		Step:              1,
		ModelMessages:     toolResultMessages("tool-1", strings.Repeat("changed", 50)),
		ToolResultBudget:  ToolResultBudgetConfig{MaxSingleResultChars: 50, MessageBudgetChars: 1000},
		CanonicalMessages: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mustToolResultText(t, first.ModelMessages[1]) != mustToolResultText(t, second.ModelMessages[1]) {
		t.Fatalf("replacement was not byte-identical across resume")
	}
}

func TestToolResultBudgetGroupsParallelResultsByMessage(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Step:      1,
		ModelMessages: []fantasy.Message{
			{
				Role: fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{
					fantasy.ToolResultPart{ToolCallID: "tool-1", Output: fantasy.ToolResultOutputContentText{Text: strings.Repeat("a", 5000)}},
					fantasy.ToolResultPart{ToolCallID: "tool-2", Output: fantasy.ToolResultOutputContentText{Text: strings.Repeat("b", 5000)}},
				},
			},
		},
		ToolResultBudget: ToolResultBudgetConfig{MaxSingleResultChars: 10000, MessageBudgetChars: 8000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replacements) != 1 || result.Replacements[0].ToolCallID != "tool-1" {
		t.Fatalf("aggregate replacement = %#v", result.Replacements)
	}
	first := result.ModelMessages[0].Content[0].(fantasy.ToolResultPart)
	second := result.ModelMessages[0].Content[1].(fantasy.ToolResultPart)
	if first.ToolCallID != "tool-1" || second.ToolCallID != "tool-2" {
		t.Fatalf("tool result ids changed: %#v %#v", first, second)
	}
}

func testContextManager(t *testing.T) (SQLStore, *DefaultManager) {
	t.Helper()
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	store := NewSQLStore(conn)
	manager := NewManager(ManagerOptions{
		Store: store,
		Now:   func() time.Time { return time.UnixMilli(1000).UTC() },
	})
	return store, manager
}

func toolResultMessages(toolCallID, output string) []fantasy.Message {
	return []fantasy.Message{
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{ToolCallID: toolCallID, ToolName: "bash", Input: "{}"},
			},
		},
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{ToolCallID: toolCallID, Output: fantasy.ToolResultOutputContentText{Text: output}},
			},
		},
	}
}

func mustToolResultText(t *testing.T, msg fantasy.Message) string {
	t.Helper()
	for _, part := range msg.Content {
		if tr, ok := part.(fantasy.ToolResultPart); ok {
			if out, ok := tr.Output.(fantasy.ToolResultOutputContentText); ok {
				return out.Text
			}
		}
	}
	t.Fatalf("message has no text tool result: %#v", msg)
	return ""
}
