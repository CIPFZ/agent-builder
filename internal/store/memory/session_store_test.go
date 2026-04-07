package memory

import (
	"testing"

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
