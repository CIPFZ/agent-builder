package runtimeapi

import (
	"testing"
	"time"
)

func TestEventValidateRequiresStableEnvelope(t *testing.T) {
	event := NewEvent("event-1", EventTurnCompleted, time.Now().UTC())
	if err := event.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	event.Type = "unknown.event"
	if err := event.Validate(); err == nil {
		t.Fatal("unknown event type must be rejected")
	}
}

func TestEphemeralEventTypesAreKnown(t *testing.T) {
	for _, eventType := range []string{EventOutputTextDelta, EventCompactProgress} {
		if !IsEventType(eventType) || !IsEphemeralEventType(eventType) {
			t.Fatalf("ephemeral event type %q is not registered", eventType)
		}
	}
}
