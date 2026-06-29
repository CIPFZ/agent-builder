package contextmgr

import (
	"context"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestMicrocompactCountTriggerClearsOldResultsAndKeepsRecent(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	canonical := []message.Message{{
		ID:   "canonical-tool",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{ToolCallID: "tool-1", Name: "bash", Content: "canonical old output"},
		},
	}}
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		Messages:      canonical,
		ModelMessages: microcompactMessages("old-1", "old-2", "recent-1"),
		Microcompact: MicrocompactConfig{
			Enabled:              true,
			ToolResultCountLimit: 2,
			KeepRecent:           1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Boundaries) != 1 || result.Boundaries[0].Kind != "micro" || result.Boundaries[0].Trigger != "tool_result_count" {
		t.Fatalf("boundaries = %#v", result.Boundaries)
	}
	if len(result.Replacements) != 2 {
		t.Fatalf("replacements = %#v", result.Replacements)
	}
	first := mustToolResultText(t, result.ModelMessages[0])
	second := mustToolResultText(t, result.ModelMessages[1])
	recent := mustToolResultText(t, result.ModelMessages[2])
	if !strings.Contains(first, "microcompact") || !strings.Contains(second, "microcompact") {
		t.Fatalf("old results were not compacted: %q %q", first, second)
	}
	if recent != "recent-1" {
		t.Fatalf("recent result should be preserved, got %q", recent)
	}
	if canonical[0].ToolResults()[0].Content != "canonical old output" {
		t.Fatalf("canonical history was mutated: %#v", canonical)
	}
	if result.Replacements[0].OriginalRef == "" {
		t.Fatalf("original ref missing: %#v", result.Replacements[0])
	}
}

func TestMicrocompactTimeAndTokenTriggers(t *testing.T) {
	t.Parallel()

	t.Run("idle time", func(t *testing.T) {
		_, manager := testContextManager(t)
		result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
			SessionID:     "session-1",
			TurnID:        "turn-time",
			Step:          1,
			ModelMessages: microcompactMessages("old", "recent"),
			Microcompact: MicrocompactConfig{
				Enabled:            true,
				IdleIntervalMillis: 10,
				LastAssistantAt:    1,
				KeepRecent:         1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Boundaries) != 1 || result.Boundaries[0].Trigger != "idle_time" {
			t.Fatalf("time trigger boundaries = %#v", result.Boundaries)
		}
	})

	t.Run("token", func(t *testing.T) {
		_, manager := testContextManager(t)
		result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
			SessionID:     "session-1",
			TurnID:        "turn-token",
			Step:          1,
			ModelMessages: microcompactMessages(strings.Repeat("a", 100), "recent"),
			Microcompact: MicrocompactConfig{
				Enabled:              true,
				ToolResultTokenLimit: 10,
				KeepRecent:           1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Boundaries) != 1 || result.Boundaries[0].Trigger != "tool_result_tokens" {
			t.Fatalf("token trigger boundaries = %#v", result.Boundaries)
		}
	})
}

func microcompactMessages(outputs ...string) []fantasy.Message {
	messages := make([]fantasy.Message, 0, len(outputs))
	for i, output := range outputs {
		messages = append(messages, fantasy.Message{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "tool-" + string(rune('a'+i)),
					Output:     fantasy.ToolResultOutputContentText{Text: output},
				},
			},
		})
	}
	return messages
}
