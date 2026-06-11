package agent

import (
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func makeTestMessagesWithAge(numAssistant int, lastAssistantAge time.Duration) []message.Message {
	var msgs []message.Message
	for i := 0; i < numAssistant; i++ {
		tc := message.ToolCall{ID: "tc_" + string(rune('a'+i)), Name: "bash", Input: "{}"}
		assistant := message.Message{
			Role:  message.Assistant,
			Parts: []message.ContentPart{tc},
		}
		// Set CreatedAt on the last assistant to simulate idle time
		if i == numAssistant-1 {
			assistant.CreatedAt = time.Now().Add(-lastAssistantAge).Unix()
		} else {
			assistant.CreatedAt = time.Now().Unix()
		}
		msgs = append(msgs, assistant)

		tr := message.ToolResult{
			ToolCallID: tc.ID,
			Name:       "bash",
			Content:    "output for turn " + string(rune('0'+i)),
		}
		tool := message.Message{
			Role:  message.Tool,
			Parts: []message.ContentPart{tr},
		}
		msgs = append(msgs, tool)
	}
	return msgs
}

func TestMicrocompact_NoTriggerWithinInterval(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 3)
	// Last assistant was just now — should not trigger
	msgs := makeTestMessagesWithAge(5, 0)

	result := mc.Compact(msgs)
	require.Equal(t, len(msgs), len(result))
}

func TestMicrocompact_TriggeredAfterInterval(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 3)
	// Last assistant was 10 minutes ago — should trigger
	msgs := makeTestMessagesWithAge(5, 10*time.Minute)

	result := mc.Compact(msgs)
	foundCleared := false
	for _, msg := range result {
		if msg.Role == message.Tool {
			for _, part := range msg.Parts {
				if tr, ok := part.(message.ToolResult); ok {
					if tr.Content == clearedContent {
						foundCleared = true
					}
				}
			}
		}
	}
	require.True(t, foundCleared, "expected at least one cleared tool result")
}

func TestMicrocompact_NonCompactableTool_NotCleared(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 1)

	tc := message.ToolCall{ID: "tc_x", Name: "todos", Input: "{}"}
	assistant := message.Message{
		Role:      message.Assistant,
		Parts:     []message.ContentPart{tc},
		CreatedAt: time.Now().Add(-10 * time.Minute).Unix(),
	}
	tr := message.ToolResult{ToolCallID: "tc_x", Name: "todos", Content: "some todo list"}
	tool := message.Message{Role: message.Tool, Parts: []message.ContentPart{tr}}

	msgs := makeTestMessagesWithAge(3, 10*time.Minute)
	msgs = append([]message.Message{assistant, tool}, msgs...)

	result := mc.Compact(msgs)
	for _, msg := range result {
		if msg.Role == message.Tool {
			for _, part := range msg.Parts {
				if tr, ok := part.(message.ToolResult); ok && tr.Name == "todos" {
					require.NotEqual(t, clearedContent, tr.Content,
						"non-compactable tool should not be cleared")
				}
			}
		}
	}
}

func TestMicrocompact_EmptyMessages(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 3)
	result := mc.Compact([]message.Message{})
	require.Empty(t, result)
}

func TestMicrocompact_AllRecent_NoneCleared(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 5)
	// 3 assistants, but keepLastAssistants=5 — nothing should be cleared
	msgs := makeTestMessagesWithAge(3, 10*time.Minute)

	result := mc.Compact(msgs)
	foundCleared := false
	for _, msg := range result {
		if msg.Role == message.Tool {
			for _, part := range msg.Parts {
				if tr, ok := part.(message.ToolResult); ok {
					if tr.Content == clearedContent {
						foundCleared = true
					}
				}
			}
		}
	}
	require.False(t, foundCleared, "no results should be cleared when all are recent")
}

func TestMicrocompact_NoAssistant_NoPanic(t *testing.T) {
	mc := NewMicrocompact(5*time.Minute, 3)
	// Messages with no assistant role — should not panic
	msgs := []message.Message{
		{Role: message.Tool, Parts: []message.ContentPart{
			message.ToolResult{Name: "bash", Content: "output"},
		}},
	}
	result := mc.Compact(msgs)
	require.Equal(t, len(msgs), len(result))
}
