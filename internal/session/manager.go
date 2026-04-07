package session

import (
	"fmt"
	"sync/atomic"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/store"
	memorystore "myclaw/internal/store/memory"
)

type Session = model.Session

type Manager struct {
	nextID    atomic.Uint64
	nextMsgID atomic.Uint64

	store store.SessionStore
}

func NewManager(sessionStore store.SessionStore) *Manager {
	if sessionStore == nil {
		sessionStore = memorystore.NewSessionStore()
	}

	return &Manager{
		store: sessionStore,
	}
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
	session, ok := m.store.GetSessionByID(sessionID)
	if !ok {
		return Message{}, fmt.Errorf("session %q not found", sessionID)
	}

	msgID := m.nextMsgID.Add(1)
	msg := Message{
		ID:        fmt.Sprintf("msg-%06d", msgID),
		SessionID: session.ID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	m.store.AppendMessage(msg)

	return msg, nil
}

func (m *Manager) Messages(sessionID string) ([]Message, bool) {
	return m.store.Messages(sessionID)
}

func (m *Manager) ListSessions() []Session {
	return m.store.ListSessions()
}

func (m *Manager) ReplaceMessages(sessionID string, messages []Message) error {
	if _, ok := m.store.GetSessionByID(sessionID); !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	m.store.ReplaceMessages(sessionID, messages)
	return nil
}
