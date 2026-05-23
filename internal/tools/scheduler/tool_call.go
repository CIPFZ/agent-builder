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
	CapabilityID  string         `json:"capability_id,omitempty"`
	JobID         string         `json:"job_id,omitempty"`
	Command       string         `json:"command,omitempty"`
	Risk          string         `json:"risk,omitempty"`
	PolicyReason  string         `json:"policy_reason,omitempty"`
	ExitCode      int            `json:"exit_code,omitempty"`
	JobStatus     string         `json:"job_status,omitempty"`
	JobStartedAt  time.Time      `json:"job_started_at,omitempty"`
	JobFinishedAt time.Time      `json:"job_finished_at,omitempty"`
	Status        ToolCallStatus `json:"status"`
	InputSummary  string         `json:"input_summary,omitempty"`
	OutputSummary string         `json:"output_summary,omitempty"`
	ModelContent  string         `json:"model_content,omitempty"`
	Structured    string         `json:"structured,omitempty"`
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
	CapabilityID string
	JobID        string
	Command      string
	Risk         string
	PolicyReason string
	JobStatus    string
	JobStartedAt time.Time
	InputSummary string
}

type ToolCallResult struct {
	ToolCallID    string
	Status        ToolCallStatus
	JobID         string
	Command       string
	Risk          string
	PolicyReason  string
	ExitCode      int
	JobStatus     string
	JobStartedAt  time.Time
	JobFinishedAt time.Time
	OutputSummary string
	ModelContent  string
	Structured    string
	Stdout        string
	Stderr        string
	IsError       bool
	Error         string
}
