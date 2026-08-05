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
	call := toolCallFromRequest(req)
	call.Status = ToolCallRunning
	call.StartedAt = s.now()
	return s.store.Upsert(ctx, call)
}

func (s *Scheduler) QueueCall(ctx context.Context, req ToolCallRequest) (ToolCall, error) {
	call := toolCallFromRequest(req)
	call.Status = ToolCallPending
	return s.store.Upsert(ctx, call)
}

func toolCallFromRequest(req ToolCallRequest) ToolCall {
	return ToolCall{
		ID:                   req.ID,
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		MessageID:            req.MessageID,
		Name:                 req.Name,
		Source:               req.Source,
		CapabilityID:         req.CapabilityID,
		JobID:                req.JobID,
		Command:              req.Command,
		CommandRef:           req.CommandRef,
		CommandByteLength:    req.CommandByteLength,
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
		SandboxDecisionID:    req.SandboxDecisionID,
		SandboxMode:          req.SandboxMode,
		SandboxStatus:        req.SandboxStatus,
		SandboxExecutor:      req.SandboxExecutor,
		SandboxReason:        req.SandboxReason,
		SandboxError:         req.SandboxError,
		JobStatus:            req.JobStatus,
		JobStartedAt:         req.JobStartedAt,
		InputSummary:         req.InputSummary,
		InputRef:             req.InputRef,
		InputByteLength:      req.InputByteLength,
	}
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
	if result.CommandRef != "" {
		call.CommandRef = result.CommandRef
	}
	if result.CommandByteLength != 0 {
		call.CommandByteLength = result.CommandByteLength
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
	if result.SandboxDecisionID != "" {
		call.SandboxDecisionID = result.SandboxDecisionID
	}
	if result.SandboxMode != "" {
		call.SandboxMode = result.SandboxMode
	}
	if result.SandboxStatus != "" {
		call.SandboxStatus = result.SandboxStatus
	}
	if result.SandboxExecutor != "" {
		call.SandboxExecutor = result.SandboxExecutor
	}
	if result.SandboxReason != "" {
		call.SandboxReason = result.SandboxReason
	}
	if result.SandboxError != "" {
		call.SandboxError = result.SandboxError
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
	call.OutputRefs = mergeStringRefs(call.OutputRefs, result.OutputRefs)
	call.ArtifactRefs = mergeStringRefs(call.ArtifactRefs, result.ArtifactRefs)
	call.DiffRefs = mergeStringRefs(call.DiffRefs, result.DiffRefs)
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

func (s *Scheduler) ListSessionCalls(ctx context.Context, sessionID string) ([]ToolCall, error) {
	return s.store.ListBySession(ctx, sessionID)
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
