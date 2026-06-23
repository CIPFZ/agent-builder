package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

const (
	runtimePermissionDecisionActionSourceKind = "permission_decision"
)

func runtimePermissionDecisionRefreshTargets() []string {
	return []string{
		"status",
		"permissions",
		"tool_calls",
		"turn_activity",
		"session_activity_window",
		"session_activity",
		"diagnostics",
	}
}

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

	action := apitypes.PermissionAction(strings.TrimSpace(req.Action))
	if action != apitypes.PermissionAllow && action != apitypes.PermissionAllowForSession && action != apitypes.PermissionDeny {
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
					ID:             perm.ID,
					SessionID:      perm.SessionID,
					TurnID:         perm.TurnID,
					ToolCallID:     perm.ToolCallID,
					ToolName:       perm.ToolName,
					Description:    perm.Description,
					Action:         perm.Action,
					Params:         perm.Params,
					Path:           perm.Path,
					Risk:           permission.Risk(perm.Risk),
					PolicyMode:     perm.PolicyMode,
					PolicyReason:   perm.PolicyReason,
					PolicyProfile:  perm.PolicyProfile,
					Headless:       perm.PolicyHeadless,
					HeadlessReason: perm.PolicyHeadlessReason,
					RuleID:         perm.PolicyRuleID,
					RuleSource:     perm.PolicyRuleSource,
					ScopeKind:      perm.PolicyScopeKind,
					ScopeValue:     perm.PolicyScopeValue,
					TargetSummary:  perm.PolicyTargetSummary,
					Decision:       perm.Decision,
					Status:         perm.Status,
					CreatedAt:      perm.CreatedAt,
					DecidedAt:      perm.DecidedAt,
				},
			}
			ok = true
		}
	}
	if !ok {
		return RuntimeStatus{}, fmt.Errorf("permission request %s is not pending", req.PermissionID)
	}

	if err := r.runtime.GrantPermission(wsID, apitypes.PermissionGrant{
		Permission: toAPITypePermissionRequest(pending.Raw),
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
		case apitypes.PermissionDeny:
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
			if action == apitypes.PermissionDeny {
				turn.Status = turnStatusFailed
				turn.FinishedAt = time.Now().UnixMilli()
				turn.Error = "permission denied"
			}
			_, _ = r.turns.Upsert(ctx, turn)
		}
	}

	r.writeAudit(auditEntry{
		RequestID:           pending.Permission.TurnID,
		Event:               "permission_decided",
		Timestamp:           time.Now().Format(time.RFC3339Nano),
		WorkspaceID:         wsID,
		SessionID:           pending.Permission.SessionID,
		PermissionTool:      pending.Permission.ToolName,
		PermissionAction:    pending.Permission.Action,
		PermissionPath:      pending.Permission.Path,
		PermissionPolicy:    string(action),
		PermissionRisk:      pending.Permission.Risk,
		PermissionReason:    firstNonEmpty(pending.Permission.Reason, pending.Permission.PolicyReason),
		PolicyMode:          pending.Permission.PolicyMode,
		PolicyProfile:       pending.Permission.PolicyProfile,
		PolicyRuleID:        pending.Permission.PolicyRuleID,
		PolicyRuleSource:    pending.Permission.PolicyRuleSource,
		PolicyScopeKind:     pending.Permission.PolicyScopeKind,
		PolicyScopeValue:    pending.Permission.PolicyScopeValue,
		PolicyTargetSummary: pending.Permission.PolicyTargetSummary,
		PermissionID:        pending.Permission.ID,
		ToolCallID:          pending.Permission.ToolCallID,
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionDecided,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  pending.Permission.SessionID,
		TurnID:     firstNonEmpty(pending.Permission.TurnID, r.activeTurnForSession(pending.Permission.SessionID)),
		ToolCallID: pending.Permission.ToolCallID,
		Payload: map[string]any{
			"permission_id":       req.PermissionID,
			"tool_name":           pending.Permission.ToolName,
			"action":              string(action),
			"path":                pending.Permission.Path,
			"risk":                pending.Permission.Risk,
			"reason":              firstNonEmpty(pending.Permission.Reason, pending.Permission.PolicyReason),
			"mode":                pending.Permission.PolicyMode,
			"matched_rule_id":     pending.Permission.PolicyRuleID,
			"matched_rule_source": pending.Permission.PolicyRuleSource,
			"scope_kind":          pending.Permission.PolicyScopeKind,
			"scope_value":         pending.Permission.PolicyScopeValue,
			"target_summary":      pending.Permission.PolicyTargetSummary,
			"status":              permissionStatusForDecision(action),
		},
	})

	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeStatus{}, err
	}
	return withRuntimePermissionDecisionAction(status, action), nil
}

func withRuntimePermissionDecisionAction(status RuntimeStatus, action apitypes.PermissionAction) RuntimeStatus {
	status.Action = &RuntimeWriteActionMetadata{
		Accepted:       true,
		Reason:         permissionStatusForDecision(action),
		RefreshTargets: runtimePermissionDecisionRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimePermissionDecisionActionSourceKind,
			Action:                string(action),
			WorkbenchOnly:         true,
			StartsWorker:          false,
			IdempotentBy:          "permission_id",
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_permissions",
				"runtime_tool_calls",
				"runtime_turns",
				"runtime_events",
				"runtime_audit",
				"session_activity",
			},
		},
	}
	return status
}

func toRuntimePermissionRequest(perm permission.PermissionRequest) RuntimePermissionRequest {
	return RuntimePermissionRequest{
		ID:                   perm.ID,
		SessionID:            perm.SessionID,
		TurnID:               perm.TurnID,
		ToolCallID:           perm.ToolCallID,
		ToolName:             perm.ToolName,
		Description:          perm.Description,
		Action:               perm.Action,
		Params:               perm.Params,
		Path:                 perm.Path,
		Target:               perm.Path,
		Risk:                 string(perm.Risk),
		PolicyMode:           perm.PolicyMode,
		PolicyReason:         perm.PolicyReason,
		PolicyProfile:        perm.PolicyProfile,
		PolicyHeadless:       perm.Headless,
		PolicyHeadlessReason: perm.HeadlessReason,
		PolicyRuleID:         perm.RuleID,
		PolicyRuleSource:     perm.RuleSource,
		PolicyScopeKind:      perm.ScopeKind,
		PolicyScopeValue:     perm.ScopeValue,
		PolicyTargetSummary:  perm.TargetSummary,
		Decision:             perm.Decision,
		Reason:               perm.PolicyReason,
		Status:               firstNonEmpty(perm.Status, "pending"),
		CreatedAt:            firstNonZero(perm.CreatedAt, time.Now().UnixMilli()),
		DecidedAt:            perm.DecidedAt,
	}
}

func toAPITypePermissionRequest(perm permission.PermissionRequest) apitypes.PermissionRequest {
	return apitypes.PermissionRequest{
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

func permissionStatusForDecision(action apitypes.PermissionAction) string {
	switch action {
	case apitypes.PermissionAllow:
		return permissionStatusAllowedOnce
	case apitypes.PermissionAllowForSession:
		return permissionStatusAllowedSession
	case apitypes.PermissionDeny:
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
		r.mu.Lock()
		_, live := r.permissions[perm.ID]
		r.mu.Unlock()
		if live {
			continue
		}
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
