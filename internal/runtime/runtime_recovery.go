package runtime

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func (r *runtimeService) RecoveryStatus(ctx context.Context) (RuntimeRecoveryStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRecoveryStatus{}, err
	}
	expired, err := r.reconcilePendingPermissions(ctx)
	if err != nil {
		return RuntimeRecoveryStatus{}, err
	}
	if len(expired) > 0 {
		r.mu.Lock()
		r.recovery.expiredPermissions = append(r.recovery.expiredPermissions, expired...)
		r.mu.Unlock()
		for _, perm := range expired {
			r.storeRuntimeEvent(RuntimeEvent{
				Type:       runtimeapi.EventPermissionDecided,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
				SessionID:  perm.SessionID,
				TurnID:     perm.TurnID,
				ToolCallID: perm.ToolCallID,
				Payload: map[string]any{
					"permission_id": perm.ID,
					"tool_name":     perm.ToolName,
					"action":        perm.Action,
					"path":          perm.Path,
					"risk":          perm.Risk,
					"status":        perm.Status,
				},
			})
		}
	}
	activeTurns, err := r.turns.List(ctx, "active")
	if err != nil {
		return RuntimeRecoveryStatus{}, err
	}
	pendingPermissions, err := r.permissionStore.List(ctx, permissionStatusPending)
	if err != nil {
		return RuntimeRecoveryStatus{}, err
	}
	var pendingMCPRequests []RuntimeMCPRequest
	if r.mcpRequestStore.db != nil {
		pendingMCPRequests, err = r.mcpRequestStore.List(ctx, RuntimeMCPRequestListRequest{Status: mcpRequestStatusPending})
		if err != nil {
			return RuntimeRecoveryStatus{}, err
		}
	}
	r.mu.Lock()
	startedAt := r.recovery.startedAt
	interruptedTurns := append([]RuntimeTurn(nil), r.recovery.interruptedTurns...)
	interruptedTasks := append([]RuntimeAgentTask(nil), r.recovery.interruptedTasks...)
	recoveredWorktrees := append([]RuntimeWorktree(nil), r.recovery.worktrees...)
	interruptedHooks := append([]RuntimeHookExecution(nil), r.recovery.interruptedHooks...)
	lastSequence := r.nextEventSequence
	snapshotRequired := len(r.events) > 0 && r.events[0].Sequence > 1
	r.mu.Unlock()
	if r.agentTasks.db != nil && len(interruptedTasks) == 0 {
		if tasks, err := r.agentTasks.ListByStatus(ctx, agentTaskStatusInterrupted); err == nil {
			interruptedTasks = tasks
		}
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	compact := r.recoveryCompactBoundaries(ctx, activeTurns, interruptedTurns)
	return RuntimeRecoveryStatus{
		RuntimeStartedAt:   startedAt.UTC().Format(time.RFC3339Nano),
		LastEventSequence:  lastSequence,
		ActiveTurns:        activeTurns,
		InterruptedTurns:   interruptedTurns,
		InterruptedTasks:   interruptedTasks,
		CompactBoundaries:  compact,
		Worktrees:          recoveredWorktrees,
		HookExecutions:     interruptedHooks,
		PendingPermissions: pendingPermissions,
		PendingMCPRequests: pendingMCPRequests,
		SnapshotRequired:   snapshotRequired,
	}, nil
}

func (r *runtimeService) recoveryCompactBoundaries(ctx context.Context, activeTurns, interruptedTurns []RuntimeTurn) []RuntimeCompactBoundary {
	if r.compactBoundaries.db == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []RuntimeCompactBoundary
	for _, turn := range append(append([]RuntimeTurn{}, activeTurns...), interruptedTurns...) {
		if turn.ID == "" {
			continue
		}
		if _, ok := seen[turn.ID]; ok {
			continue
		}
		seen[turn.ID] = struct{}{}
		boundaries, err := r.compactBoundaries.ListByTurn(ctx, turn.ID)
		if err != nil {
			continue
		}
		out = append(out, boundaries...)
	}
	return out
}

func (r *runtimeService) recoverWorktrees(ctx context.Context) ([]RuntimeWorktree, error) {
	if r.worktrees.db == nil {
		return nil, nil
	}
	active, err := r.worktrees.ListRecoverable(ctx)
	if err != nil {
		return nil, err
	}
	recovered := make([]RuntimeWorktree, 0, len(active))
	for _, wt := range active {
		wt = r.recoverWorktreeRecord(ctx, wt)
		stored, err := r.worktrees.Upsert(ctx, wt)
		if err != nil {
			return nil, err
		}
		if stored.TaskID != "" && stored.Status != worktreeStatusEntered {
			_ = r.clearWorktreeFromTask(ctx, stored)
		}
		recovered = append(recovered, stored)
	}
	return recovered, nil
}

func (r *runtimeService) recoverWorktreeRecord(ctx context.Context, wt RuntimeWorktree) RuntimeWorktree {
	wt.PreservePolicy = normalizeWorktreePreservePolicy(wt.PreservePolicy)
	wt.CleanupPolicy = normalizeWorktreeCleanupPolicy(wt.CleanupPolicy)
	if wt.Owner == "" {
		wt.Owner = "runtime"
	}
	if _, err := os.Stat(wt.WorktreePath); errors.Is(err, os.ErrNotExist) {
		wt.Status = worktreeStatusMissing
		wt.Error = "worktree path is missing during recovery"
		return wt
	} else if err != nil {
		wt.Status = worktreeStatusError
		wt.Error = "failed to inspect worktree path during recovery: " + err.Error()
		return wt
	}
	if err := validateRecoverableWorktree(ctx, wt); err != nil {
		wt.Status = worktreeStatusError
		wt.Error = "worktree recovery safety check failed: " + err.Error()
		return wt
	}
	switch wt.Status {
	case worktreeStatusPreserved:
		wt.Error = firstNonEmpty(wt.Error, "runtime restarted; worktree remains preserved")
		return wt
	case worktreeStatusCleanupPending, worktreeStatusCleaning, worktreeStatusCleanupFailed:
		return r.recoverCleanupPendingWorktree(ctx, wt)
	case worktreeStatusError:
		if shouldPreserveWorktree(wt, "runtime restarted") {
			wt.Status = worktreeStatusPreserved
			wt.Error = firstNonEmpty(wt.Error, "runtime restarted; error worktree preserved")
			return wt
		}
		wt.Status = worktreeStatusCleanupPending
		wt.Error = firstNonEmpty(wt.Error, "runtime restarted; error worktree cleanup pending")
		if wt.CleanupPolicy == worktreeCleanupExit {
			return r.recoverCleanupPendingWorktree(ctx, wt)
		}
		return wt
	default:
		if shouldPreserveWorktree(wt, "runtime restarted") {
			wt.Status = worktreeStatusPreserved
			wt.Error = firstNonEmpty(wt.Error, "runtime restarted; worktree preserved")
			return wt
		}
		wt.Status = worktreeStatusCleanupPending
		wt.Error = firstNonEmpty(wt.Error, "runtime restarted; cleanup pending")
		if wt.CleanupPolicy == worktreeCleanupExit {
			return r.recoverCleanupPendingWorktree(ctx, wt)
		}
		return wt
	}
}

func (r *runtimeService) recoverCleanupPendingWorktree(ctx context.Context, wt RuntimeWorktree) RuntimeWorktree {
	if shouldPreserveWorktree(wt, "runtime restarted") {
		wt.Status = worktreeStatusPreserved
		wt.Error = firstNonEmpty(wt.Error, "runtime restarted; cleanup skipped by preserve policy")
		return wt
	}
	if wt.CleanupPolicy != worktreeCleanupExit {
		wt.Status = worktreeStatusCleanupPending
		wt.Error = firstNonEmpty(wt.Error, "runtime restarted; cleanup remains pending")
		return wt
	}
	wt.Status = worktreeStatusCleaning
	if err := removeGitWorktree(ctx, wt); err != nil {
		if errors.Is(err, errRuntimeWorktreeMissingPath) {
			wt.Status = worktreeStatusMissing
			wt.Error = "recovery cleanup skipped because worktree path is missing"
			return wt
		}
		wt.Status = worktreeStatusCleanupFailed
		wt.Error = "recovery cleanup failed: " + err.Error()
		return wt
	}
	wt.Status = worktreeStatusCleaned
	wt.CleanedAt = time.Now().UnixMilli()
	wt.Error = ""
	return wt
}

func worktreeRecoveryEventForStatus(status string) (string, string) {
	switch status {
	case worktreeStatusMissing:
		return runtimeapi.EventWorktreeMissingPath, "worktree_missing_path"
	case worktreeStatusCleaned:
		return runtimeapi.EventWorktreeCleaned, "worktree_cleaned"
	case worktreeStatusCleanupFailed:
		return runtimeapi.EventWorktreeCleanupFailed, "worktree_cleanup_failed"
	case worktreeStatusPreserved:
		return runtimeapi.EventWorktreePreserved, "worktree_preserved"
	case worktreeStatusError:
		return runtimeapi.EventWorktreePolicyDenied, "worktree_recovery_error"
	default:
		return runtimeapi.EventWorktreeRecovered, "worktree_recovered"
	}
}
