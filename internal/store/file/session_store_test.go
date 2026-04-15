package file

import (
	"bufio"
	"encoding/json"
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
	if err := store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

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
	if err := store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "first",
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := store.ReplaceMessages(sess.ID, []model.Message{
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
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}

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
	if err := store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dir, "sessions.json"),
		filepath.Join(dir, "main_sessions.json"),
		filepath.Join(dir, "messages", "main-000001.jsonl"),
	} {
		if _, err := filepath.Abs(path); err != nil {
			t.Fatalf("abs path for %q: %v", path, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %q to exist: %v", path, err)
		}
	}
}

func TestSessionStorePersistsMessagesAsClaudeJSONLParentChain(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	if err := store.AppendMessage(model.Message{
		ID:        "user-uuid",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := store.AppendMessage(model.Message{
		ID:                "assistant-uuid",
		SessionID:         sess.ID,
		Role:              "assistant",
		Content:           "calling tool",
		ProviderMessageID: "provider-msg",
		Blocks: []model.MessageBlock{
			{Type: model.MessageBlockToolUse, ID: "toolu-1", Name: "system.run", InputObject: map[string]any{"command": "pwd"}},
		},
		CreatedAt: time.Unix(2, 0).UTC(),
	}); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	entries := readJSONLLines(t, filepath.Join(dir, "messages", "main-000001.jsonl"))
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0]["parentUuid"] != nil || entries[0]["type"] != "user" || entries[0]["uuid"] != "user-uuid" {
		t.Fatalf("first entry = %#v, want root user transcript", entries[0])
	}
	if entries[1]["parentUuid"] != "user-uuid" || entries[1]["type"] != "assistant" || entries[1]["uuid"] != "assistant-uuid" {
		t.Fatalf("second entry = %#v, want assistant chained to user", entries[1])
	}
	nested := entries[1]["message"].(map[string]any)
	content := nested["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "toolu-1" {
		t.Fatalf("assistant content = %#v, want native tool_use block", content)
	}

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := reloaded.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want chain restored from JSONL", messages)
	}
	if messages[1].ProviderMessageID != "provider-msg" || len(messages[1].Blocks) != 1 || messages[1].Blocks[0].ID != "toolu-1" {
		t.Fatalf("messages = %#v, want assistant tool_use restored", messages)
	}
}

func TestSessionStoreReplaceMessagesAppendsClaudeCompactBoundaryChain(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	if err := store.AppendMessage(model.Message{ID: "old-user", SessionID: sess.ID, Role: "user", Content: "old", CreatedAt: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatalf("append old message: %v", err)
	}
	if err := store.ReplaceMessages(sess.ID, []model.Message{
		{
			ID:              "boundary-uuid",
			SessionID:       sess.ID,
			Role:            "system",
			Subtype:         "compact_boundary",
			Content:         "Conversation compacted",
			LogicalParentID: "old-user",
			CompactMetadata: &model.CompactMetadata{Trigger: "auto", PreTokens: 100},
			CreatedAt:       time.Unix(2, 0).UTC(),
		},
		{ID: "summary-uuid", SessionID: sess.ID, Role: "user", Content: "continued summary", IsCompactSummary: true, CreatedAt: time.Unix(3, 0).UTC()},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}

	entries := readJSONLLines(t, filepath.Join(dir, "messages", "main-000001.jsonl"))
	if len(entries) != 3 {
		t.Fatalf("entry count = %d, want append-only old + compact replacement", len(entries))
	}
	if entries[1]["parentUuid"] != nil || entries[1]["logicalParentUuid"] != "old-user" || entries[1]["subtype"] != "compact_boundary" {
		t.Fatalf("boundary entry = %#v, want Claude compact chain break", entries[1])
	}
	if entries[2]["parentUuid"] != "boundary-uuid" || entries[2]["type"] != "user" {
		t.Fatalf("summary entry = %#v, want summary chained to compact boundary", entries[2])
	}

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := reloaded.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want latest compact chain only", messages)
	}
	if messages[0].ID != "boundary-uuid" || messages[1].ID != "summary-uuid" {
		t.Fatalf("messages = %#v, want compact replacement chain", messages)
	}
}

func TestSessionStoreAppendMessageReturnsTranscriptWriteErrorAndDoesNotMutateMemory(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	blockingPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	if err := os.Mkdir(blockingPath, 0o755); err != nil {
		t.Fatalf("create blocking transcript directory: %v", err)
	}

	err = store.AppendMessage(model.Message{
		ID:        "msg-000001",
		SessionID: sess.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err == nil {
		t.Fatal("expected append to return transcript write error")
	}
	if messages, ok := store.Messages(sess.ID); ok && len(messages) != 0 {
		t.Fatalf("messages = %#v, want no in-memory append after transcript write failure", messages)
	}
}

func TestSessionStoreReplaceMessagesReturnsTranscriptWriteErrorAndDoesNotMutateMemory(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}

	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	if err := store.AppendMessage(model.Message{ID: "msg-000001", SessionID: sess.ID, Role: "user", Content: "old", CreatedAt: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatalf("append seed message: %v", err)
	}
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	if err := os.Remove(transcriptPath); err != nil {
		t.Fatalf("remove transcript file: %v", err)
	}
	if err := os.Mkdir(transcriptPath, 0o755); err != nil {
		t.Fatalf("create blocking transcript directory: %v", err)
	}

	err = store.ReplaceMessages(sess.ID, []model.Message{{ID: "msg-000002", SessionID: sess.ID, Role: "assistant", Content: "new", CreatedAt: time.Unix(2, 0).UTC()}})
	if err == nil {
		t.Fatal("expected replace to return transcript write error")
	}
	messages, ok := store.Messages(sess.ID)
	if !ok || len(messages) != 1 || messages[0].ID != "msg-000001" {
		t.Fatalf("messages = %#v, want previous in-memory messages preserved after transcript write failure", messages)
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
			LastUserMessageID:       "msg-000001",
			LastAssistantMessageID:  "msg-000002",
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
			LastCompactionReason:    "message-limit",
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

func readJSONLLines(t *testing.T, path string) []map[string]any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer file.Close()

	var entries []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode jsonl line %q: %v", scanner.Text(), err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan jsonl: %v", err)
	}
	return entries
}
