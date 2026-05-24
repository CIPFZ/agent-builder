package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeService) ReplayExport(ctx context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.TurnID != "" {
		return r.replayExportTurn(ctx, req)
	}
	return r.replayExportSession(ctx, req)
}

func (r *runtimeService) replayExportTurn(ctx context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {
	audit, err := r.replayAuditTurn(ctx, req.TurnID)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	events, err := r.Events(ctx, req.After)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	filtered := filterReplayEvents(events.Events, req.SessionID, req.TurnID)
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = audit.Summary.SessionID
	}
	resp := RuntimeReplayExportResponse{
		SessionID:        sessionID,
		TurnID:           req.TurnID,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Source:           "runtime_audit_events+runtime_event_buffer",
		SnapshotRequired: events.SnapshotRequired,
		FirstSequence:    events.FirstSequence,
		LastSequence:     events.LastSequence,
		Events:           filtered,
		Audit:            audit.Events,
	}
	resp.Summary = buildRuntimeReplaySummary(audit.Summary, filtered, audit.Events)
	resp.Summary.Recovery.SnapshotRequired = events.SnapshotRequired || resp.Summary.Recovery.SnapshotRequired
	resp.Summary.Recovery.LastEventSequence = events.LastSequence
	return resp, nil
}

func (r *runtimeService) replayExportSession(ctx context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {
	audit, err := r.replayAuditSession(ctx, req.SessionID)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	events, err := r.Events(ctx, req.After)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	filtered := filterReplayEvents(events.Events, req.SessionID, "")
	resp := RuntimeReplayExportResponse{
		SessionID:        req.SessionID,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Source:           "runtime_audit_events+runtime_event_buffer",
		SnapshotRequired: events.SnapshotRequired,
		FirstSequence:    events.FirstSequence,
		LastSequence:     events.LastSequence,
		Events:           filtered,
		Audit:            audit.Events,
	}
	resp.Summary = buildRuntimeReplaySummary(RuntimeAuditTurnSummary{}, filtered, audit.Events)
	resp.Summary.Recovery.SnapshotRequired = events.SnapshotRequired || resp.Summary.Recovery.SnapshotRequired
	resp.Summary.Recovery.LastEventSequence = events.LastSequence
	return resp, nil
}

func (r *runtimeService) replayAuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {
	if r.turns.db != nil {
		resp, err := newRuntimeAuditStore(r.turns.db).ListTurn(ctx, turnID)
		if err != nil {
			return RuntimeAuditResponse{}, err
		}
		resp.Summary = r.auditTurnSummary(ctx, turnID, resp.Events)
		return resp, nil
	}
	return r.AuditTurn(ctx, turnID)
}

func (r *runtimeService) replayAuditSession(ctx context.Context, sessionID string) (RuntimeAuditResponse, error) {
	if r.turns.db != nil {
		return newRuntimeAuditStore(r.turns.db).ListSession(ctx, sessionID)
	}
	return r.AuditSession(ctx, sessionID)
}

func filterReplayEvents(events []RuntimeEvent, sessionID, turnID string) []RuntimeEvent {
	filtered := make([]RuntimeEvent, 0, len(events))
	for _, event := range events {
		if turnID != "" && event.TurnID != turnID {
			continue
		}
		if sessionID != "" && event.SessionID != sessionID {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func buildRuntimeReplaySummary(auditSummary RuntimeAuditTurnSummary, events []RuntimeEvent, auditEvents []RuntimeAuditEvent) RuntimeReplayExportSummary {
	summary := RuntimeReplayExportSummary{
		CompactBoundaries: auditSummary.Compact,
		Budget:            auditSummary.Budget,
		ToolCalls:         auditSummary.ToolCalls,
		EventCounts:       map[string]int{},
		AuditCounts:       map[string]int{},
		Redacted:          true,
	}
	for _, event := range events {
		summary.EventCounts[event.Type]++
		switch event.Type {
		case runtimeapi.EventToolSearchPerformed:
			summary.ToolSearches = append(summary.ToolSearches, runtimeReplayToolSearchFromEvent(event))
			for _, omission := range asSlice(event.Payload["omitted"]) {
				item := asMap(omission)
				if stringFromMap(item, "reason") == "policy_denied" {
					summary.ToolDiscovery.Denied = appendUniqueStrings(summary.ToolDiscovery.Denied, stringFromMap(item, "name"))
				}
			}
		case runtimeapi.EventToolDiscoverySelected:
			summary.ToolDiscovery.Selected = appendUniqueStrings(summary.ToolDiscovery.Selected, stringSliceFromMap(event.Payload, "selected")...)
		case runtimeapi.EventToolDiscoveryOmitted:
			summary.ToolDiscovery.Omitted = appendUniqueStrings(summary.ToolDiscovery.Omitted, stringSliceFromMap(event.Payload, "omitted")...)
		case runtimeapi.EventTaskMessageCreated:
			summary.AgentTaskMessages = append(summary.AgentTaskMessages, runtimeReplayTaskMessageFromEvent(event))
		case runtimeapi.EventTaskResultUpdated:
			summary.AgentTaskResults = append(summary.AgentTaskResults, runtimeReplayTaskResultFromEvent(event))
		case runtimeapi.EventPermissionPolicyApplied, runtimeapi.EventPolicyRuleMatched, runtimeapi.EventPolicyRuleDenied, runtimeapi.EventPolicyRuleAsk:
			summary.PolicyDecisions = append(summary.PolicyDecisions, runtimeReplayPolicyFromEvent(event))
		case runtimeapi.EventPermissionRequested, runtimeapi.EventPermissionDecided:
			summary.PermissionEvents = append(summary.PermissionEvents, runtimeReplayPermissionFromEvent(event))
		case runtimeapi.EventSnapshotRequired:
			summary.Recovery.SnapshotRequired = true
		}
	}
	for _, audit := range auditEvents {
		summary.AuditCounts[audit.Type]++
		switch audit.Type {
		case "tool_search_performed":
			selected := stringSliceFromMap(asMap(audit.Payload["extra"]), "selected")
			summary.ToolDiscovery.Selected = appendUniqueStrings(summary.ToolDiscovery.Selected, selected...)
			for _, omission := range asSlice(asMap(audit.Payload["extra"])["omitted"]) {
				item := asMap(omission)
				if stringFromMap(item, "reason") == "policy_denied" {
					summary.ToolDiscovery.Denied = appendUniqueStrings(summary.ToolDiscovery.Denied, stringFromMap(item, "name"))
				}
			}
		case "permission_policy_applied":
			summary.PolicyDecisions = append(summary.PolicyDecisions, runtimeReplayPolicyFromAudit(audit))
		case "permission_requested":
			summary.PermissionEvents = append(summary.PermissionEvents, runtimeReplayPermissionFromAudit(audit))
		case "task_message_created", "task_artifact_created":
			if msg := runtimeReplayTaskMessageFromAudit(audit); msg.ID != "" {
				summary.AgentTaskMessages = append(summary.AgentTaskMessages, msg)
			}
		case "task_result_updated":
			if result := runtimeReplayTaskResultFromAudit(audit); result.TaskID != "" {
				summary.AgentTaskResults = append(summary.AgentTaskResults, result)
			}
		}
	}
	return summary
}

func runtimeReplayTaskMessageFromEvent(event RuntimeEvent) RuntimeAgentTaskMessage {
	return RuntimeAgentTaskMessage{
		ID:                stringFromMap(event.Payload, "message_id"),
		TaskID:            stringFromMap(event.Payload, "task_id"),
		ParentTurnID:      event.TurnID,
		ParentSessionID:   event.SessionID,
		ChildSessionID:    stringFromMap(event.Payload, "child_session_id"),
		Direction:         stringFromMap(event.Payload, "direction"),
		Kind:              stringFromMap(event.Payload, "kind"),
		Status:            stringFromMap(event.Payload, "status"),
		ContentSummary:    stringFromMap(event.Payload, "summary"),
		RelatedToolCallID: event.ToolCallID,
		ArtifactRefs:      stringSliceFromMap(event.Payload, "artifact_refs"),
	}
}

func runtimeReplayTaskResultFromEvent(event RuntimeEvent) RuntimeAgentTaskResult {
	return RuntimeAgentTaskResult{
		TaskID:              stringFromMap(event.Payload, "task_id"),
		Status:              stringFromMap(event.Payload, "status"),
		Summary:             stringFromMap(event.Payload, "summary"),
		ErrorDetail:         stringFromMap(event.Payload, "error_detail"),
		CancellationDetail:  stringFromMap(event.Payload, "cancellation_detail"),
		ArtifactRefs:        stringSliceFromMap(event.Payload, "artifact_refs"),
		RelatedMessageRefs:  stringSliceFromMap(event.Payload, "related_message_refs"),
		RelatedToolCallRefs: stringSliceFromMap(event.Payload, "related_tool_refs"),
		CompactBoundaryRefs: stringSliceFromMap(event.Payload, "compact_boundary_refs"),
	}
}

func runtimeReplayTaskMessageFromAudit(event RuntimeAuditEvent) RuntimeAgentTaskMessage {
	extra := asMap(event.Payload["extra"])
	return runtimeAgentTaskMessageFromPayload(asMap(extra["task_message"]))
}

func runtimeReplayTaskResultFromAudit(event RuntimeAuditEvent) RuntimeAgentTaskResult {
	extra := asMap(event.Payload["extra"])
	return runtimeAgentTaskResultFromPayload(asMap(extra["task_result"]))
}

func runtimeAgentTaskMessageFromPayload(payload map[string]any) RuntimeAgentTaskMessage {
	return RuntimeAgentTaskMessage{
		ID:                stringFromMap(payload, "id"),
		TaskID:            stringFromMap(payload, "taskId"),
		ParentTaskID:      stringFromMap(payload, "parentTaskId"),
		ParentTurnID:      stringFromMap(payload, "parentTurnId"),
		ParentSessionID:   stringFromMap(payload, "parentSessionId"),
		ChildSessionID:    stringFromMap(payload, "childSessionId"),
		Direction:         stringFromMap(payload, "direction"),
		Kind:              stringFromMap(payload, "kind"),
		Status:            stringFromMap(payload, "status"),
		ContentSummary:    stringFromMap(payload, "contentSummary"),
		RelatedToolCallID: stringFromMap(payload, "relatedToolCallId"),
		RelatedMessageID:  stringFromMap(payload, "relatedMessageId"),
		ArtifactRefs:      stringSliceFromMap(payload, "artifactRefs"),
		CreatedAt:         int64(intFromMap(payload, "createdAt")),
		DeliveredAt:       int64(intFromMap(payload, "deliveredAt")),
	}
}

func runtimeAgentTaskResultFromPayload(payload map[string]any) RuntimeAgentTaskResult {
	return RuntimeAgentTaskResult{
		TaskID:              stringFromMap(payload, "taskId"),
		Status:              stringFromMap(payload, "status"),
		Summary:             stringFromMap(payload, "summary"),
		ErrorDetail:         stringFromMap(payload, "errorDetail"),
		CancellationDetail:  stringFromMap(payload, "cancellationDetail"),
		ArtifactRefs:        stringSliceFromMap(payload, "artifactRefs"),
		RelatedMessageRefs:  stringSliceFromMap(payload, "relatedMessageRefs"),
		RelatedToolCallRefs: stringSliceFromMap(payload, "relatedToolCallRefs"),
		CompactBoundaryRefs: stringSliceFromMap(payload, "compactBoundaryRefs"),
		CreatedAt:           int64(intFromMap(payload, "createdAt")),
		UpdatedAt:           int64(intFromMap(payload, "updatedAt")),
	}
}

func runtimeReplayToolSearchFromEvent(event RuntimeEvent) RuntimeReplayToolSearch {
	return RuntimeReplayToolSearch{
		Query:        stringFromMap(event.Payload, "query"),
		Selected:     stringSliceFromMap(event.Payload, "selected"),
		OmittedCount: intFromMap(event.Payload, "omitted_count"),
		BudgetImpact: runtimeToolSchemaBudgetImpactFromPayload(event.Payload["budget_impact"]),
		Guardrail:    stringFromMap(event.Payload, "guardrail"),
	}
}

func runtimeReplayPolicyFromEvent(event RuntimeEvent) RuntimeReplayPolicyDecision {
	return RuntimeReplayPolicyDecision{
		ToolCallID:        event.ToolCallID,
		ToolName:          stringFromMap(event.Payload, "tool_name"),
		Decision:          stringFromMap(event.Payload, "decision"),
		Risk:              stringFromMap(event.Payload, "risk"),
		Reason:            stringFromMap(event.Payload, "reason"),
		Mode:              stringFromMap(event.Payload, "mode"),
		Profile:           stringFromMap(event.Payload, "profile"),
		MatchedRuleID:     firstNonEmpty(stringFromMap(event.Payload, "matched_rule_id"), stringFromMap(event.Payload, "rule_id")),
		MatchedRuleSource: firstNonEmpty(stringFromMap(event.Payload, "matched_rule_source"), stringFromMap(event.Payload, "source")),
		ScopeKind:         stringFromMap(event.Payload, "scope_kind"),
		ScopeValue:        stringFromMap(event.Payload, "scope_value"),
		ShellRisk:         stringFromMap(event.Payload, "shell_risk"),
		ShellReason:       stringFromMap(event.Payload, "shell_reason"),
	}
}

func runtimeReplayPolicyFromAudit(event RuntimeAuditEvent) RuntimeReplayPolicyDecision {
	return RuntimeReplayPolicyDecision{
		ToolCallID:        firstNonEmpty(event.ToolCallID, stringFromMap(event.Payload, "tool_call_id")),
		ToolName:          stringFromMap(event.Payload, "permission_tool"),
		Decision:          stringFromMap(event.Payload, "permission_policy"),
		Risk:              stringFromMap(event.Payload, "permission_risk"),
		Reason:            stringFromMap(event.Payload, "permission_reason"),
		Mode:              stringFromMap(event.Payload, "policy_mode"),
		Profile:           stringFromMap(event.Payload, "policy_profile"),
		MatchedRuleID:     stringFromMap(event.Payload, "policy_rule_id"),
		MatchedRuleSource: stringFromMap(event.Payload, "policy_rule_source"),
		ScopeKind:         stringFromMap(event.Payload, "policy_scope_kind"),
		ScopeValue:        stringFromMap(event.Payload, "policy_scope_value"),
		ShellRisk:         stringFromMap(event.Payload, "shell_risk"),
		ShellReason:       stringFromMap(event.Payload, "shell_reason"),
	}
}

func runtimeReplayPermissionFromEvent(event RuntimeEvent) RuntimeReplayPermission {
	return RuntimeReplayPermission{
		PermissionID: stringFromMap(event.Payload, "permission_id"),
		ToolCallID:   event.ToolCallID,
		ToolName:     stringFromMap(event.Payload, "tool_name"),
		Action:       stringFromMap(event.Payload, "action"),
		Decision:     stringFromMap(event.Payload, "decision"),
		Status:       stringFromMap(event.Payload, "status"),
		Risk:         stringFromMap(event.Payload, "risk"),
		Reason:       stringFromMap(event.Payload, "reason"),
	}
}

func runtimeReplayPermissionFromAudit(event RuntimeAuditEvent) RuntimeReplayPermission {
	return RuntimeReplayPermission{
		PermissionID: firstNonEmpty(event.PermissionID, stringFromMap(event.Payload, "permission_id")),
		ToolCallID:   firstNonEmpty(event.ToolCallID, stringFromMap(event.Payload, "tool_call_id")),
		ToolName:     stringFromMap(event.Payload, "permission_tool"),
		Action:       stringFromMap(event.Payload, "permission_action"),
		Decision:     stringFromMap(event.Payload, "permission_policy"),
		Risk:         stringFromMap(event.Payload, "permission_risk"),
		Reason:       stringFromMap(event.Payload, "permission_reason"),
	}
}

func runtimeToolSchemaBudgetImpactFromPayload(value any) RuntimeToolSchemaBudgetImpact {
	payload := asMap(value)
	return RuntimeToolSchemaBudgetImpact{
		Selected: replayBudgetBucketFromPayload(payload["selected"]),
		Omitted:  replayBudgetBucketFromPayload(payload["omitted"]),
	}
}

func replayBudgetBucketFromPayload(value any) RuntimeBudgetBucket {
	payload := asMap(value)
	return RuntimeBudgetBucket{
		Count:           intFromMap(payload, "count"),
		EstimatedTokens: intFromMap(payload, "estimatedTokens"),
	}
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	out := make([]string, 0, len(values)+len(additions))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range additions {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}
