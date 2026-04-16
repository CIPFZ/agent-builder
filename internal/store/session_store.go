package store

import "myclaw/internal/model"

type SessionStore interface {
	GetSessionByID(id string) (model.Session, bool)
	GetSessionByKey(key string) (model.Session, bool)
	SaveSession(sess model.Session)
	ListSessions() []model.Session
	GetMainSessionKey(agentID string) (string, bool)
	SaveMainSessionKey(agentID, sessionKey string)

	AppendMessage(model.Message) error
	Messages(sessionID string) ([]model.Message, bool)
	TranscriptMessages(sessionID string) ([]model.ClaudeTranscriptMessage, bool)
	TranscriptEntries(sessionID string) ([]model.ClaudeTranscriptEntry, bool)
	ReplaceMessages(sessionID string, messages []model.Message) error
}

type TranscriptSessionStore interface {
	AppendTranscriptMessage(sessionID string, entry model.ClaudeTranscriptMessage) error
	AppendTranscriptEntry(sessionID string, entry model.ClaudeTranscriptEntry) error
	TranscriptMessages(sessionID string) ([]model.ClaudeTranscriptMessage, bool)
	TranscriptEntries(sessionID string) ([]model.ClaudeTranscriptEntry, bool)
	ReplaceTranscriptMessages(sessionID string, entries []model.ClaudeTranscriptMessage) error
}
