package runtime

import (
	"context"
	"errors"
	"strings"
)

const (
	runtimeRunSchedulerExecuteTaskSourceKind = "run_scheduler_execute_task"
	runtimeRunSchedulerExecuteTaskAction     = "execute_task"

	runtimeRunSchedulerExecuteTaskReasonAcceptedPendingImplementation = "accepted_pending_foreground_execution_implementation"
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
	return RuntimeRunSchedulerExecuteTaskResponse{
		Accepted:         true,
		ExecutionStarted: false,
		Reason:           runtimeRunSchedulerExecuteTaskReasonAcceptedPendingImplementation,
		Plan:             plan,
		Task:             task,
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
