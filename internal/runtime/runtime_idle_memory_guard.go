package runtime

import "context"

const runtimeIdleMemoryGuardMinimumIdleMS int64 = 2 * 60 * 1000

func (r *runtimeService) IdleMemoryGuard(ctx context.Context, req RuntimeIdleMemoryGuardRequest) (RuntimeIdleMemoryGuardResponse, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeIdleMemoryGuardResponse{}, err
	}
	r.mu.Lock()
	pendingPermissions := len(r.permissions)
	r.mu.Unlock()
	return evaluateRuntimeIdleMemoryGuard(req, status, pendingPermissions), nil
}

func evaluateRuntimeIdleMemoryGuard(req RuntimeIdleMemoryGuardRequest, status RuntimeStatus, pendingPermissions int) RuntimeIdleMemoryGuardResponse {
	response := RuntimeIdleMemoryGuardResponse{MinimumIdleMS: runtimeIdleMemoryGuardMinimumIdleMS}
	deny := func(reason string) RuntimeIdleMemoryGuardResponse {
		response.Reason = reason
		return response
	}
	if req.ClientIdleMS < runtimeIdleMemoryGuardMinimumIdleMS {
		return deny("client_not_idle")
	}
	if req.HasUnsavedDraft {
		return deny("unsaved_draft")
	}
	if req.HasActiveOverlay {
		return deny("active_overlay")
	}
	if req.HasTerminalInteraction {
		return deny("terminal_interaction")
	}
	if pendingPermissions > 0 {
		return deny("pending_permission")
	}
	if status.Busy || status.Requests.SessionBusy || status.Requests.Running > 0 || status.Requests.Queued > 0 {
		return deny("active_turn")
	}
	for _, active := range status.ActiveSessions {
		switch active.Status {
		case turnStatusQueued, turnStatusRunning, turnStatusWaitingPermission, turnStatusCancelling:
			return deny("active_session")
		}
	}
	for _, resource := range status.ResourceGovernor.Resources {
		// An idle terminal's bounded replay buffer is deliberately resident;
		// recent terminal input is represented by HasTerminalInteraction.
		if resource.Kind == string(runtimeResourceTerminal) {
			continue
		}
		if resource.InUseCount > 0 || resource.QueuedCount > 0 || resource.InUseBytes > 0 {
			return deny("resident_resource")
		}
	}
	response.Eligible = true
	return response
}
