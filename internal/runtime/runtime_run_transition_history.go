package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const runtimeRunTransitionHistorySourceKind = "run_transition_history"

func (r *runtimeService) RunTransitionHistory(ctx context.Context, req RuntimeRunTransitionHistoryRequest) (RuntimeRunTransitionHistoryResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunTransitionHistoryResponse{}, err
	}
	if r.transitions.db == nil {
		return RuntimeRunTransitionHistoryResponse{}, errors.New("runtime run transition database is not available")
	}
	transitions, err := r.runTransitionHistoryRows(ctx, req)
	if err != nil {
		return RuntimeRunTransitionHistoryResponse{}, err
	}
	return buildRuntimeRunTransitionHistoryResponse(transitions, strings.TrimSpace(req.Cursor), normalizeRuntimeRunTransitionHistoryLimit(req.Limit)), nil
}

func (r *runtimeService) runTransitionHistoryRows(ctx context.Context, req RuntimeRunTransitionHistoryRequest) ([]RuntimeRunTransition, error) {
	runID := strings.TrimSpace(req.RunID)
	sessionID := strings.TrimSpace(req.SessionID)
	turnID := strings.TrimSpace(req.TurnID)
	switch {
	case runID != "":
		return r.transitions.ListByRun(ctx, runID)
	case sessionID != "":
		return r.transitions.ListBySession(ctx, sessionID)
	case turnID != "":
		return r.transitions.ListByTurn(ctx, turnID)
	default:
		return nil, errors.New("run, session, or turn id is required")
	}
}

func buildRuntimeRunTransitionHistoryResponse(transitions []RuntimeRunTransition, cursor string, limit int) RuntimeRunTransitionHistoryResponse {
	filtered := runtimeRunTransitionHistoryAfterCursor(transitions, cursor)
	total := len(filtered)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	window := RuntimeActivityWindow{
		Limit:         limit,
		Cursor:        cursor,
		EvidenceCount: total,
		FromStart:     len(filtered) == len(transitions) || (len(filtered) > 0 && filtered[0].ID == transitions[0].ID),
		ToEnd:         true,
	}
	if len(filtered) > 0 {
		window.FirstCursor = runtimeRunTransitionHistoryCursor(filtered[0])
		window.LastCursor = runtimeRunTransitionHistoryCursor(filtered[len(filtered)-1])
		window.HasMoreBefore = len(filtered) < len(transitions)
	}
	return RuntimeRunTransitionHistoryResponse{
		Transitions: filtered,
		Window:      window,
		Source: RuntimeRunTransitionHistorySource{
			Kind:                  runtimeRunTransitionHistorySourceKind,
			ReadOnly:              true,
			AuditOnly:             true,
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_run_transitions",
				"runtime_runs",
				"runtime_turns",
				"runtime_agent_tasks",
				"session_activity",
				"run_projection",
			},
		},
	}
}

func runtimeRunTransitionHistoryAfterCursor(transitions []RuntimeRunTransition, cursor string) []RuntimeRunTransition {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return append([]RuntimeRunTransition(nil), transitions...)
	}
	out := make([]RuntimeRunTransition, 0, len(transitions))
	for _, transition := range transitions {
		if strings.Compare(runtimeRunTransitionHistoryCursor(transition), cursor) > 0 {
			out = append(out, transition)
		}
	}
	return out
}

func runtimeRunTransitionHistoryCursor(transition RuntimeRunTransition) string {
	return fmt.Sprintf("v1:%020d:transition:%s", transition.CreatedAt, transition.ID)
}

func normalizeRuntimeRunTransitionHistoryLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}
