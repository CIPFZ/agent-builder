package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

const (
	compactKindBoundary = "boundary"
	compactKindMicro    = "micro"
	compactKindFull     = "full"

	compactStatusRecorded  = "recorded"
	compactStatusCompleted = "completed"
	compactStatusSkipped   = "skipped"
	compactStatusFailed    = "failed"

	microCompactKeepRecent          = 2
	microCompactMinOutputTokens     = 128
	microCompactReplacementText     = "[Old tool result content compacted; original output preserved by runtime ToolCall ref]"
	fullCompactMinEstimatedTokens   = 1024
	fullCompactSummaryPreviewLimit  = 600
	postCompactMaxReadFiles         = 5
	postCompactMaxTokensPerReadFile = 5000
	postCompactReadFileTokenBudget  = 50000
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
	if stored.Kind == compactKindFull && stored.Status == compactStatusCompleted {
		eventType = runtimeapi.EventCompactFullCompleted
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
			"compact_id":           stored.ID,
			"kind":                 stored.Kind,
			"trigger":              stored.Trigger,
			"status":               stored.Status,
			"summary_ref":          stored.SummaryRef,
			"tool_ref_count":       len(stored.ToolCallRefs),
			"message_ref_count":    len(stored.MessageRefs),
			"reinjected_ref_count": len(stored.ReinjectedRefs),
			"summary":              compactBoundarySummary(stored),
			"error":                stored.Error,
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

func (r *runtimeService) maybeFullCompact(ctx context.Context, sessionID, turnID, model, prompt string, before RuntimeBudgetReport) (RuntimeBudgetReport, *RuntimeCompactBoundary) {
	after := before
	if sessionID == "" || turnID == "" || before.TotalEstimatedTokens < fullCompactMinEstimatedTokens {
		return after, nil
	}
	if r.runtime == nil || r.workspace == nil {
		return after, nil
	}
	now := time.Now().UTC()
	boundaryID := newCompactBoundaryID("full", turnID)
	boundary := RuntimeCompactBoundary{
		ID:           boundaryID,
		SessionID:    sessionID,
		TurnID:       turnID,
		Kind:         compactKindFull,
		Trigger:      "turn_finished",
		Status:       compactStatusCompleted,
		BudgetBefore: &before,
		SummaryRef:   "runtime://turns/" + turnID + "/compact/full/summary",
		CreatedAt:    now.UnixMilli(),
		CompletedAt:  now.UnixMilli(),
	}
	msgs, err := r.runtime.ListSessionMessages(ctx, r.workspace.ID, sessionID)
	if err != nil {
		return after, r.recordFailedFullCompact(ctx, boundary, err)
	}
	for _, msg := range msgs {
		runtimeMsg := toRuntimeMessage(toProtoMessage(msg))
		if runtimeMsg.ID != "" {
			boundary.MessageRefs = append(boundary.MessageRefs, runtimeMsg.ID)
		}
	}
	if r.toolCalls != nil {
		if calls, err := r.toolCalls.ListCalls(ctx, turnID); err == nil {
			for _, call := range calls {
				boundary.ToolCallRefs = append(boundary.ToolCallRefs, RuntimeCompactToolCallRef{
					ToolCallID:      call.ID,
					Name:            call.Name,
					Ref:             firstNonEmpty(call.CompactRef, "runtime://tool-calls/"+call.ID),
					EstimatedTokens: estimateRuntimeTokens(firstNonEmpty(call.ModelContent, call.OutputSummary, call.Structured, call.Stdout, call.Stderr, call.Error)),
					Preserved:       true,
					Reason:          "full_compact_provenance",
				})
			}
		}
	}
	summaryTokens := estimateRuntimeTokens(fullCompactSummaryText(prompt, boundary))
	after.Messages.EstimatedTokens = minInt(after.Messages.EstimatedTokens, summaryTokens)
	after.ToolOutputs.EstimatedTokens = minInt(after.ToolOutputs.EstimatedTokens, len(boundary.ToolCallRefs)*estimateRuntimeTokens("[tool output preserved by runtime ToolCall ref]"))
	after.TotalEstimatedTokens = after.InputBudget.EstimatedTokens +
		after.Messages.EstimatedTokens +
		after.ContextSources.EstimatedTokens +
		after.ToolSchemas.EstimatedTokens +
		after.Skills.EstimatedTokens +
		after.MCP.EstimatedTokens +
		after.ToolOutputs.EstimatedTokens +
		after.SelectedToolSchemas.EstimatedTokens
	after.UpdatedAt = now.UnixMilli()
	boundary.BudgetAfter = &after
	reinjected := r.reinjectPostCompactContext(ctx, sessionID, turnID, boundaryID)
	boundary.ReinjectedRefs = reinjected
	stored, err := r.recordCompactBoundary(ctx, boundary)
	if err != nil {
		return after, nil
	}
	return after, &stored
}

func (r *runtimeService) recordFailedFullCompact(ctx context.Context, boundary RuntimeCompactBoundary, cause error) *RuntimeCompactBoundary {
	boundary.Kind = compactKindFull
	boundary.Status = compactStatusFailed
	boundary.Error = cause.Error()
	boundary.CompletedAt = time.Now().UTC().UnixMilli()
	stored, err := r.recordCompactBoundary(ctx, boundary)
	if err != nil {
		return nil
	}
	return &stored
}

func fullCompactSummaryText(prompt string, boundary RuntimeCompactBoundary) string {
	return fmt.Sprintf("Full compact summary for continued runtime context. User prompt preview: %s. Provenance: %d messages and %d tool calls are preserved by runtime refs.",
		preview(prompt, fullCompactSummaryPreviewLimit), len(boundary.MessageRefs), len(boundary.ToolCallRefs))
}

func (r *runtimeService) reinjectPostCompactContext(ctx context.Context, sessionID, turnID, boundaryID string) []RuntimeReinjectedRef {
	var refs []RuntimeReinjectedRef
	contextResp, err := r.ContextSources(ctx)
	if err != nil {
		ref := RuntimeReinjectedRef{
			ID:     "context:sources",
			Kind:   "context_source",
			Status: compactStatusFailed,
			Reason: "context_sources_unavailable",
			Error:  err.Error(),
		}
		r.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, sessionID, turnID, boundaryID, ref)
		return append(refs, ref)
	}
	for _, source := range contextResp.Sources {
		ref := reinjectedRefFromContextSource(source)
		switch {
		case source.State == capabilityStateLoaded:
			ref.Status = compactStatusCompleted
			ref.Reason = firstNonEmpty(source.Reason, "post_compact_context_reinjected")
			r.publishReinjectionEvent(runtimeapi.EventContextReinjected, sessionID, turnID, boundaryID, ref)
		case source.State == capabilityStateFailed || source.Error != "":
			ref.Status = compactStatusFailed
			ref.Error = source.Error
			ref.Reason = firstNonEmpty(source.Reason, "context_source_failed")
			r.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, sessionID, turnID, boundaryID, ref)
		default:
			ref.Status = compactStatusSkipped
			ref.Reason = firstNonEmpty(source.Reason, "context_source_skipped")
			r.publishReinjectionEvent(runtimeapi.EventContextSourceSkipped, sessionID, turnID, boundaryID, ref)
		}
		refs = append(refs, ref)
	}
	refs = append(refs, r.reinjectReadFiles(ctx, sessionID, turnID, boundaryID)...)
	return refs
}

func (r *runtimeService) reinjectReadFiles(ctx context.Context, sessionID, turnID, boundaryID string) []RuntimeReinjectedRef {
	conn := r.compactBoundaries.db
	if conn == nil && r.turns.db != nil {
		conn = r.turns.db
	}
	if conn == nil {
		ref := RuntimeReinjectedRef{ID: "read_files:" + sessionID, Kind: "read_file_state", Status: compactStatusFailed, Reason: "read_file_store_unavailable", Error: "runtime database is not available"}
		r.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, sessionID, turnID, boundaryID, ref)
		return []RuntimeReinjectedRef{ref}
	}
	readFiles, err := db.New(conn).ListSessionReadFiles(ctx, sessionID)
	if err != nil {
		ref := RuntimeReinjectedRef{ID: "read_files:" + sessionID, Kind: "read_file_state", Status: compactStatusFailed, Reason: "read_file_state_failed", Error: err.Error()}
		r.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, sessionID, turnID, boundaryID, ref)
		return []RuntimeReinjectedRef{ref}
	}
	refs := make([]RuntimeReinjectedRef, 0, minInt(len(readFiles), postCompactMaxReadFiles))
	usedTokens := 0
	for i, rf := range readFiles {
		path := r.resolveReadFilePath(rf.Path)
		ref := RuntimeReinjectedRef{
			ID:     "read_file:" + filepath.ToSlash(path),
			Kind:   "read_file",
			Name:   filepath.Base(path),
			Path:   filepath.ToSlash(path),
			Ref:    "runtime://sessions/" + sessionID + "/read-files/" + filepath.ToSlash(rf.Path),
			Status: compactStatusCompleted,
			Reason: "recent_read_file",
		}
		if i >= postCompactMaxReadFiles {
			ref.Status = compactStatusSkipped
			ref.Reason = "post_compact_read_file_limit"
			r.publishReinjectionEvent(runtimeapi.EventContextSourceSkipped, sessionID, turnID, boundaryID, ref)
			refs = append(refs, ref)
			continue
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			ref.Status = compactStatusFailed
			ref.Reason = "read_file_missing_or_unreadable"
			ref.Error = statErr.Error()
			r.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, sessionID, turnID, boundaryID, ref)
			refs = append(refs, ref)
			continue
		}
		tokens := estimateRuntimeTokens(strings.Repeat("x", int(minInt64(info.Size(), int64(postCompactMaxTokensPerReadFile*4)))))
		if usedTokens+tokens > postCompactReadFileTokenBudget {
			ref.Status = compactStatusSkipped
			ref.Reason = "post_compact_read_file_token_budget"
			r.publishReinjectionEvent(runtimeapi.EventContextSourceSkipped, sessionID, turnID, boundaryID, ref)
			refs = append(refs, ref)
			continue
		}
		ref.TokenEstimate = tokens
		ref.ContentSummary = fmt.Sprintf("Read-file state restored (%d bytes).", info.Size())
		usedTokens += tokens
		r.publishReinjectionEvent(runtimeapi.EventContextReinjected, sessionID, turnID, boundaryID, ref)
		refs = append(refs, ref)
	}
	return refs
}

func (r *runtimeService) resolveReadFilePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := ""
	if r.workspace != nil {
		base = r.workspace.Path
	}
	if base == "" {
		if cwd, err := os.Getwd(); err == nil {
			base = cwd
		}
	}
	return filepath.Clean(filepath.Join(base, path))
}

func reinjectedRefFromContextSource(source RuntimeContextSource) RuntimeReinjectedRef {
	return RuntimeReinjectedRef{
		ID:             source.ID,
		Kind:           reinjectionKindForContextSource(source),
		Name:           source.Name,
		Path:           source.Path,
		URI:            source.URI,
		Ref:            reinjectionRefURI(source),
		ContentSummary: source.ContentSummary,
		TokenEstimate:  source.TokenEstimate,
	}
}

func reinjectionKindForContextSource(source RuntimeContextSource) string {
	if strings.EqualFold(source.Name, "AGENTS.md") || strings.EqualFold(filepath.Base(source.Path), "AGENTS.md") {
		return "agents"
	}
	if strings.EqualFold(source.Name, "CLAUDE.md") || strings.EqualFold(filepath.Base(source.Path), "CLAUDE.md") {
		return "claude"
	}
	if source.Kind != "" {
		return "context_source:" + source.Kind
	}
	return "context_source"
}

func reinjectionRefURI(source RuntimeContextSource) string {
	switch {
	case source.URI != "":
		return source.URI
	case source.Path != "":
		return "runtime://context-sources/" + filepath.ToSlash(source.Path)
	default:
		return "runtime://context-sources/" + source.ID
	}
}

func (r *runtimeService) publishReinjectionEvent(eventType, sessionID, turnID, boundaryID string, ref RuntimeReinjectedRef) {
	payload := map[string]any{
		"compact_id":      boundaryID,
		"source_id":       ref.ID,
		"kind":            ref.Kind,
		"name":            ref.Name,
		"path":            ref.Path,
		"uri":             ref.URI,
		"ref":             ref.Ref,
		"status":          ref.Status,
		"reason":          ref.Reason,
		"error":           ref.Error,
		"token_estimate":  ref.TokenEstimate,
		"content_summary": ref.ContentSummary,
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload:   payload,
	})
	r.writeAudit(auditEntry{
		RequestID: turnID,
		Event:     strings.ReplaceAll(eventType, ".", "_"),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Extra: map[string]any{
			"compact_id":     boundaryID,
			"reinjected_ref": ref,
		},
		Error: ref.Error,
	})
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
	if boundary.Kind == compactKindFull {
		return fmt.Sprintf("full compact %s: %d messages, %d tool calls, %d reinjected refs", boundary.Status, len(boundary.MessageRefs), len(boundary.ToolCallRefs), len(boundary.ReinjectedRefs))
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
