package file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"myclaw/internal/model"
	"myclaw/internal/store"
)

var _ store.SessionStore = (*SessionStore)(nil)

type SessionStore struct {
	mu            sync.RWMutex
	root          string
	sessionsByID  map[string]model.Session
	sessionsByKey map[string]model.Session
	mainByAgentID map[string]string
	messagesByID  map[string][]model.Message
}

func NewSessionStore(root string) (*SessionStore, error) {
	if root == "" {
		return nil, fmt.Errorf("root is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "messages"), 0o755); err != nil {
		return nil, err
	}
	s := &SessionStore{
		root:          root,
		sessionsByID:  make(map[string]model.Session),
		sessionsByKey: make(map[string]model.Session),
		mainByAgentID: make(map[string]string),
		messagesByID:  make(map[string][]model.Message),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
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
	_ = s.persistSessionsLocked()
}

func (s *SessionStore) ListSessions() []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Session, 0, len(s.sessionsByID))
	for _, sess := range s.sessionsByID {
		out = append(out, sess)
	}
	return out
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
	_ = s.persistMainSessionsLocked()
}

func (s *SessionStore) AppendMessage(msg model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesByID[msg.SessionID] = append(s.messagesByID[msg.SessionID], msg)
	_ = s.persistMessagesLocked(msg.SessionID)
}

func (s *SessionStore) Messages(sessionID string) ([]model.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages, ok := s.messagesByID[sessionID]
	if !ok {
		return nil, false
	}
	return append([]model.Message(nil), messages...), true
}

func (s *SessionStore) ReplaceMessages(sessionID string, messages []model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesByID[sessionID] = append([]model.Message(nil), messages...)
	_ = s.persistMessagesLocked(sessionID)
}

func (s *SessionStore) load() error {
	if err := s.loadSessions(); err != nil {
		return err
	}
	if err := s.loadMainSessions(); err != nil {
		return err
	}
	return s.loadMessages()
}

func (s *SessionStore) loadSessions() error {
	var sessions []model.Session
	if err := readJSON(filepath.Join(s.root, "sessions.json"), &sessions); err != nil {
		return err
	}
	for _, sess := range sessions {
		s.sessionsByID[sess.ID] = sess
		s.sessionsByKey[sess.Key] = sess
	}
	return nil
}

func (s *SessionStore) loadMainSessions() error {
	var data map[string]string
	if err := readJSON(filepath.Join(s.root, "main_sessions.json"), &data); err != nil {
		return err
	}
	for key, value := range data {
		s.mainByAgentID[key] = value
	}
	return nil
}

func (s *SessionStore) loadMessages() error {
	entries, err := os.ReadDir(filepath.Join(s.root, "messages"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		sessionID := entry.Name()[:len(entry.Name())-len(".json")]
		var messages []model.Message
		if err := readJSON(filepath.Join(s.root, "messages", entry.Name()), &messages); err != nil {
			return err
		}
		s.messagesByID[sessionID] = messages
	}
	return nil
}

func (s *SessionStore) persistSessionsLocked() error {
	sessions := make([]model.Session, 0, len(s.sessionsByID))
	for _, sess := range s.sessionsByID {
		sessions = append(sessions, sess)
	}
	return writeJSON(filepath.Join(s.root, "sessions.json"), sessions)
}

func (s *SessionStore) persistMainSessionsLocked() error {
	data := make(map[string]string, len(s.mainByAgentID))
	for key, value := range s.mainByAgentID {
		data[key] = value
	}
	return writeJSON(filepath.Join(s.root, "main_sessions.json"), data)
}

func (s *SessionStore) persistMessagesLocked(sessionID string) error {
	return writeJSON(filepath.Join(s.root, "messages", sessionID+".json"), s.messagesByID[sessionID])
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
