package runtime

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestCanonicalDiffEntityEventsDoesNotAmplifyTransportMetadataChanges(t *testing.T) {
	previous := canonicalDiffFixtureSnapshot(100, 10_000, true)
	current := canonicalDiffFixtureSnapshot(100, 20_000, false)
	current.Messages[73].Content = "changed"

	raw := RuntimeEvent{
		ID:        "raw-1000",
		Sequence:  1000,
		Type:      runtimeapi.EventConversationReconciled,
		SessionID: "session-1",
		CreatedAt: time.Unix(1_000, 0).UTC().Format(time.RFC3339Nano),
	}
	events, err := canonicalDiffEntityEvents(raw, previous, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("one semantic change produced %d entity events", len(events))
	}
	event := events[0]
	if event.EntityType != RuntimeConversationEntityMessage || event.EntityID != "message-73" || event.Revision != "1000" {
		t.Fatalf("unexpected changed entity event: %#v", event)
	}
}

func TestCanonicalDiffEntityEventsEmitsNothingForRepeatedSemanticState(t *testing.T) {
	previous := canonicalDiffFixtureSnapshot(100, 10_000, true)
	current := canonicalDiffFixtureSnapshot(100, 20_000, false)
	raw := RuntimeEvent{
		ID:        "raw-1001",
		Sequence:  1001,
		Type:      runtimeapi.EventConversationReconciled,
		SessionID: "session-1",
		CreatedAt: time.Unix(1_001, 0).UTC().Format(time.RFC3339Nano),
	}

	events, err := canonicalDiffEntityEvents(raw, previous, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("repeated semantic state produced %d entity events", len(events))
	}
}

func canonicalDiffFixtureSnapshot(count, revisionBase int, inflated bool) RuntimeCanonicalConversationSnapshot {
	messages := make([]RuntimeCanonicalMessage, 0, count)
	for index := 0; index < count; index++ {
		sequence := strconv.Itoa(index + 1)
		updatedAt := int64(index + 1)
		activitySequence := sequence
		if inflated {
			updatedAt += 10_000
			activitySequence = strconv.Itoa(30_000 + index)
		}
		messages = append(messages, RuntimeCanonicalMessage{
			RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{
				ID:               fmt.Sprintf("message-%d", index),
				SessionID:        "session-1",
				TurnID:           "turn-1",
				ActivitySequence: activitySequence,
				Revision:         strconv.Itoa(revisionBase + index),
				CreatedAt:        1,
				UpdatedAt:        updatedAt,
			},
			Role: "assistant", Status: "completed", Content: "same",
		})
	}
	return RuntimeCanonicalConversationSnapshot{Messages: messages}
}
