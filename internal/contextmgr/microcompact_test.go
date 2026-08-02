package contextmgr

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestMicrocompactReactiveTriggerClearsOldResultsAndKeepsRecent(t *testing.T) {
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
			Enabled:    true,
			Trigger:    "reactive",
			MainTurn:   true,
			KeepRecent: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Boundaries) != 1 || result.Boundaries[0].Kind != "micro" || result.Boundaries[0].Trigger != "reactive" {
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
	if !strings.HasPrefix(recent, "recent-1") || strings.Contains(recent, "compacted by projection") {
		t.Fatalf("recent result should be preserved, got %q", recent)
	}
	if canonical[0].ToolResults()[0].Content != "canonical old output" {
		t.Fatalf("canonical history was mutated: %#v", canonical)
	}
	if result.Replacements[0].OriginalRef == "" {
		t.Fatalf("original ref missing: %#v", result.Replacements[0])
	}
}

func TestMicrocompactTimeThreshold(t *testing.T) {
	t.Parallel()

	t.Run("idle time", func(t *testing.T) {
		t.Parallel()

		_, manager := testContextManager(t)
		result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
			SessionID:     "session-1",
			TurnID:        "turn-time",
			Step:          1,
			ModelMessages: microcompactMessages("old", "recent"),
			Microcompact: MicrocompactConfig{
				Enabled:            true,
				Trigger:            "time_based",
				MainTurn:           true,
				IdleIntervalMillis: int64(time.Hour / time.Millisecond),
				LastAssistantAt:    1,
				KeepRecent:         1,
			},
			Now: time.UnixMilli(1 + int64(time.Hour/time.Millisecond)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Boundaries) != 1 || result.Boundaries[0].Trigger != "idle_time" {
			t.Fatalf("time trigger boundaries = %#v", result.Boundaries)
		}
	})

	t.Run("59 minutes does not trigger", func(t *testing.T) {
		t.Parallel()

		_, manager := testContextManager(t)
		result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
			SessionID:     "session-1",
			TurnID:        "turn-59",
			Step:          1,
			ModelMessages: microcompactMessages(strings.Repeat("a", 100), "recent"),
			Microcompact: MicrocompactConfig{
				Enabled:            true,
				Trigger:            "time_based",
				MainTurn:           true,
				IdleIntervalMillis: int64(time.Hour / time.Millisecond),
				LastAssistantAt:    1,
				KeepRecent:         1,
			},
			Now: time.UnixMilli(1 + int64(59*time.Minute/time.Millisecond)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Boundaries) != 0 {
			t.Fatalf("59-minute boundaries = %#v", result.Boundaries)
		}
	})
}

func TestMicrocompactRunsOncePerAssistantIdlePeriodAndOnlyOnMainTurn(t *testing.T) {
	_, manager := testContextManager(t)
	base := BuildInputRequest{
		SessionID: "session-1", Step: 1,
		ModelMessages: microcompactMessages("old", "recent"),
		Microcompact:  MicrocompactConfig{Enabled: true, Trigger: "time_based", MainTurn: true, IdleIntervalMillis: 60_000, LastAssistantAt: 1, KeepRecent: 1},
		Now:           time.UnixMilli(60_001),
	}
	base.TurnID = "turn-first"
	first, err := manager.BuildModelInput(context.Background(), base)
	if err != nil || len(first.Boundaries) != 1 {
		t.Fatalf("first = %#v, err=%v", first.Boundaries, err)
	}
	base.TurnID = "turn-second"
	base.Now = time.UnixMilli(120_001)
	second, err := manager.BuildModelInput(context.Background(), base)
	if err != nil || len(second.Boundaries) != 0 {
		t.Fatalf("second = %#v, err=%v", second.Boundaries, err)
	}
	base.TurnID = "turn-helper"
	base.Microcompact.MainTurn = false
	base.Microcompact.LastAssistantAt = 200_000
	base.Now = time.UnixMilli(300_000)
	helper, err := manager.BuildModelInput(context.Background(), base)
	if err != nil || len(helper.Boundaries) != 0 {
		t.Fatalf("helper = %#v, err=%v", helper.Boundaries, err)
	}
}

func microcompactMessages(outputs ...string) []fantasy.Message {
	messages := make([]fantasy.Message, 0, len(outputs))
	for i, output := range outputs {
		messages = append(messages, fantasy.Message{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "tool-" + string(rune('a'+i)),
					Output:     fantasy.ToolResultOutputContentText{Text: output + " runtime://objects/tool-" + string(rune('a'+i)) + "-object"},
				},
			},
		})
	}
	return messages
}
