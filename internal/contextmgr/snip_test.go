package contextmgr

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestSnipProjectionExcludesMiddleAndKeepsMarker(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	canonical := []message.Message{
		{ID: "m1", Role: message.User},
		{ID: "m2", Role: message.Assistant},
		{ID: "m3", Role: message.User},
		{ID: "m4", Role: message.Assistant},
	}
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		Messages:      canonical,
		ModelMessages: textMessages("head", "snipped-1", "snipped-2", "tail"),
		Snip: SnipConfig{
			Enabled:              true,
			PreserveHeadMessages: 1,
			PreserveTailMessages: 1,
			Summary:              "middle history snipped",
			Reason:               "manual",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SnipBoundaries) != 1 || len(result.SnipBoundaries[0].RemovedMessageRefs) != 2 {
		t.Fatalf("snip boundaries = %#v", result.SnipBoundaries)
	}
	if promptContains(result.ModelMessages, "snipped-1") || promptContains(result.ModelMessages, "snipped-2") {
		t.Fatalf("snipped messages remained in projection: %#v", result.ModelMessages)
	}
	if !promptContains(result.ModelMessages, "head") || !promptContains(result.ModelMessages, "tail") || !promptContains(result.ModelMessages, "middle history snipped") {
		t.Fatalf("head/tail/marker missing: %#v", result.ModelMessages)
	}
	if len(canonical) != 4 || canonical[1].ID != "m2" {
		t.Fatalf("canonical history mutated: %#v", canonical)
	}
}

func TestSnipProjectionReplayIsStable(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	req := BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: textMessages("head", "drop", "tail"),
		Snip: SnipConfig{
			Enabled:              true,
			PreserveHeadMessages: 1,
			PreserveTailMessages: 1,
			Summary:              "stable snip",
		},
	}
	first, err := manager.BuildModelInput(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.BuildModelInput(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ModelMessages) != len(second.ModelMessages) || !promptContains(second.ModelMessages, "stable snip") {
		t.Fatalf("replayed snip mismatch first=%#v second=%#v", first.ModelMessages, second.ModelMessages)
	}
}

func TestSnipProjectionKeepsToolPairsWhenTailContainsPair(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	messages := []fantasy.Message{
		fantasy.NewUserMessage("head"),
		fantasy.NewUserMessage("snipped"),
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{ToolCallID: "tool-1", ToolName: "bash", Input: "{}"},
			},
		},
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{ToolCallID: "tool-1", Output: fantasy.ToolResultOutputContentText{Text: "tail result"}},
			},
		},
	}
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: messages,
		Snip: SnipConfig{
			Enabled:              true,
			PreserveHeadMessages: 1,
			PreserveTailMessages: 2,
			Summary:              "snipped one message",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var callSeen, resultSeen bool
	for _, msg := range result.ModelMessages {
		for _, part := range msg.Content {
			if call, ok := part.(fantasy.ToolCallPart); ok && call.ToolCallID == "tool-1" {
				callSeen = true
			}
			if tr, ok := part.(fantasy.ToolResultPart); ok && tr.ToolCallID == "tool-1" {
				resultSeen = true
			}
		}
	}
	if !callSeen || !resultSeen {
		t.Fatalf("tool pair not preserved: %#v", result.ModelMessages)
	}
}
