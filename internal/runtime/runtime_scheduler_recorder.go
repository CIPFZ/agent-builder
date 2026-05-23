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

func (r *runtimeSchedulerRecorder) EvaluateToolCall(ctx context.Context, call agent.SchedulerToolCall) (agent.SchedulerToolPolicyDecision, error) {
	if r == nil || r.service == nil {
		return agent.SchedulerToolPolicyDecision{}, nil
	}
	r.service.mu.Lock()
	mode := permission.PolicyMode(r.service.policy.Mode)
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
	result := permission.NewPermissionPolicy(mode).Evaluate(scheduler.ToolCall{
		ID:           call.ID,
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		MessageID:    call.MessageID,
		Name:         call.Name,
		Source:       source,
		CapabilityID: call.CapabilityID,
		Status:       scheduler.ToolCallPending,
		InputSummary: call.InputSummary,
	})
	if result.Decision != permission.PolicyAsk {
		r.recordPolicyDecision(call, result)
		return agent.SchedulerToolPolicyDecision{
			Decision: string(result.Decision),
			Risk:     string(result.Risk),
			Reason:   result.Reason,
			Mode:     string(result.Mode),
		}, nil
	}
	if runtimeBackend == nil || workspaceID == "" {
		return agent.SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyDeny),
			Risk:     string(result.Risk),
			Reason:   "Runtime policy requires approval, but no permission service is available.",
			Mode:     string(result.Mode),
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
			Decision: string(permission.PolicyDeny),
			Risk:     string(result.Risk),
			Reason:   firstNonEmpty(result.Reason, "Permission denied."),
			Mode:     string(result.Mode),
		}, nil
	}
	return agent.SchedulerToolPolicyDecision{
		Decision: string(permission.PolicyAllow),
		Risk:     string(result.Risk),
		Reason:   result.Reason,
		Mode:     string(result.Mode),
	}, nil
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
			"tool_name":     call.Name,
			"capability_id": call.CapabilityID,
			"decision":      result.Decision,
			"risk":          result.Risk,
			"reason":        result.Reason,
			"mode":          result.Mode,
			"summary":       call.Name,
		},
	})
	r.service.writeAudit(auditEntry{
		RequestID:        call.TurnID,
		Event:            "permission_policy_applied",
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:        call.SessionID,
		PermissionTool:   call.Name,
		PermissionAction: policyActionForToolCall(call, result.Risk),
		PermissionPath:   policyTargetForToolCall(call),
		PermissionPolicy: string(result.Decision),
		PermissionRisk:   string(result.Risk),
		PermissionReason: result.Reason,
		PolicyMode:       string(result.Mode),
		ToolCallID:       call.ID,
		CapabilityID:     call.CapabilityID,
	})
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
		ID:           call.ID,
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		MessageID:    call.MessageID,
		Name:         call.Name,
		Source:       scheduler.ToolSource(call.Source),
		CapabilityID: call.CapabilityID,
		JobID:        call.JobID,
		Command:      call.Command,
		Risk:         call.Risk,
		PolicyReason: call.PolicyReason,
		InputSummary: preview(call.InputSummary, runtimePartPreviewLimit),
	})
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallStarted, stored, map[string]any{
		"name":          stored.Name,
		"source":        string(stored.Source),
		"capability_id": stored.CapabilityID,
		"input":         stored.InputSummary,
		"job_id":        stored.JobID,
		"command":       stored.Command,
		"risk":          stored.Risk,
		"policy_reason": stored.PolicyReason,
		"status":        string(stored.Status),
		"summary":       stored.Name,
	}))
	r.service.writeAudit(auditEntry{
		RequestID:    stored.TurnID,
		Event:        "tool_call_started",
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:    stored.SessionID,
		ToolCallID:   stored.ID,
		CapabilityID: stored.CapabilityID,
		ToolCalls: []auditToolCall{{
			ID:      stored.ID,
			Name:    stored.Name,
			Input:   stored.InputSummary,
			JobID:   stored.JobID,
			Command: stored.Command,
			Risk:    stored.Risk,
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
		"name":       call.Name,
		"summary":    call.OutputSummary,
		"job_id":     call.JobID,
		"job_status": string(call.Status),
		"is_error":   result.IsError,
		"status":     string(call.Status),
		"has_stdout": call.Stdout != "",
		"has_stderr": call.Stderr != "",
	}))
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallCompleted(ctx context.Context, result agent.SchedulerToolCallResult) error {
	call, err := r.updateToolCall(ctx, result, scheduler.ToolCallCompleted)
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCompleted, call, map[string]any{
		"name":    call.Name,
		"summary": call.OutputSummary,
		"job_id":  call.JobID,
		"status":  string(call.Status),
	}))
	r.auditToolResult(call)
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallFailed(ctx context.Context, result agent.SchedulerToolCallResult) error {
	status := scheduler.ToolCallFailed
	if result.Status == string(scheduler.ToolCallDenied) {
		status = scheduler.ToolCallDenied
	}
	call, err := r.updateToolCall(ctx, result, status)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"name":     call.Name,
		"summary":  call.OutputSummary,
		"job_id":   call.JobID,
		"status":   string(call.Status),
		"is_error": true,
		"error":    call.Error,
	}
	if call.Status == scheduler.ToolCallDenied {
		payload["denied"] = true
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallFailed, call, payload))
	r.auditToolResult(call)
	return nil
}

func (r *runtimeSchedulerRecorder) ToolCallCancelled(ctx context.Context, result agent.SchedulerToolCallResult) error {
	call, err := r.updateToolCall(ctx, result, scheduler.ToolCallCancelled)
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, call, map[string]any{
		"name":    call.Name,
		"summary": call.OutputSummary,
		"job_id":  call.JobID,
		"status":  string(call.Status),
		"error":   call.Error,
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
			ID:           result.ToolCallID,
			SessionID:    result.SessionID,
			TurnID:       result.TurnID,
			MessageID:    result.MessageID,
			Name:         result.Name,
			Source:       scheduler.ToolSource(result.Source),
			CapabilityID: capabilityIDForToolName(result.Name),
			JobID:        result.JobID,
			Command:      result.Command,
			Risk:         result.Risk,
			PolicyReason: result.PolicyReason,
		})
	}
	return r.service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
		ToolCallID:    result.ToolCallID,
		Status:        status,
		JobID:         result.JobID,
		Command:       result.Command,
		Risk:          result.Risk,
		PolicyReason:  result.PolicyReason,
		ExitCode:      result.ExitCode,
		OutputSummary: preview(firstNonEmpty(result.StructuredOutputSummary, result.ModelVisibleContent, result.Error), runtimePartPreviewLimit),
		ModelContent:  preview(result.ModelVisibleContent, runtimePartPreviewLimit),
		Structured:    preview(result.StructuredOutputSummary, runtimePartPreviewLimit),
		Stdout:        preview(result.Stdout, runtimePartPreviewLimit),
		Stderr:        preview(result.Stderr, runtimePartPreviewLimit),
		IsError:       result.IsError || status == scheduler.ToolCallFailed || status == scheduler.ToolCallCancelled,
		Error:         preview(result.Error, runtimePartPreviewLimit),
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
			ID:       call.ID,
			Name:     call.Name,
			Input:    call.InputSummary,
			Output:   call.OutputSummary,
			JobID:    call.JobID,
			Command:  call.Command,
			Risk:     call.Risk,
			ExitCode: call.ExitCode,
			IsError:  call.IsError,
		}},
		Error:            call.Error,
		PermissionRisk:   call.Risk,
		PermissionReason: call.PolicyReason,
	})
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
