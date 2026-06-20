package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const (
	runtimeAgentTaskCancelSourceKind = "agent_task"
	runtimeAgentTaskCancelAction     = "cancel_task"

	runtimeAgentTaskCancelReasonAlreadyFinal = "agent_task_already_final"
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

func (r *runtimeService) SessionAgentTasks(ctx context.Context, sessionID string) (RuntimeAgentTasksResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeAgentTasksResponse{}, errors.New("session id is required")
	}
	tasks, err := r.agentTasks.ListBySession(ctx, sessionID)
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
		_, _ = r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
			Direction:      taskMessageDirectionParentToChild,
			Kind:           taskMessageKindControl,
			Status:         taskMessageStatusRejected,
			ContentSummary: "cancel rejected",
			Error:          "agent task is already final",
			Payload: map[string]any{
				"action": "cancel",
				"reason": "agent task is already final",
			},
		})
		return withRuntimeAgentTaskCancelAction(RuntimeAgentTaskResponse{Task: task}, false, runtimeAgentTaskCancelReasonAlreadyFinal), nil
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
	cancellationDetail := "Child session cancellation was requested through the runtime when available."
	task.CancellationDetail = cancellationDetail
	task, err = r.agentTasks.Upsert(ctx, task)
	if err != nil {
		return RuntimeAgentTaskResponse{}, err
	}
	task.CancellationDetail = cancellationDetail
	result, _ := r.upsertAgentTaskResult(ctx, RuntimeAgentTaskResult{
		TaskID:             task.ID,
		Status:             agentTaskStatusCancelled,
		Summary:            task.ResultSummary,
		ErrorDetail:        task.Error,
		CancellationDetail: cancellationDetail,
		ArtifactRefs:       task.ArtifactRefs,
	})
	_, _ = r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindControl,
		Status:            taskMessageStatusProcessed,
		ContentSummary:    "cancel requested",
		RelatedToolCallID: task.ParentToolCallID,
		Payload: map[string]any{
			"action": "cancel",
			"reason": cancellationDetail,
		},
	})
	r.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskCancelled, "task_cancelled", task)
	r.reconcileRuntimeRunForTerminalTask(ctx, task)
	return withRuntimeAgentTaskCancelAction(RuntimeAgentTaskResponse{Task: task, Result: &result}, true, cancellationDetail), nil
}

func withRuntimeAgentTaskCancelAction(resp RuntimeAgentTaskResponse, accepted bool, reason string) RuntimeAgentTaskResponse {
	resp.Action = &RuntimeWriteActionMetadata{
		Accepted:       accepted,
		Reason:         reason,
		RefreshTargets: runtimeRunSchedulerRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeAgentTaskCancelSourceKind,
			Action:                runtimeAgentTaskCancelAction,
			BackendOnly:           true,
			StartsWorker:          false,
			IdempotentBy:          "task_id",
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_agent_tasks",
				"runtime_agent_task_results",
				"runtime_agent_task_messages",
				"runtime_events",
				"runtime_run_scheduler_plan",
				"session_activity",
			},
		},
	}
	return resp
}

func (r *runtimeSchedulerRecorder) AgentTaskStarted(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	record := runtimeAgentTaskFromRecord(task, agentTaskStatusRunning)
	record.ArtifactRefs = r.service.ensureTaskArtifactRefs(ctx, record, record.ArtifactRefs)
	stored, err := r.service.agentTasks.Upsert(ctx, record)
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
	record.ArtifactRefs = r.service.ensureTaskArtifactRefs(ctx, record, record.ArtifactRefs)
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
	r.service.reconcileRuntimeRunForTerminalTask(ctx, stored)
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
	record.ArtifactRefs = nil
	record.Progress = 100
	if record.FinishedAt == 0 {
		record.FinishedAt = time.Now().UnixMilli()
	}
	stored, err := r.service.agentTasks.Upsert(ctx, record)
	if err != nil {
		return err
	}
	resultStatus := stored.Status
	errorDetail := stored.Error
	cancellationDetail := stored.CancellationDetail
	if stored.Status == agentTaskStatusCancelled {
		cancellationDetail = firstNonEmpty(cancellationDetail, stored.Error)
		errorDetail = ""
	}
	_, _ = r.service.upsertAgentTaskResult(ctx, RuntimeAgentTaskResult{
		TaskID:             stored.ID,
		Status:             resultStatus,
		Summary:            stored.ResultSummary,
		ErrorDetail:        errorDetail,
		CancellationDetail: cancellationDetail,
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
	r.service.reconcileRuntimeRunForTerminalTask(ctx, stored)
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
	if isFinalAgentTaskStatus(task.Status) && normalizeTaskMessageDirection(req.Direction) == taskMessageDirectionParentToChild {
		msg, createErr := r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
			Direction:         req.Direction,
			Kind:              req.Kind,
			Status:            taskMessageStatusRejected,
			ContentSummary:    firstNonEmpty(req.ContentSummary, "message rejected"),
			Payload:           req.Payload,
			RelatedToolCallID: req.RelatedToolCallID,
			RelatedMessageID:  req.RelatedMessageID,
			ArtifactRefs:      req.ArtifactRefs,
			Error:             "agent task is already final",
		})
		if createErr != nil {
			return RuntimeAgentTaskMessageResponse{}, createErr
		}
		return RuntimeAgentTaskMessageResponse{Message: msg}, nil
	}
	msg, err := r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
		Direction:         req.Direction,
		Kind:              req.Kind,
		Status:            req.Status,
		ContentSummary:    req.ContentSummary,
		Payload:           req.Payload,
		RelatedToolCallID: req.RelatedToolCallID,
		RelatedMessageID:  req.RelatedMessageID,
		ArtifactRefs:      req.ArtifactRefs,
		Error:             req.Error,
	})
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	return RuntimeAgentTaskMessageResponse{Message: msg}, nil
}

func (r *runtimeService) SendAgentTaskFollowUp(ctx context.Context, taskID string, req RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return RuntimeAgentTaskMessageResponse{}, errors.New("task id is required")
	}
	content := strings.TrimSpace(req.ContentSummary)
	if content == "" {
		return RuntimeAgentTaskMessageResponse{}, errors.New("message content is required")
	}
	task, err := r.agentTasks.Get(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, fmt.Errorf("agent task %s was not found: %w", taskID, err)
	}
	if isFinalAgentTaskStatus(task.Status) {
		msg, createErr := r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
			Direction:         taskMessageDirectionParentToChild,
			Kind:              taskMessageKindInstruction,
			Status:            taskMessageStatusRejected,
			ContentSummary:    content,
			RelatedToolCallID: req.RelatedToolCallID,
			RelatedMessageID:  req.RelatedMessageID,
			Error:             "agent task is already final",
			Payload: map[string]any{
				"reason": "agent task is already final",
			},
		})
		if createErr != nil {
			return RuntimeAgentTaskMessageResponse{}, createErr
		}
		return RuntimeAgentTaskMessageResponse{Message: msg}, nil
	}
	msg, err := r.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindInstruction,
		Status:            taskMessageStatusCreated,
		ContentSummary:    content,
		RelatedToolCallID: req.RelatedToolCallID,
		RelatedMessageID:  req.RelatedMessageID,
		Payload:           req.Payload,
	})
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	r.mu.Lock()
	workspaceID := ""
	if r.workspace != nil {
		workspaceID = r.workspace.ID
	}
	runtimeBackend := r.runtime
	r.mu.Unlock()
	if runtimeBackend == nil || workspaceID == "" || task.ChildSessionID == "" {
		stored, _ := newRuntimeAgentTaskMessageStore(r.turns.db).UpdateStatus(ctx, msg.ID, taskMessageStatusRejected, "agent task child session is not deliverable")
		r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageRejected, stored))
		r.writeAgentTaskMessageStatusAudit("task_message_rejected", task, stored)
		return RuntimeAgentTaskMessageResponse{Message: stored}, nil
	}
	delivered, err := newRuntimeAgentTaskMessageStore(r.turns.db).UpdateStatus(ctx, msg.ID, taskMessageStatusDelivered, "")
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageDelivered, delivered))
	r.writeAgentTaskMessageStatusAudit("task_message_delivered", task, delivered)
	if err := runtimeBackend.SendSessionMessage(ctx, workspaceID, proto.AgentMessage{
		SessionID: task.ChildSessionID,
		TurnID:    task.ParentTurnID,
		Prompt:    content,
	}); err != nil {
		rejected, _ := newRuntimeAgentTaskMessageStore(r.turns.db).UpdateStatus(ctx, msg.ID, taskMessageStatusRejected, err.Error())
		r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageRejected, rejected))
		r.writeAgentTaskMessageStatusAudit("task_message_rejected", task, rejected)
		return RuntimeAgentTaskMessageResponse{Message: rejected}, nil
	}
	processed, err := newRuntimeAgentTaskMessageStore(r.turns.db).UpdateStatus(ctx, msg.ID, taskMessageStatusProcessed, "")
	if err != nil {
		return RuntimeAgentTaskMessageResponse{}, err
	}
	r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageProcessed, processed))
	r.writeAgentTaskMessageStatusAudit("task_message_processed", task, processed)
	return RuntimeAgentTaskMessageResponse{Message: processed}, nil
}

func (r *runtimeService) AgentTaskResult(ctx context.Context, taskID string) (RuntimeAgentTaskResultResponse, error) {
	result, err := r.agentTaskResult(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskResultResponse{}, err
	}
	return RuntimeAgentTaskResultResponse{Result: result}, nil
}

func (r *runtimeService) AgentTaskOutput(ctx context.Context, taskID string) (RuntimeAgentTaskOutputResponse, error) {
	resp, err := r.AgentTask(ctx, taskID)
	if err != nil {
		return RuntimeAgentTaskOutputResponse{}, err
	}
	out := RuntimeAgentTaskOutputResponse{
		TaskID:       resp.Task.ID,
		Status:       resp.Task.Status,
		Summary:      preview(firstNonEmpty(resp.Task.ResultSummary, resp.Task.PromptSummary), runtimePartPreviewLimit),
		Error:        preview(resp.Task.Error, runtimePartPreviewLimit),
		ArtifactRefs: append([]string(nil), resp.Task.ArtifactRefs...),
		UpdatedAt:    resp.Task.UpdatedAt,
	}
	if resp.Result != nil {
		out.Status = firstNonEmpty(resp.Result.Status, out.Status)
		out.Summary = preview(firstNonEmpty(resp.Result.Summary, out.Summary), runtimePartPreviewLimit)
		out.Error = preview(firstNonEmpty(resp.Result.ErrorDetail, out.Error), runtimePartPreviewLimit)
		out.CancellationDetail = preview(resp.Result.CancellationDetail, runtimePartPreviewLimit)
		out.ArtifactRefs = appendUniqueStrings(out.ArtifactRefs, resp.Result.ArtifactRefs...)
		out.RelatedMessageRefs = append([]string(nil), resp.Result.RelatedMessageRefs...)
		out.RelatedToolCallRefs = append([]string(nil), resp.Result.RelatedToolCallRefs...)
		out.CompactBoundaryRefs = append([]string(nil), resp.Result.CompactBoundaryRefs...)
		out.UpdatedAt = resp.Result.UpdatedAt
	}
	for _, msg := range resp.Messages {
		if msg.Kind == taskMessageKindResult || msg.Kind == taskMessageKindArtifact {
			out.Messages = append(out.Messages, msg)
			out.ArtifactRefs = appendUniqueStrings(out.ArtifactRefs, msg.ArtifactRefs...)
		}
	}
	if refs, err := r.Refs(ctx, RuntimeRefListRequest{TaskID: resp.Task.ID}); err == nil {
		for _, ref := range refs.Refs {
			out.OutputRefs = appendUniqueStrings(out.OutputRefs, ref.URI)
		}
	}
	return out, nil
}

func (r *runtimeService) agentTaskMessages(ctx context.Context, taskID string) ([]RuntimeAgentTaskMessage, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return nil, err
	}
	return newRuntimeAgentTaskMessageStore(db).ListByTask(ctx, strings.TrimSpace(taskID))
}

func (r *runtimeService) createAgentTaskMessage(ctx context.Context, task RuntimeAgentTask, msg RuntimeAgentTaskMessage) (RuntimeAgentTaskMessage, error) {
	db := r.turns.db
	if db == nil {
		var err error
		db, err = r.workspaceDB(ctx)
		if err != nil {
			return RuntimeAgentTaskMessage{}, err
		}
	}
	msg.TaskID = task.ID
	msg.ParentTurnID = firstNonEmpty(msg.ParentTurnID, task.ParentTurnID)
	msg.ParentSessionID = firstNonEmpty(msg.ParentSessionID, task.ParentSessionID)
	msg.ChildSessionID = firstNonEmpty(msg.ChildSessionID, task.ChildSessionID)
	msg.ContentSummary = preview(msg.ContentSummary, runtimePartPreviewLimit)
	msg.ArtifactRefs = r.ensureTaskArtifactRefs(ctx, task, msg.ArtifactRefs)
	msg.Payload = redactRuntimePayload(msg.Payload)
	stored, err := newRuntimeAgentTaskMessageStore(db).Create(ctx, msg)
	if err != nil {
		return RuntimeAgentTaskMessage{}, err
	}
	r.storeRuntimeEvent(runtimeAgentTaskMessageEvent(stored))
	switch stored.Status {
	case taskMessageStatusDelivered:
		r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageDelivered, stored))
	case taskMessageStatusProcessed:
		r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageProcessed, stored))
	case taskMessageStatusRejected:
		r.storeRuntimeEvent(runtimeAgentTaskMessageStatusEvent(runtimeapi.EventTaskMessageRejected, stored))
	}
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
			"task_message":   stored,
			"message_status": stored.Status,
		},
	})
	return stored, nil
}

func (r *runtimeService) ensureTaskArtifactRefs(ctx context.Context, task RuntimeAgentTask, refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, value := range refs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "runtime://refs/") {
			out = appendUniqueStrings(out, value)
			continue
		}
		ref, err := r.createRuntimeRef(ctx, runtimeRefCreateRequest{
			SessionID:   task.ParentSessionID,
			TurnID:      task.ParentTurnID,
			ToolCallID:  task.ParentToolCallID,
			TaskID:      task.ID,
			Kind:        runtimeRefKindTaskArtifact,
			MediaType:   "text/plain",
			ContentType: "task_artifact_ref",
			Payload:     []byte(value),
			Summary:     value,
		})
		if err != nil {
			out = appendUniqueStrings(out, value)
			continue
		}
		out = appendUniqueStrings(out, ref.URI)
	}
	return out
}

func (r *runtimeService) writeAgentTaskMessageStatusAudit(event string, task RuntimeAgentTask, msg RuntimeAgentTaskMessage) {
	r.writeAudit(auditEntry{
		RequestID:  msg.ParentTurnID,
		Event:      event,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  msg.ParentSessionID,
		ToolCallID: msg.RelatedToolCallID,
		AgentTask:  &task,
		Error:      msg.Error,
		Extra: map[string]any{
			"task_message": msg,
		},
	})
}

func (r *runtimeService) agentTaskResult(ctx context.Context, taskID string) (RuntimeAgentTaskResult, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
	return newRuntimeAgentTaskResultStore(db).Get(ctx, strings.TrimSpace(taskID))
}

func (r *runtimeService) upsertAgentTaskResult(ctx context.Context, result RuntimeAgentTaskResult) (RuntimeAgentTaskResult, error) {
	db := r.turns.db
	if db == nil {
		var err error
		db, err = r.workspaceDB(ctx)
		if err != nil {
			return RuntimeAgentTaskResult{}, err
		}
	}
	task, _ := r.agentTasks.Get(ctx, result.TaskID)
	result.ArtifactRefs = r.ensureTaskArtifactRefs(ctx, task, result.ArtifactRefs)
	stored, err := newRuntimeAgentTaskResultStore(db).Upsert(ctx, result)
	if err != nil {
		return RuntimeAgentTaskResult{}, err
	}
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
				"task_id":            stored.TaskID,
				"artifact_refs":      stored.ArtifactRefs,
				"artifact_ref_count": len(stored.ArtifactRefs),
				"summary":            fmt.Sprintf("%d artifact refs", len(stored.ArtifactRefs)),
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
		"message_id":         msg.ID,
		"task_id":            msg.TaskID,
		"parent_task_id":     msg.ParentTaskID,
		"direction":          msg.Direction,
		"kind":               msg.Kind,
		"status":             msg.Status,
		"sequence":           msg.Sequence,
		"child_session_id":   msg.ChildSessionID,
		"summary":            msg.ContentSummary,
		"artifact_refs":      msg.ArtifactRefs,
		"artifact_ref_count": len(msg.ArtifactRefs),
	}
	if msg.Error != "" {
		event.Payload["error"] = msg.Error
	}
	return event
}

func runtimeAgentTaskMessageStatusEvent(eventType string, msg RuntimeAgentTaskMessage) RuntimeEvent {
	event := runtimeAgentTaskMessageEvent(msg)
	event.ID = newRuntimeEventID()
	event.Type = eventType
	event.Payload["status"] = msg.Status
	event.Payload["delivered_at"] = msg.DeliveredAt
	event.Payload["processed_at"] = msg.ProcessedAt
	return event
}

func runtimeAgentTaskArtifactEvent(msg RuntimeAgentTaskMessage) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventTaskArtifactCreated, time.Now().UTC())
	event.SessionID = msg.ParentSessionID
	event.TurnID = msg.ParentTurnID
	event.ToolCallID = msg.RelatedToolCallID
	event.Payload = map[string]any{
		"message_id":         msg.ID,
		"task_id":            msg.TaskID,
		"artifact_refs":      msg.ArtifactRefs,
		"artifact_ref_count": len(msg.ArtifactRefs),
		"summary":            msg.ContentSummary,
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
