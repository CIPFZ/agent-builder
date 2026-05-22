package scheduler

import "time"

type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceMCP     ToolSource = "mcp"
	ToolSourceShell   ToolSource = "shell"
	ToolSourceUnknown ToolSource = "unknown"
)

type ToolCallStatus string

const (
	ToolCallPending           ToolCallStatus = "pending"
	ToolCallWaitingPermission ToolCallStatus = "waiting_permission"
	ToolCallRunning           ToolCallStatus = "running"
	ToolCallCompleted         ToolCallStatus = "completed"
	ToolCallFailed            ToolCallStatus = "failed"
	ToolCallCancelled         ToolCallStatus = "cancelled"
	ToolCallDenied            ToolCallStatus = "denied"
)

type ToolCall struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	TurnID        string         `json:"turn_id"`
	MessageID     string         `json:"message_id,omitempty"`
	Name          string         `json:"name"`
	Source        ToolSource     `json:"source"`
	Status        ToolCallStatus `json:"status"`
	InputSummary  string         `json:"input_summary,omitempty"`
	OutputSummary string         `json:"output_summary,omitempty"`
	Stdout        string         `json:"stdout,omitempty"`
	Stderr        string         `json:"stderr,omitempty"`
	IsError       bool           `json:"is_error,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    time.Time      `json:"finished_at,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type ToolCallRequest struct {
	ID           string
	SessionID    string
	TurnID       string
	MessageID    string
	Name         string
	Source       ToolSource
	InputSummary string
}

type ToolCallResult struct {
	ToolCallID    string
	Status        ToolCallStatus
	OutputSummary string
	Stdout        string
	Stderr        string
	IsError       bool
	Error         string
}
