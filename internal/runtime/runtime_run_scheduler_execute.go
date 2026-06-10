package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const (
	runtimeRunSchedulerExecuteTaskSourceKind = "run_scheduler_execute_task"
	runtimeRunSchedulerExecuteTaskAction     = "execute_task"

	runtimeRunSchedulerExecuteTaskReasonAlreadyRunning              = "task_already_running"
	runtimeRunSchedulerExecuteTaskReasonForegroundExecutionStarted  = "foreground_execution_started"
	runtimeRunSchedulerExecuteTaskReasonUnsupportedForegroundStatus = "unsupported_foreground_task_status"
)

func (r *runtimeService) runtimeRunSchedulerExecuteTask(ctx context.Context, req RuntimeRunSchedulerExecuteTaskRequest) (RuntimeRunSchedulerExecuteTaskResponse, error) {
	runID := strings.TrimSpace(req.RunID)
	taskID := strings.TrimSpace(req.TaskID)
	if runID == "" {
		return RuntimeRunSchedulerExecuteTaskResponse{}, errors.New("run id is required")
	}
	if taskID == "" {
		return RuntimeRunSchedulerExecuteTaskResponse{}, errors.New("task id is required")
	}
	if r.runs.db == nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, errors.New("runtime run database is not available")
	}
	if r.agentTasks.db == nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, errors.New("runtime agent task database is not available")
	}
	run, err := r.runs.Get(ctx, runID)
	if err != nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, err
	}
	plan, err := r.runtimeRunSchedulerDelegateTaskTurn(ctx, run, taskID)
	if err != nil {
		return RuntimeRunSchedulerExecuteTaskResponse{
			Accepted:       false,
			Reason:         err.Error(),
			Plan:           plan,
			RefreshTargets: runtimeRunSchedulerRefreshTargets(),
			Source:         runtimeRunSchedulerExecuteTaskSource(),
		}, err
	}
	task, err := r.agentTasks.Get(ctx, taskID)
	if err != nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, err
	}
	if task.Status == agentTaskStatusRunning {
		return RuntimeRunSchedulerExecuteTaskResponse{
			Accepted:         true,
			ExecutionStarted: false,
			Reason:           runtimeRunSchedulerExecuteTaskReasonAlreadyRunning,
			Plan:             plan,
			Task:             task,
			RefreshTargets:   runtimeRunSchedulerRefreshTargets(),
			Source:           runtimeRunSchedulerExecuteTaskSource(),
		}, nil
	}
	if task.Status != agentTaskStatusQueued {
		return RuntimeRunSchedulerExecuteTaskResponse{
			Accepted:       false,
			Reason:         runtimeRunSchedulerExecuteTaskReasonUnsupportedForegroundStatus + ":" + task.Status,
			Plan:           plan,
			Task:           task,
			RefreshTargets: runtimeRunSchedulerRefreshTargets(),
			Source:         runtimeRunSchedulerExecuteTaskSource(),
		}, errors.New(runtimeRunSchedulerExecuteTaskReasonUnsupportedForegroundStatus + ": " + task.Status)
	}
	task.Status = agentTaskStatusRunning
	if task.Progress == 0 {
		task.Progress = 10
	}
	started, err := r.agentTasks.Upsert(ctx, task)
	if err != nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, err
	}
	_, err = r.createAgentTaskMessage(ctx, started, RuntimeAgentTaskMessage{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindInstruction,
		Status:            taskMessageStatusProcessed,
		ContentSummary:    firstNonEmpty(started.PromptSummary, started.Title),
		RelatedToolCallID: started.ParentToolCallID,
		Payload: map[string]any{
			"action": "execute_task",
			"run_id": run.ID,
		},
	})
	if err != nil {
		return RuntimeRunSchedulerExecuteTaskResponse{}, err
	}
	r.recordAgentTaskLifecycle(ctx, runtimeapi.EventTaskStarted, "task_started", started)
	r.recordRunTaskTransition(ctx, runtimeRunTransitionSourceTaskStarted, started, "", runtimeRunStatusActive, "foreground task execution started")
	return RuntimeRunSchedulerExecuteTaskResponse{
		Accepted:         true,
		ExecutionStarted: true,
		Reason:           runtimeRunSchedulerExecuteTaskReasonForegroundExecutionStarted,
		Plan:             plan,
		Task:             started,
		RefreshTargets:   runtimeRunSchedulerRefreshTargets(),
		Source:           runtimeRunSchedulerExecuteTaskSource(),
	}, nil
}

func runtimeRunSchedulerExecuteTaskSource() RuntimeRunSchedulerExecuteTaskSource {
	return RuntimeRunSchedulerExecuteTaskSource{
		Kind:                  runtimeRunSchedulerExecuteTaskSourceKind,
		Action:                runtimeRunSchedulerExecuteTaskAction,
		BackendOnly:           true,
		StartsWorker:          false,
		IdempotentByTaskID:    true,
		SessionActivityParity: true,
		Evidence:              []string{"runtime_runs", "runtime_run_sessions", "runtime_turns", "runtime_agent_tasks", "runtime_run_scheduler_plan"},
	}
}
