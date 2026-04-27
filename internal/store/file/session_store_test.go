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

func TestSessionStoreLoadsLatestNonSidechainLeafFromClaudeTranscript(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "messages"), 0o755); err != nil {
		t.Fatalf("mkdir messages: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	if err := writeJSON(filepath.Join(dir, "sessions.json"), []model.Session{sess}); err != nil {
		t.Fatalf("write sessions: %v", err)
	}
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	entries := []model.ClaudeTranscriptMessage{
		{
			ParentUUID:  nil,
			IsSidechain: false,
			Type:        "user",
			UUID:        "root-user",
			Timestamp:   time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
			Message:     &model.ClaudeAPIMessage{Role: "user", Content: "root"},
		},
		{
			ParentUUID:  stringPtr("root-user"),
			IsSidechain: false,
			Type:        "assistant",
			UUID:        "main-leaf",
			Timestamp:   time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
			Message:     &model.ClaudeAPIMessage{ID: "provider-main", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "main answer"}}},
		},
		{
			ParentUUID:  stringPtr("root-user"),
			IsSidechain: true,
			Type:        "assistant",
			UUID:        "sidechain-leaf",
			Timestamp:   time.Unix(3, 0).UTC().Format(time.RFC3339Nano),
			Message:     &model.ClaudeAPIMessage{ID: "provider-side", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "sidechain answer"}}},
		},
	}
	writeTranscriptEntries(t, transcriptPath, entries)

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := store.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want main chain only", messages)
	}
	if messages[1].ID != "main-leaf" || messages[1].Content != "main answer" {
		t.Fatalf("messages = %#v, want latest non-sidechain leaf chain", messages)
	}
	transcript, ok := store.TranscriptMessages(sess.ID)
	if !ok || len(transcript) != 3 {
		t.Fatalf("transcript = %#v, want all Claude transcript entries preserved", transcript)
	}
	if !transcript[2].IsSidechain || transcript[2].UUID != "sidechain-leaf" {
		t.Fatalf("transcript = %#v, want sidechain substrate preserved", transcript)
	}
}

func TestSessionStorePreservesMetadataEntriesAcrossReload(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "messages"), 0o755); err != nil {
		t.Fatalf("mkdir messages: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	if err := writeJSON(filepath.Join(dir, "sessions.json"), []model.Session{sess}); err != nil {
		t.Fatalf("write sessions: %v", err)
	}
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	writeRawJSONLLines(t, transcriptPath, []map[string]any{
		{"type": "custom-title", "sessionId": sess.ID, "customTitle": "Claude parity"},
		{"type": "tag", "sessionId": sess.ID, "tag": "review"},
		{"type": "mode", "sessionId": sess.ID, "mode": "normal"},
		{"type": "content-replacement", "sessionId": sess.ID, "replacements": []map[string]any{{"messageId": "msg-1", "blockIndex": 0}}},
		{"parentUuid": nil, "isSidechain": false, "type": "user", "uuid": "msg-1", "timestamp": time.Unix(1, 0).UTC().Format(time.RFC3339Nano), "message": map[string]any{"role": "user", "content": "hello"}},
	})

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	entries, ok := store.TranscriptEntries(sess.ID)
	if !ok || len(entries) != 5 {
		t.Fatalf("entries = %#v, want all transcript and metadata entries", entries)
	}
	if entries[0].Type != "custom-title" || entries[0].Raw["customTitle"] != "Claude parity" {
		t.Fatalf("entries = %#v, want custom-title metadata preserved", entries)
	}
	if entries[3].Type != "content-replacement" {
		t.Fatalf("entries = %#v, want content-replacement metadata preserved", entries)
	}
}

func TestSessionStoreAppendMetadataEntryPersistsJSONL(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)

	if err := store.AppendTranscriptEntry(sess.ID, model.NewClaudeMetadataEntry(map[string]any{
		"type":        "custom-title",
		"sessionId":   sess.ID,
		"customTitle": "Claude parity",
	})); err != nil {
		t.Fatalf("append metadata entry: %v", err)
	}

	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	entries, ok := reloaded.TranscriptEntries(sess.ID)
	if !ok || len(entries) != 1 || entries[0].Raw["customTitle"] != "Claude parity" {
		t.Fatalf("entries = %#v, want metadata entry persisted", entries)
	}
}

func TestSessionStoreAppendMessageContinuesFromMainChainAfterSidechain(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
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

func TestSessionStorePersistsAttachmentAsClaudeTranscriptEntry(t *testing.T) {
	dir := t.TempDir()

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)
	if err := store.AppendMessage(model.Message{
		ID:                        "dynamic-skill-attachment",
		SessionID:                 sess.ID,
		Role:                      "attachment",
		Subtype:                   "dynamic_skill",
		Content:                   `{"type":"dynamic_skill","skillNames":["review"]}`,
		IsMeta:                    true,
		IsVisibleInTranscriptOnly: true,
		CreatedAt:                 time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("append attachment: %v", err)
	}

	entries := readJSONLLines(t, filepath.Join(dir, "messages", sess.ID+".jsonl"))
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one attachment transcript entry", entries)
	}
	if entries[0]["type"] != "attachment" || entries[0]["subtype"] != "dynamic_skill" || entries[0]["isMeta"] != true {
		t.Fatalf("entry = %#v, want Claude attachment transcript shape", entries[0])
	}
	reloaded, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := reloaded.Messages(sess.ID)
	if !ok || len(messages) != 1 || messages[0].Role != "attachment" {
		t.Fatalf("messages = %#v, want attachment runtime view restored", messages)
	}
}

func TestSessionStoreAppendTranscriptMessageRollsBackInvalidEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	store.SaveSession(sess)

	err = store.AppendTranscriptMessage(sess.ID, model.ClaudeTranscriptMessage{
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
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	if _, statErr := os.Stat(transcriptPath); statErr == nil {
		if entries := readJSONLLines(t, transcriptPath); len(entries) != 0 {
			t.Fatalf("entries = %#v, want invalid entry not written", entries)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("stat transcript: %v", statErr)
	}
}

func TestSessionStoreLoadsNearestUserAssistantLeafWhenTranscriptEndsWithAttachment(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "messages"), 0o755); err != nil {
		t.Fatalf("mkdir messages: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	if err := writeJSON(filepath.Join(dir, "sessions.json"), []model.Session{sess}); err != nil {
		t.Fatalf("write sessions: %v", err)
	}
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	writeTranscriptEntries(t, transcriptPath, []model.ClaudeTranscriptMessage{
		{
			Type:      "user",
			UUID:      "root-user",
			Timestamp: time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
			Message:   &model.ClaudeAPIMessage{Role: "user", Content: "root"},
		},
		{
			ParentUUID: stringPtr("root-user"),
			Type:       "assistant",
			UUID:       "assistant-leaf",
			Timestamp:  time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
			Message:    &model.ClaudeAPIMessage{ID: "provider-main", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "main answer"}}},
		},
		{
			ParentUUID:                stringPtr("assistant-leaf"),
			Type:                      "attachment",
			UUID:                      "trailing-attachment",
			Subtype:                   "dynamic_skill",
			Content:                   `{"type":"dynamic_skill"}`,
			IsMeta:                    true,
			IsVisibleInTranscriptOnly: true,
			Timestamp:                 time.Unix(3, 0).UTC().Format(time.RFC3339Nano),
		},
	})

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := store.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want user -> assistant leaf chain without trailing attachment", messages)
	}
	if messages[1].ID != "assistant-leaf" {
		t.Fatalf("messages = %#v, want assistant leaf selected for resume", messages)
	}
}

func TestSessionStoreBridgesLegacyProgressParentChain(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "messages"), 0o755); err != nil {
		t.Fatalf("mkdir messages: %v", err)
	}
	sess := model.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true}
	if err := writeJSON(filepath.Join(dir, "sessions.json"), []model.Session{sess}); err != nil {
		t.Fatalf("write sessions: %v", err)
	}
	transcriptPath := filepath.Join(dir, "messages", sess.ID+".jsonl")
	lines := []map[string]any{
		{
			"parentUuid":  nil,
			"isSidechain": false,
			"type":        "user",
			"uuid":        "root-user",
			"timestamp":   time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
			"message":     map[string]any{"role": "user", "content": "root"},
		},
		{
			"parentUuid":  "root-user",
			"isSidechain": false,
			"type":        "progress",
			"uuid":        "legacy-progress",
			"timestamp":   time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
		},
		{
			"parentUuid":  "legacy-progress",
			"isSidechain": false,
			"type":        "assistant",
			"uuid":        "assistant-leaf",
			"timestamp":   time.Unix(3, 0).UTC().Format(time.RFC3339Nano),
			"message":     map[string]any{"id": "provider-main", "role": "assistant", "type": "message", "content": []map[string]any{{"type": "text", "text": "main answer"}}},
		},
	}
	writeRawJSONLLines(t, transcriptPath, lines)

	store, err := NewSessionStore(dir)
	if err != nil {
		t.Fatalf("reload session store: %v", err)
	}
	messages, ok := store.Messages(sess.ID)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want progress bridged out of chain", messages)
	}
	if messages[0].ID != "root-user" || messages[1].ID != "assistant-leaf" {
		t.Fatalf("messages = %#v, want root -> assistant chain", messages)
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

func writeTranscriptEntries(t *testing.T, path string, entries []model.ClaudeTranscriptMessage) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			t.Fatalf("encode transcript entry: %v", err)
		}
	}
}

func writeRawJSONLLines(t *testing.T, path string, entries []map[string]any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			t.Fatalf("encode transcript entry: %v", err)
		}
	}
}

func TestSessionStoreListSessionsHasDeterministicRecentOrder(t *testing.T) {
	store, err := NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
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
