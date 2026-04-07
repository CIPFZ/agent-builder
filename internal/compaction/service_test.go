package compaction_test

import (
	"strings"
	"testing"
	"time"

	"myclaw/internal/compaction"
	"myclaw/internal/model"
)

func TestServiceCompactIfNeededSummarizesOldMessages(t *testing.T) {
	service := compaction.NewService(compaction.Config{
		MaxMessages:        3,
		SummaryPrefix:      "Summary:",
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
		MaxMessages:        4,
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
		ContextWindowTokens:      100,
		WarningBufferTokens:      20,
		ErrorBufferTokens:        10,
		AutoCompactBufferTokens:  5,
		BlockingBufferTokens:     2,
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
