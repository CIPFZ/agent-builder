package model

type Session struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	AgentID string `json:"agent_id"`
	IsMain  bool   `json:"is_main"`
}
