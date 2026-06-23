package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestRuntimeUserInputStorePersistsNormalizedEvidenceWithoutRawImageData(t *testing.T) {
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = db.Release(dataDir)
	})
	store := newRuntimeUserInputStore(conn)
	normalized := RuntimeNormalizedInput{
		ID:          "input-1",
		SessionID:   "session-1",
		Mode:        runtimeInputModePrompt,
		Prompt:      "describe",
		ShouldQuery: true,
		Attachments: []RuntimeAttachmentDraft{{
			ID:       "att-1",
			Type:     runtimeInputItemImage,
			MIMEType: "image/png",
		}},
		CreatedAt: 10,
	}
	items := []RuntimeUserInputItem{{
		Type:     runtimeInputItemImage,
		MIMEType: "image/png",
		Data:     "raw-base64-image-data",
	}}
	if _, err := store.Upsert(context.Background(), normalized, items, "turn-1"); err != nil {
		t.Fatal(err)
	}
	var itemsJSON string
	if err := conn.QueryRowContext(context.Background(), `SELECT items_json FROM runtime_user_inputs WHERE id = ?`, "input-1").Scan(&itemsJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(itemsJSON, "raw-base64-image-data") {
		t.Fatalf("raw image data was persisted: %s", itemsJSON)
	}
	if !strings.Contains(itemsJSON, "[redacted:") {
		t.Fatalf("redacted image marker missing: %s", itemsJSON)
	}
	stored, err := store.Get(context.Background(), "input-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != "input-1" || stored.SessionID != "session-1" || len(stored.Attachments) != 1 {
		t.Fatalf("stored = %#v", stored)
	}
	if _, err := store.Get(context.Background(), "missing"); err == nil || !errors.Is(err, errRuntimeUserInputNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}
