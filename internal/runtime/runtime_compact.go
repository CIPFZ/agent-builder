package runtime

import (
	"context"
	"errors"
	"strings"
)

const (
	compactKindBoundary = "boundary"
	compactKindMicro    = "micro"
	compactKindFull     = "full"

	compactStatusRecorded  = "recorded"
	compactStatusCompleted = "completed"
	compactStatusSkipped   = "skipped"
	compactStatusFailed    = "failed"
)

func (r *runtimeService) TurnCompactBoundaries(ctx context.Context, turnID string) (RuntimeCompactBoundariesResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeCompactBoundariesResponse{}, errors.New("turn id is required")
	}
	if r.compactBoundaries.db == nil {
		return RuntimeCompactBoundariesResponse{}, nil
	}
	boundaries, err := r.compactBoundaries.ListByTurn(ctx, turnID)
	if err != nil {
		return RuntimeCompactBoundariesResponse{}, err
	}
	return RuntimeCompactBoundariesResponse{Boundaries: boundaries}, nil
}

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
	boundaries, err := r.compactBoundaries.ListBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCompactBoundariesResponse{}, err
	}
	return RuntimeCompactBoundariesResponse{Boundaries: boundaries}, nil
}
