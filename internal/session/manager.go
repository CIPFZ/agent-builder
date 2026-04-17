package session

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/store"
	memorystore "myclaw/internal/store/memory"
)

type Session = model.Session
type SessionMetadata = model.SessionMetadata

type Manager struct {
	nextID    atomic.Uint64
	nextMsgID atomic.Uint64

	store store.SessionStore
}

func NewManager(sessionStore store.SessionStore) *Manager {
	if sessionStore == nil {
		sessionStore = memorystore.NewSessionStore()
	}

	manager := &Manager{
		store: sessionStore,
	}
	manager.seedCountersFromStore()
	return manager
}

func (m *Manager) GetByID(id string) (Session, bool) {
	return m.store.GetSessionByID(id)
}

func (m *Manager) GetByKey(key string) (Session, bool) {
	return m.store.GetSessionByKey(key)
}

func (m *Manager) GetOrCreateMain(agentID string) Session {
	if agentID == "" {
		agentID = "main"
	}

	mainKey, ok := m.store.GetMainSessionKey(agentID)
	if ok {
		session, _ := m.store.GetSessionByKey(mainKey)
		return session
	}

	id := m.nextID.Add(1)
	session := Session{
		ID:      fmt.Sprintf("main-%06d", id),
		Key:     fmt.Sprintf("agent:%s:main", agentID),
		AgentID: agentID,
		IsMain:  true,
	}

	if mainKey, ok := m.store.GetMainSessionKey(agentID); ok {
		session, _ := m.store.GetSessionByKey(mainKey)
		return session
	}

	m.store.SaveSession(session)
	m.store.SaveMainSessionKey(agentID, session.Key)

	return session
}

func (m *Manager) Resolve(agentID, sessionKey string) (Session, error) {
	if agentID == "" {
		agentID = "main"
	}
	if sessionKey == "" {
		return m.GetOrCreateMain(agentID), nil
	}

	session, ok := m.GetByKey(sessionKey)
	if !ok {
		return Session{}, fmt.Errorf("session %q not found", sessionKey)
	}
	if session.AgentID != agentID {
		return Session{}, fmt.Errorf("session %q belongs to agent %q, not %q", sessionKey, session.AgentID, agentID)
	}

	return session, nil
}

func (m *Manager) CreateChild(agentID, key string) Session {
	if agentID == "" {
		agentID = "main"
	}
	id := m.nextID.Add(1)
	session := Session{
		ID:      fmt.Sprintf("session-%06d", id),
		Key:     key,
		AgentID: agentID,
		IsMain:  false,
	}
	m.store.SaveSession(session)
	return session
}

func (m *Manager) AppendMessage(sessionID, role, content string) (Message, error) {
	return m.AppendMessageWithBlocks(sessionID, role, content, "", nil)
}

func (m *Manager) AppendMessageWithBlocks(sessionID, role, content, providerMessageID string, blocks []model.MessageBlock) (Message, error) {
	session, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return Message{}, fmt.Errorf("session %q not found", sessionID)
	}

	msgID := m.nextMsgID.Add(1)
	msg := Message{
		ID:                fmt.Sprintf("msg-%06d", msgID),
		SessionID:         session.ID,
		Role:              role,
		Content:           content,
		ProviderMessageID: providerMessageID,
		Blocks:            append([]model.MessageBlock(nil), blocks...),
		CreatedAt:         time.Now().UTC(),
	}
	if err := m.store.AppendMessage(msg); err != nil {
		return Message{}, err
	}
	session.Metadata.LastActivityAt = msg.CreatedAt
	switch role {
	case "user":
		session.Metadata.LastUserMessageID = msg.ID
	case "assistant":
		session.Metadata.LastAssistantMessageID = msg.ID
	}
	m.store.SaveSession(session)

	return msg, nil
}

func (m *Manager) AppendModelMessage(sessionID string, msg Message) (Message, error) {
	session, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return Message{}, fmt.Errorf("session %q not found", sessionID)
	}
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%06d", m.nextMsgID.Add(1))
	}
	msg.SessionID = session.ID
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	msg.Blocks = append([]model.MessageBlock(nil), msg.Blocks...)
	if err := m.store.AppendMessage(msg); err != nil {
		return Message{}, err
	}
	session.Metadata.LastActivityAt = msg.CreatedAt
	switch msg.Role {
	case "user":
		session.Metadata.LastUserMessageID = msg.ID
	case "assistant":
		session.Metadata.LastAssistantMessageID = msg.ID
	}
	m.store.SaveSession(session)
	return msg, nil
}

func (m *Manager) Messages(sessionID string) ([]Message, bool) {
	return m.store.Messages(sessionID)
}

func (m *Manager) TranscriptMessages(sessionID string) ([]model.ClaudeTranscriptMessage, bool) {
	transcriptStore, ok := m.store.(store.TranscriptSessionStore)
	if !ok {
		return nil, false
	}
	return transcriptStore.TranscriptMessages(sessionID)
}

func (m *Manager) TranscriptEntries(sessionID string) ([]model.ClaudeTranscriptEntry, bool) {
	transcriptStore, ok := m.store.(store.TranscriptSessionStore)
	if !ok {
		return nil, false
	}
	return transcriptStore.TranscriptEntries(sessionID)
}

func (m *Manager) AppendTranscriptMessage(sessionID string, entry model.ClaudeTranscriptMessage) (Message, error) {
	return m.AppendTranscriptEntry(sessionID, model.NewClaudeTranscriptEntry(entry))
}

func (m *Manager) AppendTranscriptEntry(sessionID string, entry model.ClaudeTranscriptEntry) (Message, error) {
	session, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return Message{}, fmt.Errorf("session %q not found", sessionID)
	}
	transcriptStore, ok := m.store.(store.TranscriptSessionStore)
	if !ok {
		if entry.Message == nil {
			return Message{}, nil
		}
		message, err := model.MessageFromClaudeTranscript(*entry.Message, sessionID)
		if err != nil {
			return Message{}, err
		}
		return m.AppendModelMessage(sessionID, message)
	}
	if entry.Message != nil && entry.Message.UUID == "" {
		entry.Message.UUID = fmt.Sprintf("msg-%06d", m.nextMsgID.Add(1))
	}
	if entry.Message != nil && entry.Message.Timestamp == "" {
		entry.Message.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := transcriptStore.AppendTranscriptEntry(sessionID, entry); err != nil {
		return Message{}, err
	}
	if entry.Message == nil {
		return Message{}, nil
	}
	message, err := model.MessageFromClaudeTranscript(*entry.Message, sessionID)
	if err != nil {
		return Message{}, err
	}
	session.Metadata.LastActivityAt = message.CreatedAt
	switch message.Role {
	case "user":
		session.Metadata.LastUserMessageID = message.ID
	case "assistant":
		session.Metadata.LastAssistantMessageID = message.ID
	}
	m.store.SaveSession(session)
	return message, nil
}

func (m *Manager) ContinuationMessages(sessionID string) ([]Message, bool) {
	snapshot, ok := m.RecoverySnapshot(sessionID)
	if !ok {
		return nil, false
	}
	return append([]Message(nil), snapshot.Continuation...), true
}

func (m *Manager) RecoverySnapshot(sessionID string) (RecoverySnapshot, bool) {
	sess, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return RecoverySnapshot{}, false
	}
	messages, _ := m.store.Messages(sessionID)
	return BuildRecoverySnapshot(sess, messages), true
}

func (m *Manager) ContinuationState(sessionID string) (ContinuationState, bool) {
	snapshot, ok := m.RecoverySnapshot(sessionID)
	if !ok {
		return ContinuationState{}, false
	}
	return snapshot.ContinuationState(), true
}

func (m *Manager) ListSessions() []Session {
	return m.store.ListSessions()
}

func (m *Manager) ReplaceMessages(sessionID string, messages []Message) error {
	if _, ok := m.store.GetSessionByID(sessionID); !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return m.store.ReplaceMessages(sessionID, messages)
}

func (m *Manager) ReplaceTranscriptMessages(sessionID string, entries []model.ClaudeTranscriptMessage) error {
	if _, ok := m.store.GetSessionByID(sessionID); !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	transcriptStore, ok := m.store.(store.TranscriptSessionStore)
	if !ok {
		messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(entries, sessionID)
		if !ok {
			messages = nil
		}
		return m.store.ReplaceMessages(sessionID, messages)
	}
	return transcriptStore.ReplaceTranscriptMessages(sessionID, entries)
}

func (m *Manager) UpdateMetadata(sessionID string, update func(*SessionMetadata)) error {
	sess, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	update(&sess.Metadata)
	m.store.SaveSession(sess)
	return nil
}

func (m *Manager) seedCountersFromStore() {
	var maxSessionID uint64
	var maxMessageID uint64

	for _, sess := range m.store.ListSessions() {
		if n, ok := parseCounterSuffix(sess.ID, "main-", "session-"); ok && n > maxSessionID {
			maxSessionID = n
		}
		messages, ok := m.store.Messages(sess.ID)
		if !ok {
			continue
		}
		for _, msg := range messages {
			if n, ok := parseCounterSuffix(msg.ID, "msg-"); ok && n > maxMessageID {
				maxMessageID = n
			}
		}
	}

	m.nextID.Store(maxSessionID)
	m.nextMsgID.Store(maxMessageID)
}

func parseCounterSuffix(value string, prefixes ...string) (uint64, bool) {
	for _, prefix := range prefixes {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
