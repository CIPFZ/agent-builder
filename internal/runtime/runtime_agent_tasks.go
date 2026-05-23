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
	return RuntimeAgentTaskResponse{Task: task}, nil
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
	r.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskCancelled, "task_cancelled", task)
	return RuntimeAgentTaskResponse{Task: task}, nil
}

func (r *runtimeSchedulerRecorder) AgentTaskStarted(ctx context.Context, task agent.AgentTaskRecord) error {
	if r == nil || r.service == nil {
		return nil
	}
	stored, err := r.service.agentTasks.Upsert(ctx, runtimeAgentTaskFromRecord(task, agentTaskStatusRunning))
	if err != nil {
		return err
	}
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
	eventType := runtimeapi.EventTaskFailed
	auditType := "task_failed"
	if stored.Status == agentTaskStatusCancelled {
		eventType = runtimeapi.EventTaskCancelled
		auditType = "task_cancelled"
	}
	r.service.recordAgentTaskLifecycle(ctx, eventType, auditType, stored)
	return nil
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
