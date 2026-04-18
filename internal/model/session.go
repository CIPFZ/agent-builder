package model

import "time"

type SessionMetadata struct {
	LastUserMessageID              string           `json:"last_user_message_id,omitempty"`
	LastAssistantMessageID         string           `json:"last_assistant_message_id,omitempty"`
	LastCompactBoundaryID          string           `json:"last_compact_boundary_id,omitempty"`
	LastCompactionSummaryID        string           `json:"last_compaction_summary_id,omitempty"`
	LastSummarizedMessageID        string           `json:"last_summarized_message_id,omitempty"`
	LastCompactionReason           string           `json:"last_compaction_reason,omitempty"`
	LastCompactedAt                time.Time        `json:"last_compacted_at,omitempty"`
	LastActivityAt                 time.Time        `json:"last_activity_at,omitempty"`
	InitialMainLoopModel           string           `json:"initial_main_loop_model,omitempty"`
	MainLoopModelOverride          string           `json:"main_loop_model_override,omitempty"`
	MainLoopEffortOverride         string           `json:"main_loop_effort_override,omitempty"`
	AgentType                      string           `json:"agent_type,omitempty"`
	AgentSystemPrompt              string           `json:"agent_system_prompt,omitempty"`
	AgentMemoryScope               string           `json:"agent_memory_scope,omitempty"`
	AgentMaxTurns                  int              `json:"agent_max_turns,omitempty"`
	PendingApprovalID              string           `json:"pending_approval_id,omitempty"`
	PendingApprovalStatus          string           `json:"pending_approval_status,omitempty"`
	PendingApprovalToolName        string           `json:"pending_approval_tool_name,omitempty"`
	PendingApprovalToolInput       string           `json:"pending_approval_tool_input,omitempty"`
	PendingApprovalToolInputObject map[string]any   `json:"pending_approval_tool_input_object,omitempty"`
	PendingApprovalToolUseID       string           `json:"pending_approval_tool_use_id,omitempty"`
	PendingApprovalProviderMsgID   string           `json:"pending_approval_provider_message_id,omitempty"`
	PendingApprovalReason          string           `json:"pending_approval_reason,omitempty"`
	PendingApprovalDecisionReason  string           `json:"pending_approval_decision_reason,omitempty"`
	PendingApprovalAcceptFeedback  string           `json:"pending_approval_accept_feedback,omitempty"`
	PendingApprovalContentBlocks   []map[string]any `json:"pending_approval_content_blocks,omitempty"`
	PendingApprovalRunID           string           `json:"pending_approval_run_id,omitempty"`
	PendingApprovalUserMessageID   string           `json:"pending_approval_user_message_id,omitempty"`
	PendingApprovalCategory        string           `json:"pending_approval_category,omitempty"`
	PendingApprovalRuleSource      string           `json:"pending_approval_rule_source,omitempty"`
}

type Session struct {
	ID       string          `json:"id"`
	Key      string          `json:"key"`
	AgentID  string          `json:"agent_id"`
	IsMain   bool            `json:"is_main"`
	Metadata SessionMetadata `json:"metadata,omitempty"`
}
