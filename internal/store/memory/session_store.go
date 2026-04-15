package memory

import (
	"sync"

	"myclaw/internal/model"
	"myclaw/internal/store"
)

var _ store.SessionStore = (*SessionStore)(nil)

type SessionStore struct {
	mu                  sync.RWMutex
	sessionsByID        map[string]model.Session
	sessionsByKey       map[string]model.Session
	mainByAgentID       map[string]string
	messagesBySessionID map[string][]model.Message
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessionsByID:        make(map[string]model.Session),
		sessionsByKey:       make(map[string]model.Session),
		mainByAgentID:       make(map[string]string),
		messagesBySessionID: make(map[string][]model.Message),
	}
}

func (s *SessionStore) GetSessionByID(id string) (model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessionsByID[id]
	return sess, ok
}

func (s *SessionStore) GetSessionByKey(key string) (model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessionsByKey[key]
	return sess, ok
}

func (s *SessionStore) SaveSession(sess model.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionsByID[sess.ID] = sess
	s.sessionsByKey[sess.Key] = sess
}

func (s *SessionStore) ListSessions() []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]model.Session, 0, len(s.sessionsByID))
	for _, sess := range s.sessionsByID {
		sessions = append(sessions, sess)
	}
	return sessions
}

func (s *SessionStore) GetMainSessionKey(agentID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, ok := s.mainByAgentID[agentID]
	return key, ok
}

func (s *SessionStore) SaveMainSessionKey(agentID, sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mainByAgentID[agentID] = sessionKey
}

func (s *SessionStore) AppendMessage(msg model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messagesBySessionID[msg.SessionID] = append(s.messagesBySessionID[msg.SessionID], msg)
	return nil
}

func (s *SessionStore) Messages(sessionID string) ([]model.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages, ok := s.messagesBySessionID[sessionID]
	if !ok {
		return nil, false
	}

	cloned := append([]model.Message(nil), messages...)
	return cloned, true
}

func (s *SessionStore) ReplaceMessages(sessionID string, messages []model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := append([]model.Message(nil), messages...)
	s.messagesBySessionID[sessionID] = cloned
	return nil
}
