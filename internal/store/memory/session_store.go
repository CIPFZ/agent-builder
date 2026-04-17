package memory

import (
	"sync"

	"myclaw/internal/model"
	"myclaw/internal/store"
)

var _ store.SessionStore = (*SessionStore)(nil)

type SessionStore struct {
	mu                    sync.RWMutex
	sessionsByID          map[string]model.Session
	sessionsByKey         map[string]model.Session
	mainByAgentID         map[string]string
	messagesBySessionID   map[string][]model.Message
	transcriptBySessionID map[string][]model.ClaudeTranscriptMessage
	entriesBySessionID    map[string][]model.ClaudeTranscriptEntry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessionsByID:          make(map[string]model.Session),
		sessionsByKey:         make(map[string]model.Session),
		mainByAgentID:         make(map[string]string),
		messagesBySessionID:   make(map[string][]model.Message),
		transcriptBySessionID: make(map[string][]model.ClaudeTranscriptMessage),
		entriesBySessionID:    make(map[string][]model.ClaudeTranscriptEntry),
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

	var parentUUID *string
	if transcript := s.transcriptBySessionID[msg.SessionID]; len(transcript) > 0 {
		for i := len(transcript) - 1; i >= 0; i-- {
			if model.IsClaudeTranscriptChainParticipant(transcript[i]) && !transcript[i].IsSidechain {
				parent := transcript[i].UUID
				parentUUID = &parent
				break
			}
		}
	}
	s.messagesBySessionID[msg.SessionID] = append(s.messagesBySessionID[msg.SessionID], msg)
	entry := model.NewClaudeTranscriptMessage(msg, model.ClaudeTranscriptOptions{ParentUUID: parentUUID})
	s.transcriptBySessionID[msg.SessionID] = append(s.transcriptBySessionID[msg.SessionID], entry)
	s.entriesBySessionID[msg.SessionID] = append(s.entriesBySessionID[msg.SessionID], model.NewClaudeTranscriptEntry(entry))
	return nil
}

func (s *SessionStore) Messages(sessionID string) ([]model.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(s.transcriptBySessionID[sessionID], sessionID)
	if ok {
		return messages, true
	}
	messages, ok = s.messagesBySessionID[sessionID]
	if !ok {
		return nil, false
	}

	cloned := append([]model.Message(nil), messages...)
	return cloned, true
}

func (s *SessionStore) TranscriptMessages(sessionID string) ([]model.ClaudeTranscriptMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	transcript, ok := s.transcriptBySessionID[sessionID]
	if !ok {
		return nil, false
	}
	return append([]model.ClaudeTranscriptMessage(nil), transcript...), true
}

func (s *SessionStore) TranscriptEntries(sessionID string) ([]model.ClaudeTranscriptEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.entriesBySessionID[sessionID]
	if !ok {
		return nil, false
	}
	return append([]model.ClaudeTranscriptEntry(nil), entries...), true
}

func (s *SessionStore) AppendTranscriptMessage(sessionID string, entry model.ClaudeTranscriptMessage) error {
	return s.AppendTranscriptEntry(sessionID, model.NewClaudeTranscriptEntry(entry))
}

func (s *SessionStore) AppendTranscriptEntry(sessionID string, entry model.ClaudeTranscriptEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.Message == nil {
		s.entriesBySessionID[sessionID] = append(s.entriesBySessionID[sessionID], entry)
		return nil
	}
	message, err := model.MessageFromClaudeTranscript(*entry.Message, sessionID)
	if err != nil {
		return err
	}
	s.transcriptBySessionID[sessionID] = append(s.transcriptBySessionID[sessionID], *entry.Message)
	s.entriesBySessionID[sessionID] = append(s.entriesBySessionID[sessionID], entry)
	s.messagesBySessionID[sessionID] = append(s.messagesBySessionID[sessionID], message)
	return nil
}

func (s *SessionStore) ReplaceMessages(sessionID string, messages []model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := append([]model.Message(nil), messages...)
	s.messagesBySessionID[sessionID] = cloned
	transcript := model.NewClaudeTranscriptMessages(cloned, nil)
	s.transcriptBySessionID[sessionID] = transcript
	s.entriesBySessionID[sessionID] = nil
	for _, entry := range transcript {
		s.entriesBySessionID[sessionID] = append(s.entriesBySessionID[sessionID], model.NewClaudeTranscriptEntry(entry))
	}
	return nil
}

func (s *SessionStore) ReplaceTranscriptMessages(sessionID string, entries []model.ClaudeTranscriptMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := append([]model.ClaudeTranscriptMessage(nil), entries...)
	s.transcriptBySessionID[sessionID] = cloned
	s.entriesBySessionID[sessionID] = nil
	for _, entry := range cloned {
		s.entriesBySessionID[sessionID] = append(s.entriesBySessionID[sessionID], model.NewClaudeTranscriptEntry(entry))
	}
	if messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(cloned, sessionID); ok {
		s.messagesBySessionID[sessionID] = messages
	} else {
		delete(s.messagesBySessionID, sessionID)
	}
	return nil
}
