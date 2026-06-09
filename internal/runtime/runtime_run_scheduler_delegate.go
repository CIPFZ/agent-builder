package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const runtimeRunSchedulerDelegateRejectPrefix = "scheduler preflight rejected turn"

func (r *runtimeService) runtimeRunSchedulerDelegateUserTurn(ctx context.Context, run RuntimeRun, turn RuntimeTurn) (RuntimeRunSchedulerPlanResponse, error) {
	if strings.TrimSpace(run.ID) == "" {
		return RuntimeRunSchedulerPlanResponse{}, errors.New("run id is required")
	}
	if strings.TrimSpace(turn.ID) == "" {
		return RuntimeRunSchedulerPlanResponse{}, errors.New("turn id is required")
	}
	plan, err := r.runtimeRunSchedulerPlan(ctx, RuntimeRunSchedulerPlanRequest{
		RunID:     run.ID,
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Mode:      runtimeRunSchedulerPlanModeUserTurn,
	})
	if err != nil {
		return plan, err
	}
	if len(plan.Plan.Items) == 0 {
		return plan, fmt.Errorf("%s: missing plan item", runtimeRunSchedulerDelegateRejectPrefix)
	}
	item := plan.Plan.Items[0]
	if !item.CanSchedule {
		reason := firstNonEmpty(item.PreflightReason, "not_schedulable")
		return plan, fmt.Errorf("%s: %s", runtimeRunSchedulerDelegateRejectPrefix, reason)
	}
	return plan, nil
}

func (r *runtimeService) failRuntimeRunScheduledTurn(ctx context.Context, turn RuntimeTurn, reason string) (RuntimeTurn, error) {
	now := time.Now().UTC()
	turn.Status = turnStatusFailed
	turn.Error = firstNonEmpty(strings.TrimSpace(reason), runtimeRunSchedulerDelegateRejectPrefix)
	if turn.FinishedAt == 0 {
		turn.FinishedAt = now.UnixMilli()
	}
	failed, err := r.turns.Upsert(ctx, turn)
	if err != nil {
		return RuntimeTurn{}, err
	}
	r.mu.Lock()
	state := r.requests[failed.ID]
	state.SessionID = firstNonEmpty(state.SessionID, failed.SessionID)
	state.Status = "failed"
	state.Finished = true
	state.FinishedAt = failed.FinishedAt
	state.Error = failed.Error
	r.requests[failed.ID] = state
	r.mu.Unlock()
	if _, err := r.reconcileRuntimeRunForSession(ctx, failed.SessionID); err != nil {
		slog.Warn("Failed to reconcile runtime run after scheduler preflight rejection", "session_id", failed.SessionID, "turn_id", failed.ID, "error", err)
	}
	return failed, nil
}
