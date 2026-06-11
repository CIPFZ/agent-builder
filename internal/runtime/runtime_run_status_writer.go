package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	runtimeRunStatusWriteSourceProjectionReconcile = "projection_reconcile"

	runtimeRunStatusWriteEvidenceProjection = "run_projection"
	runtimeRunStatusWriteEvidenceTurn       = "runtime_turn"
	runtimeRunStatusWriteEvidenceTask       = "runtime_task"
	runtimeRunStatusWriteEvidenceCheckpoint = "runtime_checkpoint"
	runtimeRunStatusWriteEvidenceRecovery   = "runtime_recovery"
)

type runtimeRunStatusWriteRequest struct {
	RunID                    string
	SessionID                string
	Status                   string
	Source                   string
	Reason                   string
	EvidenceKind             string
	TurnID                   string
	TaskID                   string
	CheckpointID             string
	Timestamp                int64
	RequiresProjectionParity bool
	Projection               *RuntimeRunProjection
}

func (s runtimeRunStore) writeRuntimeRunStatus(ctx context.Context, req runtimeRunStatusWriteRequest) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	run, err := s.runtimeRunForStatusWrite(ctx, req)
	if err != nil {
		return RuntimeRun{}, err
	}
	req.RunID = run.ID
	if err := validateRuntimeRunStatusWrite(req); err != nil {
		return RuntimeRun{}, err
	}
	timestamp := req.Timestamp
	if timestamp <= 0 {
		timestamp = time.Now().UnixMilli()
	}
	var finishedAt any
	if isFinalRuntimeRunStatus(req.Status) {
		finishedAt = timestamp
	} else {
		finishedAt = nil
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE runtime_runs
SET status = ?, updated_at = ?, finished_at = ?
WHERE id = ?`, strings.TrimSpace(req.Status), timestamp, finishedAt, run.ID); err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to write runtime run status: %w", err)
	}
	return s.Get(ctx, run.ID)
}

func (s runtimeRunStore) runtimeRunForStatusWrite(ctx context.Context, req runtimeRunStatusWriteRequest) (RuntimeRun, error) {
	runID := strings.TrimSpace(req.RunID)
	sessionID := strings.TrimSpace(req.SessionID)
	if runID != "" {
		return s.Get(ctx, runID)
	}
	if sessionID != "" {
		return s.GetBySession(ctx, sessionID)
	}
	return RuntimeRun{}, errors.New("run id or session id is required")
}

func validateRuntimeRunStatusWrite(req runtimeRunStatusWriteRequest) error {
	status := strings.TrimSpace(req.Status)
	source := strings.TrimSpace(req.Source)
	if status == "" {
		return errors.New("run status is required")
	}
	if !isKnownRuntimeRunStatus(status) {
		return fmt.Errorf("unsupported runtime run status %q", status)
	}
	if source == "" {
		return errors.New("run status write source is required")
	}
	if !isKnownRuntimeRunStatusWriteSource(source) {
		return fmt.Errorf("unsupported runtime run status write source %q", source)
	}
	if strings.TrimSpace(req.RunID) == "" && strings.TrimSpace(req.SessionID) == "" {
		return errors.New("run id or session id is required")
	}
	if err := validateRuntimeRunStatusWriteEvidence(req); err != nil {
		return err
	}
	if req.RequiresProjectionParity || isFinalRuntimeRunStatus(status) || source == runtimeRunTransitionSourceStartupRecovery {
		return validateRuntimeRunStatusWriteProjectionParity(req)
	}
	return nil
}

func validateRuntimeRunStatusWriteEvidence(req runtimeRunStatusWriteRequest) error {
	source := strings.TrimSpace(req.Source)
	switch source {
	case runtimeRunTransitionSourceTurnStarted,
		runtimeRunTransitionSourceTurnFinished,
		runtimeRunTransitionSourceTurnCancelled,
		runtimeRunTransitionSourceInterruptedMarkedDone:
		if strings.TrimSpace(req.TurnID) == "" {
			return errors.New("turn id is required for run status write")
		}
	case runtimeRunTransitionSourceTaskStarted:
		if strings.TrimSpace(req.TaskID) == "" {
			return errors.New("task id is required for run status write")
		}
	case runtimeRunTransitionSourceCheckpointResume:
		if strings.TrimSpace(req.CheckpointID) == "" {
			return errors.New("checkpoint id is required for run status write")
		}
		if strings.TrimSpace(req.TurnID) == "" {
			return errors.New("resume turn id is required for run status write")
		}
	case runtimeRunTransitionSourceStartupRecovery:
		if strings.TrimSpace(req.TurnID) == "" && strings.TrimSpace(req.TaskID) == "" {
			return errors.New("turn id or task id is required for startup recovery status write")
		}
	case runtimeRunStatusWriteSourceProjectionReconcile:
		if strings.TrimSpace(req.EvidenceKind) != runtimeRunStatusWriteEvidenceProjection {
			return errors.New("projection reconcile status write requires projection evidence")
		}
	}
	return nil
}

func validateRuntimeRunStatusWriteProjectionParity(req runtimeRunStatusWriteRequest) error {
	if !req.RequiresProjectionParity {
		return errors.New("run status write requires full projection parity")
	}
	if req.Projection == nil {
		return errors.New("run status write projection is required")
	}
	projection := *req.Projection
	if projection.Status != strings.TrimSpace(req.Status) {
		return fmt.Errorf("run status %q diverges from projection status %q", req.Status, projection.Status)
	}
	if projection.Source.Kind != runtimeRunProjectionSourceKind || !projection.Source.ReadOnly || !projection.Source.SessionActivityParity {
		return errors.New("run status write projection source must preserve SessionActivity parity")
	}
	if !runtimeActivityWindowIsFull(projection.ActivityWindow) {
		return errors.New("run status write projection must be full activity parity")
	}
	return nil
}

func isKnownRuntimeRunStatusWriteSource(source string) bool {
	switch strings.TrimSpace(source) {
	case runtimeRunStatusWriteSourceProjectionReconcile,
		runtimeRunTransitionSourceTurnStarted,
		runtimeRunTransitionSourceTurnFinished,
		runtimeRunTransitionSourceTurnCancelled,
		runtimeRunTransitionSourceInterruptedMarkedDone,
		runtimeRunTransitionSourceStartupRecovery,
		runtimeRunTransitionSourceCheckpointResume,
		runtimeRunTransitionSourceTaskStarted:
		return true
	default:
		return false
	}
}

func isKnownRuntimeRunStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case runtimeRunStatusActive,
		runtimeRunStatusWaitingUser,
		runtimeRunStatusInterrupted,
		runtimeRunStatusCompleted,
		runtimeRunStatusFailed,
		runtimeRunStatusCancelled:
		return true
	default:
		return false
	}
}

func runtimeActivityWindowIsFull(window RuntimeActivityWindow) bool {
	return window.Limit <= 0 && window.FromStart && window.ToEnd && strings.TrimSpace(window.Cursor) == "" && !window.HasMoreBefore && !window.HasMoreAfter
}
