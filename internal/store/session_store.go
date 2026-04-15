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
	ReplaceMessages(sessionID string, messages []model.Message) error
}
