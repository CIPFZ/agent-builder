package contextmgr

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func IsContextLengthError(errText string) bool {
	normalized := strings.ToLower(strings.TrimSpace(errText))
	if normalized == "" {
		return false
	}
	patterns := []string{
		"context length",
		"context_length",
		"maximum context",
		"max context",
		"prompt too long",
		"prompt_too_long",
		"too many tokens",
		"token limit",
		"input is too long",
	}
	for _, pattern := range patterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

func (m *DefaultManager) ReactiveCompact(ctx context.Context, req ReactiveCompactRequest) (ReactiveCompactResult, error) {
	if m == nil || m.store == nil {
		return ReactiveCompactResult{}, fmt.Errorf("context manager store is not available")
	}
	if !IsContextLengthError(req.Error) {
		attempt := ReactiveAttempt{
			ID:           reactiveAttemptID(req.TurnID, req.Attempt),
			SessionID:    req.SessionID,
			TurnID:       req.TurnID,
			ProjectionID: req.ProjectionID,
			Attempt:      req.Attempt,
			Action:       "none",
			Status:       "skipped",
			Error:        req.Error,
			CreatedAt:    time.Now().UTC().UnixMilli(),
			CompletedAt:  time.Now().UTC().UnixMilli(),
		}
		stored, err := m.store.UpsertReactiveAttempt(ctx, attempt)
		return ReactiveCompactResult{Attempt: stored}, err
	}
	action := "projection_reduction"
	if req.Attempt > 1 {
		action = "full_compact"
	}
	attempt := ReactiveAttempt{
		ID:           reactiveAttemptID(req.TurnID, req.Attempt),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		Attempt:      req.Attempt,
		Action:       action,
		Status:       ProjectionStatusCompleted,
		Error:        req.Error,
		CreatedAt:    time.Now().UTC().UnixMilli(),
		CompletedAt:  time.Now().UTC().UnixMilli(),
	}
	stored, err := m.store.UpsertReactiveAttempt(ctx, attempt)
	if err != nil {
		return ReactiveCompactResult{}, err
	}
	return ReactiveCompactResult{Attempt: stored}, nil
}

func reactiveAttemptID(turnID string, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("ctxreact_%s_%03d", stableIDPart(turnID), attempt)
}
