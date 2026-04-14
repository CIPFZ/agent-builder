package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"myclaw/internal/model"
)

func TestSessionStorePersistsSessionsAndMessagesAcrossReload(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	store.SaveMainSessionKey("main", sess.Key)
	store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	})

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}

	got, ok := reloaded.GetSessionByKey(sess.Key)
	if !ok {
		t.Fatal("expected session after reload")
	}
	if got.ID != sess.ID {
		t.Fatalf("session id = %q, want %q", got.ID, sess.ID)
	}

	mainKey, ok := reloaded.GetMainSessionKey("main")
	if !ok || mainKey != sess.Key {
		t.Fatalf("main key = %q, want %q", mainKey, sess.Key)
	}

	messages, ok := reloaded.Messages(sess.ID)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want one message after reload", messages)
	}
	if messages[0].Content != "hello" {
		t.Fatalf("message content = %q, want hello", messages[0].Content)
	}
}

func TestSessionStoreReplaceMessagesPersistsReplacementAcrossReload(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "first",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	store.ReplaceMessages(sess.ID, []model.Message{
		{
			ID:        "summary-1",
			SessionID: sess.ID,
			Role:      "summary",
			Content:   "Summary: compacted",
			CreatedAt: time.Unix(2, 0).UTC(),
		},
		{
			ID:        "compact-1",
			SessionID: sess.ID,
			Role:      "system",
			Content:   "[compact_boundary]",
			CreatedAt: time.Unix(3, 0).UTC(),
		},
	})

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}

	messages, ok := reloaded.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want two replaced messages", messages)
	}
	if messages[0].Role != "summary" {
		t.Fatalf("messages = %#v, want replaced summary message first", messages)
	}
	if messages[1].Content != "[compact_boundary]" {
		t.Fatalf("messages = %#v, want persisted compact boundary", messages)
	}
}

func TestSessionStoreCreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	store.SaveMainSessionKey("main", sess.Key)
	store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	})

	for _, path := range []string{
		filepath.Join(dir, "sessions.json"),
		filepath.Join(dir, "main_sessions.json"),
		filepath.Join(dir, "messages", "main-000001.json"),
	} {
		if _, err := filepath.Abs(path); err != nil {
			t.Fatalf("abs path for %q: %v", path, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %q to exist: %v", path, err)
		}
	}
}

func TestSessionStorePersistsSessionMetadataAcrossReload(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
		Metadata: model.SessionMetadata{
			LastUserMessageID:      "msg-000001",
			LastAssistantMessageID: "msg-000002",
			LastCompactBoundaryID:  "compact-1",
			LastCompactionSummaryID:"summary-1",
			LastCompactionReason:   "message-limit",
		},
	}
	store.SaveSession(sess)

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}

	got, ok := reloaded.GetSessionByID(sess.ID)
	if !ok {
		t.Fatal("expected session after reload")
	}
	if got.Metadata.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("metadata = %#v, want persisted compact boundary", got.Metadata)
	}
	if got.Metadata.LastCompactionSummaryID != "summary-1" {
		t.Fatalf("metadata = %#v, want persisted compaction summary", got.Metadata)
	}
	if got.Metadata.LastCompactionReason != "message-limit" {
		t.Fatalf("metadata = %#v, want persisted compaction reason", got.Metadata)
	}
}
