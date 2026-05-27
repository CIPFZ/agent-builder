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
	events, source, err := r.replayEventsTurn(ctx, req.TurnID, req.After)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	filtered := events.Events
	if req.SessionID != "" {
		filtered = filterReplayEvents(filtered, req.SessionID, req.TurnID)
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = audit.Summary.SessionID
	}
	resp := RuntimeReplayExportResponse{
		SessionID:        sessionID,
		TurnID:           req.TurnID,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Source:           "runtime_audit_events+" + source,
		SnapshotRequired: events.SnapshotRequired,
		FirstSequence:    events.FirstSequence,
		LastSequence:     events.LastSequence,
		Events:           filtered,
		Audit:            audit.Events,
	}
	resp.Summary = buildRuntimeReplaySummary(audit.Summary, filtered, audit.Events)
	r.attachRuntimeReplayRefs(ctx, &resp.Summary, RuntimeRefListRequest{SessionID: sessionID, TurnID: req.TurnID})
	resp.Summary.Recovery.SnapshotRequired = events.SnapshotRequired || resp.Summary.Recovery.SnapshotRequired
	resp.Summary.Recovery.LastEventSequence = events.LastSequence
	return resp, nil
}

func (r *runtimeService) replayExportSession(ctx context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {
	audit, err := r.replayAuditSession(ctx, req.SessionID)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	events, source, err := r.replayEventsSession(ctx, req.SessionID, req.After)
	if err != nil {
		return RuntimeReplayExportResponse{}, err
	}
	filtered := events.Events
	if req.SessionID != "" {
		filtered = filterReplayEvents(filtered, req.SessionID, "")
	}
	resp := RuntimeReplayExportResponse{
		SessionID:        req.SessionID,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Source:           "runtime_audit_events+" + source,
		SnapshotRequired: events.SnapshotRequired,
		FirstSequence:    events.FirstSequence,
		LastSequence:     events.LastSequence,
		Events:           filtered,
		Audit:            audit.Events,
	}
	resp.Summary = buildRuntimeReplaySummary(RuntimeAuditTurnSummary{}, filtered, audit.Events)
	r.attachRuntimeReplayRefs(ctx, &resp.Summary, RuntimeRefListRequest{SessionID: req.SessionID})
	resp.Summary.Recovery.SnapshotRequired = events.SnapshotRequired || resp.Summary.Recovery.SnapshotRequired
	resp.Summary.Recovery.LastEventSequence = events.LastSequence
	return resp, nil
}

func (r *runtimeService) attachRuntimeReplayRefs(ctx context.Context, summary *RuntimeReplayExportSummary, req RuntimeRefListRequest) {
	if summary == nil {
		return
	}
	store, err := r.ensureRuntimeRefStore(ctx)
	if err != nil {
		return
	}
	refs, err := store.List(ctx, req)
	if err != nil {
		return
	}
	for _, ref := range refs {
		switch ref.Kind {
		case runtimeRefKindArtifact:
			summary.ArtifactRefs = appendRuntimeReplayRef(summary.ArtifactRefs, ref)
		case runtimeRefKindTaskArtifact:
			summary.ArtifactRefs = appendRuntimeReplayRef(summary.ArtifactRefs, ref)
			summary.TaskArtifactRefs = appendRuntimeReplayRef(summary.TaskArtifactRefs, ref)
		case runtimeRefKindCompactOriginalOutput:
			summary.OutputRefs = appendRuntimeReplayRef(summary.OutputRefs, ref)
			summary.CompactOutputRefs = appendRuntimeReplayRef(summary.CompactOutputRefs, ref)
		default:
			summary.OutputRefs = appendRuntimeReplayRef(summary.OutputRefs, ref)
		}
	}
}

func (r *runtimeService) replayEventsTurn(ctx context.Context, turnID string, after int64) (RuntimeEventsResponse, string, error) {
	if r.eventStore.db != nil {
		events, err := r.eventStore.ListTurn(ctx, turnID, after)
		if err != nil {
			return RuntimeEventsResponse{}, "", err
		}
		return events, "runtime_events", nil
	}
	events, err := r.Events(ctx, after)
	if err != nil {
		return RuntimeEventsResponse{}, "", err
	}
	events.Events = filterReplayEvents(events.Events, "", turnID)
	return events, "runtime_event_buffer", nil
}

func (r *runtimeService) replayEventsSession(ctx context.Context, sessionID string, after int64) (RuntimeEventsResponse, string, error) {
	if r.eventStore.db != nil {
		var (
			events RuntimeEventsResponse
			err    error
		)
		if sessionID == "" {
			events, err = r.eventStore.List(ctx, after)
		} else {
			events, err = r.eventStore.ListSession(ctx, sessionID, after)
		}
		if err != nil {
			return RuntimeEventsResponse{}, "", err
		}
		return events, "runtime_events", nil
	}
	events, err := r.Events(ctx, after)
	if err != nil {
		return RuntimeEventsResponse{}, "", err
	}
	events.Events = filterReplayEvents(events.Events, sessionID, "")
	return events, "runtime_event_buffer", nil
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
		Worktrees:         auditSummary.Worktrees,
		ToolCalls:         auditSummary.ToolCalls,
		EventCounts:       map[string]int{},
		AuditCounts:       map[string]int{},
		Redacted:          true,
	}
	for _, event := range events {
		summary.EventCounts[event.Type]++
		switch event.Type {
		case runtimeapi.EventToolSearchPerformed:
			search := runtimeReplayToolSearchFromEvent(event)
			summary.ToolSearches = append(summary.ToolSearches, search)
			if search.Guardrail != "" {
				summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, search.Guardrail)
			}
			for _, omission := range asSlice(event.Payload["omitted"]) {
				item := asMap(omission)
				if stringFromMap(item, "reason") == "policy_denied" {
					summary.ToolDiscovery.Denied = appendUniqueStrings(summary.ToolDiscovery.Denied, stringFromMap(item, "name"))
				}
			}
			summary.ToolDiscovery.BudgetImpact = mergeToolSchemaBudgetImpact(summary.ToolDiscovery.BudgetImpact, search.BudgetImpact)
		case runtimeapi.EventToolDiscoverySelected:
			summary.ToolDiscovery.Selected = appendUniqueStrings(summary.ToolDiscovery.Selected, stringSliceFromMap(event.Payload, "selected")...)
			summary.ToolDiscovery.BudgetImpact = mergeToolSchemaBudgetImpact(summary.ToolDiscovery.BudgetImpact, runtimeToolSchemaBudgetImpactFromPayload(event.Payload["budget"]))
		case runtimeapi.EventToolDiscoveryOmitted:
			summary.ToolDiscovery.Omitted = appendUniqueStrings(summary.ToolDiscovery.Omitted, stringSliceFromMap(event.Payload, "omitted")...)
			summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, stringFromMap(event.Payload, "reason"))
			summary.ToolDiscovery.BudgetImpact = mergeToolSchemaBudgetImpact(summary.ToolDiscovery.BudgetImpact, RuntimeToolSchemaBudgetImpact{Omitted: replayBudgetBucketFromPayload(event.Payload["budget"])})
		case runtimeapi.EventSchedulerDeadlockPrevented:
			summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, stringFromMap(event.Payload, "reason"))
		case runtimeapi.EventTaskMessageCreated, runtimeapi.EventTaskMessageDelivered, runtimeapi.EventTaskMessageProcessed, runtimeapi.EventTaskMessageRejected:
			summary.AgentTaskMessages = append(summary.AgentTaskMessages, runtimeReplayTaskMessageFromEvent(event))
		case runtimeapi.EventTaskArtifactCreated:
			summary.AgentTaskMessages = append(summary.AgentTaskMessages, runtimeReplayTaskMessageFromEvent(event))
			summary.AgentTaskArtifacts = appendUniqueStrings(summary.AgentTaskArtifacts, stringSliceFromMap(event.Payload, "artifact_refs")...)
		case runtimeapi.EventOutputRefCreated, runtimeapi.EventToolOutputRefCreated:
			if ref := runtimeRefFromReplayPayload(event.Payload); ref.ID != "" {
				summary.OutputRefs = appendRuntimeReplayRef(summary.OutputRefs, ref)
				if ref.Kind == runtimeRefKindCompactOriginalOutput {
					summary.CompactOutputRefs = appendRuntimeReplayRef(summary.CompactOutputRefs, ref)
				}
			}
		case runtimeapi.EventArtifactRefCreated:
			if ref := runtimeRefFromReplayPayload(event.Payload); ref.ID != "" {
				summary.ArtifactRefs = appendRuntimeReplayRef(summary.ArtifactRefs, ref)
				if ref.Kind == runtimeRefKindTaskArtifact {
					summary.TaskArtifactRefs = appendRuntimeReplayRef(summary.TaskArtifactRefs, ref)
				}
			}
		case runtimeapi.EventTaskResultUpdated:
			summary.AgentTaskResults = append(summary.AgentTaskResults, runtimeReplayTaskResultFromEvent(event))
		case runtimeapi.EventBudgetUpdated:
			summary.Budget = runtimeBudgetReportFromPayload(event.Payload)
		case runtimeapi.EventWorktreeCreated, runtimeapi.EventWorktreeEntered, runtimeapi.EventWorktreeExited, runtimeapi.EventWorktreeCleaned, runtimeapi.EventWorktreeCleanupFailed, runtimeapi.EventWorktreePolicyDenied, runtimeapi.EventWorktreeRecovered, runtimeapi.EventWorktreeMissingPath, runtimeapi.EventWorktreePreserved:
			if wt := runtimeWorktreeFromPayload(event.Payload); wt.ID != "" {
				summary.Worktrees = appendRuntimeReplayWorktree(summary.Worktrees, wt)
			}
		case runtimeapi.EventCapabilityLoading:
			summary.Capabilities.Started = appendUniqueStrings(summary.Capabilities.Started, runtimeReplayLifecycleName(event))
		case runtimeapi.EventCapabilityLoaded:
			summary.Capabilities.Loaded = appendUniqueStrings(summary.Capabilities.Loaded, runtimeReplayLifecycleName(event))
		case runtimeapi.EventCapabilityFailed:
			summary.Capabilities.Failed = appendUniqueStrings(summary.Capabilities.Failed, runtimeReplayLifecycleName(event))
		case runtimeapi.EventSkillDiscoveryStarted, runtimeapi.EventSkillActivated, runtimeapi.EventSkillActivationAllowed, runtimeapi.EventSkillContextInjected:
			summary.Skills.Started = appendUniqueStrings(summary.Skills.Started, runtimeReplayLifecycleName(event))
			if event.Type == runtimeapi.EventSkillActivationAllowed {
				summary.Skills.Allowed = appendUniqueStrings(summary.Skills.Allowed, runtimeReplayLifecycleName(event))
			}
		case runtimeapi.EventSkillDiscoveryCompleted:
			summary.Skills.Loaded = appendUniqueStrings(summary.Skills.Loaded, runtimeReplayLifecycleName(event))
		case runtimeapi.EventSkillDiscoveryFailed, runtimeapi.EventSkillActivationFailed:
			summary.Skills.Failed = appendUniqueStrings(summary.Skills.Failed, runtimeReplayLifecycleName(event))
		case runtimeapi.EventSkillDisabled, runtimeapi.EventSkillContextOmitted:
			summary.Skills.Disabled = appendUniqueStrings(summary.Skills.Disabled, runtimeReplayLifecycleName(event))
		case runtimeapi.EventSkillActivationDenied:
			summary.Skills.Denied = appendUniqueStrings(summary.Skills.Denied, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPServerStarting, runtimeapi.EventMCPServerLazyStarted:
			summary.MCP.Started = appendUniqueStrings(summary.MCP.Started, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPServerConnected:
			summary.MCP.Loaded = appendUniqueStrings(summary.MCP.Loaded, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPServerFailed, runtimeapi.EventMCPServerLazyFailed:
			summary.MCP.Failed = appendUniqueStrings(summary.MCP.Failed, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPServerBlocked:
			summary.MCP.Failed = appendUniqueStrings(summary.MCP.Failed, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPServerDisabled:
			summary.MCP.Disabled = appendUniqueStrings(summary.MCP.Disabled, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPToolsUpdated, runtimeapi.EventMCPResourcesUpdated, runtimeapi.EventMCPPromptsUpdated:
			summary.MCP.Updated = appendUniqueStrings(summary.MCP.Updated, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPCapabilityAllowed:
			summary.MCP.Allowed = appendUniqueStrings(summary.MCP.Allowed, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPCapabilityDenied:
			summary.MCP.Denied = appendUniqueStrings(summary.MCP.Denied, runtimeReplayLifecycleName(event))
		case runtimeapi.EventMCPAuthRequested, runtimeapi.EventMCPAuthCompleted, runtimeapi.EventMCPAuthDenied, runtimeapi.EventMCPAuthFailed,
			runtimeapi.EventMCPElicitationRequested, runtimeapi.EventMCPElicitationCompleted, runtimeapi.EventMCPElicitationDenied, runtimeapi.EventMCPElicitationFailed:
			req := runtimeReplayMCPRequestFromEvent(event)
			summary.MCPRequests = appendRuntimeReplayMCPRequest(summary.MCPRequests, req)
			if req.Status == mcpRequestStatusPending {
				summary.Recovery.PendingMCPRequests++
			}
		case runtimeapi.EventPermissionPolicyApplied, runtimeapi.EventPolicyRuleMatched, runtimeapi.EventPolicyRuleDenied, runtimeapi.EventPolicyRuleAsk:
			summary.PolicyDecisions = append(summary.PolicyDecisions, runtimeReplayPolicyFromEvent(event))
		case runtimeapi.EventSandboxDecisionRecorded, runtimeapi.EventSandboxApplied, runtimeapi.EventSandboxUnavailable, runtimeapi.EventSandboxDenied, runtimeapi.EventSandboxFailed:
			if decision := runtimeSandboxDecisionFromPayload(event.Payload); decision.ID != "" {
				summary.SandboxDecisions = appendRuntimeReplaySandboxDecision(summary.SandboxDecisions, decision)
			}
		case runtimeapi.EventHookExecutionStarted, runtimeapi.EventHookExecutionCompleted, runtimeapi.EventHookExecutionSkipped, runtimeapi.EventHookExecutionBlocked, runtimeapi.EventHookExecutionFailed, runtimeapi.EventHookContextInjected, runtimeapi.EventHookInputRewritten:
			if hook := runtimeHookExecutionFromPayload(event.Payload); hook.ID != "" {
				summary.Hooks = appendRuntimeReplayHookExecution(summary.Hooks, hook)
			}
		case runtimeapi.EventPermissionRequested, runtimeapi.EventPermissionDecided:
			summary.PermissionEvents = append(summary.PermissionEvents, runtimeReplayPermissionFromEvent(event))
		case runtimeapi.EventCompactBoundaryRecorded, runtimeapi.EventCompactMicroCompleted, runtimeapi.EventCompactFullCompleted, runtimeapi.EventCompactFailed:
			if boundary := runtimeReplayCompactBoundaryFromEvent(event); boundary.ID != "" {
				summary.CompactBoundaries = appendRuntimeReplayCompactBoundary(summary.CompactBoundaries, boundary)
			}
		case runtimeapi.EventCompactOutputPreserved:
			if ref := runtimeRefFromReplayPayload(event.Payload); ref.ID != "" {
				summary.OutputRefs = appendRuntimeReplayRef(summary.OutputRefs, ref)
				summary.CompactOutputRefs = appendRuntimeReplayRef(summary.CompactOutputRefs, ref)
			}
		case runtimeapi.EventContextReinjected, runtimeapi.EventContextSourceSkipped, runtimeapi.EventContextSourceFailed:
			attachRuntimeReplayReinjectedRef(&summary, event)
		case runtimeapi.EventReadFileStale, runtimeapi.EventReadFileMissing, runtimeapi.EventReadFileRecorded:
			summary.ReadFiles = appendRuntimeReplayReadFile(summary.ReadFiles, runtimeReplayReadFileFromEvent(event))
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
			extra := asMap(audit.Payload["extra"])
			summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, stringFromMap(extra, "guardrail"))
			summary.ToolDiscovery.BudgetImpact = mergeToolSchemaBudgetImpact(summary.ToolDiscovery.BudgetImpact, runtimeToolSchemaBudgetImpactFromPayload(extra["budget_impact"]))
			for _, omission := range asSlice(asMap(audit.Payload["extra"])["omitted"]) {
				item := asMap(omission)
				if stringFromMap(item, "reason") == "policy_denied" {
					summary.ToolDiscovery.Denied = appendUniqueStrings(summary.ToolDiscovery.Denied, stringFromMap(item, "name"))
				}
			}
		case "tool_discovery_selected":
			extra := asMap(audit.Payload["extra"])
			summary.ToolDiscovery.Selected = appendUniqueStrings(summary.ToolDiscovery.Selected, stringSliceFromMap(extra, "selected")...)
			summary.ToolDiscovery.Omitted = appendUniqueStrings(summary.ToolDiscovery.Omitted, stringSliceFromMap(extra, "omitted_names")...)
			summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, stringFromMap(extra, "reason"))
			summary.ToolDiscovery.BudgetImpact = mergeToolSchemaBudgetImpact(summary.ToolDiscovery.BudgetImpact, runtimeToolSchemaBudgetImpactFromPayload(extra["budget"]))
		case "scheduler_deadlock_prevented":
			extra := asMap(audit.Payload["extra"])
			summary.ToolDiscovery.GuardrailReasons = appendUniqueStrings(summary.ToolDiscovery.GuardrailReasons, stringFromMap(extra, "reason"))
		case "permission_policy_applied":
			summary.PolicyDecisions = append(summary.PolicyDecisions, runtimeReplayPolicyFromAudit(audit))
		case "sandbox_decision_recorded":
			if decision := runtimeSandboxDecisionFromAudit(audit); decision.ID != "" {
				summary.SandboxDecisions = appendRuntimeReplaySandboxDecision(summary.SandboxDecisions, decision)
			}
		case "hook_execution_started", "hook_execution_completed", "hook_execution_skipped", "hook_execution_blocked", "hook_execution_failed", "hook_execution_interrupted", "hook_context_injected", "hook_input_rewritten":
			if hook := runtimeHookExecutionFromAudit(audit); hook.ID != "" {
				summary.Hooks = appendRuntimeReplayHookExecution(summary.Hooks, hook)
			}
		case "permission_requested":
			summary.PermissionEvents = append(summary.PermissionEvents, runtimeReplayPermissionFromAudit(audit))
		case "mcp_auth_pending", "mcp_auth_completed", "mcp_auth_denied", "mcp_auth_failed", "mcp_auth_cancelled", "mcp_elicitation_pending", "mcp_elicitation_completed", "mcp_elicitation_denied", "mcp_elicitation_failed", "mcp_elicitation_cancelled":
			if req := runtimeReplayMCPRequestFromAudit(audit); req.RequestID != "" {
				summary.MCPRequests = appendRuntimeReplayMCPRequest(summary.MCPRequests, req)
			}
		case "task_message_created", "task_artifact_created", "task_message_delivered", "task_message_processed", "task_message_rejected":
			if msg := runtimeReplayTaskMessageFromAudit(audit); msg.ID != "" {
				summary.AgentTaskMessages = append(summary.AgentTaskMessages, msg)
			}
		case "task_result_updated":
			if result := runtimeReplayTaskResultFromAudit(audit); result.TaskID != "" {
				summary.AgentTaskResults = append(summary.AgentTaskResults, result)
			}
		case "worktree_created", "worktree_entered", "worktree_exited", "worktree_cleaned", "worktree_cleanup_failed", "worktree_policy_denied", "worktree_recovered", "worktree_missing_path", "worktree_preserved", "worktree_recovery_error":
			if wt := runtimeWorktreeFromPayload(asMap(asMap(audit.Payload["extra"])["worktree"])); wt.ID != "" {
				summary.Worktrees = appendRuntimeReplayWorktree(summary.Worktrees, wt)
			}
		case "compact_full_completed", "compact_full_failed", "compact_micro_completed", "compact_micro_failed", "compact_boundary_recorded", "compact_full_recorded":
			if boundary := runtimeCompactBoundaryFromPayload(asMap(audit.Payload["compact_boundary"])); boundary.ID != "" {
				summary.CompactBoundaries = appendRuntimeReplayCompactBoundary(summary.CompactBoundaries, boundary)
			}
		case "context_reinjected", "context_source_skipped", "context_source_failed":
			if ref := runtimeReinjectedRefFromPayload(asMap(asMap(audit.Payload["extra"])["reinjected_ref"])); ref.ID != "" {
				boundaryID := stringFromMap(asMap(audit.Payload["extra"]), "compact_id")
				attachRuntimeReplayReinjectedRefToBoundary(&summary, boundaryID, ref)
			}
		}
	}
	return summary
}

func appendRuntimeReplayHookExecution(items []RuntimeHookExecution, hook RuntimeHookExecution) []RuntimeHookExecution {
	if hook.ID == "" {
		return items
	}
	for i := range items {
		if items[i].ID == hook.ID {
			if hook.CompletedAt == 0 && items[i].CompletedAt > 0 {
				hook.CompletedAt = items[i].CompletedAt
			}
			if hook.Status == "" {
				hook.Status = items[i].Status
			}
			items[i] = hook
			return items
		}
	}
	return append(items, hook)
}

func appendRuntimeReplayReadFile(items []RuntimeReadFileState, file RuntimeReadFileState) []RuntimeReadFileState {
	if file.Path == "" {
		return items
	}
	for i := range items {
		if items[i].Path == file.Path && items[i].SessionID == file.SessionID {
			items[i] = file
			return items
		}
	}
	return append(items, file)
}

func runtimeReplayReadFileFromEvent(event RuntimeEvent) RuntimeReadFileState {
	return RuntimeReadFileState{
		SessionID:     event.SessionID,
		TurnID:        firstNonEmpty(event.TurnID, stringFromMap(event.Payload, "turn_id")),
		ToolCallID:    firstNonEmpty(event.ToolCallID, stringFromMap(event.Payload, "tool_call_id")),
		Path:          stringFromMap(event.Payload, "path"),
		ReadAt:        int64(intFromMap(event.Payload, "read_at")),
		Partial:       boolFromMap(event.Payload, "partial"),
		TokenEstimate: intFromMap(event.Payload, "token_estimate"),
		State:         strings.TrimPrefix(event.Type, "read_file."),
		Reason:        stringFromMap(event.Payload, "reason"),
		Diagnostics:   stringFromMap(event.Payload, "diagnostics"),
	}
}

func appendRuntimeReplayMCPRequest(items []RuntimeReplayMCPRequest, req RuntimeReplayMCPRequest) []RuntimeReplayMCPRequest {
	if req.RequestID == "" {
		return items
	}
	for i := range items {
		if items[i].RequestID == req.RequestID {
			items[i] = req
			return items
		}
	}
	return append(items, req)
}

func appendRuntimeReplaySandboxDecision(items []RuntimeSandboxDecision, decision RuntimeSandboxDecision) []RuntimeSandboxDecision {
	if decision.ID == "" {
		return items
	}
	for i := range items {
		if items[i].ID == decision.ID {
			items[i] = decision
			return items
		}
	}
	return append(items, decision)
}

func runtimeReplayLifecycleName(event RuntimeEvent) string {
	for _, key := range []string{"capability_id", "capabilityId", "server", "name", "skill", "id", "summary"} {
		if value := stringFromMap(event.Payload, key); value != "" {
			return value
		}
	}
	return event.Type
}

func appendRuntimeReplayCompactBoundary(items []RuntimeCompactBoundary, boundary RuntimeCompactBoundary) []RuntimeCompactBoundary {
	for i := range items {
		if items[i].ID == boundary.ID {
			if len(boundary.ReinjectedRefs) == 0 && len(items[i].ReinjectedRefs) > 0 {
				boundary.ReinjectedRefs = items[i].ReinjectedRefs
			}
			items[i] = boundary
			return items
		}
	}
	return append(items, boundary)
}

func runtimeReplayCompactBoundaryFromEvent(event RuntimeEvent) RuntimeCompactBoundary {
	boundary := RuntimeCompactBoundary{
		ID:         stringFromMap(event.Payload, "compact_id"),
		SessionID:  event.SessionID,
		TurnID:     event.TurnID,
		Kind:       stringFromMap(event.Payload, "kind"),
		Trigger:    stringFromMap(event.Payload, "trigger"),
		Status:     stringFromMap(event.Payload, "status"),
		SummaryRef: stringFromMap(event.Payload, "summary_ref"),
		Error:      stringFromMap(event.Payload, "error"),
	}
	return boundary
}

func attachRuntimeReplayReinjectedRef(summary *RuntimeReplayExportSummary, event RuntimeEvent) {
	ref := RuntimeReinjectedRef{
		ID:             stringFromMap(event.Payload, "source_id"),
		Kind:           stringFromMap(event.Payload, "kind"),
		Name:           stringFromMap(event.Payload, "name"),
		Path:           stringFromMap(event.Payload, "path"),
		URI:            stringFromMap(event.Payload, "uri"),
		Ref:            stringFromMap(event.Payload, "ref"),
		Status:         stringFromMap(event.Payload, "status"),
		Reason:         stringFromMap(event.Payload, "reason"),
		Error:          stringFromMap(event.Payload, "error"),
		ContentSummary: stringFromMap(event.Payload, "content_summary"),
		TokenEstimate:  intFromMap(event.Payload, "token_estimate"),
	}
	attachRuntimeReplayReinjectedRefToBoundary(summary, stringFromMap(event.Payload, "compact_id"), ref)
}

func attachRuntimeReplayReinjectedRefToBoundary(summary *RuntimeReplayExportSummary, boundaryID string, ref RuntimeReinjectedRef) {
	if ref.ID == "" {
		return
	}
	for i := range summary.CompactBoundaries {
		if summary.CompactBoundaries[i].ID == boundaryID {
			for _, existing := range summary.CompactBoundaries[i].ReinjectedRefs {
				if existing.ID == ref.ID && existing.Status == ref.Status {
					return
				}
			}
			summary.CompactBoundaries[i].ReinjectedRefs = append(summary.CompactBoundaries[i].ReinjectedRefs, ref)
			return
		}
	}
	summary.CompactBoundaries = append(summary.CompactBoundaries, RuntimeCompactBoundary{
		ID:             boundaryID,
		Kind:           compactKindFull,
		Status:         compactStatusRecorded,
		ReinjectedRefs: []RuntimeReinjectedRef{ref},
	})
}

func appendRuntimeReplayWorktree(items []RuntimeWorktree, wt RuntimeWorktree) []RuntimeWorktree {
	for i := range items {
		if items[i].ID == wt.ID {
			items[i] = wt
			return items
		}
	}
	return append(items, wt)
}

func appendRuntimeReplayRef(items []RuntimeRef, ref RuntimeRef) []RuntimeRef {
	for i := range items {
		if items[i].ID == ref.ID {
			items[i] = ref
			return items
		}
	}
	return append(items, ref)
}

func runtimeRefFromReplayPayload(payload map[string]any) RuntimeRef {
	ref := RuntimeRef{
		ID:                stringFromMap(payload, "id"),
		URI:               stringFromMap(payload, "uri"),
		SessionID:         stringFromMap(payload, "session_id"),
		TurnID:            stringFromMap(payload, "turn_id"),
		ToolCallID:        stringFromMap(payload, "tool_call_id"),
		TaskID:            stringFromMap(payload, "task_id"),
		SandboxDecisionID: stringFromMap(payload, "sandbox_decision_id"),
		SandboxMode:       stringFromMap(payload, "sandbox_mode"),
		SandboxStatus:     stringFromMap(payload, "sandbox_status"),
		Kind:              stringFromMap(payload, "kind"),
		MediaType:         stringFromMap(payload, "media_type"),
		ContentType:       stringFromMap(payload, "content_type"),
		SizeBytes:         int64(intFromMap(payload, "size_bytes")),
		EstimatedTokens:   intFromMap(payload, "estimated_tokens"),
		Preview:           stringFromMap(payload, "preview"),
		Summary:           stringFromMap(payload, "summary"),
		StorageKind:       stringFromMap(payload, "storage_kind"),
		RedactionStatus:   stringFromMap(payload, "redaction_status"),
		CreatedAt:         int64(intFromMap(payload, "created_at")),
	}
	ref.CanReadContent = ref.RedactionStatus == runtimeRefRedactionSafe
	return ref
}

func runtimeReplayTaskMessageFromEvent(event RuntimeEvent) RuntimeAgentTaskMessage {
	return RuntimeAgentTaskMessage{
		ID:                stringFromMap(event.Payload, "message_id"),
		TaskID:            stringFromMap(event.Payload, "task_id"),
		ParentTaskID:      stringFromMap(event.Payload, "parent_task_id"),
		ParentTurnID:      event.TurnID,
		ParentSessionID:   event.SessionID,
		ChildSessionID:    stringFromMap(event.Payload, "child_session_id"),
		Direction:         stringFromMap(event.Payload, "direction"),
		Kind:              stringFromMap(event.Payload, "kind"),
		Status:            stringFromMap(event.Payload, "status"),
		Sequence:          int64(intFromMap(event.Payload, "sequence")),
		ContentSummary:    stringFromMap(event.Payload, "summary"),
		RelatedToolCallID: event.ToolCallID,
		ArtifactRefs:      stringSliceFromMap(event.Payload, "artifact_refs"),
		Error:             stringFromMap(event.Payload, "error"),
		DeliveredAt:       int64(intFromMap(event.Payload, "delivered_at")),
		ProcessedAt:       int64(intFromMap(event.Payload, "processed_at")),
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
		Sequence:          int64(intFromMap(payload, "sequence")),
		ContentSummary:    stringFromMap(payload, "contentSummary"),
		RelatedToolCallID: stringFromMap(payload, "relatedToolCallId"),
		RelatedMessageID:  stringFromMap(payload, "relatedMessageId"),
		ArtifactRefs:      stringSliceFromMap(payload, "artifactRefs"),
		CreatedAt:         int64(intFromMap(payload, "createdAt")),
		DeliveredAt:       int64(intFromMap(payload, "deliveredAt")),
		ProcessedAt:       int64(intFromMap(payload, "processedAt")),
		Error:             stringFromMap(payload, "error"),
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
		Reason:       firstNonEmpty(stringFromMap(event.Payload, "guardrail_error"), stringFromMap(event.Payload, "max_results_reason")),
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
		Headless:          boolFromMap(event.Payload, "headless"),
		HeadlessReason:    stringFromMap(event.Payload, "headless_reason"),
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
		Headless:          boolFromMap(event.Payload, "policy_headless"),
		HeadlessReason:    firstNonEmpty(stringFromMap(event.Payload, "policy_headless_reason"), stringFromMap(asMap(event.Payload["extra"]), "headless_reason")),
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

func runtimeReplayMCPRequestFromEvent(event RuntimeEvent) RuntimeReplayMCPRequest {
	return RuntimeReplayMCPRequest{
		RequestID:      stringFromMap(event.Payload, "request_id"),
		Kind:           stringFromMap(event.Payload, "kind"),
		Server:         stringFromMap(event.Payload, "server"),
		CapabilityID:   stringFromMap(event.Payload, "capability_id"),
		SessionID:      firstNonEmpty(event.SessionID, stringFromMap(event.Payload, "session_id")),
		TurnID:         firstNonEmpty(event.TurnID, stringFromMap(event.Payload, "turn_id")),
		Status:         stringFromMap(event.Payload, "status"),
		Decision:       stringFromMap(event.Payload, "decision"),
		Error:          stringFromMap(event.Payload, "error"),
		PolicyDecision: stringFromMap(event.Payload, "policy_decision"),
		PolicyMode:     stringFromMap(event.Payload, "policy_mode"),
		PolicyProfile:  stringFromMap(event.Payload, "policy_profile"),
		PolicyReason:   stringFromMap(event.Payload, "policy_reason"),
		Redacted:       boolFromMap(event.Payload, "redacted"),
	}
}

func runtimeReplayMCPRequestFromAudit(event RuntimeAuditEvent) RuntimeReplayMCPRequest {
	extra := asMap(event.Payload["extra"])
	req := asMap(extra["mcp_request"])
	return RuntimeReplayMCPRequest{
		RequestID:      stringFromMap(req, "request_id"),
		Kind:           stringFromMap(req, "kind"),
		Server:         firstNonEmpty(stringFromMap(req, "server"), stringFromMap(event.Payload, "mcp_server")),
		CapabilityID:   firstNonEmpty(stringFromMap(req, "capability_id"), stringFromMap(event.Payload, "capability_id")),
		SessionID:      firstNonEmpty(event.SessionID, stringFromMap(req, "session_id")),
		TurnID:         firstNonEmpty(event.TurnID, stringFromMap(req, "turn_id")),
		Status:         firstNonEmpty(stringFromMap(req, "status"), stringFromMap(event.Payload, "mcp_status")),
		Error:          firstNonEmpty(stringFromMap(req, "error"), stringFromMap(event.Payload, "capability_error")),
		PolicyDecision: firstNonEmpty(stringFromMap(req, "policy_decision"), stringFromMap(event.Payload, "mcp_decision")),
		PolicyMode:     firstNonEmpty(stringFromMap(req, "policy_mode"), stringFromMap(event.Payload, "policy_mode")),
		PolicyProfile:  firstNonEmpty(stringFromMap(req, "policy_profile"), stringFromMap(event.Payload, "policy_profile")),
		PolicyReason:   firstNonEmpty(stringFromMap(req, "policy_reason"), stringFromMap(event.Payload, "mcp_reason")),
		Redacted:       true,
	}
}

func runtimeToolSchemaBudgetImpactFromPayload(value any) RuntimeToolSchemaBudgetImpact {
	payload := asMap(value)
	return RuntimeToolSchemaBudgetImpact{
		Selected: replayBudgetBucketFromPayload(payload["selected"]),
		Omitted:  replayBudgetBucketFromPayload(payload["omitted"]),
	}
}

func mergeToolSchemaBudgetImpact(base, next RuntimeToolSchemaBudgetImpact) RuntimeToolSchemaBudgetImpact {
	base.Selected.Count += next.Selected.Count
	base.Selected.EstimatedTokens += next.Selected.EstimatedTokens
	base.Omitted.Count += next.Omitted.Count
	base.Omitted.EstimatedTokens += next.Omitted.EstimatedTokens
	return base
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
