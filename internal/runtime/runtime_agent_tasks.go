package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeService) AgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return RuntimeAgentTaskResponse{}, errors.New("task id is required")
	}
	task, err := r.agentTasks.Get(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskResponse{}, fmt.Errorf("agent task %s was not found: %w", taskID, err)
	}
	messages, _ := r.agentTaskMessages(ctx, task.ID)
	result, _ := r.agentTaskResult(ctx, task.ID)
	if result.TaskID != "" {
		task.Result = &result
	}
	resp := RuntimeAgentTaskResponse{Task: task, Messages: messages}
	if result.TaskID != "" {
		resp.Result = &result
	}
	return resp, nil
}

func (r *runtimeService) TurnAgentTasks(ctx context.Context, turnID string) (RuntimeAgentTasksResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeAgentTasksResponse{}, errors.New("turn id is required")
	}
	tasks, err := r.agentTasks.ListByTurn(ctx, turnID)
	if err != nil {
		return RuntimeAgentTasksResponse{}, err
	}
	return RuntimeAgentTasksResponse{Tasks: tasks}, nil
}

func (r *runtimeService) CancelAgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return RuntimeAgentTaskResponse{}, errors.New("task id is required")
	}
	task, err := r.agentTasks.Get(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskResponse{}, fmt.Errorf("agent task %s was not found: %w", taskID, err)
	}
	if isFinalAgentTaskStatus(task.Status) {
		return RuntimeAgentTaskResponse{Task: task}, nil
	}

	r.mu.Lock()
	workspaceID := ""
	if r.workspace != nil {
		workspaceID = r.workspace.ID
	}
	runtimeBackend := r.runtime
	r.mu.Unlock()
	if runtimeBackend != nil && workspaceID != "" && task.ChildSessionID != "" {
		_ = runtimeBackend.CancelSession(workspaceID, task.ChildSessionID)
	}
	task.Status = agentTaskStatusCancelled
	task.Progress = 100
	task.FinishedAt = time.Now().UnixMilli()
	task.Error = firstNonEmpty(task.Error, "agent task cancellation requested")
	task.CancellationDetail = "Child session cancellation was requested through the runtime when available."
	task, err = r.agentTasks.Upsert(ctx, task)
	if err != nil {
		return RuntimeAgentTaskResponse{}, err
	}
	result, _ := r.upsertAgentTaskResult(ctx, RuntimeAgentTaskResult{
		TaskID:             task.ID,
		Status:             agentTaskStatusCancelled,
		Summary:            task.ResultSummary,
		ErrorDetail:        task.Error,
		CancellationDetail: task.CancellationDetail,
		ArtifactRefs:       task.ArtifactRefs,
	})
	_, _ = r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindControl,
		ContentSummary:    "cancel requested",
		RelatedToolCallID: task.ParentToolCallID,
		Payload: map[string]any{
			"action": "cancel",
			"reason": task.CancellationDetail,
		},
	})
	r.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskCancelled, "task_cancelled", task)
	return RuntimeAgentTaskResponse{Task: task, Result: &result}, nil
}

func (r *runtimeSchedulerRecorder) AgentTaskStarted(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	stored, err := r.service.agentTasks.Upsert(ctx, runtimeAgentTaskFromRecord(task, agentTaskStatusRunning))
	if err != nil {
		return err
	}
	_ = r.service.ensureAgentRolesLoaded(ctx)
	r.service.recordAgentTaskScope(ctx, stored, true, "task scope applied")
	_, _ = r.service.createAgentTaskMessage(ctx, stored, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindInstruction,
		ContentSummary:    stored.PromptSummary,
		RelatedToolCallID: stored.ParentToolCallID,
		Payload: map[string]any{
			"role":             stored.Role,
			"allowed_tools":    stored.AllowedTools,
			"capability_scope": stored.CapabilityScope,
			"model":            stored.Model,
			"provider":         stored.Provider,
			"cwd":              stored.CWD,
			"worktree":         stored.Worktree,
		},
	})
	r.service.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskStarted, "task_started", stored)
	return nil
}

func (r *runtimeSchedulerRecorder) AgentTaskProgress(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	stored, err := r.service.agentTasks.Upsert(ctx, runtimeAgentTaskFromRecord(task, firstNonEmpty(task.Status, agentTaskStatusRunning)))
	if err != nil {
		return err
	}
	_, _ = r.service.createAgentTaskMessage(ctx, stored, RuntimeAgentTaskMessage{
		Direction:      taskMessageDirectionChildToParent,
		Kind:           taskMessageKindProgress,
		ContentSummary: firstNonEmpty(stored.ResultSummary, stored.PromptSummary, stored.Title),
	})
	r.service.storeRuntimeEvent(runtimeAgentTaskEvent(runtimeapi.EventTaskProgress, stored))
	return nil
}

func (r *runtimeSchedulerRecorder) AgentTaskCompleted(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	record := runtimeAgentTaskFromRecord(task, agentTaskStatusCompleted)
	record.Progress = 100
	if record.FinishedAt == 0 {
		record.FinishedAt = time.Now().UnixMilli()
	}
	stored, err := r.service.agentTasks.Upsert(ctx, record)
	if err != nil {
		return err
	}
	result, _ := r.service.upsertAgentTaskResult(ctx, RuntimeAgentTaskResult{
		TaskID:              stored.ID,
		Status:              stored.Status,
		Summary:             stored.ResultSummary,
		ArtifactRefs:        stored.ArtifactRefs,
		RelatedToolCallRefs: []string{stored.ParentToolCallID},
	})
	msg, _ := r.service.createAgentTaskMessage(ctx, stored, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionChildToParent,
		Kind:              taskMessageKindResult,
		ContentSummary:    stored.ResultSummary,
		RelatedToolCallID: stored.ParentToolCallID,
		ArtifactRefs:      stored.ArtifactRefs,
		Payload: map[string]any{
			"status":  stored.Status,
			"summary": stored.ResultSummary,
		},
	})
	if msg.ID != "" {
		result.RelatedMessageRefs = append(result.RelatedMessageRefs, msg.ID)
		_, _ = r.service.upsertAgentTaskResult(ctx, result)
	}
	r.service.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskCompleted, "task_completed", stored)
	return nil
}

func (r *runtimeSchedulerRecorder) AgentTaskFailed(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	status := agentTaskStatusFailed
	if task.Status == agentTaskStatusCancelled {
		status = agentTaskStatusCancelled
	}
	record := runtimeAgentTaskFromRecord(task, status)
	record.Progress = 100
	if record.FinishedAt == 0 {
		record.FinishedAt = time.Now().UnixMilli()
	}
	stored, err := r.service.agentTasks.Upsert(ctx, record)
	if err != nil {
		return err
	}
	resultStatus := stored.Status
	_, _ = r.service.upsertAgentTaskResult(ctx, RuntimeAgentTaskResult{
		TaskID:             stored.ID,
		Status:             resultStatus,
		Summary:            stored.ResultSummary,
		ErrorDetail:        stored.Error,
		CancellationDetail: stored.CancellationDetail,
		ArtifactRefs:       stored.ArtifactRefs,
		RelatedToolCallRefs: []string{
			stored.ParentToolCallID,
		},
	})
	_, _ = r.service.createAgentTaskMessage(ctx, stored, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionChildToParent,
		Kind:              taskMessageKindResult,
		ContentSummary:    firstNonEmpty(stored.Error, stored.ResultSummary),
		RelatedToolCallID: stored.ParentToolCallID,
		ArtifactRefs:      stored.ArtifactRefs,
		Payload: map[string]any{
			"status": stored.Status,
			"error":  stored.Error,
		},
	})
	eventType := runtimeapi.EventTaskFailed
	auditType := "task_failed"
	if stored.Status == agentTaskStatusCancelled {
		eventType = runtimeapi.EventTaskCancelled
		auditType = "task_cancelled"
	}
	r.service.recordAgentTaskLifecycle(ctx, eventType, auditType, stored)
	return nil
}

func (r *runtimeService) AgentTaskMessages(ctx context.Context, taskID string) (RuntimeAgentTaskMessagesResponse, error) {
	messages, err := r.agentTaskMessages(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskMessagesResponse{}, err
	}
	return RuntimeAgentTaskMessagesResponse{Messages: messages}, nil
}

func (r *runtimeService) CreateAgentTaskMessage(ctx context.Context, taskID string, req RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	task, err := r.agentTasks.Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	msg, err := r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
		Direction:         req.Direction,
		Kind:              req.Kind,
		ContentSummary:    req.ContentSummary,
		Payload:           req.Payload,
		RelatedToolCallID: req.RelatedToolCallID,
		RelatedMessageID:  req.RelatedMessageID,
		ArtifactRefs:      req.ArtifactRefs,
	})
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	return RuntimeAgentTaskMessageResponse{Message: msg}, nil
}

func (r *runtimeService) AgentTaskResult(ctx context.Context, taskID string) (RuntimeAgentTaskResultResponse, error) {
	result, err := r.agentTaskResult(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskResultResponse{}, err
	}
	return RuntimeAgentTaskResultResponse{Result: result}, nil
}

func (r *runtimeService) agentTaskMessages(ctx context.Context, taskID string) ([]RuntimeAgentTaskMessage, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return nil, err
	}
	return newRuntimeAgentTaskMessageStore(db).ListByTask(ctx, strings.TrimSpace(taskID))
}

func (r *runtimeService) createAgentTaskMessage(ctx context.Context, task RuntimeAgentTask, msg RuntimeAgentTaskMessage) (RuntimeAgentTaskMessage, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	msg.TaskID = task.ID
	msg.ParentTurnID = firstNonEmpty(msg.ParentTurnID, task.ParentTurnID)
	msg.ParentSessionID = firstNonEmpty(msg.ParentSessionID, task.ParentSessionID)
	msg.ChildSessionID = firstNonEmpty(msg.ChildSessionID, task.ChildSessionID)
	msg.ContentSummary = preview(msg.ContentSummary, runtimePartPreviewLimit)
	msg.Payload = redactRuntimePayload(msg.Payload)
	stored, err := newRuntimeAgentTaskMessageStore(db).Create(ctx, msg)
	if err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	r.storeRuntimeEvent(runtimeAgentTaskMessageEvent(stored))
	auditType := "task_message_created"
	if stored.Kind == taskMessageKindArtifact || len(stored.ArtifactRefs) > 0 {
		r.storeRuntimeEvent(runtimeAgentTaskArtifactEvent(stored))
		auditType = "task_artifact_created"
	}
	r.writeAudit(auditEntry{
		RequestID:      stored.ParentTurnID,
		Event:          auditType,
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:      stored.ParentSessionID,
		ToolCallID:     stored.RelatedToolCallID,
		PermissionTool: task.Name,
		AgentTask:      &task,
		Extra: map[string]any{
			"task_message": stored,
		},
	})
	return stored, nil
}

func (r *runtimeService) agentTaskResult(ctx context.Context, taskID string) (RuntimeAgentTaskResult, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	return newRuntimeAgentTaskResultStore(db).Get(ctx, strings.TrimSpace(taskID))
}

func (r *runtimeService) upsertAgentTaskResult(ctx context.Context, result RuntimeAgentTaskResult) (RuntimeAgentTaskResult, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	stored, err := newRuntimeAgentTaskResultStore(db).Upsert(ctx, result)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	task, _ := r.agentTasks.Get(ctx, stored.TaskID)
	r.storeRuntimeEvent(runtimeAgentTaskResultEvent(stored, task))
	if len(stored.ArtifactRefs) > 0 {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:         newRuntimeEventID(),
			Type:       runtimeapi.EventTaskArtifactCreated,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:  task.ParentSessionID,
			TurnID:     task.ParentTurnID,
			ToolCallID: task.ParentToolCallID,
			Payload: map[string]any{
				"task_id":       stored.TaskID,
				"artifact_refs": stored.ArtifactRefs,
				"summary":       fmt.Sprintf("%d artifact refs", len(stored.ArtifactRefs)),
			},
		})
	}
	r.writeAudit(auditEntry{
		RequestID:  firstNonEmpty(task.ParentTurnID, stored.TaskID),
		Event:      "task_result_updated",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  task.ParentSessionID,
		ToolCallID: task.ParentToolCallID,
		AgentTask:  &task,
		Extra: map[string]any{
			"task_result": stored,
		},
	})
	return stored, nil
}

func runtimeAgentTaskFromRecord(record agent.AgentTaskRecord, status string) RuntimeAgentTask {
	return RuntimeAgentTask{
		ID:               record.ID,
		ParentTurnID:     record.ParentTurnID,
		ParentSessionID:  record.ParentSessionID,
		ParentToolCallID: record.ParentToolCallID,
		ChildSessionID:   record.ChildSessionID,
		Title:            record.Title,
		Kind:             record.Kind,
		Role:             record.Role,
		Name:             record.Name,
		PromptSummary:    preview(record.PromptSummary, auditPreviewLimit),
		Model:            record.Model,
		Provider:         record.Provider,
		AllowedTools:     append([]string(nil), record.AllowedTools...),
		CapabilityScope:  append([]string(nil), record.CapabilityScope...),
		CWD:              record.CWD,
		Worktree:         record.Worktree,
		Status:           firstNonEmpty(status, record.Status, agentTaskStatusRunning),
		Progress:         record.Progress,
		ResultSummary:    preview(record.ResultSummary, runtimePartPreviewLimit),
		ArtifactRefs:     append([]string(nil), record.ArtifactRefs...),
		StartedAt:        record.StartedAt,
		FinishedAt:       record.FinishedAt,
		Error:            preview(record.Error, runtimePartPreviewLimit),
	}
}

func (r *runtimeService) recordAgentTaskLifecycle(ctx context.Context, eventType, auditType string, task RuntimeAgentTask) {
	r.storeRuntimeEvent(runtimeAgentTaskEvent(eventType, task))
	r.writeAudit(auditEntry{
		RequestID:        task.ParentTurnID,
		Event:            auditType,
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:        task.ParentSessionID,
		ToolCallID:       task.ParentToolCallID,
		PermissionTool:   task.Name,
		PermissionAction: task.Kind,
		PermissionRisk:   strings.Join(task.CapabilityScope, ","),
		PermissionReason: "AgentTask lifecycle update",
		Provider:         task.Provider,
		Model:            task.Model,
		Error:            task.Error,
		AgentTask:        &task,
	})
	_ = ctx
}

func runtimeAgentTaskEvent(eventType string, task RuntimeAgentTask) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, time.Now().UTC())
	event.SessionID = task.ParentSessionID
	event.TurnID = task.ParentTurnID
	event.ToolCallID = task.ParentToolCallID
	event.Payload = map[string]any{
		"task_id":             task.ID,
		"parent_turn_id":      task.ParentTurnID,
		"parent_session_id":   task.ParentSessionID,
		"parent_tool_call_id": task.ParentToolCallID,
		"child_session_id":    task.ChildSessionID,
		"title":               task.Title,
		"kind":                task.Kind,
		"role":                task.Role,
		"name":                task.Name,
		"provider":            task.Provider,
		"model":               task.Model,
		"status":              task.Status,
		"progress":            task.Progress,
		"summary":             firstNonEmpty(task.ResultSummary, task.PromptSummary, task.Title),
	}
	if task.Error != "" {
		event.Payload["error"] = task.Error
	}
	return event
}

func runtimeAgentTaskMessageEvent(msg RuntimeAgentTaskMessage) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventTaskMessageCreated, time.Now().UTC())
	event.SessionID = msg.ParentSessionID
	event.TurnID = msg.ParentTurnID
	event.ToolCallID = msg.RelatedToolCallID
	event.Payload = map[string]any{
		"message_id":       msg.ID,
		"task_id":          msg.TaskID,
		"direction":        msg.Direction,
		"kind":             msg.Kind,
		"status":           msg.Status,
		"child_session_id": msg.ChildSessionID,
		"summary":          msg.ContentSummary,
		"artifact_refs":    msg.ArtifactRefs,
	}
	return event
}

func runtimeAgentTaskArtifactEvent(msg RuntimeAgentTaskMessage) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventTaskArtifactCreated, time.Now().UTC())
	event.SessionID = msg.ParentSessionID
	event.TurnID = msg.ParentTurnID
	event.ToolCallID = msg.RelatedToolCallID
	event.Payload = map[string]any{
		"message_id":    msg.ID,
		"task_id":       msg.TaskID,
		"artifact_refs": msg.ArtifactRefs,
		"summary":       msg.ContentSummary,
	}
	return event
}

func runtimeAgentTaskResultEvent(result RuntimeAgentTaskResult, task RuntimeAgentTask) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventTaskResultUpdated, time.Now().UTC())
	event.SessionID = task.ParentSessionID
	event.TurnID = task.ParentTurnID
	event.ToolCallID = task.ParentToolCallID
	event.Payload = map[string]any{
		"task_id":               result.TaskID,
		"status":                result.Status,
		"summary":               result.Summary,
		"error_detail":          result.ErrorDetail,
		"cancellation_detail":   result.CancellationDetail,
		"artifact_refs":         result.ArtifactRefs,
		"related_message_refs":  result.RelatedMessageRefs,
		"related_tool_refs":     result.RelatedToolCallRefs,
		"compact_boundary_refs": result.CompactBoundaryRefs,
	}
	return event
}

func (r *runtimeService) recordAgentTaskScope(ctx context.Context, task RuntimeAgentTask, allowed bool, reason string) {
	eventType := runtimeapi.EventTaskScopeApplied
	auditType := "task_scope_applied"
	if !allowed {
		eventType = runtimeapi.EventTaskScopeDenied
		auditType = "task_scope_denied"
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       eventType,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  task.ParentSessionID,
		TurnID:     task.ParentTurnID,
		ToolCallID: task.ParentToolCallID,
		Payload: map[string]any{
			"task_id":          task.ID,
			"role":             task.Role,
			"allowed_tools":    task.AllowedTools,
			"capability_scope": task.CapabilityScope,
			"model":            task.Model,
			"provider":         task.Provider,
			"cwd":              task.CWD,
			"worktree":         task.Worktree,
			"allowed":          allowed,
			"reason":           reason,
			"summary":          reason,
		},
	})
	r.writeAudit(auditEntry{
		RequestID:        task.ParentTurnID,
		Event:            auditType,
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:        task.ParentSessionID,
		ToolCallID:       task.ParentToolCallID,
		PermissionTool:   task.Name,
		PermissionAction: task.Kind,
		PermissionPolicy: map[bool]string{true: "allow", false: "deny"}[allowed],
		PermissionReason: reason,
		AgentTask:        &task,
	})
	_ = ctx
}
