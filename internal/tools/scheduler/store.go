package scheduler

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrToolCallNotFound = errors.New("tool call not found")

type Store interface {
	Upsert(context.Context, ToolCall) (ToolCall, error)
	Get(context.Context, string) (ToolCall, error)
	ListByTurn(context.Context, string) ([]ToolCall, error)
}

type MemoryStore struct {
	mu    sync.RWMutex
	calls map[string]ToolCall
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{calls: make(map[string]ToolCall)}
}

func (s *MemoryStore) Upsert(_ context.Context, call ToolCall) (ToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if call.ID == "" {
		return ToolCall{}, errors.New("tool call id is required")
	}
	existing, ok := s.calls[call.ID]
	if ok {
		call = mergeToolCall(existing, call)
	}
	if call.Source == "" {
		call.Source = ToolSourceUnknown
	}
	if call.Status == "" {
		call.Status = ToolCallPending
	}
	if call.StartedAt.IsZero() {
		call.StartedAt = time.Now().UTC()
	}
	s.calls[call.ID] = call
	return call, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	call, ok := s.calls[id]
	if !ok {
		return ToolCall{}, ErrToolCallNotFound
	}
	return call, nil
}

func (s *MemoryStore) ListByTurn(_ context.Context, turnID string) ([]ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var calls []ToolCall
	for _, call := range s.calls {
		if call.TurnID == turnID {
			calls = append(calls, call)
		}
	}
	sort.SliceStable(calls, func(i, j int) bool {
		return calls[i].StartedAt.Before(calls[j].StartedAt)
	})
	return calls, nil
}

func mergeToolCall(existing, next ToolCall) ToolCall {
	if next.SessionID == "" {
		next.SessionID = existing.SessionID
	}
	if next.TurnID == "" {
		next.TurnID = existing.TurnID
	}
	if next.MessageID == "" {
		next.MessageID = existing.MessageID
	}
	if next.Name == "" {
		next.Name = existing.Name
	}
	if next.Source == "" {
		next.Source = existing.Source
	}
	if next.Status == "" {
		next.Status = existing.Status
	}
	if next.InputSummary == "" {
		next.InputSummary = existing.InputSummary
	}
	if next.OutputSummary == "" {
		next.OutputSummary = existing.OutputSummary
	}
	if next.Stdout == "" {
		next.Stdout = existing.Stdout
	}
	if next.Stderr == "" {
		next.Stderr = existing.Stderr
	}
	if next.Error == "" {
		next.Error = existing.Error
	}
	if !existing.StartedAt.IsZero() {
		next.StartedAt = existing.StartedAt
	} else if next.StartedAt.IsZero() {
		next.StartedAt = existing.StartedAt
	}
	if next.FinishedAt.IsZero() {
		next.FinishedAt = existing.FinishedAt
	}
	next.IsError = next.IsError || existing.IsError
	return next
}
