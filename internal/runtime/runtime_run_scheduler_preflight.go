package runtime

import (
	"context"
	"errors"
	"strings"
)

const (
	runtimeRunSchedulerPreflightSourceKind = "run_scheduler_preflight"

	runtimeRunSchedulerPreflightReasonMissingTurnID      = "missing_turn_id"
	runtimeRunSchedulerPreflightReasonMissingTurn        = "missing_turn"
	runtimeRunSchedulerPreflightReasonSessionMismatch    = "session_mismatch"
	runtimeRunSchedulerPreflightReasonMissingRun         = "missing_run"
	runtimeRunSchedulerPreflightReasonRunSessionMismatch = "run_session_mismatch"
	runtimeRunSchedulerPreflightReasonMissingTurnLink    = "missing_run_turn_link"
	runtimeRunSchedulerPreflightReasonTerminalTurn       = "terminal_turn"
)

func (r *runtimeService) runtimeRunSchedulerPreflight(ctx context.Context, req RuntimeRunSchedulerPreflightRequest) (RuntimeRunSchedulerPreflightResponse, error) {
	resp := RuntimeRunSchedulerPreflightResponse{
		RunID:     strings.TrimSpace(req.RunID),
		SessionID: strings.TrimSpace(req.SessionID),
		TurnID:    strings.TrimSpace(req.TurnID),
		Source: RuntimeRunSchedulerPreflightSource{
			Kind:         runtimeRunSchedulerPreflightSourceKind,
			ReadOnly:     true,
			StartsWorker: false,
			Evidence:     []string{"runtime_runs", "runtime_run_sessions", "runtime_turns"},
		},
	}
	if r.runs.db == nil {
		return resp, errors.New("runtime run database is not available")
	}
	if r.turns.db == nil {
		return resp, errors.New("runtime turn database is not available")
	}
	if resp.TurnID == "" {
		resp.Reason = runtimeRunSchedulerPreflightReasonMissingTurnID
		return resp, nil
	}
	turn, err := r.turns.Get(ctx, resp.TurnID)
	if errors.Is(err, errRuntimeTurnNotFound) {
		resp.Reason = runtimeRunSchedulerPreflightReasonMissingTurn
		return resp, nil
	}
	if err != nil {
		return resp, err
	}
	if resp.SessionID == "" {
		resp.SessionID = turn.SessionID
	}
	if resp.SessionID != turn.SessionID {
		resp.Reason = runtimeRunSchedulerPreflightReasonSessionMismatch
		return resp, nil
	}
	run, err := runtimeRunForSchedulerPreflight(ctx, r.runs, resp.RunID, resp.SessionID)
	if errors.Is(err, errRuntimeRunNotFound) {
		resp.Reason = runtimeRunSchedulerPreflightReasonMissingRun
		return resp, nil
	}
	if err != nil {
		return resp, err
	}
	resp.RunID = run.ID
	if !runtimeRunContainsSession(run, resp.SessionID) {
		resp.Reason = runtimeRunSchedulerPreflightReasonRunSessionMismatch
		return resp, nil
	}
	if !runtimeRunSessionLinkedToTurn(ctx, r.runs, run.ID, resp.SessionID, resp.TurnID) {
		resp.Reason = runtimeRunSchedulerPreflightReasonMissingTurnLink
		return resp, nil
	}
	if isFinalTurnStatus(turn.Status) {
		resp.Reason = runtimeRunSchedulerPreflightReasonTerminalTurn
		return resp, nil
	}
	resp.CanSchedule = true
	return resp, nil
}

func runtimeRunForSchedulerPreflight(ctx context.Context, store runtimeRunStore, runID, sessionID string) (RuntimeRun, error) {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		return store.Get(ctx, runID)
	}
	return store.GetBySession(ctx, strings.TrimSpace(sessionID))
}

func runtimeRunContainsSession(run RuntimeRun, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	if run.PrimarySessionID == sessionID {
		return true
	}
	for _, candidate := range run.SessionIDs {
		if candidate == sessionID {
			return true
		}
	}
	return false
}
