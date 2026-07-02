package runtime

import (
	"context"
	"errors"
	"time"
)

func (r *runtimeService) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errSelectedModelMissing) || errors.Is(err, errModelConfigMissing) {
			return r.fallbackProjectStatus(), nil
		}
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	sessionID := r.sessionID
	explicitProject := r.projectPath != ""
	workingDir := firstNonEmpty(r.projectPath, ws.Path)
	events := r.eventStats.snapshot()
	requests := r.runtimeRequestsLocked()
	sessionRequestID := r.sessionTurns[sessionID]
	sessionRequest := r.requests[sessionRequestID]
	sessionBusy := sessionRequestID != "" && !isFinalRuntimeRequestState(sessionRequest)
	if sessionBusy {
		requests.SessionRequestID = sessionRequestID
		requests.SessionStartedAt = sessionRequest.StartedAt
		requests.SessionBusy = true
	}
	r.mu.Unlock()
	if sessionBusy {
		return RuntimeStatus{
			Ready:           true,
			WorkspaceID:     ws.ID,
			SessionID:       sessionID,
			WorkingDir:      workingDir,
			ExplicitProject: explicitProject,
			Model:           sessionRequest.Model,
			Provider:        sessionRequest.Provider,
			Busy:            true,
			Usage:           sessionRequest.UsageBefore,
			Events:          events,
			Requests:        requests,
		}, nil
	}
	if requests.Running == 0 {
		if turns, err := r.turns.List(ctx, "active"); err == nil {
			now := time.Now().UnixMilli()
			for _, turn := range turns {
				requests.Running++
				if requests.ActiveStartedAt == 0 || turn.StartedAt < requests.ActiveStartedAt {
					requests.ActiveRequestID = turn.ID
					requests.ActiveStartedAt = turn.StartedAt
					requests.ActiveDurationMS = now - turn.StartedAt
				}
				if turn.SessionID == sessionID && !isFinalTurnStatus(turn.Status) {
					requests.SessionRequestID = turn.ID
					requests.SessionStartedAt = turn.StartedAt
					requests.SessionBusy = true
				}
			}
		}
	}

	info, err := r.runtime.GetAgentInfo(ws.ID)
	if err != nil {
		return RuntimeStatus{}, err
	}

	var usage RuntimeUsage
	if sessionID != "" {
		usage, err = r.sessionUsage(ctx, ws.ID, sessionID)
		if err != nil {
			return RuntimeStatus{}, err
		}
	}

	return RuntimeStatus{
		Ready:           info.IsReady,
		WorkspaceID:     ws.ID,
		SessionID:       sessionID,
		WorkingDir:      workingDir,
		ExplicitProject: explicitProject,
		Model:           info.ModelCfg.Model,
		Provider:        info.ModelCfg.Provider,
		Busy:            requests.SessionBusy,
		Usage:           usage,
		Events:          events,
		Requests:        requests,
	}, nil
}

func (r *runtimeService) fallbackProjectStatus() RuntimeStatus {
	r.mu.Lock()
	projectPath := r.projectPath
	activeProjectID := r.activeProjectID
	explicitProject := projectPath != ""
	sessionID := r.sessionID
	events := r.eventStats.snapshot()
	requests := r.runtimeRequestsLocked()
	r.mu.Unlock()
	if projectPath == "" {
		projectPath = runtimeDefaultWorkingDir()
	}
	return RuntimeStatus{
		Ready:           false,
		WorkspaceID:     firstNonEmpty(activeProjectID, runtimeFallbackWorkspaceID(projectPath)),
		SessionID:       sessionID,
		WorkingDir:      projectPath,
		ExplicitProject: explicitProject,
		Events:          events,
		Requests:        requests,
	}
}
