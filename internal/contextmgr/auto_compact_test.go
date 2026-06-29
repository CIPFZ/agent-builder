package contextmgr

import (
	"context"
	"testing"
)

func TestAutoCompactTriggersFullCompactBeforeProviderCall(t *testing.T) {
	t.Parallel()

	summarizer := &fakeCompactSummarizer{summary: CompactSummaryResult{Summary: "auto summary"}}
	_, manager := testContextManagerWithSummarizer(t, summarizer)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Step:          1,
		ModelMessages: textMessages("old context with enough text to exceed threshold", "tail"),
		FullCompact: FullCompactConfig{
			PreserveTailMessages: 1,
		},
		AutoCompact: AutoCompactConfig{
			Enabled:                      true,
			EffectiveContextWindowTokens: 20,
			OutputReserveTokens:          4,
			WarningBufferTokens:          10,
			AutoCompactBufferTokens:      8,
			BlockingBufferTokens:         2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summarizer.calls != 1 {
		t.Fatalf("summarizer calls = %d", summarizer.calls)
	}
	if len(result.Boundaries) != 1 || result.Boundaries[0].Kind != "full" || result.Boundaries[0].Trigger != "auto" {
		t.Fatalf("auto compact boundaries = %#v", result.Boundaries)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected threshold warning")
	}
}

func TestAutoCompactSkipsHelperCalls(t *testing.T) {
	t.Parallel()

	summarizer := &fakeCompactSummarizer{summary: CompactSummaryResult{Summary: "should not run"}}
	_, manager := testContextManagerWithSummarizer(t, summarizer)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-helper",
		Step:          1,
		ModelMessages: textMessages("large helper context"),
		AutoCompact: AutoCompactConfig{
			Enabled:                      true,
			EffectiveContextWindowTokens: 5,
			OutputReserveTokens:          1,
			AutoCompactBufferTokens:      10,
			HelperCall:                   true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summarizer.calls != 0 {
		t.Fatalf("helper call should not auto compact")
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "auto_compact_skipped" {
		t.Fatalf("helper warnings = %#v", result.Warnings)
	}
}

func TestAutoCompactCircuitBreaker(t *testing.T) {
	t.Parallel()

	summarizer := &fakeCompactSummarizer{summary: CompactSummaryResult{Summary: "should not run"}}
	_, manager := testContextManagerWithSummarizer(t, summarizer)
	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID:     "session-1",
		TurnID:        "turn-circuit",
		Step:          1,
		ModelMessages: textMessages("large context"),
		AutoCompact: AutoCompactConfig{
			Enabled:                      true,
			EffectiveContextWindowTokens: 5,
			OutputReserveTokens:          1,
			AutoCompactBufferTokens:      10,
			MaxConsecutiveFailures:       3,
			ConsecutiveFailures:          3,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summarizer.calls != 0 {
		t.Fatalf("circuit breaker should not auto compact")
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "auto_compact_circuit_open" {
		t.Fatalf("circuit warnings = %#v", result.Warnings)
	}
}
