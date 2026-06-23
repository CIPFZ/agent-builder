package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	runtimeMCPRequestDecisionActionSourceKind      = "mcp_request_decision"
	runtimeMCPRequestDecisionReasonAccepted        = "mcp_request_decision_recorded"
	runtimeMCPRequestDecisionReasonAlreadyTerminal = "mcp_request_already_terminal"
)

func runtimeMCPRequestDecisionRefreshTargets() []string {
	return []string{
		"mcp_requests",
		"mcp_servers",
		"session_activity",
		"diagnostics",
		"status",
	}
}

func (r *runtimeService) MCPRequests(ctx context.Context, filter RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeMCPRequestsResponse{}, err
	}
	requests, err := r.mcpRequestStore.List(ctx, filter)
	if err != nil {
		return RuntimeMCPRequestsResponse{}, err
	}
	return RuntimeMCPRequestsResponse{Requests: requests}, nil
}

func (r *runtimeService) MCPRequest(ctx context.Context, id string) (RuntimeMCPRequestResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeMCPRequestResponse{}, err
	}
	req, err := r.mcpRequestStore.Get(ctx, id)
	if err != nil {
		return RuntimeMCPRequestResponse{}, err
	}
	return RuntimeMCPRequestResponse{Request: req}, nil
}

func (r *runtimeService) DecideMCPRequest(ctx context.Context, decision RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeMCPRequestResponse{}, err
	}
	decision.RequestID = strings.TrimSpace(decision.RequestID)
	action := strings.TrimSpace(decision.Action)
	if decision.RequestID == "" {
		return RuntimeMCPRequestResponse{}, errors.New("mcp request id is required")
	}
	req, err := r.mcpRequestStore.Get(ctx, decision.RequestID)
	if err != nil {
		return RuntimeMCPRequestResponse{}, err
	}
	if mcpRequestStatusTerminal(req.Status) {
		return withRuntimeMCPRequestDecisionAction(RuntimeMCPRequestResponse{Request: req}, false, action, runtimeMCPRequestDecisionReasonAlreadyTerminal), nil
	}
	status := ""
	errText := decision.Error
	switch action {
	case "approve", "complete", "answer", "submit":
		if req.Kind == mcpRequestKindAuth {
			status = mcpRequestStatusCompleted
		} else {
			status = mcpRequestStatusCompleted
		}
	case "deny":
		status = mcpRequestStatusDenied
	case "cancel":
		status = mcpRequestStatusCancelled
	case "fail":
		status = mcpRequestStatusFailed
	default:
		return RuntimeMCPRequestResponse{}, fmt.Errorf("unsupported mcp request action: %s", action)
	}
	updated, err := r.mcpRequestStore.Mark(ctx, req.ID, status, decision.ResponseSummary, errText)
	if err != nil {
		return RuntimeMCPRequestResponse{}, err
	}
	r.publishMCPRequestLifecycle(updated)
	r.writeMCPRequestAudit(updated)
	if updated.Kind == mcpRequestKindAuth && updated.Status == mcpRequestStatusCompleted {
		r.markMCPServerRetryNeeded(updated.Server, "auth_completed")
	}
	if updated.Kind == mcpRequestKindElicitation && updated.Status == mcpRequestStatusCompleted {
		r.markMCPServerRetryNeeded(updated.Server, "elicitation_completed")
	}
	return withRuntimeMCPRequestDecisionAction(RuntimeMCPRequestResponse{Request: updated}, true, action, runtimeMCPRequestDecisionReasonAccepted), nil
}

func withRuntimeMCPRequestDecisionAction(resp RuntimeMCPRequestResponse, accepted bool, action, reason string) RuntimeMCPRequestResponse {
	resp.Action = &RuntimeWriteActionMetadata{
		Accepted:       accepted,
		Reason:         reason,
		RefreshTargets: runtimeMCPRequestDecisionRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeMCPRequestDecisionActionSourceKind,
			Action:                action,
			WorkbenchOnly:         true,
			StartsWorker:          false,
			IdempotentBy:          "mcp_request_id",
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_mcp_requests",
				"runtime_mcp_servers",
				"runtime_events",
				"runtime_audit",
				"session_activity",
			},
		},
	}
	return resp
}

func (r *runtimeService) RetryMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {
	return r.RefreshMCPServer(ctx, name)
}

func (r *runtimeService) createMCPAuthRequest(ctx context.Context, server, capabilityID, description string, decision permission.PolicyResult) (RuntimeMCPRequest, error) {
	return r.createMCPRequest(ctx, RuntimeMCPRequest{
		ID:           newRuntimeEventID(),
		Kind:         mcpRequestKindAuth,
		Server:       server,
		CapabilityID: capabilityID,
		Status:       mcpRequestStatusPending,
		Prompt:       "MCP server requires runtime-managed authentication approval.",
		Description:  description,
	}, decision)
}

func (r *runtimeService) createMCPElicitationRequest(ctx context.Context, server, capabilityID, prompt, description string, decision permission.PolicyResult) (RuntimeMCPRequest, error) {
	return r.createMCPRequest(ctx, RuntimeMCPRequest{
		ID:           newRuntimeEventID(),
		Kind:         mcpRequestKindElicitation,
		Server:       server,
		CapabilityID: capabilityID,
		Status:       mcpRequestStatusPending,
		Prompt:       prompt,
		Description:  description,
	}, decision)
}

func (r *runtimeService) createMCPRequest(ctx context.Context, req RuntimeMCPRequest, decision permission.PolicyResult) (RuntimeMCPRequest, error) {
	r.mu.Lock()
	sessionID := r.sessionID
	r.mu.Unlock()
	req.SessionID = firstNonEmpty(req.SessionID, sessionID)
	req.PolicyMode = string(decision.Mode)
	req.PolicyProfile = decision.Profile
	req.PolicyDecision = string(decision.Decision)
	req.PolicyReason = decision.Reason
	req.PolicyRisk = string(decision.Risk)
	req.PolicyRuleID = decision.RuleID
	req.PolicyRuleSource = decision.RuleSource
	req.PolicyScopeKind = decision.RuleScopeKind
	req.PolicyScopeValue = decision.RuleScopeValue
	req.PolicyTargetSummary = decision.TargetSummary
	req.PolicyHeadless = decision.Headless
	req.PolicyHeadlessReason = decision.HeadlessReason
	if decision.Headless && decision.Decision == permission.PolicyAsk {
		req.Status = mcpRequestStatusFailed
		req.Error = firstNonEmpty(decision.HeadlessReason, "headless policy cannot ask for MCP runtime input")
		req.CompletedAt = time.Now().UnixMilli()
	}
	stored, err := r.mcpRequestStore.Upsert(ctx, req)
	if err != nil {
		return RuntimeMCPRequest{}, err
	}
	r.publishMCPRequestLifecycle(stored)
	r.writeMCPRequestAudit(stored)
	return stored, nil
}

func (r *runtimeService) publishMCPRequestLifecycle(req RuntimeMCPRequest) {
	eventType := ""
	switch req.Kind {
	case mcpRequestKindAuth:
		switch req.Status {
		case mcpRequestStatusPending, mcpRequestStatusRequired:
			eventType = runtimeapi.EventMCPAuthRequested
		case mcpRequestStatusCompleted, mcpRequestStatusApproved:
			eventType = runtimeapi.EventMCPAuthCompleted
		case mcpRequestStatusDenied, mcpRequestStatusCancelled:
			eventType = runtimeapi.EventMCPAuthDenied
		case mcpRequestStatusFailed, mcpRequestStatusExpired:
			eventType = runtimeapi.EventMCPAuthFailed
		}
	case mcpRequestKindElicitation:
		switch req.Status {
		case mcpRequestStatusPending, mcpRequestStatusRequired:
			eventType = runtimeapi.EventMCPElicitationRequested
		case mcpRequestStatusCompleted, mcpRequestStatusApproved:
			eventType = runtimeapi.EventMCPElicitationCompleted
		case mcpRequestStatusDenied, mcpRequestStatusCancelled:
			eventType = runtimeapi.EventMCPElicitationDenied
		case mcpRequestStatusFailed, mcpRequestStatusExpired:
			eventType = runtimeapi.EventMCPElicitationFailed
		}
	}
	if eventType == "" {
		return
	}
	payload := mcpRequestEventPayload(req)
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		Payload:   payload,
	})
}

func (r *runtimeService) writeMCPRequestAudit(req RuntimeMCPRequest) {
	r.writeAudit(auditEntry{
		Event:                "mcp_" + req.Kind + "_" + req.Status,
		Timestamp:            time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:            req.SessionID,
		RequestID:            req.TurnID,
		CapabilityID:         req.CapabilityID,
		CapabilityKind:       "mcp_" + req.Kind,
		CapabilitySource:     req.Server,
		CapabilityState:      req.Status,
		CapabilityReason:     req.PolicyReason,
		CapabilityError:      req.Error,
		MCPServer:            req.Server,
		MCPKind:              req.Kind,
		MCPStatus:            req.Status,
		MCPDecision:          req.PolicyDecision,
		MCPRisk:              req.PolicyRisk,
		MCPReason:            req.PolicyReason,
		PolicyMode:           req.PolicyMode,
		PolicyProfile:        req.PolicyProfile,
		PolicyHeadless:       req.PolicyHeadless,
		PolicyHeadlessReason: req.PolicyHeadlessReason,
		PolicyRuleID:         req.PolicyRuleID,
		PolicyRuleSource:     req.PolicyRuleSource,
		PolicyScopeKind:      req.PolicyScopeKind,
		PolicyScopeValue:     req.PolicyScopeValue,
		PolicyTargetSummary:  req.PolicyTargetSummary,
		Extra: map[string]any{
			"mcp_request": mcpRequestEventPayload(req),
		},
	})
}

func mcpRequestEventPayload(req RuntimeMCPRequest) map[string]any {
	payload := map[string]any{
		"request_id":       req.ID,
		"kind":             req.Kind,
		"server":           req.Server,
		"capability_id":    req.CapabilityID,
		"session_id":       req.SessionID,
		"turn_id":          req.TurnID,
		"status":           req.Status,
		"description":      req.Description,
		"response_summary": req.ResponseSummary,
		"policy_mode":      req.PolicyMode,
		"policy_profile":   req.PolicyProfile,
		"policy_decision":  req.PolicyDecision,
		"policy_reason":    req.PolicyReason,
		"policy_risk":      req.PolicyRisk,
		"redacted":         true,
	}
	if req.PolicyHeadless {
		payload["policy_headless"] = true
		payload["policy_headless_reason"] = req.PolicyHeadlessReason
	}
	if req.Error != "" {
		payload["error"] = req.Error
	}
	return redactRuntimePayload(payload)
}

func (r *runtimeService) markMCPServerRetryNeeded(server, reason string) {
	for _, capability := range r.mcpCapabilitiesForServer(server) {
		if !capability.Enabled {
			continue
		}
		capability.State = capabilityStateUnloaded
		capability.Reason = reason
		capability.Diagnostics = "MCP server request completed; refresh retry is available."
		r.setCapabilityLoadRecord(capability.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateUnloaded,
			Reason:      capability.Reason,
			Diagnostics: capability.Diagnostics,
			UpdatedAt:   time.Now().UnixMilli(),
		})
	}
}

func (r *runtimeService) hasBlockingMCPRequest(ctx context.Context, server string) (RuntimeMCPRequest, bool) {
	requests, err := r.mcpRequestStore.List(ctx, RuntimeMCPRequestListRequest{Server: server})
	if err != nil {
		return RuntimeMCPRequest{}, false
	}
	for i := len(requests) - 1; i >= 0; i-- {
		switch requests[i].Status {
		case mcpRequestStatusPending, mcpRequestStatusRequired, mcpRequestStatusDenied, mcpRequestStatusFailed, mcpRequestStatusExpired, mcpRequestStatusCancelled:
			return requests[i], true
		case mcpRequestStatusCompleted, mcpRequestStatusApproved, mcpRequestStatusNotRequired, mcpRequestStatusNone:
			return RuntimeMCPRequest{}, false
		}
	}
	return RuntimeMCPRequest{}, false
}
