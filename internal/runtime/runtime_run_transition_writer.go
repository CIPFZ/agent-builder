package runtime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
)

const (
	runtimeRunTransitionSourceTurnStarted           = "turn_started"
	runtimeRunTransitionSourceTurnFinished          = "turn_finished"
	runtimeRunTransitionSourceTurnCancelled         = "turn_cancelled"
	runtimeRunTransitionSourceInterruptedMarkedDone = "interrupted_marked_done"
	runtimeRunTransitionSourceStartupRecovery       = "startup_recovery"
	runtimeRunTransitionSourceCheckpointResume      = "checkpoint_resume"
	runtimeRunTransitionSourceRecoveryResume        = "recovery_resume"
	runtimeRunTransitionSourceRecoveryDiscard       = "recovery_discard"
	runtimeRunTransitionSourceTaskStarted           = "task_started"
)

func (r *runtimeService) recordRunTransition(ctx context.Context, transition RuntimeRunTransition) {
	if r.transitions.db == nil {
		return
	}
	if strings.TrimSpace(transition.RunID) == "" || strings.TrimSpace(transition.ToStatus) == "" || strings.TrimSpace(transition.Source) == "" {
		return
	}
	if !isKnownRuntimeRunTransitionSource(transition.Source) {
		return
	}
	if transition.CreatedAt == 0 {
		return
	}
	if _, err := r.transitions.Upsert(ctx, transition); err != nil {
		slog.Warn("Failed to record runtime run transition", "run_id", transition.RunID, "turn_id", transition.TurnID, "source", transition.Source, "error", err)
	}
}

func (r *runtimeService) recordRunTurnTransition(ctx context.Context, source string, turn RuntimeTurn, fromStatus, toStatus, reason string) {
	if r.transitions.db == nil || r.runs.db == nil {
		return
	}
	if strings.TrimSpace(turn.SessionID) == "" || strings.TrimSpace(turn.ID) == "" || strings.TrimSpace(toStatus) == "" {
		return
	}
	run, err := r.runs.GetBySession(ctx, turn.SessionID)
	if err != nil {
		if errors.Is(err, errRuntimeRunNotFound) {
			return
		}
		slog.Warn("Failed to load runtime run for transition", "session_id", turn.SessionID, "turn_id", turn.ID, "source", source, "error", err)
		return
	}
	if source == runtimeRunTransitionSourceTurnStarted && !runtimeRunSessionLinkedToTurn(ctx, r.runs, run.ID, turn.SessionID, turn.ID) {
		return
	}
	createdAt := transitionCreatedAtForSource(source, turn)
	if createdAt == 0 {
		return
	}
	r.recordRunTransition(ctx, RuntimeRunTransition{
		RunID:      run.ID,
		SessionID:  turn.SessionID,
		TurnID:     turn.ID,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Reason:     reason,
		Source:     source,
		CreatedAt:  createdAt,
		Metadata: map[string]any{
			"turnStatus": turn.Status,
		},
	})
}

func (r *runtimeService) recordCheckpointResumeTransition(ctx context.Context, run RuntimeRun, checkpoint RuntimeRunCheckpoint, turnID string) {
	if r.transitions.db == nil {
		return
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		slog.Warn("Failed to load resumed turn for runtime run transition", "run_id", run.ID, "checkpoint_id", checkpoint.ID, "turn_id", turnID, "error", err)
		return
	}
	r.recordRunTransition(ctx, RuntimeRunTransition{
		RunID:      run.ID,
		SessionID:  run.PrimarySessionID,
		TurnID:     turn.ID,
		TaskID:     checkpoint.TaskID,
		FromStatus: runtimeRunStatusInterrupted,
		ToStatus:   runtimeRunStatusActive,
		Reason:     "checkpoint resumed by explicit user action",
		Source:     runtimeRunTransitionSourceCheckpointResume,
		CreatedAt:  firstPositiveInt64(turn.StartedAt, turn.FinishedAt),
		Metadata: map[string]any{
			"checkpointID": checkpoint.ID,
			"sourceTurnID": checkpoint.TurnID,
		},
	})
}

func (r *runtimeService) recordRunTaskTransition(ctx context.Context, source string, task RuntimeAgentTask, fromStatus, toStatus, reason string) {
	if r.transitions.db == nil || r.runs.db == nil {
		return
	}
	sessionID := firstNonEmpty(task.ParentSessionID, task.ChildSessionID)
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(toStatus) == "" {
		return
	}
	run, err := r.runs.GetBySession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, errRuntimeRunNotFound) {
			return
		}
		slog.Warn("Failed to load runtime run for task transition", "session_id", sessionID, "task_id", task.ID, "source", source, "error", err)
		return
	}
	createdAt := firstPositiveInt64(task.FinishedAt, task.UpdatedAt, task.StartedAt)
	if createdAt == 0 {
		return
	}
	r.recordRunTransition(ctx, RuntimeRunTransition{
		RunID:      run.ID,
		SessionID:  sessionID,
		TurnID:     task.ParentTurnID,
		TaskID:     task.ID,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Reason:     reason,
		Source:     source,
		CreatedAt:  createdAt,
		Metadata: map[string]any{
			"taskStatus": task.Status,
		},
	})
}

func (r *runtimeService) runtimeRunStatusForSession(ctx context.Context, sessionID string) string {
	if r.runs.db == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	run, err := r.runs.GetBySession(ctx, sessionID)
	if err != nil {
		return ""
	}
	return run.Status
}

func transitionCreatedAtForSource(source string, turn RuntimeTurn) int64 {
	switch source {
	case runtimeRunTransitionSourceTurnStarted:
		return firstPositiveInt64(turn.StartedAt, turn.FinishedAt)
	default:
		return firstPositiveInt64(turn.FinishedAt, turn.StartedAt)
	}
}

func isKnownRuntimeRunTransitionSource(source string) bool {
	switch strings.TrimSpace(source) {
	case runtimeRunTransitionSourceTurnStarted,
		runtimeRunTransitionSourceTurnFinished,
		runtimeRunTransitionSourceTurnCancelled,
		runtimeRunTransitionSourceInterruptedMarkedDone,
		runtimeRunTransitionSourceStartupRecovery,
		runtimeRunTransitionSourceCheckpointResume,
		runtimeRunTransitionSourceRecoveryResume,
		runtimeRunTransitionSourceRecoveryDiscard,
		runtimeRunTransitionSourceTaskStarted:
		return true
	default:
		return false
	}
}
