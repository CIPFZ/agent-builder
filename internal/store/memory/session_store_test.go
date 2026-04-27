package memory

import (
	"testing"
	"time"

	"myclaw/internal/model"
)

func TestSessionStoreSavesSessionAndMessages(t *testing.T) {
	store := NewSessionStore()
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}

	store.SaveSession(sess)
	store.SaveMainSessionKey("main", sess.Key)
	store.AppendMessage(model.Message{ID: "msg-000001", SessionID: sess.ID, Role: "user", Content: "hello"})

	got, ok := store.GetSessionByKey(sess.Key)
	if !ok {
		t.Fatal("expected session by key")
	}
	if got.ID != sess.ID {
		t.Fatalf("session id = %q, want %q", got.ID, sess.ID)
	}

	mainKey, ok := store.GetMainSessionKey("main")
	if !ok || mainKey != sess.Key {
		t.Fatalf("main key = %q, want %q", mainKey, sess.Key)
	}

	messages, ok := store.Messages(sess.ID)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want 1 message", messages)
	}
}

func TestSessionStoreAppendMessageContinuesFromMainChainAfterSidechain(t *testing.T) {
	store := NewSessionStore()
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	if err := store.AppendTranscriptMessage(sess.ID, model.ClaudeTranscriptMessage{
		Type:      "user",
		UUID:      "root-user",
		Timestamp: time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
		Message:   &model.ClaudeAPIMessage{Role: "user", Content: "root"},
	}); err != nil {
		t.Fatalf("append root transcript: %v", err)
	}
	if err := store.AppendTranscriptMessage(sess.ID, model.ClaudeTranscriptMessage{
		ParentUUID:  stringPtr("root-user"),
		IsSidechain: true,
		Type:        "assistant",
		UUID:        "sidechain-leaf",
		Timestamp:   time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
		Message:     &model.ClaudeAPIMessage{ID: "provider-side", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "sidechain"}}},
	}); err != nil {
		t.Fatalf("append sidechain transcript: %v", err)
	}
	if err := store.AppendMessage(model.Message{
		ID:        "main-followup",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "continue main",
		CreatedAt: time.Unix(3, 0).UTC(),
	}); err != nil {
		t.Fatalf("append main message: %v", err)
	}

	transcript, ok := store.TranscriptMessages(sess.ID)
	if !ok || len(transcript) != 3 {
		t.Fatalf("transcript = %#v, want three entries", transcript)
	}
	if transcript[2].ParentUUID == nil || *transcript[2].ParentUUID != "root-user" {
		t.Fatalf("transcript = %#v, want main follow-up parented to non-sidechain leaf", transcript)
	}
}

func TestSessionStoreAppendTranscriptMessageRollsBackInvalidEntry(t *testing.T) {
	store := NewSessionStore()
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)

	err := store.AppendTranscriptMessage(sess.ID, model.ClaudeTranscriptMessage{
		Type:      "user",
		UUID:      "bad-timestamp",
		Timestamp: "not-a-timestamp",
		Message:   &model.ClaudeAPIMessage{Role: "user", Content: "bad"},
	})
	if err == nil {
		t.Fatal("append transcript message succeeded, want timestamp parse error")
	}
	if transcript, ok := store.TranscriptMessages(sess.ID); ok && len(transcript) != 0 {
		t.Fatalf("transcript = %#v, want invalid entry not retained", transcript)
	}
}

func TestSessionStoreListSessionsHasDeterministicRecentOrder(t *testing.T) {
	store := NewSessionStore()
	store.SaveSession(model.Session{
		ID:      "session-000001",
		Key:     "agent:main:session:000001",
		AgentID: "main",
		Metadata: model.SessionMetadata{
			LastActivityAt: time.Unix(10, 0).UTC(),
		},
	})
	store.SaveSession(model.Session{
		ID:      "session-000003",
		Key:     "agent:main:session:000003",
		AgentID: "main",
		Metadata: model.SessionMetadata{
			LastActivityAt: time.Unix(10, 0).UTC(),
		},
	})
	store.SaveSession(model.Session{
		ID:      "session-000002",
		Key:     "agent:main:session:000002",
		AgentID: "main",
		Metadata: model.SessionMetadata{
			LastActivityAt: time.Unix(20, 0).UTC(),
		},
	})

	got := store.ListSessions()
	want := []string{"session-000002", "session-000003", "session-000001"}
	if len(got) != len(want) {
		t.Fatalf("session count = %d, want %d", len(got), len(want))
	}
	for i, session := range got {
		if session.ID != want[i] {
			t.Fatalf("session[%d] = %q, want %q; all = %#v", i, session.ID, want[i], got)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}
