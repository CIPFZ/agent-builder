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
		ID:                   req.ID,
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		MessageID:            req.MessageID,
		Name:                 req.Name,
		Source:               req.Source,
		CapabilityID:         req.CapabilityID,
		JobID:                req.JobID,
		Command:              req.Command,
		Risk:                 req.Risk,
		PolicyReason:         req.PolicyReason,
		PolicyMode:           req.PolicyMode,
		PolicyProfile:        req.PolicyProfile,
		PolicyHeadless:       req.PolicyHeadless,
		PolicyHeadlessReason: req.PolicyHeadlessReason,
		PolicyRuleID:         req.PolicyRuleID,
		PolicyRuleSource:     req.PolicyRuleSource,
		PolicyScopeKind:      req.PolicyScopeKind,
		PolicyScopeValue:     req.PolicyScopeValue,
		PolicyTargetSummary:  req.PolicyTargetSummary,
		ShellRisk:            req.ShellRisk,
		ShellReason:          req.ShellReason,
		JobStatus:            req.JobStatus,
		JobStartedAt:         req.JobStartedAt,
		Status:               ToolCallRunning,
		InputSummary:         req.InputSummary,
		StartedAt:            s.now(),
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
	if result.JobID != "" {
		call.JobID = result.JobID
	}
	if result.Command != "" {
		call.Command = result.Command
	}
	if result.Risk != "" {
		call.Risk = result.Risk
	}
	if result.PolicyReason != "" {
		call.PolicyReason = result.PolicyReason
	}
	if result.PolicyMode != "" {
		call.PolicyMode = result.PolicyMode
	}
	if result.PolicyProfile != "" {
		call.PolicyProfile = result.PolicyProfile
	}
	if result.PolicyHeadless {
		call.PolicyHeadless = result.PolicyHeadless
	}
	if result.PolicyHeadlessReason != "" {
		call.PolicyHeadlessReason = result.PolicyHeadlessReason
	}
	if result.PolicyRuleID != "" {
		call.PolicyRuleID = result.PolicyRuleID
	}
	if result.PolicyRuleSource != "" {
		call.PolicyRuleSource = result.PolicyRuleSource
	}
	if result.PolicyScopeKind != "" {
		call.PolicyScopeKind = result.PolicyScopeKind
	}
	if result.PolicyScopeValue != "" {
		call.PolicyScopeValue = result.PolicyScopeValue
	}
	if result.PolicyTargetSummary != "" {
		call.PolicyTargetSummary = result.PolicyTargetSummary
	}
	if result.ShellRisk != "" {
		call.ShellRisk = result.ShellRisk
	}
	if result.ShellReason != "" {
		call.ShellReason = result.ShellReason
	}
	if result.ExitCode != 0 {
		call.ExitCode = result.ExitCode
	}
	if result.JobStatus != "" {
		call.JobStatus = result.JobStatus
	} else if result.JobID != "" && call.JobStatus == "" {
		call.JobStatus = string(result.Status)
	}
	if !result.JobStartedAt.IsZero() {
		call.JobStartedAt = result.JobStartedAt
	}
	if !result.JobFinishedAt.IsZero() {
		call.JobFinishedAt = result.JobFinishedAt
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

func (s *Scheduler) UpdateCall(ctx context.Context, call ToolCall) (ToolCall, error) {
	return s.store.Upsert(ctx, call)
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
