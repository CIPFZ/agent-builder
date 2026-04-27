package file

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"myclaw/internal/model"
	"myclaw/internal/store"
)

var _ store.SessionStore = (*SessionStore)(nil)

type SessionStore struct {
	mu             sync.RWMutex
	root           string
	sessionsByID   map[string]model.Session
	sessionsByKey  map[string]model.Session
	mainByAgentID  map[string]string
	messagesByID   map[string][]model.Message
	transcriptByID map[string][]model.ClaudeTranscriptMessage
	entriesByID    map[string][]model.ClaudeTranscriptEntry
}

func NewSessionStore(root string) (*SessionStore, error) {
	if root == "" {
		return nil, fmt.Errorf("root is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "messages"), 0o755); err != nil {
		return nil, err
	}
	s := &SessionStore{
		root:           root,
		sessionsByID:   make(map[string]model.Session),
		sessionsByKey:  make(map[string]model.Session),
		mainByAgentID:  make(map[string]string),
		messagesByID:   make(map[string][]model.Message),
		transcriptByID: make(map[string][]model.ClaudeTranscriptMessage),
		entriesByID:    make(map[string][]model.ClaudeTranscriptEntry),
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

func (s *SessionStore) DeleteSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessionsByID[sessionID]
	if !ok {
		return false
	}
	delete(s.sessionsByID, sessionID)
	delete(s.sessionsByKey, sess.Key)
	delete(s.messagesByID, sessionID)
	delete(s.transcriptByID, sessionID)
	delete(s.entriesByID, sessionID)
	for agentID, key := range s.mainByAgentID {
		if key == sess.Key {
			delete(s.mainByAgentID, agentID)
		}
	}
	_ = s.persistSessionsLocked()
	_ = s.persistMainSessionsLocked()
	_ = os.Remove(filepath.Join(s.root, "messages", sessionID+".jsonl"))
	_ = os.Remove(filepath.Join(s.root, "messages", sessionID+".json"))
	return true
}

func (s *SessionStore) ListSessions() []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Session, 0, len(s.sessionsByID))
	for _, sess := range s.sessionsByID {
		out = append(out, sess)
	}
	sortSessions(out)
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
	if transcript := s.transcriptByID[msg.SessionID]; len(transcript) > 0 {
		for i := len(transcript) - 1; i >= 0; i-- {
			if model.IsClaudeTranscriptChainParticipant(transcript[i]) && !transcript[i].IsSidechain {
				parent := transcript[i].UUID
				parentUUID = &parent
				break
			}
		}
	}
	entries := model.NewClaudeTranscriptMessages([]model.Message{msg}, parentUUID)
	entryEnvelopes := transcriptEntries(entries)
	if err := s.appendTranscriptEntriesLocked(msg.SessionID, entryEnvelopes); err != nil {
		return err
	}
	s.messagesByID[msg.SessionID] = append(s.messagesByID[msg.SessionID], msg)
	s.transcriptByID[msg.SessionID] = append(s.transcriptByID[msg.SessionID], entries...)
	s.entriesByID[msg.SessionID] = append(s.entriesByID[msg.SessionID], entryEnvelopes...)
	return nil
}

func (s *SessionStore) Messages(sessionID string) ([]model.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(s.transcriptByID[sessionID], sessionID); ok {
		return messages, true
	}
	messages, ok := s.messagesByID[sessionID]
	if !ok {
		return nil, false
	}
	return append([]model.Message(nil), messages...), true
}

func (s *SessionStore) TranscriptMessages(sessionID string) ([]model.ClaudeTranscriptMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	transcript, ok := s.transcriptByID[sessionID]
	if !ok {
		return nil, false
	}
	return append([]model.ClaudeTranscriptMessage(nil), transcript...), true
}

func (s *SessionStore) TranscriptEntries(sessionID string) ([]model.ClaudeTranscriptEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, ok := s.entriesByID[sessionID]
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
		if err := s.appendTranscriptEntriesLocked(sessionID, []model.ClaudeTranscriptEntry{entry}); err != nil {
			return err
		}
		s.entriesByID[sessionID] = append(s.entriesByID[sessionID], entry)
		return nil
	}
	messageEntry := *entry.Message
	if !model.IsClaudeTranscriptMessage(messageEntry) {
		return nil
	}
	if _, err := model.MessageFromClaudeTranscript(messageEntry, sessionID); err != nil {
		return err
	}
	transcript := append(append([]model.ClaudeTranscriptMessage(nil), s.transcriptByID[sessionID]...), messageEntry)
	messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(transcript, sessionID)
	if !ok {
		return nil
	}
	if err := s.appendTranscriptEntriesLocked(sessionID, []model.ClaudeTranscriptEntry{entry}); err != nil {
		return err
	}
	s.transcriptByID[sessionID] = transcript
	s.entriesByID[sessionID] = append(s.entriesByID[sessionID], entry)
	s.messagesByID[sessionID] = messages
	return nil
}

func (s *SessionStore) ReplaceMessages(sessionID string, messages []model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]model.Message(nil), messages...)
	entries := model.NewClaudeTranscriptMessages(cloned, nil)
	entryEnvelopes := transcriptEntries(entries)
	if err := s.appendTranscriptEntriesLocked(sessionID, entryEnvelopes); err != nil {
		return err
	}
	s.messagesByID[sessionID] = cloned
	s.transcriptByID[sessionID] = append(s.transcriptByID[sessionID], entries...)
	s.entriesByID[sessionID] = append(s.entriesByID[sessionID], entryEnvelopes...)
	return nil
}

func (s *SessionStore) ReplaceTranscriptMessages(sessionID string, entries []model.ClaudeTranscriptMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]model.ClaudeTranscriptMessage(nil), entries...)
	entryEnvelopes := make([]model.ClaudeTranscriptEntry, 0, len(cloned))
	for _, entry := range cloned {
		entryEnvelopes = append(entryEnvelopes, model.NewClaudeTranscriptEntry(entry))
	}
	if err := s.appendTranscriptEntriesLocked(sessionID, entryEnvelopes); err != nil {
		return err
	}
	s.transcriptByID[sessionID] = append(s.transcriptByID[sessionID], cloned...)
	s.entriesByID[sessionID] = append(s.entriesByID[sessionID], entryEnvelopes...)
	if messages, ok := model.RuntimeMessagesFromClaudeTranscriptEntries(s.transcriptByID[sessionID], sessionID); ok {
		s.messagesByID[sessionID] = messages
	} else {
		delete(s.messagesByID, sessionID)
	}
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
			entries, transcript, messages, err := s.loadTranscriptMessages(filepath.Join(s.root, "messages", entry.Name()), sessionID)
			if err != nil {
				return err
			}
			s.entriesByID[sessionID] = entries
			s.transcriptByID[sessionID] = transcript
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
	sortSessions(sessions)
	return writeJSON(filepath.Join(s.root, "sessions.json"), sessions)
}

func sortSessions(sessions []model.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		left := sessions[i]
		right := sessions[j]
		leftActivity := left.Metadata.LastActivityAt
		rightActivity := right.Metadata.LastActivityAt
		if !leftActivity.Equal(rightActivity) {
			if leftActivity.IsZero() {
				return false
			}
			if rightActivity.IsZero() {
				return true
			}
			return leftActivity.After(rightActivity)
		}
		if left.ID != right.ID {
			return left.ID > right.ID
		}
		return left.Key > right.Key
	})
}

func (s *SessionStore) persistMainSessionsLocked() error {
	data := make(map[string]string, len(s.mainByAgentID))
	for key, value := range s.mainByAgentID {
		data[key] = value
	}
	return writeJSON(filepath.Join(s.root, "main_sessions.json"), data)
}

func (s *SessionStore) appendTranscriptEntriesLocked(sessionID string, entries []model.ClaudeTranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}
	path := filepath.Join(s.root, "messages", sessionID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStore) loadTranscriptMessages(path, sessionID string) ([]model.ClaudeTranscriptEntry, []model.ClaudeTranscriptMessage, []model.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()

	var allEntries []model.ClaudeTranscriptEntry
	var ordered []model.ClaudeTranscriptMessage
	progressParents := make(map[string]*string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var entry model.ClaudeTranscriptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, nil, nil, err
		}
		allEntries = append(allEntries, entry)
		if entry.Type == "progress" && entry.Raw != nil {
			uuid, _ := entry.Raw["uuid"].(string)
			parent := rawStringPtr(entry.Raw["parentUuid"])
			if uuid != "" {
				progressParents[uuid] = parent
			}
			continue
		}
		if entry.Message == nil || !model.IsClaudeTranscriptMessage(*entry.Message) {
			continue
		}
		messageEntry := *entry.Message
		messageEntry.ParentUUID = bridgeProgressParent(messageEntry.ParentUUID, progressParents)
		ordered = append(ordered, messageEntry)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(ordered) == 0 {
		return allEntries, nil, nil, nil
	}

	chain, err := model.LatestClaudeTranscriptChain(ordered)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w for %s", err, sessionID)
	}
	messages := make([]model.Message, 0, len(chain))
	for _, entry := range chain {
		message, err := model.MessageFromClaudeTranscript(entry, sessionID)
		if err != nil {
			return nil, nil, nil, err
		}
		messages = append(messages, message)
	}
	return allEntries, ordered, messages, nil
}

func rawStringPtr(value any) *string {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		return &text
	}
	return nil
}

func transcriptEntries(messages []model.ClaudeTranscriptMessage) []model.ClaudeTranscriptEntry {
	entries := make([]model.ClaudeTranscriptEntry, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, model.NewClaudeTranscriptEntry(message))
	}
	return entries
}

func bridgeProgressParent(parent *string, progressParents map[string]*string) *string {
	if parent == nil || len(progressParents) == 0 {
		return parent
	}
	current := *parent
	seen := make(map[string]struct{}, len(progressParents))
	for current != "" {
		next, ok := progressParents[current]
		if !ok {
			break
		}
		if _, exists := seen[current]; exists {
			return parent
		}
		seen[current] = struct{}{}
		if next == nil {
			return nil
		}
		current = *next
	}
	if current == *parent {
		return parent
	}
	return &current
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
