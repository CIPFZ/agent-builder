package ws

type Message struct {
	Type    string         `json:"type"`
	ID      string         `json:"id,omitempty"`
	Method  string         `json:"method,omitempty"`
	Event   string         `json:"event,omitempty"`
	OK      bool           `json:"ok,omitempty"`
	Error   *ErrorPayload  `json:"error,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type ConnectPayload struct {
	Role                      string `json:"role"`
	ClientIdentity            string `json:"client_identity"`
	AgentID                   string `json:"agent_id,omitempty"`
	SessionKey                string `json:"session_key,omitempty"`
	SupportsPermissionControl bool   `json:"supports_permission_control,omitempty"`
}

type SendMessagePayload struct {
	Content string `json:"content"`
}

type SpawnSubagentPayload struct {
	Label        string   `json:"label"`
	Prompt       string   `json:"prompt"`
	AgentType    string   `json:"agent_type,omitempty"`
	Model        string   `json:"model,omitempty"`
	Effort       string   `json:"effort,omitempty"`
	Isolation    string   `json:"isolation,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	UseFork      bool     `json:"use_fork,omitempty"`
}

type SessionStatusPayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
}

type SessionNewPayload struct {
	AgentID string `json:"agent_id,omitempty"`
}

type SessionMessagesPayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
}

type SessionDeletePayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
}

type SessionSetPermissionPayload struct {
	SessionID        string   `json:"session_id,omitempty"`
	SessionKey       string   `json:"session_key,omitempty"`
	Mode             string   `json:"mode"`
	SubagentMode     string   `json:"subagent_mode,omitempty"`
	PlanMode         *bool    `json:"plan_mode,omitempty"`
	AutoMode         *bool    `json:"auto_mode,omitempty"`
	WorkspaceRoots   []string `json:"workspace_roots,omitempty"`
	CascadeSubagents bool     `json:"cascade_subagents,omitempty"`
}

type SessionSetModelPayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	Model      string `json:"model"`
}

type MCPStatusPayload struct {
	Server string `json:"server,omitempty"`
}

type MCPActionPayload struct {
	Server string `json:"server"`
}

type ApprovalListPayload struct {
	Status string `json:"status,omitempty"`
}

type ApprovalClearPayload struct {
	Status string `json:"status,omitempty"`
}

type OrchestrationHistoryPayload struct {
	SessionID        string `json:"session_id,omitempty"`
	SessionKey       string `json:"session_key,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Status           string `json:"status,omitempty"`
	DecisionPriority string `json:"decision_priority,omitempty"`
}

type OrchestrationEvaluatePayload struct {
	SessionID    string `json:"session_id,omitempty"`
	SessionKey   string `json:"session_key,omitempty"`
	Category     string `json:"category,omitempty"`
	Priority     string `json:"priority,omitempty"`
	BlockingOnly bool   `json:"blocking_only,omitempty"`
}

type OrchestrationPlanStepUpdatePayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	ActionID   string `json:"action_id"`
	State      string `json:"state"`
	Result     string `json:"result,omitempty"`
}

type OrchestrationPlanStepHistoryPayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	ActionID   string `json:"action_id"`
	State      string `json:"state,omitempty"`
}

type OrchestrationPlanExecutionHistoryPayload struct {
	SessionID  string `json:"session_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	State      string `json:"state,omitempty"`
	ActionID   string `json:"action_id,omitempty"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
}

const (
	TypeRequest         = "req"
	TypeResponse        = "res"
	TypeEvent           = "event"
	TypeControlRequest  = "control_request"
	TypeControlResponse = "control_response"

	MethodConnect                           = "connect"
	MethodSendMessage                       = "send_message"
	MethodSpawnSubagent                     = "spawn_subagent"
	MethodSessionStatus                     = "session_status"
	MethodSessionList                       = "session_list"
	MethodSessionNew                        = "session_new"
	MethodSessionMessages                   = "session_messages"
	MethodSessionDelete                     = "session_delete"
	MethodSessionSetPermission              = "session_set_permission"
	MethodSessionSetModel                   = "session_set_model"
	MethodMCPStatus                         = "mcp_status"
	MethodMCPReconnect                      = "mcp_reconnect"
	MethodMCPAuthenticate                   = "mcp_authenticate"
	MethodOrchestrationStatus               = "orchestration_status"
	MethodOrchestrationHistory              = "orchestration_history"
	MethodOrchestrationSummary              = "orchestration_summary"
	MethodOrchestrationEvaluate             = "orchestration_evaluate"
	MethodOrchestrationPlan                 = "orchestration_plan"
	MethodOrchestrationPlanOverview         = "orchestration_plan_overview"
	MethodOrchestrationPlanExecutionHistory = "orchestration_plan_execution_history"
	MethodOrchestrationPlanStepUpdate       = "orchestration_plan_step_update"
	MethodOrchestrationPlanStepHistory      = "orchestration_plan_step_history"
	MethodTasksList                         = "tasks_list"
	MethodSubagentList                      = "subagent_list"
	MethodSubagentStatus                    = "subagent_status"
	MethodSubagentStop                      = "subagent_stop"
	MethodMemoryList                        = "memory_list"
	MethodApprovalList                      = "approval_list"
	MethodApprovalApprove                   = "approval_approve"
	MethodApprovalReject                    = "approval_reject"
	MethodApprovalClear                     = "approval_clear"
	MethodSubagentSteer                     = "subagent_steer"
	MethodSubagentResume                    = "subagent_resume"

	EventHello                        = "hello"
	EventMessageCreated               = "message.created"
	EventOrchestrationUpdated         = "orchestration.updated"
	EventOrchestrationPlanStepUpdated = "orchestration.plan_step.updated"
	EventSubagentUpdated              = "subagent.updated"
	EventSubagentCompleted            = "subagent.completed"
)

type ErrorPayload struct {
	Message string `json:"message"`
}

func ConnectResponse(id, sessionID, sessionKey string) Message {
	return Message{
		Type: TypeResponse,
		ID:   id,
		OK:   true,
		Payload: map[string]any{
			"session_id":  sessionID,
			"session_key": sessionKey,
			"status":      "connected",
		},
	}
}

func InvalidConnectResponse(id, message string) Message {
	return ErrorResponse(id, message)
}

func EventMessage(event string, payload map[string]any) Message {
	return Message{
		Type:    TypeEvent,
		Event:   event,
		Payload: payload,
	}
}

func ErrorResponse(id, message string) Message {
	return Message{
		Type: TypeResponse,
		ID:   id,
		OK:   false,
		Error: &ErrorPayload{
			Message: message,
		},
	}
}
