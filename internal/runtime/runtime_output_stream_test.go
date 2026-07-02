package runtime

import (
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestEphemeralEventsSkipRingBufferAndPersistence(t *testing.T) {
	svc := newRuntimeService()
	before := len(svc.events)
	kept := svc.appendRuntimeEventLocked(RuntimeEvent{
		Type:      runtimeapi.EventOutputTextDelta,
		SessionID: "session-1",
		MessageID: "msg-1",
		Payload:   map[string]any{"part_type": "text", "delta": "hi", "content_len": 2},
	})
	if kept.Sequence != 0 {
		t.Fatalf("ephemeral event should not receive a persisted sequence, got %d", kept.Sequence)
	}
	if len(svc.events) != before {
		t.Fatalf("ephemeral event appended to ring buffer: %d -> %d", before, len(svc.events))
	}
	if svc.nextEventSequence != 0 {
		t.Fatalf("ephemeral event advanced global sequence: %d", svc.nextEventSequence)
	}
}

func TestDeriveOutputTextDeltasEmitsSuffixAndIsIdempotent(t *testing.T) {
	svc := newRuntimeService()
	msg := apitypes.Message{ID: "msg-1", SessionID: "session-1", Role: apitypes.Assistant}
	msg.Parts = append(msg.Parts, apitypes.TextContent{Text: "hello"})
	now := time.Unix(0, 1_000_000).UTC()
	first := svc.deriveOutputTextDeltasLocked(msg, "turn-1", now)
	if len(first) != 1 || first[0].Type != runtimeapi.EventOutputTextDelta {
		t.Fatalf("first delta = %#v", first)
	}
	if first[0].Payload["delta"] != "hello" {
		t.Fatalf("first delta text = %v", first[0].Payload["delta"])
	}
	if got := first[0].Payload["content_len"]; got != 5 {
		t.Fatalf("first delta content_len = %v", got)
	}

	// Same message, unchanged: nothing to emit.
	if again := svc.deriveOutputTextDeltasLocked(msg, "turn-1", now); len(again) != 0 {
		t.Fatalf("no-op delta produced events: %#v", again)
	}

	// Growth in text only produces the suffix.
	msg.Parts = []apitypes.ContentPart{apitypes.TextContent{Text: "hello world"}}
	next := svc.deriveOutputTextDeltasLocked(msg, "turn-1", now)
	if len(next) != 1 || next[0].Payload["delta"] != " world" {
		t.Fatalf("next delta = %#v", next)
	}
	if got := next[0].Payload["content_len"]; got != 11 {
		t.Fatalf("next delta content_len = %v", got)
	}

	// Reasoning also gets streamed independently.
	msg.Parts = append(msg.Parts, apitypes.ReasoningContent{Thinking: "planning"})
	rsn := svc.deriveOutputTextDeltasLocked(msg, "turn-1", now)
	if len(rsn) != 1 || rsn[0].Payload["part_type"] != "reasoning" || rsn[0].Payload["delta"] != "planning" {
		t.Fatalf("reasoning delta = %#v", rsn)
	}
}

func TestDeriveOutputTextDeltasIgnoresNonAssistant(t *testing.T) {
	svc := newRuntimeService()
	msg := apitypes.Message{ID: "msg-user", SessionID: "session-1", Role: apitypes.User}
	msg.Parts = []apitypes.ContentPart{apitypes.TextContent{Text: "hi"}}
	if events := svc.deriveOutputTextDeltasLocked(msg, "turn-1", time.Now()); len(events) != 0 {
		t.Fatalf("user delta emitted: %#v", events)
	}
}

func TestMessageUpdatedCoalescedWithin250msWindow(t *testing.T) {
	svc := newRuntimeService()
	base := time.Unix(0, 1_000_000_000).UTC()
	// Cursor must exist before we can coalesce; create via delta path first.
	msg := apitypes.Message{ID: "msg-1", SessionID: "session-1", Role: apitypes.Assistant}
	msg.Parts = []apitypes.ContentPart{apitypes.TextContent{Text: "a"}}
	_ = svc.deriveOutputTextDeltasLocked(msg, "turn-1", base)

	first := RuntimeEvent{Type: runtimeapi.EventMessageUpdated, MessageID: "msg-1", SessionID: "session-1"}
	if out := svc.recordMessageEventWithCoalesceLocked(first, base); out == nil {
		t.Fatal("first message.updated should be emitted")
	}
	// A second update within 100ms should be dropped.
	if out := svc.recordMessageEventWithCoalesceLocked(RuntimeEvent{Type: runtimeapi.EventMessageUpdated, MessageID: "msg-1", SessionID: "session-1"}, base.Add(100*time.Millisecond)); out != nil {
		t.Fatalf("coalesced message.updated leaked: %#v", out)
	}
	// After the 250ms window a new update should emit again.
	if out := svc.recordMessageEventWithCoalesceLocked(RuntimeEvent{Type: runtimeapi.EventMessageUpdated, MessageID: "msg-1", SessionID: "session-1"}, base.Add(300*time.Millisecond)); out == nil {
		t.Fatal("update after window should be emitted")
	}
	// message.completed always emits.
	if out := svc.recordMessageEventWithCoalesceLocked(RuntimeEvent{Type: runtimeapi.EventMessageCompleted, MessageID: "msg-1", SessionID: "session-1"}, base.Add(310*time.Millisecond)); out == nil {
		t.Fatal("message.completed must never be coalesced")
	}
}

func TestPublishSessionOutputEventFromRuntimeDeltaFanOut(t *testing.T) {
	svc := newRuntimeService()
	broker := newRuntimeSessionOutputBroker()
	svc.sessionOutputStream = broker

	sub := &runtimeSessionOutputSubscriber{
		sessionID: "session-1",
		events:    make(chan RuntimeOutputEvent, 4),
		dirty:     make(chan struct{}, 1),
		cancel:    make(chan struct{}),
	}
	broker.register(sub)
	defer broker.unregister(sub)

	svc.publishSessionOutputEventFromRuntime(RuntimeEvent{
		ID: "delta-1", Type: runtimeapi.EventOutputTextDelta, SessionID: "session-1", MessageID: "msg-1", TurnID: "turn-1",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   map[string]any{"part_type": "text", "delta": "hi", "content_len": 2},
	})
	select {
	case ev := <-sub.events:
		if ev.TextDelta == nil || ev.TextDelta.Delta != "hi" || ev.TextDelta.ContentLen != 2 {
			t.Fatalf("delta forwarded incorrectly: %#v", ev)
		}
		if ev.Kind != runtimeapi.EventOutputTextDelta {
			t.Fatalf("delta kind = %q", ev.Kind)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("delta event not forwarded")
	}
}

func TestPublishSessionOutputEventFromRuntimePersistedMarksDirty(t *testing.T) {
	svc := newRuntimeService()
	broker := newRuntimeSessionOutputBroker()
	svc.sessionOutputStream = broker

	sub := &runtimeSessionOutputSubscriber{
		sessionID: "session-1",
		events:    make(chan RuntimeOutputEvent, 4),
		dirty:     make(chan struct{}, 1),
		cancel:    make(chan struct{}),
	}
	broker.register(sub)
	defer broker.unregister(sub)

	svc.publishSessionOutputEventFromRuntime(RuntimeEvent{
		ID: "msg-updated", Type: runtimeapi.EventMessageUpdated, SessionID: "session-1", MessageID: "msg-1", TurnID: "turn-1", Sequence: 42,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	select {
	case <-sub.dirty:
		// good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("persisted event did not mark subscriber dirty")
	}
	select {
	case ev := <-sub.events:
		t.Fatalf("persisted event was forwarded directly: %#v", ev)
	default:
	}
}

func TestSessionOutputOverflowSignalsSnapshotRequired(t *testing.T) {
	svc := newRuntimeService()
	broker := newRuntimeSessionOutputBroker()
	svc.sessionOutputStream = broker

	sub := &runtimeSessionOutputSubscriber{
		sessionID: "session-1",
		events:    make(chan RuntimeOutputEvent, 1),
		dirty:     make(chan struct{}, 1),
		cancel:    make(chan struct{}),
	}
	broker.register(sub)
	defer broker.unregister(sub)

	// Fill the channel so the next send overflows.
	sub.events <- RuntimeOutputEvent{ID: "seed"}
	svc.publishSessionOutputEventFromRuntime(RuntimeEvent{
		ID: "delta-1", Type: runtimeapi.EventOutputTextDelta, SessionID: "session-1", MessageID: "msg-1",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   map[string]any{"part_type": "text", "delta": "hi", "content_len": 2},
	})
	// Overflow path drops one queued event to make room for the reset signal
	// (subscriber will re-hydrate from a snapshot anyway).
	select {
	case ev := <-sub.events:
		if ev.Kind != runtimeapi.EventSnapshotRequired {
			t.Fatalf("expected snapshot_required, got %#v", ev)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("overflow did not emit snapshot_required")
	}
}

func TestReplayInvariantPersistedOnlyDeltasIgnored(t *testing.T) {
	// The invariant only applies to persisted events. We build a projection,
	// run a synthesized runtime event through eventsFromRuntimeEvents, and
	// verify that ephemeral delta payloads do not appear in the result set
	// even if they arrive in the same batch of raw events.
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{{
			ID: "msg-1", SessionID: "session-1", Role: "assistant",
			Parts:     []RuntimeMessagePart{{Type: "text", Text: "hi"}},
			CreatedAt: 20, UpdatedAt: 21,
		}},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", StartedAt: 10}},
	})
	events := projection.eventsFromRuntimeEvents([]RuntimeEvent{{
		ID: "e-1", Sequence: 1, SessionID: "session-1", TurnID: "turn-1", MessageID: "msg-1", Type: runtimeapi.EventMessageUpdated, CreatedAt: "2026-06-28T00:00:00Z",
	}, {
		ID: "e-2", Sequence: 2, SessionID: "session-1", TurnID: "turn-1", MessageID: "msg-1", Type: runtimeapi.EventOutputTextDelta, CreatedAt: "2026-06-28T00:00:00Z",
		Payload: map[string]any{"part_type": "text", "delta": "hi", "content_len": 2},
	}})
	// Delta events should never produce projection sub-events because the
	// eventsFromRuntimeEvents switch does not know about them — this
	// asserts they don't accidentally get materialized as item updates.
	for _, ev := range events {
		if ev.Kind == runtimeapi.EventOutputTextDelta {
			t.Fatalf("delta leaked into materialized events: %#v", ev)
		}
	}
}
