package runtimeapi

import (
	"errors"
	"fmt"
	"time"
)

const Version = "v1"

const (
	MethodGet  = "GET"
	MethodPost = "POST"
	MethodPut  = "PUT"
)

type Endpoint struct {
	Method string
	Path   string
}

var Endpoints = []Endpoint{
	{Method: MethodGet, Path: "/v1/runtime/status"},
	{Method: MethodGet, Path: "/v1/recovery/status"},
	{Method: MethodGet, Path: "/v1/config/model"},
	{Method: MethodPut, Path: "/v1/config/model"},
	{Method: MethodPost, Path: "/v1/config/model/verify"},
	{Method: MethodGet, Path: "/v1/config/models"},
	{Method: MethodGet, Path: "/v1/sessions"},
	{Method: MethodPost, Path: "/v1/sessions"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}/messages"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}/todos"},
	{Method: MethodPost, Path: "/v1/sessions/{session_id}/turns"},
	{Method: MethodGet, Path: "/v1/turns"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}/todos"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}/tool-calls"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}/compact"},
	{Method: MethodGet, Path: "/v1/turns/{turn_id}/tasks"},
	{Method: MethodGet, Path: "/v1/tool-calls/{tool_call_id}"},
	{Method: MethodGet, Path: "/v1/refs"},
	{Method: MethodGet, Path: "/v1/refs/{ref_id}"},
	{Method: MethodGet, Path: "/v1/refs/{ref_id}/content"},
	{Method: MethodGet, Path: "/v1/tasks/{task_id}"},
	{Method: MethodPost, Path: "/v1/tasks/{task_id}/cancel"},
	{Method: MethodGet, Path: "/v1/tasks/{task_id}/effective-scope"},
	{Method: MethodPost, Path: "/v1/turns/{turn_id}/cancel"},
	{Method: MethodGet, Path: "/v1/worktrees"},
	{Method: MethodPost, Path: "/v1/worktrees"},
	{Method: MethodGet, Path: "/v1/worktrees/{worktree_id}"},
	{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/enter"},
	{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/exit"},
	{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/cleanup"},
	{Method: MethodGet, Path: "/v1/permissions"},
	{Method: MethodPost, Path: "/v1/permissions/{permission_id}/decision"},
	{Method: MethodGet, Path: "/v1/policy"},
	{Method: MethodPut, Path: "/v1/policy"},
	{Method: MethodGet, Path: "/v1/capabilities"},
	{Method: MethodPost, Path: "/v1/capabilities/{capability_id}/refresh"},
	{Method: MethodPost, Path: "/v1/tools/search"},
	{Method: MethodGet, Path: "/v1/context/sources"},
	{Method: MethodGet, Path: "/v1/skills"},
	{Method: MethodPost, Path: "/v1/skills"},
	{Method: MethodPost, Path: "/v1/skills/refresh"},
	{Method: MethodPost, Path: "/v1/skills/paths"},
	{Method: MethodGet, Path: "/v1/mcp/servers"},
	{Method: MethodPut, Path: "/v1/mcp/servers/{server_name}"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/enabled"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/refresh"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/tools"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/tools/{tool_name}/enabled"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/resources"},
	{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/prompts"},
	{Method: MethodGet, Path: "/v1/mcp/requests"},
	{Method: MethodGet, Path: "/v1/mcp/requests/{request_id}"},
	{Method: MethodPost, Path: "/v1/mcp/requests/{request_id}/decision"},
	{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/retry"},
	{Method: MethodGet, Path: "/v1/audit/turns/{turn_id}"},
	{Method: MethodGet, Path: "/v1/audit/sessions/{session_id}"},
	{Method: MethodGet, Path: "/v1/sessions/{session_id}/compact"},
	{Method: MethodGet, Path: "/v1/events"},
}

const (
	EventRuntimeStarted             = "runtime.started"
	EventRuntimeFailed              = "runtime.failed"
	EventSessionCreated             = "session.created"
	EventSessionUpdated             = "session.updated"
	EventSessionDeleted             = "session.deleted"
	EventTurnStarted                = "turn.started"
	EventTurnProgress               = "turn.progress"
	EventTurnCompleted              = "turn.completed"
	EventTurnFailed                 = "turn.failed"
	EventTurnCancelled              = "turn.cancelled"
	EventTurnInterrupted            = "turn.interrupted"
	EventMessageCreated             = "message.created"
	EventMessageUpdated             = "message.updated"
	EventMessageCompleted           = "message.completed"
	EventToolCallStarted            = "tool.call.started"
	EventToolCallOutput             = "tool.call.output"
	EventOutputRefCreated           = "output.ref.created"
	EventArtifactRefCreated         = "artifact.ref.created"
	EventToolOutputRefCreated       = "tool.output.ref.created"
	EventToolCallCompleted          = "tool.call.completed"
	EventToolCallFailed             = "tool.call.failed"
	EventToolCallCancelled          = "tool.call.cancelled"
	EventToolSearchPerformed        = "tool.search.performed"
	EventToolDiscoverySelected      = "tool.discovery.selected"
	EventToolDiscoveryOmitted       = "tool.discovery.omitted"
	EventSchedulerDeadlockPrevented = "scheduler.deadlock.prevented"
	EventTaskStarted                = "task.started"
	EventTaskProgress               = "task.progress"
	EventTaskCompleted              = "task.completed"
	EventTaskFailed                 = "task.failed"
	EventTaskCancelled              = "task.cancelled"
	EventTaskInterrupted            = "task.interrupted"
	EventTaskRoleLoaded             = "task.role.loaded"
	EventTaskScopeApplied           = "task.scope.applied"
	EventTaskScopeDenied            = "task.scope.denied"
	EventTaskMessageCreated         = "task.message.created"
	EventTaskResultUpdated          = "task.result.updated"
	EventTaskArtifactCreated        = "task.artifact.created"
	EventWorktreeCreated            = "worktree.created"
	EventWorktreeEntered            = "worktree.entered"
	EventWorktreeExited             = "worktree.exited"
	EventWorktreeCleaned            = "worktree.cleaned"
	EventWorktreeCleanupFailed      = "worktree.cleanup_failed"
	EventWorktreePolicyDenied       = "worktree.policy_denied"
	EventWorktreeRecovered          = "worktree.recovered"
	EventWorktreeMissingPath        = "worktree.missing_path"
	EventWorktreePreserved          = "worktree.preserved"
	EventPermissionRequested        = "permission.requested"
	EventPermissionDecided          = "permission.decided"
	EventPermissionPolicyApplied    = "permission.policy.applied"
	EventPolicyRuleMatched          = "policy.rule.matched"
	EventPolicyRuleDenied           = "policy.rule.denied"
	EventPolicyRuleAsk              = "policy.rule.ask"
	EventShellPolicyClassified      = "shell.policy.classified"
	EventTodoUpdated                = "todo.updated"
	EventCapabilityLoading          = "capability.loading"
	EventCapabilityLoaded           = "capability.loaded"
	EventCapabilityFailed           = "capability.failed"
	EventContextLoading             = "context.loading"
	EventContextLoaded              = "context.loaded"
	EventContextFailed              = "context.failed"
	EventContextReinjected          = "context.reinjected"
	EventContextSourceSkipped       = "context.source.skipped"
	EventContextSourceFailed        = "context.source.failed"
	EventBudgetUpdated              = "budget.updated"
	EventCompactBoundaryRecorded    = "compact.boundary.recorded"
	EventCompactMicroCompleted      = "compact.micro.completed"
	EventCompactFullCompleted       = "compact.full.completed"
	EventCompactFailed              = "compact.failed"
	EventCompactOutputPreserved     = "compact.output.preserved"
	EventSkillDiscoveryStarted      = "skill.discovery.started"
	EventSkillDiscoveryCompleted    = "skill.discovery.completed"
	EventSkillDiscoveryFailed       = "skill.discovery.failed"
	EventSkillEnabled               = "skill.enabled"
	EventSkillDisabled              = "skill.disabled"
	EventSkillActivated             = "skill.activated"
	EventSkillActivationFailed      = "skill.activation.failed"
	EventSkillActivationAllowed     = "skill.activation.allowed"
	EventSkillActivationDenied      = "skill.activation.denied"
	EventSkillContextInjected       = "skill.context.injected"
	EventSkillContextOmitted        = "skill.context.omitted"
	EventMCPCapabilityAllowed       = "mcp.capability.allowed"
	EventMCPCapabilityDenied        = "mcp.capability.denied"
	EventMCPServerLazyStarted       = "mcp.server.lazy_started"
	EventMCPServerLazyFailed        = "mcp.server.lazy_failed"
	EventMCPServerStarting          = "mcp.server.starting"
	EventMCPServerConnected         = "mcp.server.connected"
	EventMCPServerFailed            = "mcp.server.failed"
	EventMCPServerBlocked           = "mcp.server.blocked"
	EventMCPServerDisabled          = "mcp.server.disabled"
	EventMCPToolsUpdated            = "mcp.tools.updated"
	EventMCPResourcesUpdated        = "mcp.resources.updated"
	EventMCPPromptsUpdated          = "mcp.prompts.updated"
	EventMCPAuthRequested           = "mcp.auth.requested"
	EventMCPAuthCompleted           = "mcp.auth.completed"
	EventMCPAuthDenied              = "mcp.auth.denied"
	EventMCPAuthFailed              = "mcp.auth.failed"
	EventMCPElicitationRequested    = "mcp.elicitation.requested"
	EventMCPElicitationCompleted    = "mcp.elicitation.completed"
	EventMCPElicitationDenied       = "mcp.elicitation.denied"
	EventMCPElicitationFailed       = "mcp.elicitation.failed"
	EventUsageUpdated               = "usage.updated"
	EventAuditRecorded              = "audit.recorded"
	EventSnapshotRequired           = "snapshot_required"
)

var EventTypes = []string{
	EventRuntimeStarted,
	EventRuntimeFailed,
	EventSessionCreated,
	EventSessionUpdated,
	EventSessionDeleted,
	EventTurnStarted,
	EventTurnProgress,
	EventTurnCompleted,
	EventTurnFailed,
	EventTurnCancelled,
	EventTurnInterrupted,
	EventMessageCreated,
	EventMessageUpdated,
	EventMessageCompleted,
	EventToolCallStarted,
	EventToolCallOutput,
	EventOutputRefCreated,
	EventArtifactRefCreated,
	EventToolOutputRefCreated,
	EventToolCallCompleted,
	EventToolCallFailed,
	EventToolCallCancelled,
	EventToolSearchPerformed,
	EventToolDiscoverySelected,
	EventToolDiscoveryOmitted,
	EventSchedulerDeadlockPrevented,
	EventTaskStarted,
	EventTaskProgress,
	EventTaskCompleted,
	EventTaskFailed,
	EventTaskCancelled,
	EventTaskInterrupted,
	EventTaskRoleLoaded,
	EventTaskScopeApplied,
	EventTaskScopeDenied,
	EventTaskMessageCreated,
	EventTaskResultUpdated,
	EventTaskArtifactCreated,
	EventWorktreeCreated,
	EventWorktreeEntered,
	EventWorktreeExited,
	EventWorktreeCleaned,
	EventWorktreeCleanupFailed,
	EventWorktreePolicyDenied,
	EventWorktreeRecovered,
	EventWorktreeMissingPath,
	EventWorktreePreserved,
	EventPermissionRequested,
	EventPermissionDecided,
	EventPermissionPolicyApplied,
	EventPolicyRuleMatched,
	EventPolicyRuleDenied,
	EventPolicyRuleAsk,
	EventShellPolicyClassified,
	EventTodoUpdated,
	EventCapabilityLoading,
	EventCapabilityLoaded,
	EventCapabilityFailed,
	EventContextLoading,
	EventContextLoaded,
	EventContextFailed,
	EventContextReinjected,
	EventContextSourceSkipped,
	EventContextSourceFailed,
	EventBudgetUpdated,
	EventCompactBoundaryRecorded,
	EventCompactMicroCompleted,
	EventCompactFullCompleted,
	EventCompactFailed,
	EventCompactOutputPreserved,
	EventSkillDiscoveryStarted,
	EventSkillDiscoveryCompleted,
	EventSkillDiscoveryFailed,
	EventSkillEnabled,
	EventSkillDisabled,
	EventSkillActivated,
	EventSkillActivationFailed,
	EventSkillActivationAllowed,
	EventSkillActivationDenied,
	EventSkillContextInjected,
	EventSkillContextOmitted,
	EventMCPCapabilityAllowed,
	EventMCPCapabilityDenied,
	EventMCPServerLazyStarted,
	EventMCPServerLazyFailed,
	EventMCPServerStarting,
	EventMCPServerConnected,
	EventMCPServerFailed,
	EventMCPServerBlocked,
	EventMCPServerDisabled,
	EventMCPToolsUpdated,
	EventMCPResourcesUpdated,
	EventMCPPromptsUpdated,
	EventMCPAuthRequested,
	EventMCPAuthCompleted,
	EventMCPAuthDenied,
	EventMCPAuthFailed,
	EventMCPElicitationRequested,
	EventMCPElicitationCompleted,
	EventMCPElicitationDenied,
	EventMCPElicitationFailed,
	EventUsageUpdated,
	EventAuditRecorded,
	EventSnapshotRequired,
}

type Event struct {
	ID         string         `json:"id"`
	Sequence   int64          `json:"sequence"`
	Type       string         `json:"type"`
	CreatedAt  string         `json:"created_at"`
	SessionID  string         `json:"session_id"`
	TurnID     string         `json:"turn_id"`
	MessageID  string         `json:"message_id"`
	ToolCallID string         `json:"tool_call_id"`
	Payload    map[string]any `json:"payload"`
}

func NewEvent(id, eventType string, createdAt time.Time) Event {
	return Event{
		ID:        id,
		Type:      eventType,
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
	}
}

func (e Event) Validate() error {
	if e.ID == "" {
		return errors.New("runtime event id is required")
	}
	if e.Type == "" {
		return errors.New("runtime event type is required")
	}
	if !IsEventType(e.Type) {
		return fmt.Errorf("unknown runtime event type %q", e.Type)
	}
	if e.CreatedAt == "" {
		return errors.New("runtime event created_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.CreatedAt); err != nil {
		return fmt.Errorf("runtime event created_at must be RFC3339: %w", err)
	}
	return nil
}

func IsEventType(eventType string) bool {
	for _, candidate := range EventTypes {
		if eventType == candidate {
			return true
		}
	}
	return false
}
