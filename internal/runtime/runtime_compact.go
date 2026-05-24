package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

const (
	compactKindBoundary = "boundary"
	compactKindMicro    = "micro"

	compactStatusRecorded  = "recorded"
	compactStatusCompleted = "completed"
	compactStatusSkipped   = "skipped"
	compactStatusFailed    = "failed"

	microCompactKeepRecent      = 2
	microCompactMinOutputTokens = 128
	microCompactReplacementText = "[Old tool result content compacted; original output preserved by runtime ToolCall ref]"
)

func (r *runtimeService) TurnCompactBoundaries(ctx context.Context, turnID string) (RuntimeCompactBoundariesResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeCompactBoundariesResponse{}, errors.New("turn id is required")
	}
	if r.compactBoundaries.db == nil {
		return RuntimeCompactBoundariesResponse{}, nil
	}
	boundaries, err := r.compactBoundaries.ListByTurn(ctx, turnID)
	if err != nil {
		return RuntimeCompactBoundariesResponse{}, err
	}
	return RuntimeCompactBoundariesResponse{Boundaries: boundaries}, nil
}

func (r *runtimeService) SessionCompactBoundaries(ctx context.Context, sessionID string) (RuntimeCompactBoundariesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeCompactBoundariesResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	if sessionID == "" {
		return RuntimeCompactBoundariesResponse{}, errors.New("session id is required")
	}
	boundaries, err := r.compactBoundaries.ListBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCompactBoundariesResponse{}, err
	}
	return RuntimeCompactBoundariesResponse{Boundaries: boundaries}, nil
}

func (r *runtimeService) recordCompactBoundary(ctx context.Context, boundary RuntimeCompactBoundary) (RuntimeCompactBoundary, error) {
	if r.compactBoundaries.db == nil {
		db, err := r.workspaceDB(ctx)
		if err != nil {
			return RuntimeCompactBoundary{}, err
		}
		r.compactBoundaries = newRuntimeCompactBoundaryStore(db)
	}
	stored, err := r.compactBoundaries.Upsert(ctx, boundary)
	if err != nil {
		return RuntimeCompactBoundary{}, err
	}
	eventType := runtimeapi.EventCompactBoundaryRecorded
	if stored.Kind == compactKindMicro && stored.Status == compactStatusCompleted {
		eventType = runtimeapi.EventCompactMicroCompleted
	}
	if stored.Status == compactStatusFailed {
		eventType = runtimeapi.EventCompactFailed
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.TurnID,
		Payload: map[string]any{
			"compact_id":     stored.ID,
			"kind":           stored.Kind,
			"trigger":        stored.Trigger,
			"status":         stored.Status,
			"summary_ref":    stored.SummaryRef,
			"tool_ref_count": len(stored.ToolCallRefs),
			"summary":        compactBoundarySummary(stored),
			"error":          stored.Error,
		},
	})
	r.writeAudit(auditEntry{
		RequestID:       stored.TurnID,
		Event:           "compact_" + stored.Kind + "_" + stored.Status,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:       stored.SessionID,
		Budget:          stored.BudgetAfter,
		CompactBoundary: &stored,
		Error:           stored.Error,
	})
	return stored, nil
}

func (r *runtimeService) recordTurnBudgetBoundary(ctx context.Context, sessionID, turnID string, budget RuntimeBudgetReport) {
	if turnID == "" || sessionID == "" {
		return
	}
	boundary := RuntimeCompactBoundary{
		ID:           newCompactBoundaryID("budget", turnID),
		SessionID:    sessionID,
		TurnID:       turnID,
		Kind:         compactKindBoundary,
		Trigger:      "turn_budget",
		Status:       compactStatusRecorded,
		BudgetBefore: &budget,
		BudgetAfter:  &budget,
		SummaryRef:   "runtime://turns/" + turnID + "/budget",
		CreatedAt:    time.Now().UTC().UnixMilli(),
		CompletedAt:  time.Now().UTC().UnixMilli(),
	}
	_, _ = r.recordCompactBoundary(ctx, boundary)
}

func (r *runtimeService) maybeMicroCompactToolOutputs(ctx context.Context, sessionID, turnID string, before RuntimeBudgetReport) (RuntimeBudgetReport, *RuntimeCompactBoundary) {
	after := before
	if r.toolCalls == nil || turnID == "" {
		return after, nil
	}
	calls, err := r.toolCalls.ListCalls(ctx, turnID)
	if err != nil || len(calls) <= microCompactKeepRecent {
		return after, nil
	}
	cutoff := len(calls) - microCompactKeepRecent
	now := time.Now().UTC()
	var refs []RuntimeCompactToolCallRef
	for i, call := range calls {
		if i >= cutoff {
			continue
		}
		if call.Compacted || !isMicroCompactableToolCall(call) {
			continue
		}
		original := strings.TrimSpace(call.ModelContent + "\n" + call.OutputSummary + "\n" + call.Structured + "\n" + call.Stdout + "\n" + call.Stderr + "\n" + call.Error)
		tokens := estimateRuntimeTokens(original)
		if tokens < microCompactMinOutputTokens {
			continue
		}
		ref := fmt.Sprintf("runtime://tool-calls/%s/output", call.ID)
		call.Compacted = true
		call.CompactRef = ref
		call.CompactBoundaryID = newCompactBoundaryID("micro", turnID)
		call.CompactOriginalEstimatedTokens = tokens
		call.CompactedAt = now
		call.OutputSummary = microCompactReplacementText
		call.ModelContent = microCompactReplacementText
		if _, err := r.toolCalls.UpdateCall(ctx, call); err != nil {
			continue
		}
		after.ToolOutputs.EstimatedTokens -= maxInt(0, tokens-estimateRuntimeTokens(microCompactReplacementText))
		if after.ToolOutputs.EstimatedTokens < 0 {
			after.ToolOutputs.EstimatedTokens = 0
		}
		refs = append(refs, RuntimeCompactToolCallRef{
			ToolCallID:      call.ID,
			Name:            call.Name,
			Ref:             ref,
			EstimatedTokens: tokens,
			Replacement:     microCompactReplacementText,
			Reason:          "old_high_cost_tool_output",
		})
	}
	if len(refs) == 0 {
		return after, nil
	}
	after.TotalEstimatedTokens = before.TotalEstimatedTokens - (before.ToolOutputs.EstimatedTokens - after.ToolOutputs.EstimatedTokens)
	if after.TotalEstimatedTokens < 0 {
		after.TotalEstimatedTokens = 0
	}
	after.UpdatedAt = now.UnixMilli()
	boundary := RuntimeCompactBoundary{
		ID:           refs[0].Ref,
		SessionID:    sessionID,
		TurnID:       turnID,
		Kind:         compactKindMicro,
		Trigger:      "turn_finished",
		Status:       compactStatusCompleted,
		BudgetBefore: &before,
		BudgetAfter:  &after,
		SummaryRef:   "runtime://turns/" + turnID + "/compact/micro",
		ToolCallRefs: refs,
		CreatedAt:    now.UnixMilli(),
		CompletedAt:  now.UnixMilli(),
	}
	boundary.ID = newCompactBoundaryID("micro", turnID)
	for _, ref := range refs {
		if call, err := r.toolCalls.GetCall(ctx, ref.ToolCallID); err == nil {
			call.CompactBoundaryID = boundary.ID
			_, _ = r.toolCalls.UpdateCall(ctx, call)
		}
	}
	stored, err := r.recordCompactBoundary(ctx, boundary)
	if err != nil {
		return after, nil
	}
	return after, &stored
}

func isMicroCompactableToolCall(call scheduler.ToolCall) bool {
	if call.IsError {
		return false
	}
	switch strings.ToLower(call.Name) {
	case "bash", "grep", "glob", "read", "file_read", "webfetch", "websearch", "edit", "write":
		return true
	default:
		return call.Source == scheduler.ToolSourceShell
	}
}

func compactBoundarySummary(boundary RuntimeCompactBoundary) string {
	if boundary.Kind == compactKindMicro {
		return fmt.Sprintf("micro compact %s: %d tool outputs", boundary.Status, len(boundary.ToolCallRefs))
	}
	return fmt.Sprintf("compact boundary %s: %s", boundary.Status, boundary.Trigger)
}

func newCompactBoundaryID(kind, turnID string) string {
	return "compact_" + kind + "_" + strings.ReplaceAll(firstNonEmpty(turnID, newRequestID()), ":", "_")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
