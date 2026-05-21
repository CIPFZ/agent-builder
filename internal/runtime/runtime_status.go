package runtime

import "context"

func (r *runtimeService) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	sessionID := r.sessionID
	events := r.eventStats.snapshot()
	requests := r.runtimeRequestsLocked()
	r.mu.Unlock()

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
		Ready:       info.IsReady,
		WorkspaceID: ws.ID,
		SessionID:   sessionID,
		WorkingDir:  ws.Path,
		Model:       info.ModelCfg.Model,
		Provider:    info.ModelCfg.Provider,
		Busy:        info.IsBusy,
		Usage:       usage,
		Events:      events,
		Requests:    requests,
	}, nil
}
