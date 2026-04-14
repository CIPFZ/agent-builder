package session

import (
	"path/filepath"
	"testing"
	"time"

	fileStore "myclaw/internal/store/file"
)

func TestManagerSeedsSessionAndMessageCountersFromPersistentStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := fileStore.NewSessionStore(root)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	manager := NewManager(store)
	main := manager.GetOrCreateMain("main")
	first, err := manager.AppendMessage(main.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append first message: %v", err)
	}
	child := manager.CreateChild("main", "agent:main:child:1")

	reloadedStore, err := fileStore.NewSessionStore(root)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	reloaded := NewManager(reloadedStore)

	second, err := reloaded.AppendMessage(main.ID, "assistant", "world")
	if err != nil {
		t.Fatalf("append second message: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("second message id = %q, want new id after reload", second.ID)
	}
	if second.ID != "msg-000002" {
		t.Fatalf("second message id = %q, want msg-000002", second.ID)
	}

	nextChild := reloaded.CreateChild("main", "agent:main:child:2")
	if nextChild.ID == child.ID {
		t.Fatalf("child session id = %q, want new id after reload", nextChild.ID)
	}
	if nextChild.ID != "session-000003" {
		t.Fatalf("child session id = %q, want session-000003", nextChild.ID)
	}
}

func TestManagerAppendMessageUpdatesSessionMetadata(t *testing.T) {
	manager := NewManager(nil)
	main := manager.GetOrCreateMain("main")

	user, err := manager.AppendMessage(main.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	assistant, err := manager.AppendMessage(main.ID, "assistant", "world")
	if err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	updated, ok := manager.GetByID(main.ID)
	if !ok {
		t.Fatalf("session %q not found", main.ID)
	}
	if updated.Metadata.LastUserMessageID != user.ID {
		t.Fatalf("metadata = %#v, want last user message %q", updated.Metadata, user.ID)
	}
	if updated.Metadata.LastAssistantMessageID != assistant.ID {
		t.Fatalf("metadata = %#v, want last assistant message %q", updated.Metadata, assistant.ID)
	}
	if updated.Metadata.LastActivityAt.IsZero() {
		t.Fatalf("metadata = %#v, want last activity timestamp", updated.Metadata)
	}
}

func TestManagerUpdateMetadataPersistsSessionMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := fileStore.NewSessionStore(root)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	manager := NewManager(store)
	main := manager.GetOrCreateMain("main")
	now := time.Unix(100, 0).UTC()
	err = manager.UpdateMetadata(main.ID, func(metadata *SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = "message-limit"
		metadata.LastCompactedAt = now
		metadata.InitialMainLoopModel = "claude-sonnet-4-6"
		metadata.MainLoopModelOverride = "claude-opus-4-6"
	})
	if err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	reloadedStore, err := fileStore.NewSessionStore(root)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	reloaded := NewManager(reloadedStore)
	got, ok := reloaded.GetByID(main.ID)
	if !ok {
		t.Fatalf("session %q not found after reload", main.ID)
	}
	if got.Metadata.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("metadata = %#v, want compact boundary", got.Metadata)
	}
	if got.Metadata.LastCompactedAt != now {
		t.Fatalf("metadata = %#v, want compaction timestamp %v", got.Metadata, now)
	}
	if got.Metadata.InitialMainLoopModel != "claude-sonnet-4-6" {
		t.Fatalf("metadata = %#v, want initial main loop model", got.Metadata)
	}
	if got.Metadata.MainLoopModelOverride != "claude-opus-4-6" {
		t.Fatalf("metadata = %#v, want persisted main loop override", got.Metadata)
	}
}
