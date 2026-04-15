package file

import (
	"bufio"
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

func (s *SessionStore) AppendMessage(msg model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var parentUUID *string
	if messages := s.messagesByID[msg.SessionID]; len(messages) > 0 {
		parent := messages[len(messages)-1].ID
		parentUUID = &parent
	}
	if err := s.appendTranscriptMessagesLocked(msg.SessionID, []model.Message{msg}, parentUUID); err != nil {
		return err
	}
	s.messagesByID[msg.SessionID] = append(s.messagesByID[msg.SessionID], msg)
	return nil
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

func (s *SessionStore) ReplaceMessages(sessionID string, messages []model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]model.Message(nil), messages...)
	if err := s.appendTranscriptMessagesLocked(sessionID, cloned, nil); err != nil {
		return err
	}
	s.messagesByID[sessionID] = cloned
	return nil
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
		if entry.IsDir() {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".jsonl":
			sessionID := entry.Name()[:len(entry.Name())-len(".jsonl")]
			messages, err := s.loadTranscriptMessages(filepath.Join(s.root, "messages", entry.Name()), sessionID)
			if err != nil {
				return err
			}
			s.messagesByID[sessionID] = messages
		case ".json":
			sessionID := entry.Name()[:len(entry.Name())-len(".json")]
			if _, hasJSONL := s.messagesByID[sessionID]; hasJSONL {
				continue
			}
			var messages []model.Message
			if err := readJSON(filepath.Join(s.root, "messages", entry.Name()), &messages); err != nil {
				return err
			}
			s.messagesByID[sessionID] = messages
		}
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

func (s *SessionStore) appendTranscriptMessagesLocked(sessionID string, messages []model.Message, parentUUID *string) error {
	if len(messages) == 0 {
		return nil
	}
	path := filepath.Join(s.root, "messages", sessionID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	currentParent := parentUUID
	encoder := json.NewEncoder(file)
	for _, message := range messages {
		entry := model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{ParentUUID: currentParent})
		if err := encoder.Encode(entry); err != nil {
			return err
		}
		nextParent := message.ID
		currentParent = &nextParent
	}
	return nil
}

func (s *SessionStore) loadTranscriptMessages(path, sessionID string) ([]model.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entriesByID := make(map[string]model.ClaudeTranscriptMessage)
	var ordered []model.ClaudeTranscriptMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var entry model.ClaudeTranscriptMessage
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		if entry.UUID == "" {
			continue
		}
		entriesByID[entry.UUID] = entry
		ordered = append(ordered, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(ordered) == 0 {
		return nil, nil
	}

	var chain []model.ClaudeTranscriptMessage
	seen := make(map[string]bool)
	for entry := ordered[len(ordered)-1]; ; {
		if seen[entry.UUID] {
			return nil, fmt.Errorf("cycle in transcript parent chain for %s", sessionID)
		}
		seen[entry.UUID] = true
		chain = append(chain, entry)
		if entry.ParentUUID == nil || *entry.ParentUUID == "" {
			break
		}
		parent, ok := entriesByID[*entry.ParentUUID]
		if !ok {
			break
		}
		entry = parent
	}

	messages := make([]model.Message, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		message, err := model.MessageFromClaudeTranscript(chain[i], sessionID)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
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
