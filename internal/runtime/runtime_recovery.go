package runtime

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
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
	r.mu.Lock()
	startedAt := r.recovery.startedAt
	interruptedTurns := append([]RuntimeTurn(nil), r.recovery.interruptedTurns...)
	lastSequence := r.nextEventSequence
	snapshotRequired := len(r.events) > 0 && r.events[0].Sequence > 1
	r.mu.Unlock()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return RuntimeRecoveryStatus{
		RuntimeStartedAt:   startedAt.UTC().Format(time.RFC3339Nano),
		LastEventSequence:  lastSequence,
		ActiveTurns:        activeTurns,
		InterruptedTurns:   interruptedTurns,
		PendingPermissions: pendingPermissions,
		SnapshotRequired:   snapshotRequired,
	}, nil
}
