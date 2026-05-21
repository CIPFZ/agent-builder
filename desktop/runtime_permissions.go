package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
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

	r.writeAudit(auditEntry{
		Event:            "permission_decided",
		Timestamp:        time.Now().Format(time.RFC3339Nano),
		WorkspaceID:      wsID,
		SessionID:        pending.Permission.SessionID,
		PermissionTool:   pending.Permission.ToolName,
		PermissionAction: pending.Permission.Action,
		PermissionPath:   pending.Permission.Path,
		PermissionPolicy: string(action),
	})
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionDecided,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  pending.Permission.SessionID,
		TurnID:     r.activeTurnForSession(pending.Permission.SessionID),
		ToolCallID: pending.Permission.ToolCallID,
		Payload: map[string]any{
			"permission_id": req.PermissionID,
			"tool_name":     pending.Permission.ToolName,
			"action":        string(action),
			"path":          pending.Permission.Path,
		},
	})

	return r.Status(ctx)
}

func toRuntimePermissionRequest(perm permission.PermissionRequest) RuntimePermissionRequest {
	return RuntimePermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
		CreatedAt:   time.Now().UnixMilli(),
	}
}

func toProtoPermissionRequest(perm permission.PermissionRequest) proto.PermissionRequest {
	return proto.PermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
	}
}
