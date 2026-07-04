package runtime

import (
	"testing"

	"charm.land/fantasy"
)

func fantasyUser(text string) fantasy.Message {
	return fantasy.NewUserMessage(text)
}

func fantasySystem(text string) fantasy.Message {
	return fantasy.NewSystemMessage(text)
}

func fantasyAssistantText(text string) fantasy.Message {
	return fantasy.Message{
		Role:    fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: text}},
	}
}

func fantasyAssistantCalls(ids ...string) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fantasy.ToolCallPart{ToolCallID: id, ToolName: "tool_" + id, Input: "{}"})
	}
	return fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: parts}
}

func fantasyToolResult(ids ...string) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fantasy.ToolResultPart{
			ToolCallID: id,
			Output:     fantasy.ToolResultOutputContentText{Text: "result " + id},
		})
	}
	return fantasy.Message{Role: fantasy.MessageRoleTool, Content: parts}
}

func TestPairSafeTailStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []fantasy.Message
		minTail  int
		want     int
	}{
		{
			name:     "empty",
			messages: nil,
			minTail:  6,
			want:     0,
		},
		{
			name: "plain conversation cuts at min tail",
			messages: []fantasy.Message{
				fantasyUser("u0"), fantasyAssistantText("a1"),
				fantasyUser("u2"), fantasyAssistantText("a3"),
				fantasyUser("u4"), fantasyAssistantText("a5"),
				fantasyUser("u6"), fantasyAssistantText("a7"),
			},
			minTail: 4,
			want:    4,
		},
		{
			name: "orphan tool_result keeps candidate start",
			messages: []fantasy.Message{
				fantasyUser("u0"),
				fantasyAssistantText("a1"),
				fantasyUser("u2"),
				fantasyToolResult("never-called"),
				fantasyUser("u4"),
				fantasyAssistantText("a5"),
			},
			minTail: 4,
			want:    2,
		},
		{
			name: "tool_result pulls start back to its tool_use assistant",
			messages: []fantasy.Message{
				fantasyUser("u0"),
				fantasyAssistantCalls("c1"),
				fantasyToolResult("c1"),
				fantasyUser("u3"),
				fantasyAssistantCalls("c2"),
				fantasyUser("u5"),
				fantasyToolResult("c2"),
				fantasyAssistantText("a7"),
			},
			minTail: 3,
			want:    4,
		},
		{
			name: "multi call assistant with results across multiple messages",
			messages: []fantasy.Message{
				fantasyUser("u0"),
				fantasyAssistantCalls("c1"),
				fantasyToolResult("c1"),
				fantasyAssistantCalls("c2", "c3"),
				fantasyToolResult("c2"),
				fantasyToolResult("c3"),
				fantasyUser("u6"),
			},
			minTail: 2,
			want:    3,
		},
		{
			name: "trailing dangling tool_use excluded from quota",
			messages: []fantasy.Message{
				fantasyUser("u0"), fantasyAssistantText("a1"),
				fantasyUser("u2"), fantasyAssistantText("a3"),
				fantasyUser("u4"), fantasyAssistantText("a5"),
				fantasyUser("u6"), fantasyAssistantText("a7"),
				fantasyAssistantCalls("dangling"),
			},
			minTail: 3,
			want:    5,
		},
		{
			name: "everything in one pairing chain degenerates to zero",
			messages: []fantasy.Message{
				fantasyAssistantCalls("c1"),
				fantasyToolResult("c1"),
				fantasyAssistantCalls("c2"),
				fantasyToolResult("c2"),
			},
			minTail: 3,
			want:    0,
		},
		{
			name: "leading system messages never counted as tail",
			messages: []fantasy.Message{
				fantasySystem("prefix"),
				fantasySystem("system prompt"),
				fantasyUser("u2"),
				fantasyAssistantCalls("c1"),
				fantasyToolResult("c1"),
				fantasyUser("u5"),
				fantasyAssistantCalls("c2"),
				fantasyToolResult("c2"),
				fantasyAssistantText("a8"),
			},
			minTail: 4,
			want:    5,
		},
		{
			name: "tail shorter than history behind leading system degenerates",
			messages: []fantasy.Message{
				fantasySystem("prefix"),
				fantasySystem("system prompt"),
				fantasyUser("u2"),
				fantasyAssistantText("a3"),
			},
			minTail: 6,
			want:    0,
		},
		{
			name: "mid list system message is not a cut point",
			messages: []fantasy.Message{
				fantasyUser("u0"),
				fantasyAssistantText("a1"),
				fantasySystem("mid system"),
				fantasyUser("u3"),
				fantasyAssistantText("a4"),
				fantasyUser("u5"),
			},
			minTail: 4,
			want:    1,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pairSafeTailStart(tt.messages, tt.minTail)
			if got != tt.want {
				t.Fatalf("pairSafeTailStart = %d, want %d", got, tt.want)
			}
			if got < 0 || got > len(tt.messages) {
				t.Fatalf("pairSafeTailStart out of range: %d", got)
			}
			// The retained tail must be pairing safe: every result's call is
			// inside the tail as well.
			callIn := map[string]bool{}
			for _, msg := range tt.messages[got:] {
				for _, id := range toolCallIDsOfMessage(msg) {
					callIn[id] = true
				}
			}
			allCalls := map[string]bool{}
			for _, msg := range tt.messages {
				for _, id := range toolCallIDsOfMessage(msg) {
					allCalls[id] = true
				}
			}
			for _, msg := range tt.messages[got:] {
				for _, id := range toolResultIDsOfMessage(msg) {
					if allCalls[id] && !callIn[id] {
						t.Fatalf("tool result %q retained without its tool_use (start=%d)", id, got)
					}
				}
			}
		})
	}
}

func TestTrimTrailingDanglingToolCalls(t *testing.T) {
	t.Parallel()

	messages := []fantasy.Message{
		fantasyUser("u0"),
		fantasyAssistantCalls("c1"),
		fantasyToolResult("c1"),
		fantasyAssistantCalls("dangling-a"),
		fantasyAssistantCalls("dangling-b"),
	}
	trimmed := trimTrailingDanglingToolCalls(messages)
	if len(trimmed) != 3 {
		t.Fatalf("trimmed length = %d, want 3", len(trimmed))
	}
	complete := []fantasy.Message{
		fantasyUser("u0"),
		fantasyAssistantCalls("c1"),
		fantasyToolResult("c1"),
	}
	if got := trimTrailingDanglingToolCalls(complete); len(got) != len(complete) {
		t.Fatalf("complete conversation should not be trimmed: %d", len(got))
	}
}
