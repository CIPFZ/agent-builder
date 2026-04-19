package runtime

import (
	"testing"
	"time"

	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestRunnerCompactSessionUpdatesSessionMessagesAndMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "request one"},
		{"assistant", "answer one"},
		{"user", "request two"},
		{"assistant", "answer two"},
		{"user", "request three"},
		{"assistant", "answer three"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("AppendMessage(%s): %v", entry.role, err)
		}
	}

	memSvc := memory.NewService()
	memSvc.SaveCompactionSummary(sess, session.Message{
		ID:               "summary-old",
		SessionID:        sess.ID,
		Role:             "summary",
		Content:          "Summary: existing context",
		IsCompactSummary: true,
		CreatedAt:        time.Now().UTC(),
	})

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MemoryService:    memSvc,
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         3,
			PreserveRecentTurns: 1,
			SummaryPrefix:       "Summary:",
		}),
	})

	result, err := runner.CompactSession(sess.ID, "")
	if err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if !result.Changed {
		t.Fatalf("result = %#v, want changed compaction", result)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok || len(messages) == 0 {
		t.Fatalf("messages = %#v, ok=%v, want compacted transcript", messages, ok)
	}
	foundSummary := false
	for _, message := range messages {
		if message.Role == "summary" || message.IsCompactSummary {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("messages = %#v, want compact summary message", messages)
	}

	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q missing after compaction", sess.ID)
	}
	if updated.Metadata.LastCompactBoundaryID == "" {
		t.Fatalf("metadata = %#v, want compact boundary id", updated.Metadata)
	}
	if updated.Metadata.LastCompactionReason == "" {
		t.Fatalf("metadata = %#v, want compaction reason", updated.Metadata)
	}
	if updated.Metadata.LastCompactedAt.IsZero() {
		t.Fatalf("metadata = %#v, want compaction timestamp", updated.Metadata)
	}
}

func TestRunnerMicrocompactSessionMarksMetadataWhenToolOutputIsCompacted(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, msg := range []session.Message{
		{Role: "assistant", Content: "tool use wrapper", CreatedAt: time.Now().UTC()},
		{Role: "tool", Content: "system.run: very old output", CreatedAt: time.Now().UTC()},
		{Role: "assistant", Content: "next assistant", CreatedAt: time.Now().UTC()},
		{Role: "tool", Content: "system.run: recent output", CreatedAt: time.Now().UTC()},
		{Role: "assistant", Content: "tail assistant", CreatedAt: time.Now().UTC()},
	} {
		if _, err := sessions.AppendModelMessage(sess.ID, msg); err != nil {
			t.Fatalf("AppendModelMessage(%s): %v", msg.Role, err)
		}
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Compactor: compaction.NewService(compaction.Config{
			PreserveRecentTurns: 1,
		}),
	})

	result, err := runner.MicrocompactSession(sess.ID)
	if err != nil {
		t.Fatalf("MicrocompactSession: %v", err)
	}
	if !result.Changed {
		t.Fatalf("result = %#v, want changed microcompact", result)
	}

	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q missing after microcompact", sess.ID)
	}
	if updated.Metadata.LastCompactionReason != string(compaction.ReasonMicrocompact) {
		t.Fatalf("metadata = %#v, want microcompact reason", updated.Metadata)
	}
	if updated.Metadata.LastCompactedAt.IsZero() {
		t.Fatalf("metadata = %#v, want compaction timestamp", updated.Metadata)
	}
}
