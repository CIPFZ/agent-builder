package runtime

const (
	EventSessionStarted        = "session.started"
	EventSessionRecovered      = "session.recovered"
	EventMessageCreated        = "message.created"
	EventAssistantDelta        = "assistant.delta"
	EventToolCalled            = "tool.called"
	EventToolResult            = "tool.result"
	EventPermissionRequired    = "permission.required"
	EventApprovalResolved      = "approval.resolved"
	EventCompactBoundary       = "compact.boundary"
	EventCompactSummary        = "compact.summary"
	EventCommandStarted        = "command.started"
	EventCommandCompleted      = "command.completed"
	EventAgentLifecycleStart   = "agent.lifecycle.start"
	EventAgentLifecycleEnd     = "agent.lifecycle.end"
	EventMCPInventoryChanged   = "mcp.inventory.changed"
	EventSkillInventoryChanged = "skill.inventory.changed"
)

func (e RuntimeEvent) Payload() map[string]any {
	payload := map[string]any{"type": e.Type}
	if e.Session.ID != "" {
		payload["session_id"] = e.Session.ID
	}
	if e.Session.Key != "" {
		payload["session_key"] = e.Session.Key
	}
	if e.Session.AgentID != "" {
		payload["agent_id"] = e.Session.AgentID
	}
	if e.RunID != "" {
		payload["run_id"] = e.RunID
	}
	if e.Message != nil {
		payload["message_id"] = e.Message.ID
		payload["message_role"] = e.Message.Role
		payload["message_content"] = e.Message.Content
	}
	if e.Delta != "" {
		payload["delta"] = e.Delta
	}
	if e.ToolUseID != "" {
		payload["tool_use_id"] = e.ToolUseID
	}
	if e.ProviderMessageID != "" {
		payload["provider_message_id"] = e.ProviderMessageID
	}
	if e.ToolName != "" {
		payload["tool_name"] = e.ToolName
	}
	if e.ToolInput != "" {
		payload["tool_input"] = e.ToolInput
	}
	if e.ToolInputObject != nil {
		payload["tool_input_object"] = cloneAnyMap(e.ToolInputObject)
	}
	if e.ToolError {
		payload["tool_error"] = true
	}
	if e.Progress != nil {
		payload["progress"] = cloneToolProgress(e.Progress)
	}
	if e.StructuredContent != nil {
		payload["structured_content"] = e.StructuredContent
	}
	if e.Meta != nil {
		payload["meta"] = cloneAnyMap(e.Meta)
	}
	if e.DecisionReason != "" {
		payload["decision_reason"] = e.DecisionReason
	}
	if e.DecisionReasonDetails != nil {
		payload["decision_reason_details"] = cloneAnyMap(e.DecisionReasonDetails)
	}
	if e.AcceptFeedback != "" {
		payload["accept_feedback"] = e.AcceptFeedback
	}
	if e.ContentBlocks != nil {
		payload["content_blocks"] = cloneAnyMaps(e.ContentBlocks)
	}
	if e.Error != "" {
		payload["error"] = e.Error
	}
	if e.Approval != nil {
		payload["approval_id"] = e.Approval.ID
		payload["approval_status"] = string(e.Approval.Status)
	}
	return payload
}
