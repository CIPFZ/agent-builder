package contextmgr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestFullCompactReplacesOldHistoryWithSummaryTailAndReinjection(t *testing.T) {
	t.Parallel()

	_, manager := testContextManagerWithSummarizer(t, &fakeCompactSummarizer{
		summary: CompactSummaryResult{Summary: "summary of old work", Ref: "runtime://summary/ref"},
	})
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: textMessages("old-1", "old-2", "tail-1", "tail-2"),
		FullCompact: FullCompactConfig{
			Enabled:              true,
			Trigger:              "manual",
			PreserveTailMessages: 2,
			ReinjectedMessages:   textMessages("read-file-state"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Boundaries) != 1 || result.Boundaries[0].Kind != "full" {
		t.Fatalf("boundaries = %#v", result.Boundaries)
	}
	if promptContains(result.ModelMessages, "old-1") || promptContains(result.ModelMessages, "old-2") {
		t.Fatalf("old history remained in compacted projection: %#v", result.ModelMessages)
	}
	if !promptContains(result.ModelMessages, "summary of old work") || !promptContains(result.ModelMessages, "tail-1") || !promptContains(result.ModelMessages, "tail-2") || !promptContains(result.ModelMessages, "read-file-state") {
		t.Fatalf("summary/tail/reinjection missing: %#v", result.ModelMessages)
	}
}

func TestFullCompactSummaryPromptTooLongRetry(t *testing.T) {
	t.Parallel()

	summarizer := &fakeCompactSummarizer{
		failTooLong: 1,
		summary:     CompactSummaryResult{Summary: "compacted after retry"},
	}
	_, manager := testContextManagerWithSummarizer(t, summarizer)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: textMessages("old-1", "old-2", "protected-tail"),
		FullCompact: FullCompactConfig{
			Enabled:              true,
			Trigger:              "auto",
			PreserveTailMessages: 1,
			MaxSummaryRetries:    2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summarizer.calls != 2 {
		t.Fatalf("summarizer calls = %d", summarizer.calls)
	}
	if len(summarizer.inputs) != 2 || len(summarizer.inputs[0]) != 3 || len(summarizer.inputs[1]) != 2 {
		t.Fatalf("summary retry inputs = %#v", summarizer.inputs)
	}
	if !promptContains(result.ModelMessages, "protected-tail") || !promptContains(result.ModelMessages, "compacted after retry") {
		t.Fatalf("retry result missing protected tail/summary: %#v", result.ModelMessages)
	}
}

func TestFullCompactSummaryPromptTooLongBoundedFailure(t *testing.T) {
	t.Parallel()

	_, manager := testContextManagerWithSummarizer(t, &fakeCompactSummarizer{failTooLong: 10})
	_, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: textMessages("old-1", "old-2", "tail"),
		FullCompact: FullCompactConfig{
			Enabled:              true,
			PreserveTailMessages: 1,
			MaxSummaryRetries:    1,
		},
	})
	if !errors.Is(err, ErrCompactInputTooLong) {
		t.Fatalf("err = %v", err)
	}
}

type fakeCompactSummarizer struct {
	calls       int
	failTooLong int
	inputs      [][]fantasy.Message
	summary     CompactSummaryResult
}

func (f *fakeCompactSummarizer) SummarizeCompact(ctx context.Context, req CompactSummaryRequest) (CompactSummaryResult, error) {
	f.calls++
	f.inputs = append(f.inputs, cloneModelMessages(req.Messages))
	if f.calls <= f.failTooLong {
		return CompactSummaryResult{}, ErrCompactInputTooLong
	}
	return f.summary, nil
}

func testContextManagerWithSummarizer(t *testing.T, summarizer CompactSummarizer) (SQLStore, *DefaultManager) {
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
		Store:      store,
		Summarizer: summarizer,
		Now:        func() time.Time { return time.UnixMilli(1000).UTC() },
	})
	return store, manager
}

func textMessages(values ...string) []fantasy.Message {
	messages := make([]fantasy.Message, 0, len(values))
	for _, value := range values {
		messages = append(messages, fantasy.NewUserMessage(value))
	}
	return messages
}

func promptContains(messages []fantasy.Message, needle string) bool {
	for _, msg := range messages {
		for _, part := range msg.Content {
			if text, ok := part.(fantasy.TextPart); ok && text.Text == needle {
				return true
			}
			if text, ok := part.(fantasy.TextPart); ok && strings.Contains(text.Text, needle) {
				return true
			}
		}
	}
	return false
}
