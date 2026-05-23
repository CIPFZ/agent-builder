package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func (r *runtimeService) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePermissionsResponse{}, err
	}
	if _, err := r.reconcilePendingPermissions(ctx); err != nil {
		return RuntimePermissionsResponse{}, err
	}
	if r.permissionStore.db != nil {
		permissions, err := r.permissionStore.List(ctx, permissionStatusPending)
		if err != nil {
			return RuntimePermissionsResponse{}, err
		}
		return RuntimePermissionsResponse{Permissions: permissions}, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	permissions := make([]RuntimePermissionRequest, 0, len(r.permissions))
	for _, pending := range r.permissions {
		permissions = append(permissions, pending.Permission)
	}
	return RuntimePermissionsResponse{Permissions: permissions}, nil
}

func (r *runtimeService) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	action := proto.PermissionAction(strings.TrimSpace(req.Action))
	if action != proto.PermissionAllow && action != proto.PermissionAllowForSession && action != proto.PermissionDeny {
		return RuntimeStatus{}, fmt.Errorf("invalid permission action: %s", req.Action)
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	pending, ok := r.permissions[req.PermissionID]
	r.mu.Unlock()
	if !ok && r.permissionStore.db != nil {
		if perm, err := r.permissionStore.Get(ctx, req.PermissionID); err == nil && perm.Status == permissionStatusPending {
			pending = pendingRuntimePermission{
				Permission: perm,
				Raw: permission.PermissionRequest{
					ID:           perm.ID,
					SessionID:    perm.SessionID,
					TurnID:       perm.TurnID,
					ToolCallID:   perm.ToolCallID,
					ToolName:     perm.ToolName,
					Description:  perm.Description,
					Action:       perm.Action,
					Params:       perm.Params,
					Path:         perm.Path,
					Risk:         permission.Risk(perm.Risk),
					PolicyMode:   perm.PolicyMode,
					PolicyReason: perm.PolicyReason,
					Decision:     perm.Decision,
					Status:       perm.Status,
					CreatedAt:    perm.CreatedAt,
					DecidedAt:    perm.DecidedAt,
				},
			}
			ok = true
		}
	}
	if !ok {
		return RuntimeStatus{}, fmt.Errorf("permission request %s is not pending", req.PermissionID)
	}

	if err := r.runtime.GrantPermission(wsID, proto.PermissionGrant{
		Permission: toProtoPermissionRequest(pending.Raw),
		Action:     action,
	}); err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to decide permission: %w", err)
	}

	r.mu.Lock()
	delete(r.permissions, req.PermissionID)
	r.mu.Unlock()
	if r.permissionStore.db != nil {
		_, _ = r.permissionStore.Mark(ctx, req.PermissionID, permissionStatusForDecision(action), time.Now().UnixMilli())
	}
	if r.toolCalls != nil {
		switch action {
		case proto.PermissionDeny:
			_, _ = r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
				ToolCallID:    pending.Permission.ToolCallID,
				Status:        scheduler.ToolCallDenied,
				Risk:          pending.Permission.Risk,
				PolicyReason:  firstNonEmpty(pending.Permission.Reason, pending.Permission.PolicyReason),
				OutputSummary: "Permission denied.",
				IsError:       true,
				Error:         "Permission denied.",
			})
			if call, err := r.toolCalls.GetCall(ctx, pending.Permission.ToolCallID); err == nil {
				r.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallFailed, call, map[string]any{
					"name":     call.Name,
					"summary":  "Permission denied.",
					"status":   string(call.Status),
					"denied":   true,
					"risk":     call.Risk,
					"reason":   call.PolicyReason,
					"is_error": true,
				}))
			}
		default:
			if call, err := r.toolCalls.GetCall(ctx, pending.Permission.ToolCallID); err == nil && call.Status == scheduler.ToolCallWaitingPermission {
				_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
					ID:           call.ID,
					SessionID:    call.SessionID,
					TurnID:       call.TurnID,
					MessageID:    call.MessageID,
					Name:         call.Name,
					Source:       call.Source,
					InputSummary: call.InputSummary,
				})
			}
		}
	}
	if pending.Permission.TurnID != "" {
		if turn, err := r.turns.Get(ctx, pending.Permission.TurnID); err == nil && turn.Status == turnStatusWaitingPermission {
			turn.Status = turnStatusRunning
			if action == proto.PermissionDeny {
				turn.Status = turnStatusFailed
				turn.FinishedAt = time.Now().UnixMilli()
				turn.Error = "permission denied"
			}
			_, _ = r.turns.Upsert(ctx, turn)
		}
	}

	r.writeAudit(auditEntry{
		RequestID:        pending.Permission.TurnID,
		Event:            "permission_decided",
		Timestamp:        time.Now().Format(time.RFC3339Nano),
		WorkspaceID:      wsID,
		SessionID:        pending.Permission.SessionID,
		PermissionTool:   pending.Permission.ToolName,
		PermissionAction: pending.Permission.Action,
		PermissionPath:   pending.Permission.Path,
		PermissionPolicy: string(action),
		PermissionRisk:   pending.Permission.Risk,
		PermissionReason: firstNonEmpty(pending.Permission.Reason, pending.Permission.PolicyReason),
		PolicyMode:       pending.Permission.PolicyMode,
		PermissionID:     pending.Permission.ID,
		ToolCallID:       pending.Permission.ToolCallID,
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionDecided,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  pending.Permission.SessionID,
		TurnID:     firstNonEmpty(pending.Permission.TurnID, r.activeTurnForSession(pending.Permission.SessionID)),
		ToolCallID: pending.Permission.ToolCallID,
		Payload: map[string]any{
			"permission_id": req.PermissionID,
			"tool_name":     pending.Permission.ToolName,
			"action":        string(action),
			"path":          pending.Permission.Path,
			"risk":          pending.Permission.Risk,
			"reason":        firstNonEmpty(pending.Permission.Reason, pending.Permission.PolicyReason),
			"mode":          pending.Permission.PolicyMode,
			"status":        permissionStatusForDecision(action),
		},
	})

	return r.Status(ctx)
}

func toRuntimePermissionRequest(perm permission.PermissionRequest) RuntimePermissionRequest {
	return RuntimePermissionRequest{
		ID:           perm.ID,
		SessionID:    perm.SessionID,
		TurnID:       perm.TurnID,
		ToolCallID:   perm.ToolCallID,
		ToolName:     perm.ToolName,
		Description:  perm.Description,
		Action:       perm.Action,
		Params:       perm.Params,
		Path:         perm.Path,
		Target:       perm.Path,
		Risk:         string(perm.Risk),
		PolicyMode:   perm.PolicyMode,
		PolicyReason: perm.PolicyReason,
		Decision:     perm.Decision,
		Reason:       perm.PolicyReason,
		Status:       firstNonEmpty(perm.Status, "pending"),
		CreatedAt:    firstNonZero(perm.CreatedAt, time.Now().UnixMilli()),
		DecidedAt:    perm.DecidedAt,
	}
}

func toProtoPermissionRequest(perm permission.PermissionRequest) proto.PermissionRequest {
	return proto.PermissionRequest{
		ID:           perm.ID,
		SessionID:    perm.SessionID,
		TurnID:       perm.TurnID,
		ToolCallID:   perm.ToolCallID,
		ToolName:     perm.ToolName,
		Description:  perm.Description,
		Action:       perm.Action,
		Params:       perm.Params,
		Path:         perm.Path,
		Risk:         string(perm.Risk),
		PolicyMode:   perm.PolicyMode,
		PolicyReason: perm.PolicyReason,
		Decision:     perm.Decision,
		Status:       perm.Status,
		CreatedAt:    perm.CreatedAt,
		DecidedAt:    perm.DecidedAt,
	}
}

func permissionStatusForDecision(action proto.PermissionAction) string {
	switch action {
	case proto.PermissionAllow:
		return permissionStatusAllowedOnce
	case proto.PermissionAllowForSession:
		return permissionStatusAllowedSession
	case proto.PermissionDeny:
		return permissionStatusDenied
	default:
		return string(action)
	}
}

func (r *runtimeService) reconcilePendingPermissions(ctx context.Context) ([]RuntimePermissionRequest, error) {
	if r.permissionStore.db == nil {
		return nil, nil
	}
	pending, err := r.permissionStore.List(ctx, permissionStatusPending)
	if err != nil {
		return nil, err
	}
	var expired []RuntimePermissionRequest
	for _, perm := range pending {
		status := ""
		if perm.TurnID == "" {
			status = permissionStatusExpired
		} else if turn, err := r.turns.Get(ctx, perm.TurnID); err != nil || isFinalTurnStatus(turn.Status) {
			status = permissionStatusExpired
			if err == nil && turn.Status == turnStatusCancelled {
				status = permissionStatusCancelled
			}
		}
		if status == "" && perm.ToolCallID != "" && r.toolCalls != nil {
			if call, err := r.toolCalls.GetCall(ctx, perm.ToolCallID); err == nil && isFinalToolCallStatus(string(call.Status)) {
				status = permissionStatusExpired
				if string(call.Status) == "cancelled" {
					status = permissionStatusCancelled
				}
			}
		}
		if status == "" {
			continue
		}
		marked, err := r.permissionStore.Mark(ctx, perm.ID, status, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		delete(r.permissions, perm.ID)
		r.mu.Unlock()
		expired = append(expired, marked)
	}
	return expired, nil
}

func (r *runtimeService) expireInvalidPendingPermissions(ctx context.Context) ([]RuntimePermissionRequest, error) {
	return r.reconcilePendingPermissions(ctx)
}

func isFinalToolCallStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "denied":
		return true
	default:
		return false
	}
}
