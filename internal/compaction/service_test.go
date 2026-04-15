package compaction_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"myclaw/internal/compaction"
	"myclaw/internal/model"
)

func TestServiceCompactIfNeededSummarizesOldMessages(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         3,
		SummaryPrefix:       "Summary:",
		PreserveRecentTurns: 2,
	})

	messages := []model.Message{
		{ID: "msg-1", Role: "user", Content: "first request", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", Role: "assistant", Content: "first response", CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", Role: "user", Content: "second request", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "msg-4", Role: "assistant", Content: "second response", CreatedAt: time.Unix(4, 0).UTC()},
	}

	compacted, changed := service.CompactIfNeeded(messages)

	if !changed {
		t.Fatal("expected compaction to trigger")
	}
	if len(compacted) != 3 {
		t.Fatalf("compacted message count = %d, want 3", len(compacted))
	}
	if compacted[0].Role != "summary" {
		t.Fatalf("first compacted role = %q, want summary", compacted[0].Role)
	}
	if !strings.Contains(compacted[0].Content, "first request") {
		t.Fatalf("summary content = %q, want it to mention older history", compacted[0].Content)
	}
}

func TestServiceCompactIfNeededSkipsShortHistory(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         4,
		PreserveRecentTurns: 2,
	})

	messages := []model.Message{
		{ID: "msg-1", Role: "user", Content: "hello", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", Role: "assistant", Content: "world", CreatedAt: time.Unix(2, 0).UTC()},
	}

	compacted, changed := service.CompactIfNeeded(messages)

	if changed {
		t.Fatal("expected short history to skip compaction")
	}
	if len(compacted) != len(messages) {
		t.Fatalf("message count = %d, want %d", len(compacted), len(messages))
	}
}

func TestServiceCompactIfNeededUsesEstimatedTokens(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         99,
		MaxEstimatedTokens:  20,
		PreserveRecentTurns: 2,
		SummaryPrefix:       "Summary:",
	})

	messages := []model.Message{
		{ID: "msg-1", Role: "user", Content: strings.Repeat("a", 50), CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", Role: "assistant", Content: "reply", CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", Role: "tool", Content: "tool result should stay visible", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "msg-4", Role: "assistant", Content: "latest answer", CreatedAt: time.Unix(4, 0).UTC()},
	}

	compacted, changed := service.CompactIfNeeded(messages)

	if !changed {
		t.Fatal("expected compaction by token estimate")
	}
	if compacted[0].Role != "summary" {
		t.Fatalf("first role = %q, want summary", compacted[0].Role)
	}
	if compacted[1].Role != "tool" {
		t.Fatalf("expected recent tool result to remain after compaction, got %#v", compacted)
	}
}

func TestServiceAnalyzeReportsThresholdStates(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		ContextWindowTokens:     100,
		WarningBufferTokens:     20,
		ErrorBufferTokens:       10,
		AutoCompactBufferTokens: 5,
		BlockingBufferTokens:    2,
	})

	messages := []model.Message{
		{ID: "msg-1", Role: "user", Content: strings.Repeat("a", 120), CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", Role: "assistant", Content: strings.Repeat("b", 120), CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", Role: "assistant", Content: strings.Repeat("c", 112), CreatedAt: time.Unix(3, 0).UTC()},
	}

	analysis := service.Analyze(messages)

	if analysis.EstimatedTokens != 88 {
		t.Fatalf("estimated tokens = %d, want 88", analysis.EstimatedTokens)
	}
	if analysis.WarningThreshold != 80 {
		t.Fatalf("warning threshold = %d, want 80", analysis.WarningThreshold)
	}
	if analysis.ErrorThreshold != 90 {
		t.Fatalf("error threshold = %d, want 90", analysis.ErrorThreshold)
	}
	if analysis.AutoCompactThreshold != 95 {
		t.Fatalf("auto compact threshold = %d, want 95", analysis.AutoCompactThreshold)
	}
	if analysis.BlockingThreshold != 98 {
		t.Fatalf("blocking threshold = %d, want 98", analysis.BlockingThreshold)
	}
	if !analysis.IsAboveWarningThreshold {
		t.Fatal("expected warning threshold to be exceeded")
	}
	if analysis.IsAboveErrorThreshold {
		t.Fatal("expected error threshold to stay false")
	}
	if analysis.IsAboveAutoCompactThreshold {
		t.Fatal("expected auto compact threshold to stay false")
	}
	if analysis.IsAtBlockingLimit {
		t.Fatal("expected blocking threshold to stay false")
	}
}

func TestServiceAnalyzeHandlesZeroContextWindow(t *testing.T) {
	service := compaction.NewService(compaction.Config{})
	analysis := service.Analyze([]model.Message{
		{ID: "msg-1", Role: "user", Content: "hello"},
	})

	if analysis.ContextWindowTokens != 0 {
		t.Fatalf("context window = %d, want 0", analysis.ContextWindowTokens)
	}
	if analysis.IsAboveWarningThreshold || analysis.IsAboveErrorThreshold || analysis.IsAboveAutoCompactThreshold || analysis.IsAtBlockingLimit {
		t.Fatalf("unexpected thresholds triggered for zero context window: %#v", analysis)
	}
}

func TestServiceCompactReturnsStructuredResultForMessageLimit(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         3,
		PreserveRecentTurns: 2,
		SummaryPrefix:       "Summary:",
		ContextWindowTokens: 100,
		WarningBufferTokens: 20,
	})

	messages := []model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "first request", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "first response", CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "second request", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "msg-4", SessionID: "sess-1", Role: "assistant", Content: "second response", CreatedAt: time.Unix(4, 0).UTC()},
	}

	result := service.Compact(messages)

	if !result.Changed {
		t.Fatal("expected structured compaction to report change")
	}
	if result.Reason != compaction.ReasonMessageLimit {
		t.Fatalf("reason = %q, want %q", result.Reason, compaction.ReasonMessageLimit)
	}
	if result.OriginalCount != 4 || result.CompactedCount != 3 {
		t.Fatalf("counts = %#v, want original=4 compacted=3", result)
	}
	if result.SummaryMessage == nil || result.SummaryMessage.Role != "summary" {
		t.Fatalf("summary message = %#v, want summary message", result.SummaryMessage)
	}
	if result.Analysis.ContextWindowTokens != 100 {
		t.Fatalf("analysis = %#v, want context window copied into result", result.Analysis)
	}
}

func TestServiceCompactReturnsStructuredResultForTokenBudget(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         99,
		MaxEstimatedTokens:  20,
		PreserveRecentTurns: 2,
		SummaryPrefix:       "Summary:",
	})

	messages := []model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: strings.Repeat("a", 50), CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "reply", CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", SessionID: "sess-1", Role: "tool", Content: "tool result should stay visible", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "msg-4", SessionID: "sess-1", Role: "assistant", Content: "latest answer", CreatedAt: time.Unix(4, 0).UTC()},
	}

	result := service.Compact(messages)

	if !result.Changed {
		t.Fatal("expected compaction by token budget")
	}
	if result.Reason != compaction.ReasonTokenBudget {
		t.Fatalf("reason = %q, want %q", result.Reason, compaction.ReasonTokenBudget)
	}
	if result.SummaryMessage == nil || result.Messages[1].Role != "tool" {
		t.Fatalf("result = %#v, want structured result with preserved recent tool", result)
	}
}

func TestServiceCompactFlattensOlderSummaryMessagesInsteadOfNestingSummaryPrefixes(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:         2,
		PreserveRecentTurns: 1,
		SummaryPrefix:       "Summary:",
	})

	result := service.Compact([]model.Message{
		{ID: "summary-1", SessionID: "main-1", Role: "summary", Content: "Summary: earlier context"},
		{ID: "msg-2", SessionID: "main-1", Role: "assistant", Content: "recent reply"},
		{ID: "msg-3", SessionID: "main-1", Role: "user", Content: "new prompt"},
	})

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want changed summary result", result)
	}
	if strings.Contains(result.SummaryMessage.Content, "Summary: Summary:") {
		t.Fatalf("summary content = %q, did not want nested Summary prefixes", result.SummaryMessage.Content)
	}
}

func TestServiceMicrocompactClearsOlderCompactableToolResultsButKeepsRecentTail(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		PreserveRecentTurns: 2,
	})

	result := service.Microcompact([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "first prompt", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", SessionID: "sess-1", Role: "tool", Content: "system.run: " + strings.Repeat("x", 200), CreatedAt: time.Unix(2, 0).UTC()},
		{ID: "msg-3", SessionID: "sess-1", Role: "assistant", Content: "used tool", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "msg-4", SessionID: "sess-1", Role: "tool", Content: "system.run: keep recent", CreatedAt: time.Unix(4, 0).UTC()},
		{ID: "msg-5", SessionID: "sess-1", Role: "assistant", Content: "latest reply", CreatedAt: time.Unix(5, 0).UTC()},
	})

	if !result.Changed {
		t.Fatal("expected microcompact to report change")
	}
	if result.Reason != compaction.ReasonMicrocompact {
		t.Fatalf("reason = %q, want %q", result.Reason, compaction.ReasonMicrocompact)
	}
	if result.Messages[1].Content != "system.run: [Old tool result content cleared]" {
		t.Fatalf("older tool result = %q, want cleared marker", result.Messages[1].Content)
	}
	if result.Messages[3].Content != "system.run: keep recent" {
		t.Fatalf("recent tool result = %q, want recent tail preserved", result.Messages[3].Content)
	}
}

func TestServiceMicrocompactSkipsNonCompactableToolResults(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		PreserveRecentTurns: 1,
	})

	result := service.Microcompact([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "tool", Content: "text.upper: HELLO", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "latest reply", CreatedAt: time.Unix(2, 0).UTC()},
	})

	if result.Changed {
		t.Fatalf("result = %#v, want microcompact to skip non-compactable tool", result)
	}
}

func TestServiceCompactWithSessionMemoryUsesPersistedSummaryAndKeepsTailAfterAnchor(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "already summarized answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: old summarized context", "msg-2")

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want changed result with summary message", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "old summarized context") ||
		!strings.Contains(result.SummaryMessage.Content, "This session is being continued from a previous conversation that ran out of context.") {
		t.Fatalf("summary content = %q, want Claude continuation prompt with persisted session memory summary", result.SummaryMessage.Content)
	}
	if len(result.Messages) != 3 || result.Messages[0].Subtype != "compact_boundary" || result.Messages[2].ID != "msg-3" {
		t.Fatalf("messages = %#v, want preserved post-anchor tail only", result.Messages)
	}
	if result.SummarizedThroughID != "msg-2" {
		t.Fatalf("result = %#v, want summarized through msg-2", result)
	}
}

func TestServiceCompactWithSessionMemoryFallsBackWhenAnchorIsMissing(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "old answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: old summarized context", "missing-msg")

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want fallback compact result", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "old prompt") {
		t.Fatalf("summary content = %q, want traditional compact fallback content", result.SummaryMessage.Content)
	}
}

func TestServiceCompactWithSessionMemorySupportsResumedSessionWithoutAnchor(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "keep from tail"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "latest answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: resumed session memory", "")

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want resumed-session compact result", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "resumed session memory") ||
		!strings.Contains(result.SummaryMessage.Content, "This session is being continued from a previous conversation that ran out of context.") {
		t.Fatalf("summary content = %q, want Claude continuation prompt with session memory summary", result.SummaryMessage.Content)
	}
	if len(result.Messages) < 2 {
		t.Fatalf("messages = %#v, want preserved tail after resumed compact", result.Messages)
	}
}

func TestServiceCompactWithSessionMemoryDoesNotSplitAssistantToolResultPair(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "already summarized answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "assistant", Content: "tool call: system.run pwd"},
		{ID: "msg-4", SessionID: "sess-1", Role: "tool", Content: "system.run: C:\\repo"},
		{ID: "msg-5", SessionID: "sess-1", Role: "assistant", Content: "used tool result"},
	}, "Summary: old summarized context", "msg-3")

	if !result.Changed {
		t.Fatalf("result = %#v, want changed result", result)
	}
	if len(result.Messages) < 5 || result.Messages[2].ID != "msg-3" || result.Messages[3].ID != "msg-4" {
		t.Fatalf("messages = %#v, want assistant/tool pair preserved together", result.Messages)
	}
	if result.SummarizedThroughID != "msg-2" {
		t.Fatalf("result = %#v, want summarized through pre-pair anchor", result)
	}
}

func TestServiceCompactWithSessionMemoryPreservesBlockToolUseForKeptToolResult(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     1,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{
			ID:                "msg-2",
			SessionID:         "sess-1",
			Role:              "assistant",
			Content:           "thinking",
			ProviderMessageID: "provider-1",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockThinking, Text: "thinking"},
			},
		},
		{
			ID:                "msg-3",
			SessionID:         "sess-1",
			Role:              "assistant",
			Content:           "call orphan tool",
			ProviderMessageID: "provider-1",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockToolUse, ID: "orphan-tool"},
			},
		},
		{
			ID:                "msg-4",
			SessionID:         "sess-1",
			Role:              "assistant",
			Content:           "call valid tool",
			ProviderMessageID: "provider-1",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockToolUse, ID: "valid-tool"},
			},
		},
		{
			ID:        "msg-5",
			SessionID: "sess-1",
			Role:      "user",
			Content:   "tool results",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockToolResult, ToolUseID: "orphan-tool"},
				{Type: model.MessageBlockToolResult, ToolUseID: "valid-tool"},
			},
		},
	}, "Summary: old summarized context", "msg-3")

	if !result.Changed {
		t.Fatalf("result = %#v, want changed result", result)
	}
	if len(result.Messages) < 6 || result.Messages[2].ID != "msg-2" || result.Messages[3].ID != "msg-3" || result.Messages[4].ID != "msg-4" || result.Messages[5].ID != "msg-5" {
		t.Fatalf("messages = %#v, want thinking plus both tool_use blocks preserved for kept tool_result blocks", result.Messages)
	}
	if result.SummarizedThroughID != "msg-1" {
		t.Fatalf("result = %#v, want summarized through pre-provider-message boundary", result)
	}
}

func TestServiceCompactWithSessionMemoryPreservesThinkingBlocksForSameProviderMessage(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     1,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{
			ID:                "msg-2",
			SessionID:         "sess-1",
			Role:              "assistant",
			Content:           "thinking",
			ProviderMessageID: "provider-2",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockThinking, Text: "thinking"},
			},
		},
		{
			ID:                "msg-3",
			SessionID:         "sess-1",
			Role:              "assistant",
			Content:           "tool use",
			ProviderMessageID: "provider-2",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockToolUse, ID: "tool-1"},
			},
		},
		{
			ID:        "msg-4",
			SessionID: "sess-1",
			Role:      "user",
			Content:   "tool result",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockToolResult, ToolUseID: "tool-1"},
			},
		},
	}, "Summary: old summarized context", "msg-2")

	if !result.Changed {
		t.Fatalf("result = %#v, want changed result", result)
	}
	if len(result.Messages) < 5 || result.Messages[2].ID != "msg-2" || result.Messages[3].ID != "msg-3" || result.Messages[4].ID != "msg-4" {
		t.Fatalf("messages = %#v, want earlier thinking block with same provider message id preserved", result.Messages)
	}
	if result.SummarizedThroughID != "msg-1" {
		t.Fatalf("result = %#v, want summarized through message before shared provider group", result)
	}
}

func TestServiceCompactWithSessionMemoryFiltersOldCompactBoundariesFromPreservedTail(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{ID: "compact-old", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]"},
		{ID: "msg-2", SessionID: "sess-1", Role: "user", Content: "tail prompt"},
	}, "Summary: old summarized context", "msg-1")

	if !result.Changed {
		t.Fatalf("result = %#v, want changed result", result)
	}
	for _, message := range result.Messages[2:] {
		if message.Content == "[compact_boundary]" {
			t.Fatalf("messages = %#v, did not want old compact boundary in preserved tail", result.Messages)
		}
	}
	if len(result.Messages) != 3 || result.Messages[0].Subtype != "compact_boundary" || result.Messages[2].ID != "msg-2" {
		t.Fatalf("messages = %#v, want summary plus post-boundary tail", result.Messages)
	}
}

func TestServiceCompactWithSessionMemoryOptionsInjectsCompactHooksAndTranscriptNote(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemoryOptions([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "already summarized prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "already summarized answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: old summarized context", "msg-2", compaction.SessionMemoryOptions{
		HookMessages:   []model.Message{{ID: "hook-1", SessionID: "sess-1", Role: "system", Content: "CLAUDE.md compact hook context"}},
		TranscriptPath: "C:/tmp/transcript.jsonl",
	})

	if !result.Changed {
		t.Fatalf("result = %#v, want changed result", result)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("messages = %#v, want boundary, summary, tail, hook", result.Messages)
	}
	if result.Messages[0].Subtype != "compact_boundary" {
		t.Fatalf("boundary = %#v, want compact boundary first", result.Messages[0])
	}
	if !strings.Contains(result.Messages[1].Content, "C:/tmp/transcript.jsonl") {
		t.Fatalf("summary = %#v, want transcript path embedded in compact summary", result.Messages[1])
	}
	if result.Messages[2].ID != "msg-3" || result.Messages[3].ID != "hook-1" {
		t.Fatalf("messages = %#v, want preserved tail before compact hook", result.Messages)
	}
	if result.PostCompactTokenCount == 0 {
		t.Fatalf("post compact tokens = %d, want tracked token count", result.PostCompactTokenCount)
	}
}

func TestServiceCompactWithSessionMemoryBuildsClaudePostCompactShape(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemoryOptions([]model.Message{
		{
			ID:        "msg-1",
			SessionID: "sess-1",
			Role:      "user",
			Content:   "tool search result",
			Blocks: []model.MessageBlock{
				{
					Raw: map[string]any{
						"type": "tool_result",
						"content": []any{
							map[string]any{"type": "tool_reference", "tool_name": "Bash"},
						},
					},
				},
			},
		},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "already summarized answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: old summarized context", "msg-2", compaction.SessionMemoryOptions{
		HookMessages: []model.Message{
			{ID: "hook-1", SessionID: "sess-1", Role: "system", Content: "CLAUDE.md compact hook context"},
		},
		TranscriptPath: "C:/tmp/transcript.jsonl",
		PlanAttachment: &model.Message{
			ID:        "plan-1",
			SessionID: "sess-1",
			Role:      "attachment",
			Subtype:   "plan_file_reference",
			Content:   "plan content",
		},
	})

	if !result.Changed || result.BoundaryMessage == nil || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want changed result with boundary and summary", result)
	}
	if len(result.Messages) != 5 {
		t.Fatalf("messages = %#v, want boundary, summary, tail, attachment, hook", result.Messages)
	}
	if result.Messages[0].Subtype != "compact_boundary" || result.Messages[0].CompactMetadata == nil {
		t.Fatalf("boundary = %#v, want compact boundary metadata", result.Messages[0])
	}
	if result.Messages[0].CompactMetadata.Trigger != "auto" {
		t.Fatalf("boundary metadata = %#v, want auto trigger", result.Messages[0].CompactMetadata)
	}
	if result.Messages[0].CompactMetadata.PreservedSegment == nil ||
		result.Messages[0].CompactMetadata.PreservedSegment.HeadID != "msg-3" ||
		result.Messages[0].CompactMetadata.PreservedSegment.AnchorID != result.SummaryMessage.ID ||
		result.Messages[0].CompactMetadata.PreservedSegment.TailID != "msg-3" {
		t.Fatalf("boundary metadata = %#v, want preserved segment relink info", result.Messages[0].CompactMetadata)
	}
	if len(result.Messages[0].CompactMetadata.PreCompactDiscoveredTools) == 0 ||
		result.Messages[0].CompactMetadata.PreCompactDiscoveredTools[0] != "Bash" {
		t.Fatalf("boundary metadata = %#v, want discovered tool names", result.Messages[0].CompactMetadata)
	}
	if result.Messages[1].Role != "user" || !result.Messages[1].IsCompactSummary || !result.Messages[1].IsVisibleInTranscriptOnly {
		t.Fatalf("summary = %#v, want Claude compact summary user message visible only in transcript", result.Messages[1])
	}
	if !strings.Contains(result.Messages[1].Content, "C:/tmp/transcript.jsonl") {
		t.Fatalf("summary content = %q, want transcript path embedded in summary message", result.Messages[1].Content)
	}
	if result.Messages[2].ID != "msg-3" || result.Messages[3].ID != "plan-1" || result.Messages[4].ID != "hook-1" {
		t.Fatalf("messages = %#v, want kept tail before attachment and hook results", result.Messages)
	}
	if result.TruePostCompactTokenCount != result.PostCompactTokenCount || result.TruePostCompactTokenCount == 0 {
		t.Fatalf("result = %#v, want true post compact token count", result)
	}
}

func TestServiceCompactWithSessionMemoryUsesFreshIDsAndPreservedSegmentLinks(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})
	messages := []model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "old answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
		{ID: "msg-4", SessionID: "sess-1", Role: "assistant", Content: "latest answer"},
	}

	first := service.CompactWithSessionMemory(messages, "Summary: old summarized context", "msg-2")
	second := service.CompactWithSessionMemory(messages, "Summary: old summarized context", "msg-2")

	if !first.Changed || !second.Changed || first.BoundaryMessage == nil || first.SummaryMessage == nil || second.BoundaryMessage == nil || second.SummaryMessage == nil {
		t.Fatalf("first = %#v, second = %#v, want two changed session-memory compactions", first, second)
	}
	if first.BoundaryMessage.ID == "compact-boundary-1" || first.SummaryMessage.ID == "summary-1" {
		t.Fatalf("boundary/summary IDs = %q/%q, want fresh IDs", first.BoundaryMessage.ID, first.SummaryMessage.ID)
	}
	if first.BoundaryMessage.ID == second.BoundaryMessage.ID || first.SummaryMessage.ID == second.SummaryMessage.ID {
		t.Fatalf("first IDs = %q/%q, second IDs = %q/%q, want unique IDs", first.BoundaryMessage.ID, first.SummaryMessage.ID, second.BoundaryMessage.ID, second.SummaryMessage.ID)
	}
	segment := first.BoundaryMessage.CompactMetadata.PreservedSegment
	if segment == nil {
		t.Fatalf("boundary metadata = %#v, want preserved segment", first.BoundaryMessage.CompactMetadata)
	}
	if segment.AnchorID != first.SummaryMessage.ID || segment.HeadID != "msg-3" || segment.TailID != "msg-4" {
		t.Fatalf("segment = %#v, want anchor summary and kept head/tail", segment)
	}
}

func TestServiceCompactWithSessionMemorySummaryUsesClaudeContinuationPromptAndTruncatesSections(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})
	longLines := make([]string, 0, 4100)
	for i := 0; i < 4100; i++ {
		longLines = append(longLines, "abcd")
	}
	sessionMemory := "Summary: # Current State\n" + strings.Join(longLines, "\n")

	result := service.CompactWithSessionMemoryOptions([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "old answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, sessionMemory, "msg-2", compaction.SessionMemoryOptions{
		TranscriptPath:    "C:/tmp/transcript.jsonl",
		SessionMemoryPath: "C:/tmp/session-memory.md",
	})

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want compacted summary", result)
	}
	content := result.SummaryMessage.Content
	for _, want := range []string{
		"This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.",
		"read the full transcript at: C:/tmp/transcript.jsonl",
		"Recent messages are preserved verbatim.",
		"Continue the conversation from where it left off without asking the user any further questions.",
		"[... section truncated for length ...]",
		"The full session memory can be viewed at: C:/tmp/session-memory.md",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("summary content missing %q:\n%s", want, content)
		}
	}
}

func TestServiceCompactWithSessionMemoryDoesNotAppendSessionMemoryPathWithoutTruncation(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemoryOptions([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "old answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: # Current State\nshort", "msg-2", compaction.SessionMemoryOptions{
		SessionMemoryPath: "C:/tmp/session-memory.md",
	})

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want compacted summary", result)
	}
	if strings.Contains(result.SummaryMessage.Content, "full session memory can be viewed") {
		t.Fatalf("summary content = %q, did not want truncation note without truncation", result.SummaryMessage.Content)
	}
}

func TestServiceCompactWithSessionMemoryDoesNotExpandAcrossClaudeStyleBoundary(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     50,
		SessionMemoryMinTextBlocks: 3,
		SessionMemoryMaxTokens:     1000,
	})

	result := service.CompactWithSessionMemory([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: strings.Repeat("before boundary ", 40)},
		{ID: "boundary-old", SessionID: "sess-1", Role: "system", Subtype: "compact_boundary", Content: "Conversation compacted"},
		{ID: "msg-2", SessionID: "sess-1", Role: "user", Content: "after boundary"},
		{ID: "msg-3", SessionID: "sess-1", Role: "assistant", Content: "latest answer"},
	}, "Summary: old summarized context", "msg-3")

	if !result.Changed {
		t.Fatalf("result = %#v, want compacted result", result)
	}
	for _, message := range result.Messages {
		if message.ID == "msg-1" {
			t.Fatalf("messages = %#v, did not want expansion across compact boundary", result.Messages)
		}
	}
}

func TestServiceAnalyzeEstimatesStructuredMessageContent(t *testing.T) {
	service := compaction.NewService(compaction.Config{})
	raw := map[string]any{"type": "server_tool_use", "name": "web_search", "input": map[string]any{"query": strings.Repeat("raw", 80)}}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}

	analysis := service.Analyze([]model.Message{
		{
			ID:        "msg-1",
			SessionID: "sess-1",
			Role:      "user",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockText, Text: strings.Repeat("text", 80)},
				{Type: model.MessageBlockToolResult, Content: strings.Repeat("result", 80)},
				{Raw: raw},
			},
		},
		{
			ID:        "boundary-1",
			SessionID: "sess-1",
			Role:      "system",
			Subtype:   "compact_boundary",
			Content:   "Conversation compacted",
			CompactMetadata: &model.CompactMetadata{
				PreCompactDiscoveredTools: []string{strings.Repeat("tool", 80)},
			},
		},
		{
			ID:        "plan-1",
			SessionID: "sess-1",
			Role:      "attachment",
			Content:   strings.Repeat("plan", 80),
		},
	})

	minExpected := (320 + 480 + len(rawJSON) + 320 + 320) / 4
	if analysis.EstimatedTokens < minExpected {
		t.Fatalf("estimated tokens = %d, want at least %d from blocks/raw/metadata/attachments", analysis.EstimatedTokens, minExpected)
	}
}

func TestServiceCompactWithSessionMemoryOptionsFallsBackWhenPostCompactExceedsThreshold(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:                2,
		PreserveRecentTurns:        1,
		SummaryPrefix:              "Summary:",
		SessionMemoryMinTokens:     1,
		SessionMemoryMinTextBlocks: 1,
		SessionMemoryMaxTokens:     100,
	})

	result := service.CompactWithSessionMemoryOptions([]model.Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old prompt"},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "old answer"},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest prompt"},
	}, "Summary: old summarized context", "msg-2", compaction.SessionMemoryOptions{
		AutoCompactThreshold: 1,
	})

	if !result.Changed || result.SummaryMessage == nil {
		t.Fatalf("result = %#v, want traditional fallback compact result", result)
	}
	if strings.Contains(result.SummaryMessage.Content, "old summarized context") {
		t.Fatalf("summary = %q, want fallback away from session memory summary", result.SummaryMessage.Content)
	}
}
