package runtime

import (
	"context"
	"errors"
	"sort"
	"strings"
)

const (
	runtimeRunProjectionSourceKind = "session_activity_projection"
	runtimeRunStatusActive         = "active"
	runtimeRunStatusWaitingUser    = "waiting_user"
	runtimeRunStatusInterrupted    = "interrupted"
	runtimeRunStatusCompleted      = "completed"
	runtimeRunStatusFailed         = "failed"
	runtimeRunStatusCancelled      = "cancelled"
)

// RunProjection builds the read-only Run DTO from existing runtime evidence.
// SessionActivity remains the parity oracle; persisted Run rows only provide
// durable identity and summary metadata for the projection.
func (r *runtimeService) RunProjection(ctx context.Context, req RuntimeRunProjectionRequest) (RuntimeRunProjectionResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunProjectionResponse{}, err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return RuntimeRunProjectionResponse{}, errors.New("session id is required")
	}
	activity, err := r.hydrateSessionActivity(ctx, sessionID, strings.TrimSpace(req.Cursor), req.Limit)
	if err != nil {
		return RuntimeRunProjectionResponse{}, err
	}
	r.mu.Lock()
	workspaceID := ""
	if r.workspace != nil {
		workspaceID = r.workspace.ID
	}
	r.mu.Unlock()
	tasks := r.runtimeRunProjectionTasks(ctx, sessionID)
	run := buildRuntimeRunProjection(workspaceID, activity, tasks)
	if r.runs.db != nil && runtimeRunProjectionCanReconcile(req) {
		if err := validateRuntimeRunStatusWrite(runtimeRunProjectionStatusWriteRequest(run)); err == nil {
			if persisted, err := r.runs.UpsertFromProjection(ctx, run, runtimeRunSourceBackfill); err == nil {
				run.ID = persisted.ID
				_, _ = r.runs.writeRuntimeRunStatus(ctx, runtimeRunProjectionStatusWriteRequest(run))
			}
		}
	} else if r.runs.db != nil {
		if persisted, err := r.runs.GetBySession(ctx, sessionID); err == nil {
			run.ID = persisted.ID
		}
	}
	return RuntimeRunProjectionResponse{Run: run}, nil
}

func runtimeRunProjectionCanReconcile(req RuntimeRunProjectionRequest) bool {
	return strings.TrimSpace(req.Cursor) == "" && req.Limit <= 0
}

func runtimeRunProjectionStatusWriteRequest(projection RuntimeRunProjection) runtimeRunStatusWriteRequest {
	return runtimeRunStatusWriteRequest{
		RunID:                    projection.ID,
		SessionID:                projection.PrimarySessionID,
		Status:                   projection.Status,
		Source:                   runtimeRunStatusWriteSourceProjectionReconcile,
		Reason:                   "full run projection parity reconcile",
		EvidenceKind:             runtimeRunStatusWriteEvidenceProjection,
		Timestamp:                firstPositiveInt64(projection.FinishedAt, projection.UpdatedAt, projection.CreatedAt),
		RequiresProjectionParity: true,
		Projection:               &projection,
	}
}

func (r *runtimeService) runtimeRunProjectionTasks(ctx context.Context, sessionID string) []RuntimeAgentTask {
	if r.agentTasks.db == nil {
		return nil
	}
	tasks, err := r.agentTasks.ListBySession(ctx, sessionID)
	if err != nil {
		return nil
	}
	return tasks
}

func buildRuntimeRunProjection(workspaceID string, activity RuntimeSessionActivityWindowResponse, tasks []RuntimeAgentTask) RuntimeRunProjection {
	run := RuntimeRunProjection{
		ID:               runtimeRunProjectionID(activity.SessionID),
		WorkspaceID:      workspaceID,
		SessionIDs:       []string{activity.SessionID},
		PrimarySessionID: activity.SessionID,
		Status:           runtimeRunStatusCompleted,
		ActivityWindow:   activity.Window,
		Source: RuntimeRunProjectionSource{
			Kind:                  runtimeRunProjectionSourceKind,
			ReadOnly:              true,
			SessionActivityParity: true,
			Evidence:              []string{"messages", "turns", "tool_calls", "permissions", "runtime_events", "agent_tasks"},
		},
	}
	if activity.Window.LastCursor != "" {
		run.EvidenceCursor = activity.Window.LastCursor
	}
	for _, msg := range activity.Messages {
		run.CreatedAt = firstPositiveMin(run.CreatedAt, msg.CreatedAt)
		run.UpdatedAt = maxInt64(run.UpdatedAt, msg.UpdatedAt, msg.CreatedAt)
	}
	for _, turn := range activity.Turns {
		run.TurnIDs = appendUniqueString(run.TurnIDs, turn.ID)
		if run.Objective == "" {
			run.Objective = strings.TrimSpace(turn.PromptPreview)
		}
		run.CreatedAt = firstPositiveMin(run.CreatedAt, turn.StartedAt, turn.FinishedAt)
		run.UpdatedAt = maxInt64(run.UpdatedAt, turn.FinishedAt, turn.StartedAt)
		if isFinalTurnStatus(turn.Status) {
			run.FinishedAt = maxInt64(run.FinishedAt, turn.FinishedAt)
		} else {
			run.FinishedAt = 0
		}
		mergeRunTurnDiagnostics(&run, turn)
		if turn.Interrupted != nil {
			run.Interrupted = turn.Interrupted
			run.Recovery.InterruptedSourceTurns = appendUniqueString(run.Recovery.InterruptedSourceTurns, turn.ID)
			run.Checkpoints = append(run.Checkpoints, runtimeRunCheckpointFromInterrupted(turn))
			run.UserActions.Resume = append(run.UserActions.Resume, RuntimeRunUserAction{
				ID:      "resume:" + turn.ID,
				TurnID:  turn.ID,
				Kind:    "resume",
				Label:   "Resume from checkpoint",
				Enabled: true,
				Reason:  "user-triggered continuation only",
			})
			run.UserActions.Discard = append(run.UserActions.Discard, RuntimeRunUserAction{
				ID:      "discard:" + turn.ID,
				TurnID:  turn.ID,
				Kind:    "discard",
				Label:   "Dismiss interrupted checkpoint",
				Enabled: true,
				Reason:  "acknowledges the projection without deleting evidence",
			})
		}
		if turn.Diagnostics.ProviderError != nil {
			run.Recovery.RecoverableErrors++
		}
		run.Recovery.RetryAttempts += len(turn.Diagnostics.RetryAttempts)
		if turn.Diagnostics.CompactRetryResult != "" {
			run.Recovery.CompactRetryCount++
		}
	}
	for _, call := range activity.ToolCalls {
		run.ToolCallIDs = appendUniqueString(run.ToolCallIDs, call.ID)
		run.Diagnostics.ToolCallCount++
		if run.Diagnostics.ToolCountsByStatus == nil {
			run.Diagnostics.ToolCountsByStatus = map[string]int{}
		}
		run.Diagnostics.ToolCountsByStatus[call.Status]++
		if call.Status == "completed" {
			run.ProducedArtifacts = appendUniqueStrings(run.ProducedArtifacts, call.ArtifactRefs...)
		}
		run.UpdatedAt = maxInt64(run.UpdatedAt, call.FinishedAt, call.StartedAt)
	}
	for _, perm := range activity.Permissions {
		run.PermissionRequestIDs = appendUniqueString(run.PermissionRequestIDs, perm.ID)
		run.Diagnostics.PermissionRequestCount++
		mergeRunPermissionCounts(&run.Diagnostics.TerminalPermissionCounts, perm.Status)
		run.UpdatedAt = maxInt64(run.UpdatedAt, perm.DecidedAt, perm.CreatedAt)
	}
	for _, task := range tasks {
		run.TaskIDs = appendUniqueString(run.TaskIDs, task.ID)
		run.SessionIDs = appendUniqueString(run.SessionIDs, task.ParentSessionID)
		run.SessionIDs = appendUniqueString(run.SessionIDs, task.ChildSessionID)
		run.CreatedAt = firstPositiveMin(run.CreatedAt, task.StartedAt)
		run.UpdatedAt = maxInt64(run.UpdatedAt, task.UpdatedAt, task.FinishedAt, task.StartedAt)
		if task.Status == agentTaskStatusCompleted {
			run.ProducedArtifacts = appendUniqueStrings(run.ProducedArtifacts, task.ArtifactRefs...)
		}
		if isFinalAgentTaskStatus(task.Status) {
			run.Checkpoints = append(run.Checkpoints, runtimeRunCheckpointFromTask(task))
		}
	}
	run.Diagnostics.TaskCount = len(run.TaskIDs)
	run.Diagnostics.ArtifactCounts.Expected = len(run.ExpectedArtifacts)
	run.Diagnostics.ArtifactCounts.Produced = len(run.ProducedArtifacts)
	run.Diagnostics.ArtifactCounts.Verified = len(run.VerifiedArtifacts)
	run.Diagnostics.ArtifactCounts.Missing = maxInt(0, run.Diagnostics.ArtifactCounts.Expected-run.Diagnostics.ArtifactCounts.Produced)
	run.Status = runtimeRunProjectionStatus(run)
	if !isFinalRuntimeRunStatus(run.Status) {
		run.FinishedAt = 0
	}
	sortRuntimeRunProjection(&run)
	return run
}

func mergeRunTurnDiagnostics(run *RuntimeRunProjection, turn RuntimeTurn) {
	run.Diagnostics.TurnCount++
	switch turn.Status {
	case turnStatusRunning, turnStatusQueued, turnStatusCancelling:
		run.Diagnostics.RunningTurnCount++
	case turnStatusWaitingPermission:
		run.Diagnostics.WaitingPermissionTurnCount++
	case turnStatusInterrupted:
		run.Diagnostics.InterruptedTurnCount++
	case turnStatusFailed:
		run.Diagnostics.FailedTurnCount++
	case turnStatusCancelled:
		run.Diagnostics.CancelledTurnCount++
	}
	diag := turn.Diagnostics
	run.ExpectedArtifacts = appendUniqueStrings(run.ExpectedArtifacts, diag.ExpectedArtifacts...)
	run.ProducedArtifacts = appendUniqueStrings(run.ProducedArtifacts, diag.ProducedArtifacts...)
	run.VerifiedArtifacts = appendUniqueStrings(run.VerifiedArtifacts, diag.VerifiedArtifacts...)
	if run.Diagnostics.Warning == "" && diag.Warning != "" {
		run.Diagnostics.Warning = diag.Warning
		run.Diagnostics.WarningReason = diag.WarningReason
	}
}

func runtimeRunCheckpointFromInterrupted(turn RuntimeTurn) RuntimeRunCheckpoint {
	checkpoint := RuntimeRunCheckpoint{
		ID:             "turn:" + turn.ID + ":interrupted",
		TurnID:         turn.ID,
		Status:         turn.Status,
		Summary:        firstNonEmpty(turn.Diagnostics.Warning, turn.Error, turn.PromptPreview),
		ArtifactRefs:   appendUniqueStrings(nil, turn.Diagnostics.ProducedArtifacts...),
		CreatedAt:      firstPositiveInt64(turn.FinishedAt, turn.Diagnostics.ComputedAt, turn.StartedAt),
		ResumeEligible: true,
	}
	if turn.Interrupted != nil {
		checkpoint.ArtifactRefs = appendUniqueStrings(checkpoint.ArtifactRefs, turn.Interrupted.ProducedArtifacts...)
	}
	return checkpoint
}

func runtimeRunCheckpointFromTask(task RuntimeAgentTask) RuntimeRunCheckpoint {
	return RuntimeRunCheckpoint{
		ID:             "task:" + task.ID + ":" + task.Status,
		TaskID:         task.ID,
		Status:         task.Status,
		Summary:        firstNonEmpty(task.ResultSummary, task.Error, task.Title),
		ArtifactRefs:   appendUniqueStrings(nil, task.ArtifactRefs...),
		CreatedAt:      firstPositiveInt64(task.FinishedAt, task.UpdatedAt, task.StartedAt),
		ResumeEligible: task.Status == agentTaskStatusInterrupted,
	}
}

func runtimeRunProjectionStatus(run RuntimeRunProjection) string {
	if run.Diagnostics.WaitingPermissionTurnCount > 0 {
		return runtimeRunStatusWaitingUser
	}
	if run.Diagnostics.RunningTurnCount > 0 {
		return runtimeRunStatusActive
	}
	if run.Diagnostics.InterruptedTurnCount > 0 {
		return runtimeRunStatusInterrupted
	}
	if run.Diagnostics.FailedTurnCount > 0 {
		return runtimeRunStatusFailed
	}
	if run.Diagnostics.CancelledTurnCount > 0 && run.Diagnostics.TurnCount > 0 {
		return runtimeRunStatusCancelled
	}
	for _, checkpoint := range run.Checkpoints {
		switch checkpoint.Status {
		case agentTaskStatusRunning, agentTaskStatusQueued:
			return runtimeRunStatusActive
		case agentTaskStatusInterrupted:
			return runtimeRunStatusInterrupted
		case agentTaskStatusFailed:
			return runtimeRunStatusFailed
		case agentTaskStatusCancelled:
			return runtimeRunStatusCancelled
		}
	}
	return runtimeRunStatusCompleted
}

func isFinalRuntimeRunStatus(status string) bool {
	switch status {
	case runtimeRunStatusCompleted, runtimeRunStatusFailed, runtimeRunStatusCancelled, runtimeRunStatusInterrupted:
		return true
	default:
		return false
	}
}

func mergeRunPermissionCounts(counts *RuntimePermissionCounts, status string) {
	switch status {
	case permissionStatusPending:
		counts.Pending++
	case permissionStatusAllowedOnce, permissionStatusAllowedSession:
		counts.Allowed++
	case permissionStatusDenied:
		counts.Denied++
	case permissionStatusExpired:
		counts.Expired++
	case permissionStatusCancelled:
		counts.Cancelled++
	}
}

func sortRuntimeRunProjection(run *RuntimeRunProjection) {
	sort.Strings(run.SessionIDs)
	sort.Strings(run.TurnIDs)
	sort.Strings(run.TaskIDs)
	sort.Strings(run.ToolCallIDs)
	sort.Strings(run.PermissionRequestIDs)
	sort.Strings(run.ExpectedArtifacts)
	sort.Strings(run.ProducedArtifacts)
	sort.Strings(run.VerifiedArtifacts)
	sort.SliceStable(run.Checkpoints, func(i, j int) bool {
		if run.Checkpoints[i].CreatedAt != run.Checkpoints[j].CreatedAt {
			return run.Checkpoints[i].CreatedAt < run.Checkpoints[j].CreatedAt
		}
		return run.Checkpoints[i].ID < run.Checkpoints[j].ID
	})
}

func runtimeRunProjectionID(sessionID string) string {
	if sessionID == "" {
		return "run:session:"
	}
	return "run:session:" + sessionID
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstPositiveMin(current int64, values ...int64) int64 {
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if current == 0 || value < current {
			current = value
		}
	}
	return current
}

func maxInt64(current int64, values ...int64) int64 {
	for _, value := range values {
		if value > current {
			current = value
		}
	}
	return current
}
