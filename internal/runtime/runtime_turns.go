package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeService) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return RuntimeChatResponse{}, errors.New("prompt is required")
	}
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeChatResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := firstNonEmpty(strings.TrimSpace(req.SessionID), r.sessionID)
	runCtx := r.runtimeCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	r.mu.Unlock()
	if sessionID == "" {
		sess, err := r.runtime.CreateSession(ctx, wsID, "New chat")
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
	} else if strings.TrimSpace(req.SessionID) != "" {
		if _, err := r.runtime.GetSession(ctx, wsID, sessionID); err != nil {
			return RuntimeChatResponse{}, fmt.Errorf("failed to select Crush session: %w", err)
		}
		r.mu.Lock()
		r.sessionID = sessionID
		r.mu.Unlock()
	}
	if err := r.ensureSessionTitle(ctx, wsID, sessionID, prompt); err != nil {
		slog.Warn("Failed to update desktop session title", "workspace_id", wsID, "session_id", sessionID, "error", err)
	}

	requestID := newRequestID()
	start := time.Now()
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
	if _, err := r.turns.Upsert(ctx, RuntimeTurn{
		ID:            requestID,
		SessionID:     sessionID,
		Status:        turnStatusQueued,
		Provider:      status.Provider,
		Model:         status.Model,
		PromptPreview: preview(prompt, auditPreviewLimit),
		UsageBefore:   usageBefore,
		StartedAt:     start.UnixMilli(),
	}); err != nil {
		return RuntimeChatResponse{}, err
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
	r.publishBudgetUpdated(sessionID, requestID, budget)
	r.recordTurnBudgetBoundary(ctx, sessionID, requestID, budget)

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

	go r.runChat(runCtx, requestID, wsID, sessionID, prompt, start, usageBefore, status.Provider, status.Model)

	return RuntimeChatResponse{
		RequestID: requestID,
		TurnID:    requestID,
		Status:    status,
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
	r.mu.Unlock()
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
				turn.LatestAssistant = toRuntimeMessage(toProtoMessage(msg))
				break
			}
		}
	}
	return RuntimeTurnResponse{Turn: turn}, nil
}

func (r *runtimeService) Turns(ctx context.Context, status string) (RuntimeTurnsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
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
	return r.Status(ctx)
}

func (r *runtimeService) runChat(ctx context.Context, requestID, wsID, sessionID, prompt string, start time.Time, usageBefore RuntimeUsage, provider, model string) {
	err := r.runtime.SendMessage(ctx, wsID, proto.AgentMessage{
		SessionID: sessionID,
		TurnID:    requestID,
		Prompt:    prompt,
	})
	duration := time.Since(start)
	usageAfter, usageErr := r.sessionUsage(context.Background(), wsID, sessionID)
	if usageErr != nil {
		slog.Error("Desktop chat usage unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", usageErr)
	}
	assistant, assistantErr := r.latestFinishedAssistantMessage(context.Background(), wsID, sessionID)
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
	budgetBeforeCompact := r.computeRuntimeBudget(context.Background(), sessionID, requestID, model, len(prompt), nil)
	budgetAfterCompact, compactBoundary := r.maybeMicroCompactToolOutputs(context.Background(), sessionID, requestID, budgetBeforeCompact)
	budgetAfterCompact, fullCompactBoundary := r.maybeFullCompact(context.Background(), sessionID, requestID, model, prompt, budgetAfterCompact)
	r.publishBudgetUpdated(sessionID, requestID, budgetAfterCompact)
	entry.Budget = &budgetAfterCompact
	if fullCompactBoundary != nil {
		entry.CompactBoundary = fullCompactBoundary
	} else if compactBoundary != nil {
		entry.CompactBoundary = compactBoundary
	}
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
	_, _ = r.turns.Upsert(context.Background(), RuntimeTurn{
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
	})
	r.storeRuntimeEvent(newUsageRuntimeEvent(time.Now(), requestID, sessionID, usageAfter, usageDelta))
	r.storeRuntimeEvent(newTurnFinishedRuntimeEvent(time.Now(), requestID, sessionID, entry.Event, duration, provider, model, usageDelta, entry.Error))
}

func (r *runtimeService) latestFinishedAssistantMessage(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
	msgs, err := r.runtime.ListSessionMessages(ctx, workspaceID, sessionID)
	if err != nil {
		return proto.Message{}, fmt.Errorf("failed to read session messages: %w", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == message.Assistant && msgs[i].FinishPart() != nil {
			return toProtoMessage(msgs[i]), nil
		}
	}
	return proto.Message{}, errors.New("finished assistant response is not available")
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
		for _, part := range toProtoMessage(msg).Parts {
			switch p := part.(type) {
			case proto.ToolCall:
				call := auditToolCall{
					ID:    p.ID,
					Name:  p.Name,
					Input: preview(p.Input, runtimePartPreviewLimit),
				}
				calls = append(calls, call)
				if p.ID != "" {
					byID[p.ID] = &calls[len(calls)-1]
				}
			case proto.ToolResult:
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
		PromptTokens:     u.PromptTokens - before.PromptTokens,
		CompletionTokens: u.CompletionTokens - before.CompletionTokens,
		TotalTokens:      u.TotalTokens - before.TotalTokens,
		Cost:             u.Cost - before.Cost,
	}
}

func (r *runtimeService) runtimeRequestsLocked() RuntimeRequests {
	var out RuntimeRequests
	now := time.Now().UnixMilli()
	for requestID, state := range r.requests {
		if state.Finished {
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
