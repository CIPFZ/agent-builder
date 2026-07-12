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
	ListBySession(context.Context, string) ([]ToolCall, error)
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

func (s *MemoryStore) ListBySession(_ context.Context, sessionID string) ([]ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	calls := make([]ToolCall, 0)
	for _, call := range s.calls {
		if call.SessionID == sessionID {
			calls = append(calls, call)
		}
	}
	sort.SliceStable(calls, func(i, j int) bool {
		if calls[i].StartedAt.Equal(calls[j].StartedAt) {
			return calls[i].ID < calls[j].ID
		}
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
	if next.PolicyMode == "" {
		next.PolicyMode = existing.PolicyMode
	}
	if next.PolicyProfile == "" {
		next.PolicyProfile = existing.PolicyProfile
	}
	if !next.PolicyHeadless {
		next.PolicyHeadless = existing.PolicyHeadless
	}
	if next.PolicyHeadlessReason == "" {
		next.PolicyHeadlessReason = existing.PolicyHeadlessReason
	}
	if next.PolicyRuleID == "" {
		next.PolicyRuleID = existing.PolicyRuleID
	}
	if next.PolicyRuleSource == "" {
		next.PolicyRuleSource = existing.PolicyRuleSource
	}
	if next.PolicyScopeKind == "" {
		next.PolicyScopeKind = existing.PolicyScopeKind
	}
	if next.PolicyScopeValue == "" {
		next.PolicyScopeValue = existing.PolicyScopeValue
	}
	if next.PolicyTargetSummary == "" {
		next.PolicyTargetSummary = existing.PolicyTargetSummary
	}
	if next.ShellRisk == "" {
		next.ShellRisk = existing.ShellRisk
	}
	if next.ShellReason == "" {
		next.ShellReason = existing.ShellReason
	}
	if next.SandboxDecisionID == "" {
		next.SandboxDecisionID = existing.SandboxDecisionID
	}
	if next.SandboxMode == "" {
		next.SandboxMode = existing.SandboxMode
	}
	if next.SandboxStatus == "" {
		next.SandboxStatus = existing.SandboxStatus
	}
	if next.SandboxExecutor == "" {
		next.SandboxExecutor = existing.SandboxExecutor
	}
	if next.SandboxReason == "" {
		next.SandboxReason = existing.SandboxReason
	}
	if next.SandboxError == "" {
		next.SandboxError = existing.SandboxError
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
	next.OutputRefs = mergeStringRefs(existing.OutputRefs, next.OutputRefs)
	next.ArtifactRefs = mergeStringRefs(existing.ArtifactRefs, next.ArtifactRefs)
	next.DiffRefs = mergeStringRefs(existing.DiffRefs, next.DiffRefs)
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

func mergeStringRefs(existing, next []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(next))
	out := make([]string, 0, len(existing)+len(next))
	for _, value := range append(existing, next...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isFinalToolCallStatus(status ToolCallStatus) bool {
	switch status {
	case ToolCallCompleted, ToolCallFailed, ToolCallCancelled, ToolCallDenied:
		return true
	default:
		return false
	}
}
