package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func TestRuntimeEventStoreAppendListAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeEventStore(conn)
	err = store.Append(context.Background(), RuntimeEvent{
		ID:         "evt-1",
		Sequence:   1,
		Type:       runtimeapi.EventToolCallOutput,
		SessionID:  "session-1",
		TurnID:     "turn-1",
		MessageID:  "message-1",
		ToolCallID: "tool-1",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"authorization": "Bearer sk-secret",
			"summary":       "ok",
			"nested": map[string]any{
				"api_key": "sk-secret",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := store.ListTurn(context.Background(), "turn-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("events = %#v", resp.Events)
	}
	event := resp.Events[0]
	if event.Sequence != 1 || event.ID != "evt-1" || event.MessageID != "message-1" || event.ToolCallID != "tool-1" {
		t.Fatalf("event linkage = %#v", event)
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "sk-secret") || strings.Contains(strings.ToLower(string(data)), "bearer sk") {
		t.Fatalf("persisted event leaked secret: %s", data)
	}

	maxSequence, err := store.MaxSequence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if maxSequence != 1 {
		t.Fatalf("max sequence = %d", maxSequence)
	}
}

func TestRuntimeEventStoreFiltersBySessionAndCursor(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeEventStore(conn)
	for i, sessionID := range []string{"session-1", "session-2", "session-1"} {
		if err := store.Append(context.Background(), RuntimeEvent{
			ID:        "evt-filter-" + string(rune('1'+i)),
			Sequence:  int64(i + 1),
			Type:      runtimeapi.EventMessageCreated,
			SessionID: sessionID,
			TurnID:    "turn-filter",
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload:   map[string]any{"index": i},
		}); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := store.ListSession(context.Background(), "session-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 || resp.Events[0].Sequence != 3 {
		t.Fatalf("filtered events = %#v", resp.Events)
	}
}
