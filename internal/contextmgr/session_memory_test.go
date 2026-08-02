package contextmgr

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSessionMemoryLatestCompletedIgnoresStartedAndFailed(t *testing.T) {
	store, _ := testContextManager(t)
	ctx := context.Background()
	for _, revision := range []SessionMemoryRevision{
		{ID: "memory-1", SessionID: "session-1", Revision: 1, Status: SessionMemoryStatusCompleted, Content: "stable", LastSummarizedMessageID: "message-1", CreatedAt: 1, CompletedAt: 2},
		{ID: "memory-2", SessionID: "session-1", Revision: 2, Status: SessionMemoryStatusFailed, Error: "bad output", CreatedAt: 3, CompletedAt: 4},
		{ID: "memory-3", SessionID: "session-1", Revision: 3, Status: SessionMemoryStatusStarted, CreatedAt: 5},
	} {
		if _, err := store.UpsertSessionMemoryRevision(ctx, revision); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := store.LatestCompletedSessionMemory(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 1 || latest.Content != "stable" || latest.LastSummarizedMessageID != "message-1" {
		t.Fatalf("latest = %#v", latest)
	}
	next, err := store.NextSessionMemoryRevision(ctx, "session-1")
	if err != nil || next != 4 {
		t.Fatalf("next=%d err=%v", next, err)
	}
}

func TestSessionMemoryRevisionUniquePerSession(t *testing.T) {
	store, _ := testContextManager(t)
	ctx := context.Background()
	_, err := store.UpsertSessionMemoryRevision(ctx, SessionMemoryRevision{ID: "a", SessionID: "session-1", Revision: 1, CreatedAt: time.Now().UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertSessionMemoryRevision(ctx, SessionMemoryRevision{ID: "b", SessionID: "session-1", Revision: 1, CreatedAt: time.Now().UnixMilli()})
	if err == nil || err == sql.ErrNoRows {
		t.Fatalf("expected unique revision error, got %v", err)
	}
}
