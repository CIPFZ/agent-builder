package agent

import (
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/stretchr/testify/require"
)

func TestApplyHistoryHygieneDropsEmptyAndThinkingOnlyAssistant(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		{ID: "user-1", Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hello"}}},
		{ID: "assistant-empty", Role: message.Assistant},
		{ID: "assistant-thinking", Role: message.Assistant, Parts: []message.ContentPart{message.ReasoningContent{Thinking: "private scratch"}}},
	}, HistoryHygieneOptions{SupportsImages: true})
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Len(t, result.Messages, 1)
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropEmptyAssistant, "assistant-empty")
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropThinkingOnlyAssistant, "assistant-thinking")
}

func TestApplyHistoryHygieneDropsOrphanAndDuplicateToolResults(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		assistantToolCallMessage("assistant-1", "call-ok", "read"),
		toolResultMessage("tool-1", "call-ok", "first result"),
		toolResultMessage("tool-duplicate", "call-ok", "duplicate result"),
		toolResultMessage("tool-orphan", "call-missing", "orphan result"),
	}, HistoryHygieneOptions{SupportsImages: true})
	require.NoError(t, err)
	require.True(t, result.Changed)

	var results []fantasy.ToolResultPart
	for _, msg := range result.Messages {
		if msg.Role != fantasy.MessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				results = append(results, tr)
			}
		}
	}
	require.Len(t, results, 1)
	require.Equal(t, "call-ok", results[0].ToolCallID)
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropDuplicateToolResult, "tool-duplicate")
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropOrphanToolResult, "tool-orphan")
}

func TestApplyHistoryHygieneInjectsSyntheticToolResultForOrphanToolCall(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		assistantToolCallMessage("assistant-1", "call-orphan", "read"),
		{ID: "user-2", Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "next prompt"}}},
	}, HistoryHygieneOptions{SupportsImages: true})
	require.NoError(t, err)
	require.True(t, result.Changed)

	synthetic := findToolResult(result.Messages, "call-orphan")
	require.NotNil(t, synthetic)
	_, isError := synthetic.Output.(fantasy.ToolResultOutputContentError)
	require.True(t, isError)
	requireDiagnostic(t, result.Diagnostics, historyHygieneInjectSyntheticToolResult, "assistant-1")
}

func TestApplyHistoryHygieneDedupesDuplicateToolUseIDsWithoutDuplicateSyntheticResults(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		{
			ID:   "assistant-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "call-dup", Name: "read", Input: `{}`},
				message.ToolCall{ID: "call-dup", Name: "write", Input: `{}`},
			},
		},
		assistantToolCallMessage("assistant-2", "call-dup", "read"),
	}, HistoryHygieneOptions{SupportsImages: true})
	require.NoError(t, err)
	require.True(t, result.Changed)

	var toolUseCount int
	var syntheticCount int
	for _, msg := range result.Messages {
		for _, part := range msg.Content {
			if tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok && tc.ToolCallID == "call-dup" {
				toolUseCount++
			}
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && tr.ToolCallID == "call-dup" {
				syntheticCount++
			}
		}
	}
	require.Equal(t, 1, toolUseCount)
	require.Equal(t, 1, syntheticCount)
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropDuplicateToolUse, "assistant-1")
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropDuplicateToolUse, "assistant-2")
}

func TestApplyHistoryHygieneStripsUnsupportedHistoricalMedia(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		{
			ID:   "user-image",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "describe"},
				message.BinaryContent{Path: "screen.png", MIMEType: "image/png", Data: []byte("fake")},
			},
		},
	}, HistoryHygieneOptions{SupportsImages: false})
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Len(t, result.Messages, 1)
	require.Len(t, result.Messages[0].Content, 1)
	_, ok := fantasy.AsMessagePart[fantasy.TextPart](result.Messages[0].Content[0])
	require.True(t, ok)
	requireDiagnostic(t, result.Diagnostics, historyHygieneStripUnsupportedMedia, "user-image")
}

func TestApplyHistoryHygieneReordersProviderToolUseBlocks(t *testing.T) {
	msg := fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: "call-1", ToolName: "read", Input: `{}`},
			fantasy.TextPart{Text: "checking"},
			fantasy.ToolCallPart{ToolCallID: "call-2", ToolName: "list", Input: `{}`},
		},
	}
	reordered := reorderProviderToolUseBlocks(msg)
	require.Len(t, reordered.Content, 3)
	text, ok := fantasy.AsMessagePart[fantasy.TextPart](reordered.Content[0])
	require.True(t, ok)
	require.Equal(t, "checking", text.Text)
	firstTool, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](reordered.Content[1])
	require.True(t, ok)
	secondTool, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](reordered.Content[2])
	require.True(t, ok)
	require.Equal(t, "call-1", firstTool.ToolCallID)
	require.Equal(t, "call-2", secondTool.ToolCallID)
}

func TestApplyHistoryHygieneStrictModeErrorsOnChanges(t *testing.T) {
	result, err := ApplyHistoryHygiene([]message.Message{
		toolResultMessage("tool-orphan", "call-missing", "orphan result"),
	}, HistoryHygieneOptions{SupportsImages: true, Strict: true})
	require.Error(t, err)
	require.True(t, result.Changed)
	requireDiagnostic(t, result.Diagnostics, historyHygieneDropOrphanToolResult, "tool-orphan")
}

func assistantToolCallMessage(id, callID, name string) message.Message {
	return message.Message{
		ID:   id,
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: callID, Name: name, Input: `{}`},
		},
	}
}

func toolResultMessage(id, callID, content string) message.Message {
	return message.Message{
		ID:   id,
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{ToolCallID: callID, Name: "read", Content: content},
		},
	}
}

func findToolResult(messages []fantasy.Message, callID string) *fantasy.ToolResultPart {
	for _, msg := range messages {
		for _, part := range msg.Content {
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && tr.ToolCallID == callID {
				return &tr
			}
		}
	}
	return nil
}

func requireDiagnostic(t *testing.T, diagnostics []HistoryHygieneDiagnostic, kind, messageID string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == kind && diagnostic.MessageID == messageID {
			return
		}
	}
	t.Fatalf("diagnostic kind=%q message=%q missing from %#v", kind, messageID, diagnostics)
}
