package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type runtimeSchedulerRecorder struct {
	service *runtimeService
}

func (r *runtimeSchedulerRecorder) CapabilityAllowed(ctx context.Context, metadata agent.SchedulerToolMetadata) bool {
	if r == nil || r.service == nil {
		return true
	}
	capability := RuntimeCapability{
		ID:          metadata.CapabilityID,
		Kind:        "builtin_tool",
		Name:        metadata.Name,
		Source:      metadata.Source,
		Enabled:     true,
		Risk:        "read",
		Description: metadata.Description,
		State:       capabilityStateLoaded,
	}
	if metadata.Source == string(scheduler.ToolSourceMCP) {
		capability.Kind = "mcp_tool"
		capability.Risk = "network"
		if capability.ID == "" {
			capability.ID = capabilityIDForToolName(metadata.Name)
		}
	}
	if metadata.Source == string(scheduler.ToolSourceShell) {
		capability.Kind = "builtin_tool"
		capability.Risk = "execute"
	}
	if task, ok := r.service.agentTaskForChildSession(ctx, metadata.SessionID); ok {
		call := agent.SchedulerToolCall{
			SessionID:    metadata.SessionID,
			TurnID:       metadata.TurnID,
			Name:         metadata.Name,
			Source:       metadata.Source,
			CapabilityID: capability.ID,
			Risk:         capability.Risk,
			InputSummary: metadata.SchemaSummary,
		}
		if reason := r.service.agentTaskScopeViolation(task, call); reason != "" {
			r.service.recordAgentTaskScope(ctx, task, false, reason)
			r.service.storeRuntimeEvent(runtimeapi.Event{
				ID:        newRuntimeEventID(),
				Type:      runtimeapi.EventMCPCapabilityDenied,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Payload: map[string]any{
					"capability_id": capability.ID,
					"name":          capability.Name,
					"kind":          capability.Kind,
					"reason":        reason,
					"scope":         "agent_task",
					"summary":       capability.Name,
				},
			})
			return false
		}
		r.service.recordAgentTaskScope(ctx, task, true, "task scope allowed capability")
	}
	decision := r.service.evaluateCapabilitySearchPolicy(capability)
	if decision.Decision == permission.PolicyDeny {
		r.service.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventMCPCapabilityDenied,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"capability_id": capability.ID,
				"name":          capability.Name,
				"kind":          capability.Kind,
				"reason":        decision.Reason,
				"summary":       capability.Name,
			},
		})
		return false
	}
	return true
}

func (r *runtimeSchedulerRecorder) EvaluateToolCall(ctx context.Context, call agent.SchedulerToolCall) (agent.SchedulerToolPolicyDecision, error) {
	if r == nil || r.service == nil {
		return agent.SchedulerToolPolicyDecision{}, nil
	}
	r.service.mu.Lock()
	workspaceID := ""
	if r.service.workspace != nil {
		workspaceID = r.service.workspace.ID
	}
	runtimeBackend := r.service.runtime
	r.service.mu.Unlock()
	source := scheduler.ToolSource(call.Source)
	if source == "" {
		source = scheduler.ToolSourceUnknown
	}
	if call.Name == agent.ToolSearchToolName {
		if blocked, reason := r.service.preventNestedToolSearch(ctx, call); blocked {
			r.service.recordDeadlockPrevented(call.SessionID, call.TurnID, call.ID, reason, "nested tool discovery is not allowed")
			return agent.SchedulerToolPolicyDecision{
				Decision: string(permission.PolicyDeny),
				Risk:     string(permission.RiskRead),
				Reason:   "Scheduler blocked tool search recursion: " + reason,
				Mode:     r.service.policy.Mode,
				Profile:  r.service.policy.Profile,
			}, nil
		}
	}
	if decision, denied := r.evaluateAgentTaskScope(ctx, call); denied {
		return decision, nil
	}
	policy := runtimePermissionPolicy(r.service.policy)
	result := policy.Evaluate(scheduler.ToolCall{
		ID:           call.ID,
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		MessageID:    call.MessageID,
		Name:         call.Name,
		Source:       source,
		CapabilityID: call.CapabilityID,
		Command:      call.Command,
		Status:       scheduler.ToolCallPending,
		InputSummary: call.InputSummary,
	})
	if result.Decision != permission.PolicyAsk {
		if result.Decision == permission.PolicyAllow {
			if blocked, reason := r.service.incrementRunningToolGuard(call); blocked {
				r.service.recordDeadlockPrevented(call.SessionID, call.TurnID, call.ID, reason, "concurrent runtime tool limit reached")
				return agent.SchedulerToolPolicyDecision{
					Decision: string(permission.PolicyDeny),
					Risk:     string(result.Risk),
					Reason:   "Scheduler blocked tool call to avoid deadlock: " + reason,
					Mode:     string(result.Mode),
					Profile:  result.Profile,
				}, nil
			}
		}
		r.recordPolicyDecision(call, result)
		return agent.SchedulerToolPolicyDecision{
			Decision:       string(result.Decision),
			Risk:           string(result.Risk),
			Reason:         result.Reason,
			Mode:           string(result.Mode),
			Profile:        result.Profile,
			RuleID:         result.RuleID,
			RuleSource:     result.RuleSource,
			RuleScopeKind:  result.RuleScopeKind,
			RuleScopeValue: result.RuleScopeValue,
			TargetSummary:  result.TargetSummary,
			ShellRisk:      string(result.Shell.Risk),
			ShellReason:    result.Shell.Reason,
		}, nil
	}
	if runtimeBackend == nil || workspaceID == "" {
		return agent.SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyDeny),
			Risk:     string(result.Risk),
			Reason:   "Runtime policy requires approval, but no permission service is available.",
			Mode:     string(result.Mode),
			Profile:  result.Profile,
		}, nil
	}
	granted, err := runtimeBackend.GetWorkspace(workspaceID)
	if err != nil {
		return agent.SchedulerToolPolicyDecision{}, err
	}
	allowed, err := granted.Permissions.Request(ctx, permission.CreatePermissionRequest{
		SessionID:   call.SessionID,
		TurnID:      call.TurnID,
		ToolCallID:  call.ID,
		ToolName:    call.Name,
		Source:      call.Source,
		Description: call.InputSummary,
		Action:      policyActionForToolCall(call, result.Risk),
		Path:        policyTargetForToolCall(call),
		Risk:        result.Risk,
	})
	if err != nil {
		return agent.SchedulerToolPolicyDecision{}, err
	}
	if !allowed {
		return agent.SchedulerToolPolicyDecision{
			Decision:       string(permission.PolicyDeny),
			Risk:           string(result.Risk),
			Reason:         firstNonEmpty(result.Reason, "Permission denied."),
			Mode:           string(result.Mode),
			Profile:        result.Profile,
			RuleID:         result.RuleID,
			RuleSource:     result.RuleSource,
			RuleScopeKind:  result.RuleScopeKind,
			RuleScopeValue: result.RuleScopeValue,
			TargetSummary:  result.TargetSummary,
			ShellRisk:      string(result.Shell.Risk),
			ShellReason:    result.Shell.Reason,
		}, nil
	}
	if blocked, reason := r.service.incrementRunningToolGuard(call); blocked {
		r.service.recordDeadlockPrevented(call.SessionID, call.TurnID, call.ID, reason, "concurrent runtime tool limit reached")
		return agent.SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyDeny),
			Risk:     string(result.Risk),
			Reason:   "Scheduler blocked tool call to avoid deadlock: " + reason,
			Mode:     string(result.Mode),
			Profile:  result.Profile,
		}, nil
	}
	return agent.SchedulerToolPolicyDecision{
		Decision:       string(permission.PolicyAllow),
		Risk:           string(result.Risk),
		Reason:         result.Reason,
		Mode:           string(result.Mode),
		Profile:        result.Profile,
		RuleID:         result.RuleID,
		RuleSource:     result.RuleSource,
		RuleScopeKind:  result.RuleScopeKind,
		RuleScopeValue: result.RuleScopeValue,
		TargetSummary:  result.TargetSummary,
		ShellRisk:      string(result.Shell.Risk),
		ShellReason:    result.Shell.Reason,
	}, nil
}

func (r *runtimeSchedulerRecorder) evaluateAgentTaskScope(ctx context.Context, call agent.SchedulerToolCall) (agent.SchedulerToolPolicyDecision, bool) {
	task, ok := r.service.agentTaskForChildSession(ctx, call.SessionID)
	if !ok {
		return agent.SchedulerToolPolicyDecision{}, false
	}
	reason := r.service.agentTaskScopeViolation(task, call)
	if reason == "" {
		r.service.recordAgentTaskScope(ctx, task, true, "task scope allowed tool call")
		return agent.SchedulerToolPolicyDecision{}, false
	}
	r.service.recordAgentTaskScope(ctx, task, false, reason)
	result := SchedulerToolPolicyDecisionFromScopeDeny(reason)
	result.Risk = "execute"
	result.Mode = r.service.policy.Mode
	result.Profile = r.service.policy.Profile
	result.RuleScopeKind = "task_scope"
	result.RuleScopeValue = task.ID
	return result, true
}

func SchedulerToolPolicyDecisionFromScopeDeny(reason string) agent.SchedulerToolPolicyDecision {
	return agent.SchedulerToolPolicyDecision{
		Decision: string(permission.PolicyDeny),
		Reason:   reason,
	}
}

func (r *runtimeSchedulerRecorder) recordPolicyDecision(call agent.SchedulerToolCall, result permission.PolicyResult) {
	r.service.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionPolicyApplied,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  call.SessionID,
		TurnID:     call.TurnID,
		MessageID:  call.MessageID,
		ToolCallID: call.ID,
		Payload: map[string]any{
			"tool_name":           call.Name,
			"capability_id":       call.CapabilityID,
			"decision":            result.Decision,
			"risk":                result.Risk,
			"reason":              result.Reason,
			"mode":                result.Mode,
			"profile":             result.Profile,
			"matched_rule_id":     result.RuleID,
			"matched_rule_source": result.RuleSource,
			"scope_kind":          result.RuleScopeKind,
			"scope_value":         result.RuleScopeValue,
			"target_summary":      result.TargetSummary,
			"shell_risk":          result.Shell.Risk,
			"shell_reason":        result.Shell.Reason,
			"summary":             call.Name,
		},
	})
	r.recordPolicyDiagnosticsEvents(call, result)
	r.service.writeAudit(auditEntry{
		RequestID:           call.TurnID,
		Event:               "permission_policy_applied",
		Timestamp:           time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:           call.SessionID,
		PermissionTool:      call.Name,
		PermissionAction:    policyActionForToolCall(call, result.Risk),
		PermissionPath:      policyTargetForToolCall(call),
		PermissionPolicy:    string(result.Decision),
		PermissionRisk:      string(result.Risk),
		PermissionReason:    result.Reason,
		PolicyMode:          string(result.Mode),
		PolicyProfile:       result.Profile,
		PolicyRuleID:        result.RuleID,
		PolicyRuleSource:    result.RuleSource,
		PolicyScopeKind:     result.RuleScopeKind,
		PolicyScopeValue:    result.RuleScopeValue,
		PolicyTargetSummary: result.TargetSummary,
		ShellRisk:           string(result.Shell.Risk),
		ShellReason:         result.Shell.Reason,
		ToolCallID:          call.ID,
		CapabilityID:        call.CapabilityID,
	})
}

func (r *runtimeSchedulerRecorder) recordPolicyDiagnosticsEvents(call agent.SchedulerToolCall, result permission.PolicyResult) {
	if result.RuleID != "" {
		eventType := runtimeapi.EventPolicyRuleMatched
		if result.Decision == permission.PolicyDeny {
			eventType = runtimeapi.EventPolicyRuleDenied
		} else if result.Decision == permission.PolicyAsk {
			eventType = runtimeapi.EventPolicyRuleAsk
		}
		r.service.storeRuntimeEvent(runtimeapi.Event{
			ID:         newRuntimeEventID(),
			Type:       eventType,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:  call.SessionID,
			TurnID:     call.TurnID,
			MessageID:  call.MessageID,
			ToolCallID: call.ID,
			Payload: map[string]any{
				"rule_id":     result.RuleID,
				"source":      result.RuleSource,
				"scope_kind":  result.RuleScopeKind,
				"scope_value": result.RuleScopeValue,
				"decision":    result.Decision,
				"risk":        result.Risk,
				"reason":      result.Reason,
				"mode":        result.Mode,
				"profile":     result.Profile,
			},
		})
	}
	if result.Shell.Command != "" {
		r.service.storeRuntimeEvent(runtimeapi.Event{
			ID:         newRuntimeEventID(),
			Type:       runtimeapi.EventShellPolicyClassified,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:  call.SessionID,
			TurnID:     call.TurnID,
			MessageID:  call.MessageID,
			ToolCallID: call.ID,
			Payload: map[string]any{
				"risk":           result.Shell.Risk,
				"reason":         result.Shell.Reason,
				"target_summary": result.Shell.TargetSummary,
				"shell":          result.Shell.Shell,
				"decision":       result.Decision,
				"mode":           result.Mode,
			},
		})
	}
}

func policyActionForToolCall(call agent.SchedulerToolCall, risk permission.Risk) string {
	if call.Source == string(scheduler.ToolSourceShell) {
		return "execute"
	}
	if risk == permission.RiskRead {
		return "read"
	}
	return string(risk)
}

func policyTargetForToolCall(call agent.SchedulerToolCall) string {
	if call.CapabilityID != "" {
		return call.CapabilityID
	}
	if call.Source != "" {
		return fmt.Sprintf("%s:%s", call.Source, call.Name)
	}
	return call.Name
}

func (r *runtimeSchedulerRecorder) ToolCallStarted(ctx context.Context, call agent.SchedulerToolCall) error {
	if r == nil || r.service == nil || r.service.toolCalls == nil {
		return nil
	}
	stored, err := r.service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
		ID:                  call.ID,
		SessionID:           call.SessionID,
		TurnID:              call.TurnID,
		MessageID:           call.MessageID,
		Name:                call.Name,
		Source:              scheduler.ToolSource(call.Source),
		CapabilityID:        call.CapabilityID,
		JobID:               call.JobID,
		Command:             call.Command,
		Risk:                call.Risk,
		PolicyReason:        call.PolicyReason,
		PolicyMode:          call.PolicyMode,
		PolicyProfile:       call.PolicyProfile,
		PolicyRuleID:        call.PolicyRuleID,
		PolicyRuleSource:    call.PolicyRuleSource,
		PolicyScopeKind:     call.PolicyScopeKind,
		PolicyScopeValue:    call.PolicyScopeValue,
		PolicyTargetSummary: call.PolicyTargetSummary,
		ShellRisk:           call.ShellRisk,
		ShellReason:         call.ShellReason,
		JobStatus:           call.JobStatus,
		JobStartedAt:        timeFromMillis(call.JobStartedAt),
		InputSummary:        preview(call.InputSummary, runtimePartPreviewLimit),
	})
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallStarted, stored, map[string]any{
		"name":                stored.Name,
		"source":              string(stored.Source),
		"capability_id":       stored.CapabilityID,
		"input":               stored.InputSummary,
		"job_id":              stored.JobID,
		"job_status":          stored.JobStatus,
		"command":             stored.Command,
		"risk":                stored.Risk,
		"policy_reason":       stored.PolicyReason,
		"policy_mode":         stored.PolicyMode,
		"matched_rule_id":     stored.PolicyRuleID,
		"matched_rule_source": stored.PolicyRuleSource,
		"scope_kind":          stored.PolicyScopeKind,
		"scope_value":         stored.PolicyScopeValue,
		"target_summary":      stored.PolicyTargetSummary,
		"shell_risk":          stored.ShellRisk,
		"shell_reason":        stored.ShellReason,
		"status":              string(stored.Status),
		"summary":             stored.Name,
	}))
	r.service.writeAudit(auditEntry{
		RequestID:    stored.TurnID,
		Event:        "tool_call_started",
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    stored.SessionID,
		ToolCallID:   stored.ID,
		CapabilityID: stored.CapabilityID,
		ToolCalls: []auditToolCall{{
			ID:               stored.ID,
			Name:             stored.Name,
			Input:            stored.InputSummary,
			JobID:            stored.JobID,
			Command:          stored.Command,
			Risk:             stored.Risk,
			PolicyMode:       stored.PolicyMode,
			PolicyRuleID:     stored.PolicyRuleID,
			PolicyScopeKind:  stored.PolicyScopeKind,
			PolicyScopeValue: stored.PolicyScopeValue,
			ShellRisk:        stored.ShellRisk,
			ShellReason:      stored.ShellReason,
			Status:           stored.JobStatus,
			StartedAt:        millisFromTime(stored.JobStartedAt),
		}},
	})
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallOutput(ctx context.Context, result agent.SchedulerToolCallResult) error {
	call, err := r.updateToolCall(ctx, result, scheduler.ToolCallRunning)
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallOutput, call, map[string]any{
		"name":         call.Name,
		"summary":      call.OutputSummary,
		"job_id":       call.JobID,
		"job_status":   call.JobStatus,
		"shell_status": call.JobStatus,
		"is_error":     result.IsError,
		"status":       string(call.Status),
		"has_stdout":   call.Stdout != "",
		"has_stderr":   call.Stderr != "",
	}))
	if call.JobID != "" {
		r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventTaskProgress, call, map[string]any{
			"task_kind":  "background",
			"job_id":     call.JobID,
			"job_status": call.JobStatus,
			"status":     string(call.Status),
			"summary":    call.OutputSummary,
			"has_stdout": call.Stdout != "",
			"has_stderr": call.Stderr != "",
		}))
	}
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallCompleted(ctx context.Context, result agent.SchedulerToolCallResult) error {
	defer r.service.decrementRunningToolGuard(result.TurnID)
	call, err := r.updateToolCall(ctx, result, scheduler.ToolCallCompleted)
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCompleted, call, map[string]any{
		"name":       call.Name,
		"summary":    call.OutputSummary,
		"job_id":     call.JobID,
		"job_status": call.JobStatus,
		"status":     string(call.Status),
	}))
	r.auditToolResult(call)
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallFailed(ctx context.Context, result agent.SchedulerToolCallResult) error {
	defer r.service.decrementRunningToolGuard(result.TurnID)
	status := scheduler.ToolCallFailed
	if result.Status == string(scheduler.ToolCallDenied) {
		status = scheduler.ToolCallDenied
	}
	call, err := r.updateToolCall(ctx, result, status)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"name":       call.Name,
		"summary":    call.OutputSummary,
		"job_id":     call.JobID,
		"job_status": call.JobStatus,
		"status":     string(call.Status),
		"is_error":   true,
		"error":      call.Error,
	}
	if call.Status == scheduler.ToolCallDenied {
		payload["denied"] = true
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallFailed, call, payload))
	r.auditToolResult(call)
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallCancelled(ctx context.Context, result agent.SchedulerToolCallResult) error {
	defer r.service.decrementRunningToolGuard(result.TurnID)
	call, err := r.updateToolCall(ctx, result, scheduler.ToolCallCancelled)
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, call, map[string]any{
		"name":       call.Name,
		"summary":    call.OutputSummary,
		"job_id":     call.JobID,
		"job_status": call.JobStatus,
		"status":     string(call.Status),
		"error":      call.Error,
	}))
	r.auditToolResult(call)
	return nil
}

func (r *runtimeSchedulerRecorder) updateToolCall(ctx context.Context, result agent.SchedulerToolCallResult, status scheduler.ToolCallStatus) (scheduler.ToolCall, error) {
	if r == nil || r.service == nil || r.service.toolCalls == nil || result.ToolCallID == "" {
		return scheduler.ToolCall{}, nil
	}
	if _, err := r.service.toolCalls.GetCall(ctx, result.ToolCallID); err != nil {
		_, _ = r.service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:                  result.ToolCallID,
			SessionID:           result.SessionID,
			TurnID:              result.TurnID,
			MessageID:           result.MessageID,
			Name:                result.Name,
			Source:              scheduler.ToolSource(result.Source),
			CapabilityID:        capabilityIDForToolName(result.Name),
			JobID:               result.JobID,
			Command:             result.Command,
			Risk:                result.Risk,
			PolicyReason:        result.PolicyReason,
			PolicyMode:          result.PolicyMode,
			PolicyProfile:       result.PolicyProfile,
			PolicyRuleID:        result.PolicyRuleID,
			PolicyRuleSource:    result.PolicyRuleSource,
			PolicyScopeKind:     result.PolicyScopeKind,
			PolicyScopeValue:    result.PolicyScopeValue,
			PolicyTargetSummary: result.PolicyTargetSummary,
			ShellRisk:           result.ShellRisk,
			ShellReason:         result.ShellReason,
			JobStatus:           result.JobStatus,
			JobStartedAt:        timeFromMillis(result.JobStartedAt),
		})
	}
	return r.service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
		ToolCallID:          result.ToolCallID,
		Status:              status,
		JobID:               result.JobID,
		Command:             result.Command,
		Risk:                result.Risk,
		PolicyReason:        result.PolicyReason,
		PolicyMode:          result.PolicyMode,
		PolicyProfile:       result.PolicyProfile,
		PolicyRuleID:        result.PolicyRuleID,
		PolicyRuleSource:    result.PolicyRuleSource,
		PolicyScopeKind:     result.PolicyScopeKind,
		PolicyScopeValue:    result.PolicyScopeValue,
		PolicyTargetSummary: result.PolicyTargetSummary,
		ShellRisk:           result.ShellRisk,
		ShellReason:         result.ShellReason,
		ExitCode:            result.ExitCode,
		JobStatus:           result.JobStatus,
		JobStartedAt:        timeFromMillis(result.JobStartedAt),
		JobFinishedAt:       timeFromMillis(result.JobFinishedAt),
		OutputSummary:       preview(firstNonEmpty(result.StructuredOutputSummary, result.ModelVisibleContent, result.Error), runtimePartPreviewLimit),
		ModelContent:        preview(result.ModelVisibleContent, runtimePartPreviewLimit),
		Structured:          preview(result.StructuredOutputSummary, runtimePartPreviewLimit),
		Stdout:              preview(result.Stdout, runtimePartPreviewLimit),
		Stderr:              preview(result.Stderr, runtimePartPreviewLimit),
		IsError:             result.IsError || status == scheduler.ToolCallFailed || status == scheduler.ToolCallCancelled,
		Error:               preview(result.Error, runtimePartPreviewLimit),
	})
}

func (r *runtimeSchedulerRecorder) auditToolResult(call scheduler.ToolCall) {
	if call.ID == "" {
		return
	}
	r.service.writeAudit(auditEntry{
		RequestID:    call.TurnID,
		Event:        "tool_call_" + string(call.Status),
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    call.SessionID,
		ToolCallID:   call.ID,
		CapabilityID: call.CapabilityID,
		ToolCalls: []auditToolCall{{
			ID:               call.ID,
			Name:             call.Name,
			Input:            call.InputSummary,
			Output:           call.OutputSummary,
			JobID:            call.JobID,
			Command:          call.Command,
			Risk:             call.Risk,
			PolicyMode:       call.PolicyMode,
			PolicyRuleID:     call.PolicyRuleID,
			PolicyScopeKind:  call.PolicyScopeKind,
			PolicyScopeValue: call.PolicyScopeValue,
			ShellRisk:        call.ShellRisk,
			ShellReason:      call.ShellReason,
			ExitCode:         call.ExitCode,
			IsError:          call.IsError,
			Status:           firstNonEmpty(call.JobStatus, string(call.Status)),
			StartedAt:        millisFromTime(call.JobStartedAt),
			FinishedAt:       millisFromTime(call.JobFinishedAt),
		}},
		Error:               call.Error,
		PermissionRisk:      call.Risk,
		PermissionReason:    call.PolicyReason,
		PolicyMode:          call.PolicyMode,
		PolicyProfile:       call.PolicyProfile,
		PolicyRuleID:        call.PolicyRuleID,
		PolicyRuleSource:    call.PolicyRuleSource,
		PolicyScopeKind:     call.PolicyScopeKind,
		PolicyScopeValue:    call.PolicyScopeValue,
		PolicyTargetSummary: call.PolicyTargetSummary,
		ShellRisk:           call.ShellRisk,
		ShellReason:         call.ShellReason,
	})
}

func timeFromMillis(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func millisFromTime(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func runtimeToolCallEvent(eventType string, call scheduler.ToolCall, payload map[string]any) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, time.Now().UTC())
	event.SessionID = call.SessionID
	event.TurnID = call.TurnID
	event.MessageID = call.MessageID
	event.ToolCallID = call.ID
	if call.CapabilityID != "" {
		payload["capability_id"] = call.CapabilityID
	}
	event.Payload = payload
	return event
}
