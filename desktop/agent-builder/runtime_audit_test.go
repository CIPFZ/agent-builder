package main

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimeAuditStoreAppendAndListTurn(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeAuditStore(conn)
	err = store.Append(context.Background(), RuntimeAuditEvent{
		ID:        "audit-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      "started",
		CreatedAt: "2026-05-18T00:00:00Z",
		Payload:   map[string]any{"model": "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := store.ListTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("events = %#v", resp.Events)
	}
	if resp.Events[0].Payload["model"] != "test-model" {
		t.Fatalf("payload = %#v", resp.Events[0].Payload)
	}
}
