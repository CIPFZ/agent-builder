package runtime

import (
	"context"
	"errors"
	"strings"
)

const (
	runtimeRunSchedulerPlanSourceKind = "run_scheduler_plan"

	runtimeRunSchedulerPlanModeUserTurn         = "user_turn"
	runtimeRunSchedulerPlanModeCheckpointResume = "checkpoint_resume"
	runtimeRunSchedulerPlanModeTaskTurn         = "task_turn"

	runtimeRunSchedulerPlanRefreshRun                   = "run"
	runtimeRunSchedulerPlanRefreshRunProjection         = "runProjection"
	runtimeRunSchedulerPlanRefreshTurnActivity          = "turnActivity"
	runtimeRunSchedulerPlanRefreshSessionActivityWindow = "sessionActivityWindow"
	runtimeRunSchedulerPlanRefreshSessionActivity       = "sessionActivity"
	runtimeRunSchedulerPlanRefreshSchedulerPlan         = "schedulerPlan"

	runtimeRunSchedulerPlanReasonCheckpointRequiresTurn = "checkpoint_requires_explicit_resume_turn"
	runtimeRunSchedulerPlanReasonMissingCheckpoint      = "missing_checkpoint"
	runtimeRunSchedulerPlanReasonMissingTask            = "missing_task"
	runtimeRunSchedulerPlanReasonTaskSchedulerNotReady  = "task_scheduler_not_accepted"
)

func (r *runtimeService) runtimeRunSchedulerPlan(ctx context.Context, req RuntimeRunSchedulerPlanRequest) (RuntimeRunSchedulerPlanResponse, error) {
	if r.runs.db == nil {
		return RuntimeRunSchedulerPlanResponse{}, errors.New("runtime run database is not available")
	}
	run, err := runtimeRunForSchedulerPreflight(ctx, r.runs, req.RunID, req.SessionID)
	if err != nil {
		return RuntimeRunSchedulerPlanResponse{}, err
	}
	plan := RuntimeRunSchedulerPlan{
		RunID:               run.ID,
		PrimarySessionID:    run.PrimarySessionID,
		SessionIDs:          appendUniqueStrings(nil, run.SessionIDs...),
		Objective:           run.Objective,
		StatusFromRunDetail: run.Status,
		CancellationScope:   runtimeRunSchedulerCancellationScope(run.ID),
		DiagnosticsRoute:    runtimeRunSchedulerDiagnosticsRoute(run.ID),
		RefreshTargets:      runtimeRunSchedulerRefreshTargets(),
	}
	item := r.runtimeRunSchedulerPlanItem(ctx, run, req)
	if item.ID != "" {
		plan.Items = append(plan.Items, item)
	}
	return RuntimeRunSchedulerPlanResponse{
		Plan: plan,
		Source: RuntimeRunSchedulerPlanSource{
			Kind:                  runtimeRunSchedulerPlanSourceKind,
			ReadOnly:              true,
			StartsWorker:          false,
			SessionActivityParity: true,
			Evidence:              []string{"runtime_runs", "runtime_run_sessions", "runtime_turns", "runtime_run_checkpoints", "runtime_agent_tasks"},
		},
	}, nil
}

func (r *runtimeService) runtimeRunSchedulerPlanItem(ctx context.Context, run RuntimeRun, req RuntimeRunSchedulerPlanRequest) RuntimeRunSchedulerPlanItem {
	mode := firstNonEmpty(strings.TrimSpace(req.Mode), runtimeRunSchedulerPlanMode(req))
	sessionID := firstNonEmpty(strings.TrimSpace(req.SessionID), run.PrimarySessionID)
	item := RuntimeRunSchedulerPlanItem{
		Kind:              mode,
		SessionID:         sessionID,
		TurnID:            strings.TrimSpace(req.TurnID),
		CheckpointID:      strings.TrimSpace(req.CheckpointID),
		TaskID:            strings.TrimSpace(req.TaskID),
		RequiredPreflight: true,
		RefreshTargets:    runtimeRunSchedulerRefreshTargets(),
		CancellationScope: runtimeRunSchedulerCancellationScope(run.ID),
		DiagnosticsRoute:  runtimeRunSchedulerDiagnosticsRoute(run.ID),
	}
	switch mode {
	case runtimeRunSchedulerPlanModeCheckpointResume:
		return r.runtimeRunSchedulerCheckpointPlanItem(ctx, run, item)
	case runtimeRunSchedulerPlanModeTaskTurn:
		return r.runtimeRunSchedulerTaskPlanItem(ctx, run, item)
	default:
		return r.runtimeRunSchedulerTurnPlanItem(ctx, run, item)
	}
}

func (r *runtimeService) runtimeRunSchedulerTurnPlanItem(ctx context.Context, run RuntimeRun, item RuntimeRunSchedulerPlanItem) RuntimeRunSchedulerPlanItem {
	if strings.TrimSpace(item.TurnID) == "" {
		return RuntimeRunSchedulerPlanItem{}
	}
	if item.ID == "" {
		item.ID = "turn:" + item.TurnID
	}
	if item.OrderKey == "" {
		item.OrderKey = item.TurnID
	}
	preflight, err := r.runtimeRunSchedulerPreflight(ctx, RuntimeRunSchedulerPreflightRequest{
		RunID:     run.ID,
		SessionID: item.SessionID,
		TurnID:    item.TurnID,
	})
	if err != nil {
		item.PreflightReason = err.Error()
		return item
	}
	item.CanSchedule = preflight.CanSchedule
	item.PreflightReason = preflight.Reason
	return item
}

func (r *runtimeService) runtimeRunSchedulerCheckpointPlanItem(ctx context.Context, run RuntimeRun, item RuntimeRunSchedulerPlanItem) RuntimeRunSchedulerPlanItem {
	if strings.TrimSpace(item.CheckpointID) == "" {
		return RuntimeRunSchedulerPlanItem{}
	}
	item.ID = "checkpoint:" + item.CheckpointID
	item.OrderKey = item.CheckpointID
	if _, ok := runtimeRunCheckpointByID(run.Checkpoints, item.CheckpointID); !ok {
		item.PreflightReason = runtimeRunSchedulerPlanReasonMissingCheckpoint
		return item
	}
	if strings.TrimSpace(item.TurnID) == "" {
		item.PreflightReason = runtimeRunSchedulerPlanReasonCheckpointRequiresTurn
		return item
	}
	return r.runtimeRunSchedulerTurnPlanItem(ctx, run, item)
}

func (r *runtimeService) runtimeRunSchedulerTaskPlanItem(ctx context.Context, run RuntimeRun, item RuntimeRunSchedulerPlanItem) RuntimeRunSchedulerPlanItem {
	if strings.TrimSpace(item.TaskID) == "" {
		return RuntimeRunSchedulerPlanItem{}
	}
	item.ID = "task:" + item.TaskID
	item.OrderKey = item.TaskID
	if r.agentTasks.db == nil {
		item.PreflightReason = "runtime agent task database is not available"
		return item
	}
	task, err := r.agentTasks.Get(ctx, item.TaskID)
	if errors.Is(err, errRuntimeAgentTaskNotFound) {
		item.PreflightReason = runtimeRunSchedulerPlanReasonMissingTask
		return item
	}
	if err != nil {
		item.PreflightReason = err.Error()
		return item
	}
	item.SessionID = task.ParentSessionID
	item.TurnID = task.ParentTurnID
	item.TaskScope = runtimeRunSchedulerTaskScope(task)
	preflight, err := r.runtimeRunSchedulerPreflight(ctx, RuntimeRunSchedulerPreflightRequest{
		RunID:     run.ID,
		SessionID: task.ParentSessionID,
		TurnID:    task.ParentTurnID,
	})
	if err != nil {
		item.PreflightReason = err.Error()
		return item
	}
	item.OwnershipVerified = preflight.CanSchedule
	if !preflight.CanSchedule {
		item.PreflightReason = preflight.Reason
		return item
	}
	item.PreflightReason = runtimeRunSchedulerPlanReasonTaskSchedulerNotReady
	return item
}

func runtimeRunSchedulerPlanMode(req RuntimeRunSchedulerPlanRequest) string {
	if strings.TrimSpace(req.CheckpointID) != "" {
		return runtimeRunSchedulerPlanModeCheckpointResume
	}
	if strings.TrimSpace(req.TaskID) != "" {
		return runtimeRunSchedulerPlanModeTaskTurn
	}
	return runtimeRunSchedulerPlanModeUserTurn
}

func runtimeRunSchedulerRefreshTargets() []string {
	return []string{
		runtimeRunSchedulerPlanRefreshRun,
		runtimeRunSchedulerPlanRefreshRunProjection,
		runtimeRunSchedulerPlanRefreshTurnActivity,
		runtimeRunSchedulerPlanRefreshSessionActivityWindow,
		runtimeRunSchedulerPlanRefreshSessionActivity,
		runtimeRunSchedulerPlanRefreshSchedulerPlan,
	}
}

func runtimeRunSchedulerCancellationScope(runID string) string {
	return "run:" + strings.TrimSpace(runID)
}

func runtimeRunSchedulerDiagnosticsRoute(runID string) string {
	return "run:" + strings.TrimSpace(runID) + ":diagnostics"
}

func runtimeRunSchedulerTaskScope(task RuntimeAgentTask) RuntimeRunSchedulerTaskScope {
	return RuntimeRunSchedulerTaskScope{
		AllowedTools:     appendUniqueStrings(nil, task.AllowedTools...),
		CapabilityScope:  appendUniqueStrings(nil, task.CapabilityScope...),
		CWD:              task.CWD,
		Worktree:         task.Worktree,
		Role:             task.Role,
		Provider:         task.Provider,
		Model:            task.Model,
		ParentToolCallID: task.ParentToolCallID,
		ChildSessionID:   task.ChildSessionID,
	}
}
