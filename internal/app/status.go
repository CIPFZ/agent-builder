package app

import (
	"encoding/json"
	"net/http"

	"myclaw/internal/session"
)

type StatusResponse struct {
	Sessions []SessionStatus `json:"sessions"`
}

type SessionStatus struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	AgentID      string `json:"agent_id"`
	IsMain       bool   `json:"is_main"`
	MessageCount int    `json:"message_count"`
}

func StatusHandler(sessionManager *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		sessions := sessionManager.ListSessions()
		response := StatusResponse{
			Sessions: make([]SessionStatus, 0, len(sessions)),
		}
		for _, sess := range sessions {
			messages, _ := sessionManager.Messages(sess.ID)
			response.Sessions = append(response.Sessions, SessionStatus{
				ID:           sess.ID,
				Key:          sess.Key,
				AgentID:      sess.AgentID,
				IsMain:       sess.IsMain,
				MessageCount: len(messages),
			})
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}
