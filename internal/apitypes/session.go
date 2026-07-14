package apitypes

// Session represents a session in the apitypes layer.
type Session struct {
	ID               string  `json:"id"`
	ParentSessionID  string  `json:"parent_session_id"`
	Title            string  `json:"title"`
	TitleSource      string  `json:"title_source,omitempty"`
	MessageCount     int64   `json:"message_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	SummaryMessageID string  `json:"summary_message_id"`
	Cost             float64 `json:"cost"`
	ProjectID        string  `json:"project_id,omitempty"`
	Scope            string  `json:"scope,omitempty"`
	Todos            []Todo  `json:"todos,omitempty"`
	CreatedAt        int64   `json:"created_at"`
	UpdatedAt        int64   `json:"updated_at"`
}

// Todo represents a single todo entry on a session in the apitypes layer.
type Todo struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"active_form"`
}
