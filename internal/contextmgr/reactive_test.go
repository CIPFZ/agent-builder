package contextmgr

import (
	"context"
	"testing"
)

func TestIsContextLengthError(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"prompt too long":                         true,
		"context_length_exceeded":                 true,
		"maximum context length is 128000 tokens": true,
		"rate limit":                              false,
		"":                                        false,
	}
	for input, want := range cases {
		if got := IsContextLengthError(input); got != want {
			t.Fatalf("IsContextLengthError(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestReactiveCompactRecordsAttempts(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	first, err := manager.ReactiveCompact(context.Background(), ReactiveCompactRequest{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: "projection-1",
		Attempt:      1,
		Error:        "prompt too long for model context length",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempt.Action != "projection_reduction" || first.Attempt.Status != ProjectionStatusCompleted {
		t.Fatalf("first attempt = %#v", first.Attempt)
	}
	second, err := manager.ReactiveCompact(context.Background(), ReactiveCompactRequest{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: "projection-2",
		Attempt:      2,
		Error:        "context length exceeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt.Action != "full_compact" {
		t.Fatalf("second attempt = %#v", second.Attempt)
	}
}

func TestReactiveCompactSkipsNonContextLengthErrors(t *testing.T) {
	t.Parallel()

	_, manager := testContextManager(t)
	result, err := manager.ReactiveCompact(context.Background(), ReactiveCompactRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Attempt:   1,
		Error:     "rate limit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempt.Status != "skipped" || result.Attempt.Action != "none" {
		t.Fatalf("attempt = %#v", result.Attempt)
	}
}
