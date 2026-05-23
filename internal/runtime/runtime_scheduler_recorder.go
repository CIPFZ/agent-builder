package runtime

import (
	"context"
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
	r.service.mu.Unlock()
	result := permission.NewPermissionPolicy(mode).Evaluate(scheduler.ToolCall{
		ID:           call.ID,
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		MessageID:    call.MessageID,
		Name:         call.Name,
		Source:       scheduler.ToolSource(call.Source),
		Status:       scheduler.ToolCallPending,
		InputSummary: call.InputSummary,
	})
	r.service.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionPolicyApplied,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  call.SessionID,
		TurnID:     call.TurnID,
		MessageID:  call.MessageID,
		ToolCallID: call.ID,
		Payload: map[string]any{
			"tool_name": call.Name,
			"decision":  result.Decision,
			"risk":      result.Risk,
			"reason":    result.Reason,
			"mode":      result.Mode,
			"summary":   call.Name,
		},
	})
	r.service.writeAudit(auditEntry{
		RequestID:        call.TurnID,
		Event:            "permission_policy_applied",
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:        call.SessionID,
		PermissionTool:   call.Name,
		PermissionPolicy: string(result.Decision),
		PermissionRisk:   string(result.Risk),
		PermissionReason: result.Reason,
		PolicyMode:       string(result.Mode),
		ToolCallID:       call.ID,
	})
	return agent.SchedulerToolPolicyDecision{
		Decision: string(result.Decision),
		Risk:     string(result.Risk),
		Reason:   result.Reason,
		Mode:     string(result.Mode),
	}, nil
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
		InputSummary: preview(call.InputSummary, runtimePartPreviewLimit),
	})
	if err != nil {
		return err
	}
	r.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallStarted, stored, map[string]any{
		"name":    stored.Name,
		"source":  string(stored.Source),
		"input":   stored.InputSummary,
		"status":  string(stored.Status),
		"summary": stored.Name,
	}))
	r.service.writeAudit(auditEntry{
		RequestID:  stored.TurnID,
		Event:      "tool_call_started",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  stored.SessionID,
		ToolCallID: stored.ID,
		ToolCalls: []auditToolCall{{
			ID:    stored.ID,
			Name:  stored.Name,
			Input: stored.InputSummary,
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
			ID:        result.ToolCallID,
			SessionID: result.SessionID,
			TurnID:    result.TurnID,
			MessageID: result.MessageID,
			Name:      result.Name,
			Source:    scheduler.ToolSource(result.Source),
		})
	}
	return r.service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
		ToolCallID:    result.ToolCallID,
		Status:        status,
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
		RequestID:  call.TurnID,
		Event:      "tool_call_" + string(call.Status),
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  call.SessionID,
		ToolCallID: call.ID,
		ToolCalls: []auditToolCall{{
			ID:      call.ID,
			Name:    call.Name,
			Input:   call.InputSummary,
			Output:  call.OutputSummary,
			IsError: call.IsError,
		}},
		Error: call.Error,
	})
}

func runtimeToolCallEvent(eventType string, call scheduler.ToolCall, payload map[string]any) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, time.Now().UTC())
	event.SessionID = call.SessionID
	event.TurnID = call.TurnID
	event.MessageID = call.MessageID
	event.ToolCallID = call.ID
	event.Payload = payload
	return event
}
