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

	r.mu.Lock()
	defer r.mu.Unlock()

	permissions := make([]RuntimePermissionRequest, 0, len(r.permissions))
	activeSession := r.sessionID
	for _, pending := range r.permissions {
		if activeSession != "" && pending.Permission.SessionID != activeSession {
			continue
		}
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
	if r.toolCalls != nil {
		switch action {
		case proto.PermissionDeny:
			_, _ = r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
				ToolCallID: pending.Permission.ToolCallID,
				Status:     scheduler.ToolCallDenied,
				IsError:    true,
				Error:      "Permission denied.",
			})
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
			"status":        permissionStatusForDecision(action),
		},
	})

	return r.Status(ctx)
}

func toRuntimePermissionRequest(perm permission.PermissionRequest) RuntimePermissionRequest {
	return RuntimePermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		TurnID:      perm.TurnID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
		Target:      perm.Path,
		Risk:        string(perm.Risk),
		Status:      firstNonEmpty(perm.Status, "pending"),
		CreatedAt:   firstNonZero(perm.CreatedAt, time.Now().UnixMilli()),
		DecidedAt:   perm.DecidedAt,
	}
}

func toProtoPermissionRequest(perm permission.PermissionRequest) proto.PermissionRequest {
	return proto.PermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		TurnID:      perm.TurnID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
		Risk:        string(perm.Risk),
		Status:      perm.Status,
		CreatedAt:   perm.CreatedAt,
		DecidedAt:   perm.DecidedAt,
	}
}

func permissionStatusForDecision(action proto.PermissionAction) string {
	switch action {
	case proto.PermissionAllow:
		return "allowed_once"
	case proto.PermissionAllowForSession:
		return "allowed_session"
	case proto.PermissionDeny:
		return "denied"
	default:
		return string(action)
	}
}
