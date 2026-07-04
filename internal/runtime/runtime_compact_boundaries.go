package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/contextmgr"
)

// marshalJSON is a small shared helper for JSON-encoding a value into the
// text column format the runtime SQL stores use. It has no dependency on the
// (now removed) runtime_compact_boundaries store; it lives here because this
// file is the closest thing to "generic small store helpers" left after that
// store's deletion, and runtime_prompt_assembly_store.go still needs it.
func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to encode JSON: %w", err)
	}
	return string(data), nil
}

// Compact boundary kind/status vocabulary shared by the runtime output
// projection (runtime_output.go), replay export (runtime_replay_export.go),
// and audit redaction (runtime_audit.go). The boundaries themselves are no
// longer persisted through a runtime-owned SQL store: the single source of
// truth is contextmgr's runtime_context_boundaries table
// (internal/contextmgr/store.go). These read APIs project that data into the
// RuntimeCompactBoundary DTO the rest of the package already expects.
const (
	compactKindBoundary = "boundary"
	compactKindMicro    = "micro"
	compactKindFull     = "full"

	compactStatusRecorded  = "recorded"
	compactStatusCompleted = "completed"
	compactStatusSkipped   = "skipped"
	compactStatusFailed    = "failed"
)

// TurnCompactBoundaries lists the compact boundaries recorded for a turn,
// read from contextmgr's runtime_context_boundaries table (the boundary
// system of record since the context-compaction fix round).
func (r *runtimeService) TurnCompactBoundaries(ctx context.Context, turnID string) (RuntimeCompactBoundariesResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeCompactBoundariesResponse{}, errors.New("turn id is required")
	}
	boundaries, err := r.contextStore.ListBoundariesByTurn(ctx, turnID)
	if err != nil {
		// Graceful when the context store is not yet available (e.g. before
		// the workspace database connects), matching the historical
		// behavior of this read endpoint.
		return RuntimeCompactBoundariesResponse{}, nil
	}
	return RuntimeCompactBoundariesResponse{Boundaries: runtimeCompactBoundariesFromContext(boundaries)}, nil
}

// SessionCompactBoundaries lists the compact boundaries recorded for a
// session, read from contextmgr's runtime_context_boundaries table.
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
	boundaries, err := r.contextStore.ListBoundariesBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCompactBoundariesResponse{}, nil
	}
	return RuntimeCompactBoundariesResponse{Boundaries: runtimeCompactBoundariesFromContext(boundaries)}, nil
}

// runtimeCompactBoundariesFromContext maps a slice of contextmgr boundaries
// into the RuntimeCompactBoundary DTO. Unlike the leaner
// runtimeCompactBoundaryFromContextBoundary (runtime_output.go, used by the
// conversation item projection which only needs counts/summary text), these
// read APIs and the recovery status reproduce the full ref detail the
// deprecated runtime_compact_boundaries store used to return.
func runtimeCompactBoundariesFromContext(boundaries []contextmgr.Boundary) []RuntimeCompactBoundary {
	if len(boundaries) == 0 {
		return nil
	}
	out := make([]RuntimeCompactBoundary, 0, len(boundaries))
	for _, boundary := range boundaries {
		out = append(out, runtimeCompactBoundaryDetailFromContext(boundary))
	}
	return out
}

// runtimeCompactBoundaryDetailFromContext maps a single contextmgr boundary
// into RuntimeCompactBoundary, including ToolCallRefs/ReinjectedRefs (which
// contextmgr stores as plain ref strings, so they are projected here as
// minimal completed refs rather than the richer shape a live compact run
// records on RuntimeReinjectedRef/RuntimeCompactToolCallRef).
func runtimeCompactBoundaryDetailFromContext(boundary contextmgr.Boundary) RuntimeCompactBoundary {
	out := runtimeCompactBoundaryFromContextBoundary(boundary)
	for _, ref := range boundary.ToolCallRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out.ToolCallRefs = append(out.ToolCallRefs, RuntimeCompactToolCallRef{ToolCallID: ref, Ref: ref})
	}
	for _, ref := range boundary.ReinjectedRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out.ReinjectedRefs = append(out.ReinjectedRefs, RuntimeReinjectedRef{
			ID:     ref,
			Kind:   "legacy",
			Ref:    ref,
			Status: compactStatusCompleted,
			Reason: "legacy_reinjected_ref",
		})
	}
	return out
}

// runtimeCompactBoundaryFromPayload decodes a RuntimeCompactBoundary from a
// generic event/audit payload map. It backs replay export
// (runtime_replay_export.go) and audit redaction (runtime_audit.go), which
// reconstruct boundary data from previously stored event/audit JSON rather
// than a live store read.
func runtimeCompactBoundaryFromPayload(payload map[string]any) RuntimeCompactBoundary {
	boundary := RuntimeCompactBoundary{
		ID:          stringFromMap(payload, "id"),
		SessionID:   stringFromMap(payload, "sessionId"),
		TurnID:      stringFromMap(payload, "turnId"),
		Kind:        stringFromMap(payload, "kind"),
		Trigger:     stringFromMap(payload, "trigger"),
		Status:      stringFromMap(payload, "status"),
		SummaryRef:  stringFromMap(payload, "summaryRef"),
		Error:       stringFromMap(payload, "error"),
		CreatedAt:   int64(intFromMap(payload, "createdAt")),
		CompletedAt: int64(intFromMap(payload, "completedAt")),
	}
	if before, ok := payload["budgetBefore"].(map[string]any); ok {
		boundary.BudgetBefore = runtimeBudgetReportFromPayload(before)
	}
	if after, ok := payload["budgetAfter"].(map[string]any); ok {
		boundary.BudgetAfter = runtimeBudgetReportFromPayload(after)
	}
	boundary.MessageRefs = stringSliceFromMap(payload, "messageRefs")
	for _, raw := range asSlice(payload["reinjectedRefs"]) {
		switch item := raw.(type) {
		case string:
			boundary.ReinjectedRefs = append(boundary.ReinjectedRefs, RuntimeReinjectedRef{
				ID:     item,
				Kind:   "legacy",
				Ref:    item,
				Status: compactStatusCompleted,
				Reason: "legacy_reinjected_ref",
			})
		case map[string]any:
			boundary.ReinjectedRefs = append(boundary.ReinjectedRefs, runtimeReinjectedRefFromPayload(item))
		}
	}
	rawRefs, ok := payload["toolCallRefs"].([]any)
	if ok {
		for _, raw := range rawRefs {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			boundary.ToolCallRefs = append(boundary.ToolCallRefs, RuntimeCompactToolCallRef{
				ToolCallID:      stringFromMap(item, "toolCallId"),
				Name:            stringFromMap(item, "name"),
				Ref:             stringFromMap(item, "ref"),
				EstimatedTokens: intFromMap(item, "estimatedTokens"),
				Replacement:     stringFromMap(item, "replacement"),
				Preserved:       boolFromMap(item, "preserved"),
				Reason:          stringFromMap(item, "reason"),
			})
		}
	}
	return boundary
}

// runtimeReinjectedRefFromPayload decodes a RuntimeReinjectedRef from a
// generic event/audit payload map (see runtimeCompactBoundaryFromPayload).
func runtimeReinjectedRefFromPayload(payload map[string]any) RuntimeReinjectedRef {
	return RuntimeReinjectedRef{
		ID:             stringFromMap(payload, "id"),
		Kind:           stringFromMap(payload, "kind"),
		Name:           stringFromMap(payload, "name"),
		Path:           stringFromMap(payload, "path"),
		URI:            stringFromMap(payload, "uri"),
		Ref:            stringFromMap(payload, "ref"),
		Status:         stringFromMap(payload, "status"),
		Reason:         stringFromMap(payload, "reason"),
		Error:          stringFromMap(payload, "error"),
		ContentSummary: stringFromMap(payload, "contentSummary"),
		TokenEstimate:  intFromMap(payload, "tokenEstimate"),
	}
}
