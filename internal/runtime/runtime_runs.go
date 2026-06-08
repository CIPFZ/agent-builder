package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (r *runtimeService) Runs(ctx context.Context) (RuntimeRunsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunsResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunsResponse{}, errors.New("runtime run database is not available")
	}
	if err := r.backfillRuntimeRuns(ctx); err != nil {
		return RuntimeRunsResponse{}, err
	}
	runs, err := r.runs.List(ctx)
	if err != nil {
		return RuntimeRunsResponse{}, err
	}
	return RuntimeRunsResponse{Runs: runs}, nil
}

func (r *runtimeService) Run(ctx context.Context, id string) (RuntimeRunResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunResponse{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeRunResponse{}, errors.New("run id is required")
	}
	if r.runs.db == nil {
		return RuntimeRunResponse{}, errors.New("runtime run database is not available")
	}
	run, err := r.runs.Get(ctx, id)
	if errors.Is(err, errRuntimeRunNotFound) && strings.HasPrefix(id, "run:session:") {
		sessionID := strings.TrimPrefix(id, "run:session:")
		if _, backfillErr := r.backfillRuntimeRunSession(ctx, sessionID); backfillErr != nil {
			return RuntimeRunResponse{}, backfillErr
		}
		run, err = r.runs.Get(ctx, id)
	}
	if err != nil {
		return RuntimeRunResponse{}, err
	}
	projection, err := r.RunProjection(ctx, RuntimeRunProjectionRequest{SessionID: run.PrimarySessionID})
	if err != nil {
		return RuntimeRunResponse{}, fmt.Errorf("failed to build runtime run projection parity payload: %w", err)
	}
	projection.Run.ID = run.ID
	return RuntimeRunResponse{Run: run, Projection: projection.Run}, nil
}

func (r *runtimeService) backfillRuntimeRuns(ctx context.Context) error {
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	sessions, err := r.runtime.ListSessions(ctx, wsID)
	if err != nil {
		return fmt.Errorf("failed to list sessions for runtime run backfill: %w", err)
	}
	for _, sess := range sessions {
		if _, err := r.backfillRuntimeRunSession(ctx, sess.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *runtimeService) backfillRuntimeRunSession(ctx context.Context, sessionID string) (RuntimeRun, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeRun{}, errors.New("session id is required")
	}
	projection, err := r.RunProjection(ctx, RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		return RuntimeRun{}, err
	}
	return r.runs.UpsertFromProjection(ctx, projection.Run, runtimeRunSourceBackfill)
}
