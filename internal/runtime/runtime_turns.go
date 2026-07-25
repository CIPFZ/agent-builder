package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agenttools "github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

const (
	runtimeTurnActionSourceKind          = "turn_action"
	runtimeTurnActionCancel              = "cancel_turn"
	runtimeTurnActionMarkInterruptedDone = "mark_interrupted_done"

	runtimeTurnActionReasonCancelled             = "turn_cancelled"
	runtimeTurnActionReasonInterruptedMarkedDone = "interrupted_marked_done"
)

func runtimeTurnActionRefreshTargets() []string {
	return []string{
		"status",
		"turn_activity",
		"session_activity_window",
		"session_activity",
		"tool_calls",
		"run",
		"run_projection",
		"run_transition_history",
		"diagnostics",
		"permissions",
		"mcp_requests",
		"run_scheduler_plan",
	}
}

func (r *runtimeService) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return RuntimeChatResponse{}, errors.New("prompt is required")
	}
	return r.SubmitUserInput(ctx, RuntimeUserInputRequest{
		SessionID: req.SessionID,
		ProjectID: req.ProjectID,
		Scope:     req.Scope,
		Workdir:   req.Workdir,
		Mode:      runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: prompt,
		}},
	})
}

func (r *runtimeService) submitNormalizedInput(ctx context.Context, normalized RuntimeNormalizedInput, items []RuntimeUserInputItem) (RuntimeChatResponse, error) {
	var err error
	if err = r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeChatResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	activeProjectID := r.activeProjectID
	requestSessionID := strings.TrimSpace(normalized.SessionID)
	draftScope := strings.TrimSpace(normalized.Scope)
	draftProjectID := strings.TrimSpace(normalized.ProjectID)
	draftWorkdir := strings.TrimSpace(normalized.Workdir)
	sessionID := requestSessionID
	if sessionID == "" && draftScope == "" && draftProjectID == "" {
		sessionID = r.sessionID
	}
	runCtx := r.runtimeCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	r.mu.Unlock()
	if sessionID == "" {
		projectID, scope, workdir, canonicalWorkdir, workdirExists, err := r.normalizeSessionCreationContext(ctx, normalized.ProjectID, normalized.Scope, normalized.Workdir, activeProjectID)
		if err != nil {
			return RuntimeChatResponse{}, err
		}
		sess, err := r.runtime.CreateSessionWithScopeAndWorkdir(ctx, wsID, "New chat", projectID, scope, workdir, canonicalWorkdir, workdirExists)
		if err != nil {
			return RuntimeChatResponse{}, fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = sess.ID
		r.mu.Lock()
		r.sessionID = sessionID
		r.mu.Unlock()
		r.publishRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventSessionCreated,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: sessionID,
			Payload: map[string]any{
				"title": sess.Title,
			},
		})
		draftProjectID = projectID
		draftScope = scope
		draftWorkdir = workdir
	} else if requestSessionID != "" {
		if _, err := r.runtime.GetSession(ctx, wsID, sessionID); err != nil {
			return RuntimeChatResponse{}, fmt.Errorf("failed to select Agent Builder session: %w", err)
		}
		r.mu.Lock()
		r.sessionID = sessionID
		r.mu.Unlock()
	}
	normalized.SessionID = sessionID
	normalized.ProjectID = draftProjectID
	normalized.Scope = draftScope
	normalized.Workdir = draftWorkdir
	prompt := strings.TrimSpace(normalized.Prompt)
	if prompt == "" && len(normalized.Attachments) > 0 {
		prompt = "[attachments]"
		normalized.Prompt = prompt
	}
	if err := r.ensureSessionTitle(ctx, wsID, sessionID, prompt); err != nil {
		slog.Warn("Failed to update desktop session title", "workspace_id", wsID, "session_id", sessionID, "error", err)
	}
	var prevented bool
	normalized, prevented, err = r.applyUserPromptSubmitHooks(ctx, normalized, items)
	if err != nil {
		return RuntimeChatResponse{}, err
	}
	prompt = strings.TrimSpace(normalized.Prompt)
	if prevented {
		if r.userInputs.db != nil {
			stored, err := r.userInputs.Upsert(ctx, normalized, items, "")
			if err != nil {
				return RuntimeChatResponse{}, err
			}
			normalized = stored
		}
		status, err := r.Status(ctx)
		if err != nil {
			return RuntimeChatResponse{}, err
		}
		return RuntimeChatResponse{
			RequestID:       normalized.ID,
			Status:          status,
			NormalizedInput: &normalized,
		}, nil
	}
	if r.userInputs.db != nil {
		stored, err := r.userInputs.Upsert(ctx, normalized, items, "")
		if err != nil {
			return RuntimeChatResponse{}, err
		}
		normalized = stored
	}
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errSelectedModelMissing) {
			return RuntimeChatResponse{}, errSelectedModelMissing
		}
		return RuntimeChatResponse{}, err
	}
	var run RuntimeRun
	if r.runs.db != nil {
		ensuredRun, err := r.runs.EnsureForSession(ctx, wsID, sessionID, preview(prompt, auditPreviewLimit), runtimeRunSourceUserPrompt)
		if err != nil {
			slog.Warn("Failed to ensure runtime run", "workspace_id", wsID, "session_id", sessionID, "error", err)
		} else {
			run = ensuredRun
		}
	}

	requestID := newRequestID()
	start := time.Now()
	normalized.CreatedAt = firstNonZeroInt64(normalized.CreatedAt, start.UnixMilli())
	usageBefore, err := r.sessionUsage(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeChatResponse{}, err
	}
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeChatResponse{}, err
	}

	r.mu.Lock()
	r.requests[requestID] = runtimeRequestState{
		SessionID:     sessionID,
		Provider:      status.Provider,
		Model:         status.Model,
		PromptPreview: preview(prompt, auditPreviewLimit),
		StartedAt:     start.UnixMilli(),
		Status:        "running",
		UsageBefore:   usageBefore,
	}
	r.sessionTurns[sessionID] = requestID
	r.mu.Unlock()
	// A new turn starts fresh: any boundary-anchored compact projection from
	// a previous turn is superseded by the session.SummaryMessageID slice.
	r.clearCompactStatesForSession(sessionID)
	startedTurn := RuntimeTurn{
		ID:            requestID,
		SessionID:     sessionID,
		Status:        turnStatusQueued,
		Provider:      status.Provider,
		Model:         status.Model,
		PromptPreview: preview(prompt, auditPreviewLimit),
		UsageBefore:   usageBefore,
		StartedAt:     start.UnixMilli(),
	}
	if _, err := r.turns.Upsert(ctx, startedTurn); err != nil {
		return RuntimeChatResponse{}, err
	}
	if r.userInputs.db != nil {
		stored, err := r.userInputs.Upsert(ctx, normalized, items, requestID)
		if err != nil {
			return RuntimeChatResponse{}, err
		}
		normalized = stored
	}
	if run.ID != "" {
		if _, err := r.runs.LinkTurn(ctx, run.ID, sessionID, requestID, start.UnixMilli()); err != nil {
			slog.Warn("Failed to link runtime run turn", "run_id", run.ID, "session_id", sessionID, "turn_id", requestID, "error", err)
		} else {
			if _, err := r.runtimeRunSchedulerDelegateUserTurn(ctx, run, startedTurn); err != nil {
				failed, failErr := r.failRuntimeRunScheduledTurn(ctx, startedTurn, err.Error())
				if failErr != nil {
					return RuntimeChatResponse{}, failErr
				}
				r.storeRuntimeEvent(newTurnFinishedRuntimeEvent(time.Now(), failed.ID, failed.SessionID, "failed", 0, failed.Provider, failed.Model, RuntimeUsage{}, failed.Error))
				return RuntimeChatResponse{}, err
			}
			r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceTurnStarted, startedTurn, "", runtimeRunStatusActive, "turn started")
		}
	}

	skills, mcpServers, mcpTools := r.runtimeAuditInventory(ctx)
	skillSummary := runtimeTurnSkillSummary(skills, string(r.currentPolicyMode()))
	r.recordTurnSkillActivation(sessionID, requestID, skillSummary)
	contextResp, contextErr := r.ContextSources(ctx)
	if contextErr != nil {
		slog.Debug("Runtime context source inventory unavailable", "error", contextErr)
	}
	contextSummary := r.recordTurnContextSources(sessionID, requestID, contextResp.Sources)
	budget := r.computeRuntimeBudget(ctx, sessionID, requestID, status.Model, len(prompt), &contextSummary)
	r.publishContextUsageUpdated(ctx, sessionID, requestID, status.Model, len(prompt), &contextSummary)

	slog.Info("Desktop chat queued", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "prompt_len", len(prompt))
	r.writeAudit(auditEntry{
		RequestID:      requestID,
		Event:          "started",
		Timestamp:      start.Format(time.RFC3339Nano),
		WorkspaceID:    wsID,
		SessionID:      sessionID,
		Provider:       status.Provider,
		Model:          status.Model,
		PromptLength:   len(prompt),
		PromptPreview:  preview(prompt, auditPreviewLimit),
		UsageBefore:    &usageBefore,
		Skills:         skills,
		SkillSummary:   &skillSummary,
		ContextSummary: &contextSummary,
		Budget:         &budget,
		MCPServers:     mcpServers,
		MCPTools:       mcpTools,
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventTurnStarted,
		CreatedAt: start.UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    requestID,
		Payload: map[string]any{
			"provider":       status.Provider,
			"model":          status.Model,
			"prompt_length":  len(prompt),
			"prompt_preview": preview(prompt, 160),
			"input_id":       normalized.ID,
			"input_mode":     normalized.Mode,
			"usage_before":   usageBefore,
		},
	})
	if _, err := r.turns.Upsert(ctx, RuntimeTurn{
		ID:            requestID,
		SessionID:     sessionID,
		Status:        turnStatusRunning,
		Provider:      status.Provider,
		Model:         status.Model,
		PromptPreview: preview(prompt, auditPreviewLimit),
		UsageBefore:   usageBefore,
		StartedAt:     start.UnixMilli(),
	}); err != nil {
		return RuntimeChatResponse{}, err
	}

	userMessageMetadata := runtimeUserMessageMetadata(normalized)
	// Session ownership and cwd are persisted independently from the desktop's
	// currently selected project. Resolve the execution cwd from the Session so
	// switching the project picker cannot make an existing/new Turn run against
	// the process workspace (typically the agent-builder checkout).
	sessionWorkdir := ""
	if sess, sessionErr := r.runtime.GetSession(ctx, wsID, sessionID); sessionErr == nil {
		sessionWorkdir = strings.TrimSpace(sess.Workdir)
	}
	go r.runChat(runCtx, requestID, wsID, sessionID, sessionWorkdir, prompt, userMessageMetadata, start, usageBefore, status.Provider, status.Model)

	return RuntimeChatResponse{
		RequestID:       requestID,
		TurnID:          requestID,
		Status:          status,
		NormalizedInput: &normalized,
	}, nil
}

func (r *runtimeService) Turn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTurnResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTurnResponse{}, errors.New("turn id is required")
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	state, active := r.requests[turnID]
	r.mu.Unlock()
	if active && !state.Finished {
		return RuntimeTurnResponse{Turn: runtimeTurnFromRequestState(turnID, state)}, nil
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	if turn.LatestAssistantMessageID != "" {
		msgs, err := r.runtime.ListSessionMessages(ctx, wsID, turn.SessionID)
		if err != nil {
			return RuntimeTurnResponse{}, fmt.Errorf("failed to read turn messages: %w", err)
		}
		for _, msg := range msgs {
			if msg.ID == turn.LatestAssistantMessageID {
				turn.LatestAssistant = toRuntimeMessage(toAPITypeMessage(msg))
				break
			}
		}
	}
	var messages []RuntimeMessage
	if messageResp, err := r.sessionMessages(ctx, wsID, turn.SessionID); err == nil {
		messages = messageResp.Messages
	}
	var toolCalls []RuntimeToolCall
	if r.toolCalls != nil {
		if calls, err := r.toolCalls.ListCalls(ctx, turn.ID); err == nil {
			for _, call := range calls {
				toolCalls = append(toolCalls, toRuntimeToolCall(call))
			}
		}
	}
	var permissions []RuntimePermissionRequest
	if r.permissionStore.db != nil {
		if perms, err := r.permissionStore.ListBySession(ctx, turn.SessionID); err == nil {
			for _, perm := range perms {
				if perm.TurnID == turn.ID {
					permissions = append(permissions, perm)
				}
			}
		}
	}
	var events []RuntimeEvent
	if r.eventStore.db != nil {
		if resp, err := r.eventStore.ListTurn(ctx, turn.ID, 0); err == nil {
			events = resp.Events
		}
	} else {
		r.mu.Lock()
		for _, event := range r.events {
			if event.TurnID == turn.ID {
				events = append(events, event)
			}
		}
		r.mu.Unlock()
	}
	var hooks []RuntimeHookExecution
	if r.hookExecutions.db != nil {
		if listed, err := r.hookExecutions.List(ctx, RuntimeHookExecutionsRequest{TurnID: turn.ID}); err == nil {
			hooks = listed
		}
	}
	turn.Diagnostics = buildRuntimeTurnDiagnostics(turn, messages, toolCalls, permissions, events, hooks)
	turn.Interrupted = buildRuntimeInterruptedSummary(turn, turn.Diagnostics, toolCalls)
	return RuntimeTurnResponse{Turn: turn}, nil
}

func runtimeTurnFromRequestState(turnID string, state runtimeRequestState) RuntimeTurn {
	return RuntimeTurn{
		ID:              turnID,
		SessionID:       state.SessionID,
		Status:          runtimeTurnStatus(state),
		StartedAt:       state.StartedAt,
		FinishedAt:      state.FinishedAt,
		Provider:        state.Provider,
		Model:           state.Model,
		PromptPreview:   state.PromptPreview,
		UsageBefore:     state.UsageBefore,
		UsageAfter:      state.UsageAfter,
		UsageDelta:      state.UsageDelta,
		LatestMessageID: state.LatestMessageID,
		Error:           state.Error,
	}
}

func (r *runtimeService) Turns(ctx context.Context, status string) (RuntimeTurnsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeTurnsResponse{}, err
	}
	turns, err := r.turns.List(ctx, status)
	if err != nil {
		return RuntimeTurnsResponse{}, err
	}
	return RuntimeTurnsResponse{Turns: turns}, nil
}

func (r *runtimeService) Cancel(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	activeTurnID := r.sessionTurns[r.sessionID]
	r.mu.Unlock()

	return r.CancelTurn(ctx, activeTurnID)
}

func (r *runtimeService) CancelTurn(ctx context.Context, turnID string) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeStatus{}, errors.New("turn id is required")
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	state, ok := r.requests[turnID]
	r.mu.Unlock()
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	if !ok {
		state = runtimeRequestState{SessionID: turn.SessionID}
	}
	runStatusBefore := r.runtimeRunStatusForSession(ctx, turn.SessionID)

	if err := r.runtime.CancelSession(wsID, state.SessionID); err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to cancel session: %w", err)
	}
	now := time.Now()
	turn.Status = turnStatusCancelling
	if _, err := r.turns.Upsert(ctx, turn); err != nil {
		return RuntimeStatus{}, err
	}
	r.mu.Lock()
	state.Cancelled = true
	state.Finished = true
	state.Status = "cancelled"
	if state.FinishedAt == 0 {
		state.FinishedAt = now.UnixMilli()
	}
	r.requests[turnID] = state
	r.mu.Unlock()
	turn.FinishedAt = now.UnixMilli()
	turn.Status = turnStatusCancelled
	if _, err := r.turns.Upsert(ctx, turn); err != nil {
		return RuntimeStatus{}, err
	}
	if r.toolCalls != nil {
		calls, _ := r.toolCalls.ListCalls(ctx, turnID)
		for _, call := range calls {
			if isFinalToolCallStatus(string(call.Status)) {
				continue
			}
			_ = r.toolCalls.CancelCall(ctx, call.ID)
			cancelled, _ := r.toolCalls.GetCall(ctx, call.ID)
			r.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, cancelled, map[string]any{
				"name":    cancelled.Name,
				"summary": "turn cancelled",
				"status":  string(cancelled.Status),
			}))
			r.writeAudit(auditEntry{
				RequestID:   turnID,
				Event:       "tool_call_cancelled",
				Timestamp:   now.Format(time.RFC3339Nano),
				WorkspaceID: wsID,
				SessionID:   cancelled.SessionID,
				ToolCallID:  cancelled.ID,
				ToolCalls: []auditToolCall{{
					ID:      cancelled.ID,
					Name:    cancelled.Name,
					Input:   cancelled.InputSummary,
					Output:  "turn cancelled",
					IsError: true,
				}},
				Error: "turn cancelled",
			})
		}
	}
	cancelledPermissions := r.cancelPendingPermissionsForTurn(ctx, turnID, now)
	for _, perm := range cancelledPermissions {
		r.storeRuntimeEvent(runtimePermissionCancelledEvent(now, perm))
		r.writeAudit(auditEntry{
			RequestID:           turnID,
			Event:               "permission_cancelled",
			Timestamp:           now.Format(time.RFC3339Nano),
			WorkspaceID:         wsID,
			SessionID:           perm.SessionID,
			PermissionTool:      perm.ToolName,
			PermissionAction:    perm.Action,
			PermissionPath:      perm.Path,
			PermissionPolicy:    permissionStatusCancelled,
			PermissionRisk:      perm.Risk,
			PermissionReason:    firstNonEmpty(perm.Reason, perm.PolicyReason),
			PolicyMode:          perm.PolicyMode,
			PolicyProfile:       perm.PolicyProfile,
			PolicyRuleID:        perm.PolicyRuleID,
			PolicyRuleSource:    perm.PolicyRuleSource,
			PolicyScopeKind:     perm.PolicyScopeKind,
			PolicyScopeValue:    perm.PolicyScopeValue,
			PolicyTargetSummary: perm.PolicyTargetSummary,
			PermissionID:        perm.ID,
			ToolCallID:          perm.ToolCallID,
			Error:               "turn cancelled",
		})
	}
	if projection, err := r.reconcileRuntimeRunForSession(ctx, turn.SessionID); err == nil {
		r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceTurnCancelled, turn, runStatusBefore, firstNonEmpty(projection.Status, runtimeRunStatusCancelled), "turn cancelled")
	}
	r.writeAudit(auditEntry{
		RequestID:   turnID,
		Event:       "cancel_requested",
		Timestamp:   now.Format(time.RFC3339Nano),
		WorkspaceID: wsID,
		SessionID:   state.SessionID,
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventTurnCancelled,
		CreatedAt: now.UTC().Format(time.RFC3339Nano),
		SessionID: state.SessionID,
		TurnID:    turnID,
		Payload: map[string]any{
			"status": "cancelled",
		},
	})
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeStatus{}, err
	}
	return withRuntimeTurnAction(status, runtimeTurnActionCancel, runtimeTurnActionReasonCancelled), nil
}

func (r *runtimeService) cancelPendingPermissionsForTurn(ctx context.Context, turnID string, now time.Time) []RuntimePermissionRequest {
	var cancelled []RuntimePermissionRequest
	if r.permissionStore.db != nil {
		marked, err := r.permissionStore.CancelPendingByTurn(ctx, turnID, now.UnixMilli())
		if err != nil {
			slog.Warn("Failed to cancel pending runtime permissions", "turn_id", turnID, "error", err)
			return nil
		}
		cancelled = append(cancelled, marked...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[string]struct{}, len(cancelled))
	for _, perm := range cancelled {
		seen[perm.ID] = struct{}{}
		delete(r.permissions, perm.ID)
	}
	for id, pending := range r.permissions {
		if pending.Permission.TurnID != turnID {
			continue
		}
		perm := pending.Permission
		perm.Status = permissionStatusCancelled
		perm.DecidedAt = now.UnixMilli()
		if _, ok := seen[perm.ID]; !ok {
			cancelled = append(cancelled, perm)
			seen[perm.ID] = struct{}{}
		}
		delete(r.permissions, id)
	}
	return cancelled
}

func runtimePermissionCancelledEvent(now time.Time, perm RuntimePermissionRequest) runtimeapi.Event {
	return runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventPermissionDecided,
		CreatedAt:  now.UTC().Format(time.RFC3339Nano),
		SessionID:  perm.SessionID,
		TurnID:     perm.TurnID,
		ToolCallID: perm.ToolCallID,
		Payload: map[string]any{
			"permission_id":       perm.ID,
			"tool_name":           perm.ToolName,
			"action":              "cancel",
			"path":                perm.Path,
			"risk":                perm.Risk,
			"reason":              firstNonEmpty(perm.Reason, perm.PolicyReason, "turn cancelled"),
			"mode":                perm.PolicyMode,
			"matched_rule_id":     perm.PolicyRuleID,
			"matched_rule_source": perm.PolicyRuleSource,
			"scope_kind":          perm.PolicyScopeKind,
			"scope_value":         perm.PolicyScopeValue,
			"target_summary":      perm.PolicyTargetSummary,
			"status":              permissionStatusCancelled,
		},
	}
}

func withRuntimeTurnAction(status RuntimeStatus, action, reason string) RuntimeStatus {
	status.Action = runtimeTurnActionMetadata(action, reason)
	return status
}

func withRuntimeTurnResponseAction(resp RuntimeTurnResponse, action, reason string) RuntimeTurnResponse {
	resp.Action = runtimeTurnActionMetadata(action, reason)
	return resp
}

func runtimeTurnActionMetadata(action, reason string) *RuntimeWriteActionMetadata {
	return &RuntimeWriteActionMetadata{
		Accepted:       true,
		Reason:         reason,
		RefreshTargets: runtimeTurnActionRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeTurnActionSourceKind,
			Action:                action,
			WorkbenchOnly:         true,
			StartsWorker:          false,
			IdempotentBy:          "turn_id",
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_turns",
				"runtime_tool_calls",
				"runtime_events",
				"runtime_audit",
				"runtime_run_transitions",
				"runtime_run_projection",
				"session_activity",
			},
		},
	}
}

func (r *runtimeService) MarkInterruptedDone(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTurnResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTurnResponse{}, errors.New("turn id is required")
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	if turn.Status != turnStatusInterrupted {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s is not interrupted", turnID)
	}
	runStatusBefore := r.runtimeRunStatusForSession(ctx, turn.SessionID)
	now := time.Now().UTC()
	turn.Status = turnStatusCancelled
	turn.Error = firstNonEmpty(turn.Error, "interrupted turn marked done by user; no continuation required")
	if turn.FinishedAt == 0 {
		turn.FinishedAt = now.UnixMilli()
	}
	stored, err := r.turns.Upsert(ctx, turn)
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	if projection, err := r.reconcileRuntimeRunForSession(ctx, stored.SessionID); err == nil {
		r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceInterruptedMarkedDone, stored, runStatusBefore, firstNonEmpty(projection.Status, runtimeRunStatusCancelled), "interrupted turn marked done")
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryTurnDiscarded,
		CreatedAt: now.Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.ID,
		Payload: map[string]any{
			"status": "cancelled",
			"reason": "interrupted_marked_done",
			"action": runtimeTurnActionMarkInterruptedDone,
		},
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventTurnCancelled,
		CreatedAt: now.Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.ID,
		Payload: map[string]any{
			"status": "cancelled",
			"reason": "interrupted_marked_done",
		},
	})
	resp, err := r.Turn(ctx, stored.ID)
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	return withRuntimeTurnResponseAction(resp, runtimeTurnActionMarkInterruptedDone, runtimeTurnActionReasonInterruptedMarkedDone), nil
}

func (r *runtimeService) runChat(ctx context.Context, requestID, wsID, sessionID, workdir, prompt string, userMessageMetadata map[string]string, start time.Time, usageBefore RuntimeUsage, provider, model string) {
	if workdir != "" {
		ctx = context.WithValue(ctx, agenttools.EffectiveCWDContextKey, workdir)
	}
	err := r.runtime.SendMessage(ctx, wsID, apitypes.AgentMessage{
		SessionID: sessionID,
		TurnID:    requestID,
		Prompt:    prompt,
		Metadata:  userMessageMetadata,
	})
	duration := time.Since(start)
	usageAfter, usageErr := r.sessionUsage(context.Background(), wsID, sessionID)
	if usageErr != nil {
		slog.Error("Desktop chat usage unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", usageErr)
	}
	assistant, assistantErr := r.latestFinishedAssistantMessage(context.Background(), wsID, sessionID, requestID)
	if assistantErr != nil {
		slog.Warn("Desktop chat assistant message unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", assistantErr)
	}
	if titleErr := r.ensureSessionTitle(context.Background(), wsID, sessionID, prompt); titleErr != nil {
		slog.Warn("Failed to finalize desktop session title", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", titleErr)
	}

	r.mu.Lock()
	state := r.requests[requestID]
	state.Status = "completed"
	state.Finished = true
	state.FinishedAt = time.Now().UnixMilli()
	if err != nil {
		state.Status = "failed"
		state.Error = err.Error()
	}
	if state.Cancelled {
		state.Status = "cancelled"
	}
	state.UsageAfter = usageAfter
	state.UsageDelta = usageAfter.Sub(usageBefore)
	r.requests[requestID] = state
	r.mu.Unlock()
	// The turn reached a terminal state: drop its in-memory compact
	// projection state (persisted boundary + summary take over from here).
	r.clearCompactTurnState(sessionID, requestID)

	entry := auditEntry{
		RequestID:     requestID,
		Timestamp:     time.Now().Format(time.RFC3339Nano),
		WorkspaceID:   wsID,
		SessionID:     sessionID,
		Provider:      provider,
		Model:         model,
		DurationMS:    duration.Milliseconds(),
		PromptLength:  len(prompt),
		PromptPreview: preview(prompt, auditPreviewLimit),
	}
	usageDelta := usageAfter.Sub(usageBefore)
	entry.UsageBefore = &usageBefore
	entry.UsageAfter = &usageAfter
	entry.UsageDelta = &usageDelta
	if assistantErr == nil {
		runtimeMsg := toRuntimeMessage(assistant)
		entry.LatestAssistantID = runtimeMsg.ID
		entry.LatestAssistantFinish = runtimeMsg.Finished
		entry.FinishReason = runtimeMsg.FinishReason
		entry.ResponseLength = len(runtimeMsg.Content)
		entry.ResponsePreview = preview(runtimeMsg.Content, auditPreviewLimit)
		r.mu.Lock()
		state := r.requests[requestID]
		state.LatestMessageID = runtimeMsg.ID
		r.requests[requestID] = state
		r.mu.Unlock()
	}
	if toolCalls, toolErr := r.auditToolCalls(context.Background(), wsID, sessionID); toolErr != nil {
		slog.Warn("Desktop chat tool audit unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", toolErr)
	} else {
		entry.ToolCalls = toolCalls
	}
	budgetAfterTurn := r.computeRuntimeBudget(context.Background(), sessionID, requestID, model, len(prompt), nil)
	r.publishContextUsageUpdated(context.Background(), sessionID, requestID, model, len(prompt), nil)
	entry.Budget = &budgetAfterTurn
	if err != nil && !stateCancelled(r, requestID) {
		entry.Event = "failed"
		entry.Error = err.Error()
		slog.Error("Desktop chat failed", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "duration", duration.String(), "error", err)
	} else if stateCancelled(r, requestID) {
		entry.Event = "cancelled"
		slog.Info("Desktop chat cancelled", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "duration", duration.String())
	} else {
		entry.Event = "finished"
		slog.Info("Desktop chat finished", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "provider", provider, "model", model, "duration", duration.String(), "content_len", entry.ResponseLength, "finish_reason", entry.FinishReason)
	}
	r.writeAudit(entry)
	turnStatus := turnStatusCompleted
	if entry.Event == "failed" {
		turnStatus = turnStatusFailed
	} else if entry.Event == "cancelled" {
		turnStatus = turnStatusCancelled
	}
	// Tool-result messages are the durable completion evidence. Reconcile them
	// before publishing the terminal turn so canonical consumers never observe
	// a completed turn with a stale running tool call merely because recorder
	// callbacks arrived out of order.
	for _, call := range r.reconcileTerminalRuntimeToolCalls(context.Background(), sessionID, requestID) {
		eventType := runtimeapi.EventToolCallCompleted
		if call.Status == scheduler.ToolCallFailed {
			eventType = runtimeapi.EventToolCallFailed
		}
		r.storeRuntimeEvent(runtimeToolCallEvent(eventType, call, map[string]any{
			"name":    call.Name,
			"summary": call.OutputSummary,
			"status":  string(call.Status),
			"error":   call.Error,
		}))
	}
	runStatusBefore := r.runtimeRunStatusForSession(context.Background(), sessionID)
	finishedTurn := RuntimeTurn{
		ID:                       requestID,
		SessionID:                sessionID,
		Status:                   turnStatus,
		LatestAssistantMessageID: entry.LatestAssistantID,
		LatestMessageID:          entry.LatestAssistantID,
		Provider:                 provider,
		Model:                    model,
		PromptPreview:            preview(prompt, auditPreviewLimit),
		UsageBefore:              usageBefore,
		UsageAfter:               usageAfter,
		UsageDelta:               usageDelta,
		StartedAt:                start.UnixMilli(),
		FinishedAt:               time.Now().UnixMilli(),
		Error:                    entry.Error,
	}
	if _, persistErr := r.turns.Upsert(context.Background(), finishedTurn); persistErr != nil {
		r.recordPersistenceDiagnostic("runtime_turns.upsert_terminal", classifyPersistenceErrorCode(persistErr), persistErr, requestID)
		slog.Error("Failed to persist terminal runtime turn", "request_id", requestID, "session_id", sessionID, "error", persistErr)
	}
	if recoverable, ok := r.publishRecoverableErrorClassified(finishedTurn); ok {
		r.maybeStartReactiveProviderRecovery(context.Background(), finishedTurn, recoverable, userMessageMetadata)
	}
	if projection, err := r.reconcileRuntimeRunForSession(context.Background(), sessionID); err == nil {
		r.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceTurnFinished, finishedTurn, runStatusBefore, firstNonEmpty(projection.Status, runtimeRunStatusCompleted), "turn "+entry.Event)
	}
	r.storeRuntimeEvent(newUsageRuntimeEvent(time.Now(), requestID, sessionID, usageAfter, usageDelta))
	r.storeRuntimeEvent(newTurnFinishedRuntimeEvent(time.Now(), requestID, sessionID, entry.Event, duration, provider, model, usageDelta, entry.Error))
	r.releaseFinishedTurnMemory(sessionID, requestID)
}

// releaseFinishedTurnMemory drops execution-only indexes after their durable
// Turn, Message, ToolCall, and event projections have reached a terminal
// state. Historical reads use SQLite; retaining these maps would make the Go
// heap grow with every completed conversation.
func (r *runtimeService) releaseFinishedTurnMemory(sessionID, turnID string) {
	var toolCallIDs []string
	if r.toolCalls != nil {
		if calls, err := r.toolCalls.ListCalls(context.Background(), turnID); err == nil {
			toolCallIDs = make([]string, 0, len(calls))
			for _, call := range calls {
				toolCallIDs = append(toolCallIDs, call.ID)
			}
		}
	}

	r.mu.Lock()
	delete(r.requests, turnID)
	if r.sessionTurns[sessionID] == turnID {
		delete(r.sessionTurns, sessionID)
	}
	for messageID, cursor := range r.messageStream {
		if cursor != nil && cursor.sessionID == sessionID && cursor.turnID == turnID {
			delete(r.messageStream, messageID)
		}
	}
	for _, toolCallID := range toolCallIDs {
		delete(r.toolEvents, toolCallID)
	}
	r.mu.Unlock()
}

func (r *runtimeService) latestFinishedAssistantMessage(ctx context.Context, workspaceID, sessionID, turnID string) (apitypes.Message, error) {
	msgs, err := r.runtime.ListSessionMessages(ctx, workspaceID, sessionID)
	if err != nil {
		return apitypes.Message{}, fmt.Errorf("failed to read session messages: %w", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == message.Assistant && msgs[i].Metadata["turn_id"] == turnID && msgs[i].FinishPart() != nil {
			return toAPITypeMessage(msgs[i]), nil
		}
	}
	return apitypes.Message{}, errors.New("finished assistant response is not available")
}

func (r *runtimeService) runtimeAuditInventory(ctx context.Context) ([]RuntimeSkill, []RuntimeMCPServer, []RuntimeMCPTool) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		slog.Debug("Runtime audit inventory unavailable", "error", err)
		return nil, nil, nil
	}
	return r.runtimeSkillsFromWorkspaceConfig(cfg, r.desktopSkillPaths()...).Skills,
		runtimeMCPServersFromConfig(cfg).Servers,
		runtimeMCPToolsFromConfig(cfg, "").Tools
}

func (r *runtimeService) auditToolCalls(ctx context.Context, workspaceID, sessionID string) ([]auditToolCall, error) {
	msgs, err := r.runtime.ListSessionMessages(ctx, workspaceID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read session messages: %w", err)
	}
	byID := make(map[string]*auditToolCall)
	var calls []auditToolCall
	for _, msg := range msgs {
		for _, part := range toAPITypeMessage(msg).Parts {
			switch p := part.(type) {
			case apitypes.ToolCall:
				call := auditToolCall{
					ID:    p.ID,
					Name:  p.Name,
					Input: preview(p.Input, runtimePartPreviewLimit),
				}
				calls = append(calls, call)
				if p.ID != "" {
					byID[p.ID] = &calls[len(calls)-1]
				}
			case apitypes.ToolResult:
				if p.ToolCallID != "" {
					if call := byID[p.ToolCallID]; call != nil {
						if call.Name == "" {
							call.Name = p.Name
						}
						call.Output = preview(firstNonEmpty(p.Content, p.Data), runtimePartPreviewLimit)
						call.IsError = p.IsError
						continue
					}
				}
				calls = append(calls, auditToolCall{
					ID:      p.ToolCallID,
					Name:    p.Name,
					Output:  preview(firstNonEmpty(p.Content, p.Data), runtimePartPreviewLimit),
					IsError: p.IsError,
				})
			}
		}
	}
	return calls, nil
}

func (u RuntimeUsage) Sub(before RuntimeUsage) RuntimeUsage {
	return RuntimeUsage{
		InputTokens:         u.InputTokens - before.InputTokens,
		OutputTokens:        u.OutputTokens - before.OutputTokens,
		CacheReadTokens:     u.CacheReadTokens - before.CacheReadTokens,
		CacheCreationTokens: u.CacheCreationTokens - before.CacheCreationTokens,
		TotalTokens:         u.TotalTokens - before.TotalTokens,
		Cost:                u.Cost - before.Cost,
	}
}

func (r *runtimeService) runtimeRequestsLocked() RuntimeRequests {
	var out RuntimeRequests
	now := time.Now().UnixMilli()
	for requestID, state := range r.requests {
		if isFinalRuntimeRequestState(state) {
			continue
		}
		out.Running++
		if out.ActiveStartedAt == 0 || state.StartedAt < out.ActiveStartedAt {
			out.ActiveRequestID = requestID
			out.ActiveStartedAt = state.StartedAt
			out.ActiveDurationMS = now - state.StartedAt
		}
	}
	return out
}

func isFinalRuntimeRequestState(state runtimeRequestState) bool {
	if state.Finished || state.Cancelled || state.FinishedAt > 0 {
		return true
	}
	switch state.Status {
	case turnStatusCompleted, turnStatusFailed, turnStatusCancelled, turnStatusInterrupted:
		return true
	default:
		return false
	}
}

func runtimeTurnStatus(state runtimeRequestState) string {
	switch {
	case state.Status != "":
		return state.Status
	case state.Cancelled:
		return "cancelled"
	case state.Error != "":
		return "failed"
	case state.Finished:
		return "completed"
	default:
		return "running"
	}
}

func stateCancelled(r *runtimeService, requestID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[requestID].Cancelled
}

func (r *runtimeService) activeTurnForSession(sessionID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionTurns[sessionID]
}
