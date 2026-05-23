package scheduler

import (
	"context"
	"time"
)

type Scheduler struct {
	store Store
	now   func() time.Time
}

func New(store Store) *Scheduler {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Scheduler{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Scheduler) CreateCall(ctx context.Context, req ToolCallRequest) (ToolCall, error) {
	return s.store.Upsert(ctx, ToolCall{
		ID:           req.ID,
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		MessageID:    req.MessageID,
		Name:         req.Name,
		Source:       req.Source,
		CapabilityID: req.CapabilityID,
		Status:       ToolCallRunning,
		InputSummary: req.InputSummary,
		StartedAt:    s.now(),
	})
}

func (s *Scheduler) MarkWaitingPermission(ctx context.Context, id string) (ToolCall, error) {
	call, err := s.store.Get(ctx, id)
	if err != nil {
		return ToolCall{}, err
	}
	call.Status = ToolCallWaitingPermission
	return s.store.Upsert(ctx, call)
}

func (s *Scheduler) CompleteCall(ctx context.Context, result ToolCallResult) (ToolCall, error) {
	call, err := s.store.Get(ctx, result.ToolCallID)
	if err != nil {
		return ToolCall{}, err
	}
	call.Status = result.Status
	if call.Status == "" {
		call.Status = ToolCallCompleted
	}
	call.OutputSummary = result.OutputSummary
	call.ModelContent = result.ModelContent
	call.Structured = result.Structured
	call.Stdout = result.Stdout
	call.Stderr = result.Stderr
	call.IsError = result.IsError
	call.Error = result.Error
	if isFinalToolCallStatus(call.Status) {
		call.FinishedAt = s.now()
	}
	return s.store.Upsert(ctx, call)
}

func (s *Scheduler) GetCall(ctx context.Context, id string) (ToolCall, error) {
	return s.store.Get(ctx, id)
}

func (s *Scheduler) ListCalls(ctx context.Context, turnID string) ([]ToolCall, error) {
	return s.store.ListByTurn(ctx, turnID)
}

func (s *Scheduler) CancelCall(ctx context.Context, id string) error {
	call, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	call.Status = ToolCallCancelled
	call.FinishedAt = s.now()
	_, err = s.store.Upsert(ctx, call)
	return err
}
