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
	if isFinalToolCallStatus(existing.Status) && next.Status == ToolCallRunning {
		next.Status = existing.Status
		next.FinishedAt = existing.FinishedAt
	}
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
	if next.JobID == "" {
		next.JobID = existing.JobID
	}
	if next.Command == "" {
		next.Command = existing.Command
	}
	if next.Risk == "" {
		next.Risk = existing.Risk
	}
	if next.PolicyReason == "" {
		next.PolicyReason = existing.PolicyReason
	}
	if next.ExitCode == 0 {
		next.ExitCode = existing.ExitCode
	}
	if next.JobStatus == "" {
		next.JobStatus = existing.JobStatus
	}
	if next.JobStartedAt.IsZero() {
		next.JobStartedAt = existing.JobStartedAt
	}
	if next.JobFinishedAt.IsZero() {
		next.JobFinishedAt = existing.JobFinishedAt
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
	if next.ModelContent == "" {
		next.ModelContent = existing.ModelContent
	}
	if next.Structured == "" {
		next.Structured = existing.Structured
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
	if !next.Compacted {
		next.Compacted = existing.Compacted
	}
	if next.CompactRef == "" {
		next.CompactRef = existing.CompactRef
	}
	if next.CompactBoundaryID == "" {
		next.CompactBoundaryID = existing.CompactBoundaryID
	}
	if next.CompactOriginalEstimatedTokens == 0 {
		next.CompactOriginalEstimatedTokens = existing.CompactOriginalEstimatedTokens
	}
	if next.CompactedAt.IsZero() {
		next.CompactedAt = existing.CompactedAt
	}
	if !existing.StartedAt.IsZero() {
		next.StartedAt = existing.StartedAt
	} else if next.StartedAt.IsZero() {
		next.StartedAt = existing.StartedAt
	}
	if !isFinalToolCallStatus(next.Status) {
		next.FinishedAt = time.Time{}
	} else if next.FinishedAt.IsZero() {
		next.FinishedAt = existing.FinishedAt
	}
	next.IsError = next.IsError || existing.IsError
	return next
}

func isFinalToolCallStatus(status ToolCallStatus) bool {
	switch status {
	case ToolCallCompleted, ToolCallFailed, ToolCallCancelled, ToolCallDenied:
		return true
	default:
		return false
	}
}
