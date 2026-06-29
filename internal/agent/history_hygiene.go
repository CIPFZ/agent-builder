package agent

import (
	"errors"
	"fmt"
	"log/slog"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

const (
	historyHygieneDropEmptyAssistant        = "drop_empty_assistant"
	historyHygieneDropOrphanToolResult      = "drop_orphan_tool_result"
	historyHygieneInjectSyntheticToolResult = "inject_synthetic_tool_result"
	historyHygieneDropDuplicateToolResult   = "drop_duplicate_tool_result"
	historyHygieneDropDuplicateToolUse      = "drop_duplicate_tool_use"
	historyHygieneDropThinkingOnlyAssistant = "drop_thinking_only_assistant"
	historyHygieneStripUnsupportedMedia     = "strip_unsupported_media"
	historyHygieneReorderProviderToolUse    = "reorder_provider_tool_use"
)

type HistoryHygieneOptions struct {
	SupportsImages bool
	Strict         bool
	Provider       string
	Model          string
}

type HistoryHygieneResult struct {
	Messages    []fantasy.Message
	Diagnostics []HistoryHygieneDiagnostic
	Changed     bool
}

type HistoryHygieneDiagnostic struct {
	Kind       string `json:"kind"`
	MessageID  string `json:"message_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Action     string `json:"action"`
	Reason     string `json:"reason,omitempty"`
}

func PrepareProviderHistory(msgs []message.Message, opts HistoryHygieneOptions) (HistoryHygieneResult, error) {
	return ApplyHistoryHygiene(msgs, opts)
}

func ApplyHistoryHygiene(msgs []message.Message, opts HistoryHygieneOptions) (HistoryHygieneResult, error) {
	var result HistoryHygieneResult
	knownToolCallIDs := make(map[string]struct{})
	knownToolResultIDs := make(map[string]struct{})
	duplicateToolCallIDs := make(map[string]struct{})
	for _, m := range msgs {
		switch m.Role {
		case message.Assistant:
			for _, tc := range m.ToolCalls() {
				if tc.ID == "" {
					continue
				}
				if _, exists := knownToolCallIDs[tc.ID]; exists {
					duplicateToolCallIDs[tc.ID] = struct{}{}
					continue
				}
				knownToolCallIDs[tc.ID] = struct{}{}
			}
		case message.Tool:
			for _, tr := range m.ToolResults() {
				if tr.ToolCallID != "" {
					knownToolResultIDs[tr.ToolCallID] = struct{}{}
				}
			}
		}
	}

	seenToolCallIDs := make(map[string]struct{})
	seenToolResultIDs := make(map[string]struct{})
	for _, m := range msgs {
		if len(m.Parts) == 0 {
			result.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneDropEmptyAssistant, MessageID: m.ID, Action: "drop", Reason: "message has no parts"})
			continue
		}
		if m.Role == message.Assistant && len(m.ToolCalls()) == 0 && m.Content().Text == "" {
			reasoning := m.ReasoningContent().String()
			if reasoning == "" {
				result.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneDropEmptyAssistant, MessageID: m.ID, Action: "drop", Reason: "assistant message has no text, reasoning, or tool calls"})
				continue
			}
			result.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneDropThinkingOnlyAssistant, MessageID: m.ID, Action: "drop", Reason: "assistant message contains only thinking"})
			continue
		}
		if m.Role == message.Tool {
			msg, ok, diags := filterOrphanedToolResultsWithDiagnostics(m, knownToolCallIDs, seenToolResultIDs)
			for _, diag := range diags {
				result.markChanged(diag)
			}
			if ok {
				result.Messages = append(result.Messages, msg)
			}
			continue
		}
		aiMsgs := m.ToAIMessage()
		for i := range aiMsgs {
			if !opts.SupportsImages && aiMsgs[i].Role == fantasy.MessageRoleUser {
				filtered, stripped := filterFilePartsWithCount(aiMsgs[i].Content)
				if stripped > 0 {
					result.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneStripUnsupportedMedia, MessageID: m.ID, Action: "strip", Reason: "model does not support image/file media"})
				}
				aiMsgs[i].Content = filtered
			}
			if aiMsgs[i].Role == fantasy.MessageRoleAssistant {
				aiMsgs[i] = result.cleanAssistantToolUse(m.ID, aiMsgs[i], seenToolCallIDs)
			}
			reordered := reorderProviderToolUseBlocks(aiMsgs[i])
			if len(reordered.Content) > 0 && !sameMessagePartOrder(aiMsgs[i].Content, reordered.Content) {
				result.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneReorderProviderToolUse, MessageID: m.ID, Action: "reorder", Reason: "provider requires tool_use blocks after content blocks"})
				aiMsgs[i] = reordered
			}
		}
		result.Messages = append(result.Messages, aiMsgs...)
		if m.Role == message.Assistant {
			msg, ok, diags := syntheticToolResultsForOrphanedCallsWithDiagnostics(m, knownToolResultIDs)
			for _, diag := range diags {
				result.markChanged(diag)
			}
			if ok {
				result.Messages = append(result.Messages, msg)
				for _, part := range msg.Content {
					if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && tr.ToolCallID != "" {
						knownToolResultIDs[tr.ToolCallID] = struct{}{}
					}
				}
			}
		}
	}
	if opts.Strict && result.Changed {
		return result, fmt.Errorf("provider history failed strict hygiene: %d diagnostics", len(result.Diagnostics))
	}
	return result, nil
}

func (r *HistoryHygieneResult) markChanged(diag HistoryHygieneDiagnostic) {
	r.Changed = true
	r.Diagnostics = append(r.Diagnostics, diag)
}

func (r *HistoryHygieneResult) cleanAssistantToolUse(messageID string, msg fantasy.Message, seen map[string]struct{}) fantasy.Message {
	filtered := make([]fantasy.MessagePart, 0, len(msg.Content))
	for _, part := range msg.Content {
		tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
		if !ok {
			filtered = append(filtered, part)
			continue
		}
		if tc.ToolCallID == "" {
			r.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneDropDuplicateToolUse, MessageID: messageID, Action: "drop", Reason: "tool_use is missing id"})
			continue
		}
		if _, exists := seen[tc.ToolCallID]; exists {
			r.markChanged(HistoryHygieneDiagnostic{Kind: historyHygieneDropDuplicateToolUse, MessageID: messageID, ToolCallID: tc.ToolCallID, ToolName: tc.ToolName, Action: "drop", Reason: "duplicate tool_use id"})
			continue
		}
		seen[tc.ToolCallID] = struct{}{}
		filtered = append(filtered, part)
	}
	msg.Content = filtered
	return msg
}

// filterFileParts removes fantasy.FilePart entries from a slice of message
// parts. Used to strip image attachments from historical user messages when
// the current model does not support them.
func filterFileParts(parts []fantasy.MessagePart) []fantasy.MessagePart {
	filtered, _ := filterFilePartsWithCount(parts)
	return filtered
}

func filterFilePartsWithCount(parts []fantasy.MessagePart) ([]fantasy.MessagePart, int) {
	filtered := make([]fantasy.MessagePart, 0, len(parts))
	stripped := 0
	for _, part := range parts {
		if _, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
			stripped++
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered, stripped
}

// filterOrphanedToolResults converts a tool message to a fantasy.Message,
// dropping any tool result parts whose tool_call_id has no matching tool call
// in the known set.
func filterOrphanedToolResults(m message.Message, knownToolCallIDs map[string]struct{}) (fantasy.Message, bool) {
	msg, ok, _ := filterOrphanedToolResultsWithDiagnostics(m, knownToolCallIDs, map[string]struct{}{})
	return msg, ok
}

func filterOrphanedToolResultsWithDiagnostics(m message.Message, knownToolCallIDs, seenToolResultIDs map[string]struct{}) (fantasy.Message, bool, []HistoryHygieneDiagnostic) {
	aiMsgs := m.ToAIMessage()
	if len(aiMsgs) == 0 {
		return fantasy.Message{}, false, nil
	}
	var diagnostics []HistoryHygieneDiagnostic
	var validParts []fantasy.MessagePart
	for _, part := range aiMsgs[0].Content {
		tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
		if !ok {
			validParts = append(validParts, part)
			continue
		}
		if _, known := knownToolCallIDs[tr.ToolCallID]; !known {
			slog.Warn("Dropping orphaned tool result with no matching tool call", "tool_call_id", tr.ToolCallID)
			diagnostics = append(diagnostics, HistoryHygieneDiagnostic{Kind: historyHygieneDropOrphanToolResult, MessageID: m.ID, ToolCallID: tr.ToolCallID, Action: "drop", Reason: "no matching tool_use"})
			continue
		}
		if _, seen := seenToolResultIDs[tr.ToolCallID]; seen {
			diagnostics = append(diagnostics, HistoryHygieneDiagnostic{Kind: historyHygieneDropDuplicateToolResult, MessageID: m.ID, ToolCallID: tr.ToolCallID, Action: "drop", Reason: "duplicate tool_result"})
			continue
		}
		seenToolResultIDs[tr.ToolCallID] = struct{}{}
		validParts = append(validParts, part)
	}
	if len(validParts) == 0 {
		return fantasy.Message{}, false, diagnostics
	}
	msg := aiMsgs[0]
	msg.Content = validParts
	return msg, true, diagnostics
}

// syntheticToolResultsForOrphanedCalls returns a tool message containing
// synthetic tool results for any tool calls in the assistant message that
// have no matching result in knownToolResultIDs.
func syntheticToolResultsForOrphanedCalls(m message.Message, knownToolResultIDs map[string]struct{}) (fantasy.Message, bool) {
	msg, ok, _ := syntheticToolResultsForOrphanedCallsWithDiagnostics(m, knownToolResultIDs)
	return msg, ok
}

func syntheticToolResultsForOrphanedCallsWithDiagnostics(m message.Message, knownToolResultIDs map[string]struct{}) (fantasy.Message, bool, []HistoryHygieneDiagnostic) {
	var diagnostics []HistoryHygieneDiagnostic
	var syntheticParts []fantasy.MessagePart
	seenToolCallIDs := make(map[string]struct{})
	for _, tc := range m.ToolCalls() {
		if tc.ID == "" {
			continue
		}
		if _, seen := seenToolCallIDs[tc.ID]; seen {
			continue
		}
		seenToolCallIDs[tc.ID] = struct{}{}
		if _, hasResult := knownToolResultIDs[tc.ID]; hasResult {
			continue
		}
		slog.Warn("Injecting synthetic tool result for orphaned tool call", "tool_call_id", tc.ID, "tool_name", tc.Name)
		diagnostics = append(diagnostics, HistoryHygieneDiagnostic{Kind: historyHygieneInjectSyntheticToolResult, MessageID: m.ID, ToolCallID: tc.ID, ToolName: tc.Name, Action: "inject", Reason: "tool_use has no matching result"})
		syntheticParts = append(syntheticParts, fantasy.ToolResultPart{
			ToolCallID: tc.ID,
			Output: fantasy.ToolResultOutputContentError{
				Error: errors.New("tool call was interrupted and did not produce a result, you may retry this call if the result is still needed"),
			},
		})
	}
	if len(syntheticParts) == 0 {
		return fantasy.Message{}, false, diagnostics
	}
	return fantasy.Message{Role: fantasy.MessageRoleTool, Content: syntheticParts}, true, diagnostics
}

func reorderProviderToolUseBlocks(msg fantasy.Message) fantasy.Message {
	if msg.Role != fantasy.MessageRoleAssistant || len(msg.Content) < 2 {
		return msg
	}
	nonTool := make([]fantasy.MessagePart, 0, len(msg.Content))
	tool := make([]fantasy.MessagePart, 0)
	for _, part := range msg.Content {
		if _, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
			tool = append(tool, part)
			continue
		}
		nonTool = append(nonTool, part)
	}
	if len(tool) == 0 || len(nonTool) == 0 {
		return msg
	}
	msg.Content = append(nonTool, tool...)
	return msg
}

func sameMessagePartOrder(left, right []fantasy.MessagePart) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if fmt.Sprintf("%#v", left[i]) != fmt.Sprintf("%#v", right[i]) {
			return false
		}
	}
	return true
}
