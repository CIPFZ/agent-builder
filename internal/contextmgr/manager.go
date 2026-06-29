package contextmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

type Store interface {
	UpsertProjection(context.Context, Projection) (Projection, error)
	GetProjection(context.Context, string) (Projection, error)
	ListProjectionsByTurn(context.Context, string) ([]Projection, error)
	UpsertProjectionMessage(context.Context, ProjectionMessage) (ProjectionMessage, error)
	ListProjectionMessages(context.Context, string) ([]ProjectionMessage, error)
	UpsertBoundary(context.Context, Boundary) (Boundary, error)
	UpsertContentReplacement(context.Context, ContentReplacement) (ContentReplacement, error)
	GetContentReplacementByToolCall(context.Context, string) (ContentReplacement, error)
	UpsertSnipBoundary(context.Context, SnipBoundary) (SnipBoundary, error)
	UpsertWarning(context.Context, Warning) (Warning, error)
	UpsertReactiveAttempt(context.Context, ReactiveAttempt) (ReactiveAttempt, error)
}

type ManagerOptions struct {
	Store      Store
	Summarizer CompactSummarizer
	Now        func() time.Time
}

type DefaultManager struct {
	store      Store
	summarizer CompactSummarizer
	now        func() time.Time
}

func NewManager(opts ManagerOptions) *DefaultManager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &DefaultManager{store: opts.Store, summarizer: opts.Summarizer, now: now}
}

func (m *DefaultManager) BuildModelInput(ctx context.Context, req BuildInputRequest) (BuildInputResult, error) {
	if m == nil || m.store == nil {
		return BuildInputResult{}, errors.New("context manager store is not available")
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.SessionID == "" {
		return BuildInputResult{}, errors.New("context projection session id is required")
	}
	if req.TurnID == "" {
		return BuildInputResult{}, errors.New("context projection turn id is required")
	}
	if req.Step <= 0 {
		req.Step = 1
	}
	if strings.TrimSpace(req.Source) == "" {
		req.Source = ProjectionSourceMainTurn
	}
	now := req.Now
	if now.IsZero() {
		now = m.now()
	}
	projectionID := ProjectionID(req.TurnID, req.Step)
	projectedModelMessages, replacements, err := m.applyToolResultBudget(ctx, projectionID, req, req.ModelMessages, now.UnixMilli())
	if err != nil {
		return BuildInputResult{}, err
	}
	if projectedModelMessages != nil {
		req.ModelMessages = projectedModelMessages
	}
	microMessages, microReplacements, boundaries, err := m.applyMicrocompact(ctx, projectionID, req, req.ModelMessages, now.UnixMilli())
	if err != nil {
		return BuildInputResult{}, err
	}
	if microMessages != nil {
		req.ModelMessages = microMessages
	}
	replacements = append(replacements, microReplacements...)
	snippedMessages, snipBoundaries, err := m.applySnip(ctx, projectionID, req, req.ModelMessages, now.UnixMilli())
	if err != nil {
		return BuildInputResult{}, err
	}
	if snippedMessages != nil {
		req.ModelMessages = snippedMessages
	}
	fullConfig, autoWarnings, err := m.applyAutoCompact(ctx, projectionID, req, req.ModelMessages, now.UnixMilli())
	if err != nil {
		return BuildInputResult{}, err
	}
	req.FullCompact = fullConfig
	fullMessages, fullBoundaries, err := m.applyFullCompact(ctx, projectionID, req, req.ModelMessages, now.UnixMilli())
	if err != nil {
		return BuildInputResult{}, err
	}
	if fullMessages != nil {
		req.ModelMessages = fullMessages
	}
	boundaries = append(boundaries, fullBoundaries...)
	canonicalCount := req.CanonicalMessages
	if canonicalCount == 0 {
		canonicalCount = len(req.Messages)
		if canonicalCount == 0 {
			canonicalCount = len(req.ModelMessages)
		}
	}
	projectedCount := req.ProjectedMessages
	if projectedCount == 0 {
		projectedCount = len(req.ModelMessages)
		if projectedCount == 0 {
			projectedCount = len(req.Messages)
		}
	}
	projection := Projection{
		ID:                    projectionID,
		SessionID:             req.SessionID,
		TurnID:                req.TurnID,
		Step:                  req.Step,
		Provider:              req.Provider,
		Model:                 req.Model,
		Source:                req.Source,
		Status:                ProjectionStatusCompleted,
		CanonicalMessageCount: canonicalCount,
		ProjectedMessageCount: projectedCount,
		BudgetBefore:          req.BudgetBefore,
		BudgetAfter:           req.BudgetAfter,
		CreatedAt:             now.UnixMilli(),
		CompletedAt:           now.UnixMilli(),
	}
	stored, err := m.store.UpsertProjection(ctx, projection)
	if err != nil {
		return BuildInputResult{}, err
	}
	for i, msg := range req.Messages {
		role := string(msg.Role)
		if role == "" {
			role = "unknown"
		}
		_, err := m.store.UpsertProjectionMessage(ctx, ProjectionMessage{
			ID:                 fmt.Sprintf("%s_msg_%04d", stored.ID, i+1),
			ProjectionID:       stored.ID,
			SessionID:          stored.SessionID,
			TurnID:             stored.TurnID,
			Sequence:           i + 1,
			Role:               role,
			CanonicalMessageID: msg.ID,
			Status:             "selected",
			CreatedAt:          now.UnixMilli(),
		})
		if err != nil {
			return BuildInputResult{}, err
		}
	}
	if len(req.Messages) == 0 {
		for i, msg := range req.ModelMessages {
			role := string(msg.Role)
			if role == "" {
				role = "unknown"
			}
			_, err := m.store.UpsertProjectionMessage(ctx, ProjectionMessage{
				ID:           fmt.Sprintf("%s_msg_%04d", stored.ID, i+1),
				ProjectionID: stored.ID,
				SessionID:    stored.SessionID,
				TurnID:       stored.TurnID,
				Sequence:     i + 1,
				Role:         role,
				Status:       "selected",
				CreatedAt:    now.UnixMilli(),
			})
			if err != nil {
				return BuildInputResult{}, err
			}
		}
	}
	return BuildInputResult{
		Messages:       append([]message.Message(nil), req.Messages...),
		ModelMessages:  append([]fantasy.Message(nil), req.ModelMessages...),
		Projection:     stored,
		Boundaries:     boundaries,
		Replacements:   replacements,
		Warnings:       autoWarnings,
		SnipBoundaries: snipBoundaries,
	}, nil
}

func (m *DefaultManager) ManualCompact(ctx context.Context, req ManualCompactRequest) (CompactResult, error) {
	if m == nil || m.store == nil {
		return CompactResult{}, errors.New("context manager store is not available")
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.ProjectionID = strings.TrimSpace(req.ProjectionID)
	if req.SessionID == "" {
		return CompactResult{}, errors.New("manual compact session id is required")
	}
	if req.TurnID == "" {
		return CompactResult{}, errors.New("manual compact turn id is required")
	}
	now := m.now().UnixMilli()
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_compact"
	}
	boundary, err := m.store.UpsertBoundary(ctx, Boundary{
		ID:           fmt.Sprintf("ctxmanual_compact_%s_%d", strings.ReplaceAll(req.TurnID, ":", "_"), now),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		Kind:         "manual",
		Trigger:      reason,
		Status:       "recorded",
		SummaryRef:   fmt.Sprintf("runtime://turns/%s/context/manual-compact", req.TurnID),
		CreatedAt:    now,
		CompletedAt:  now,
	})
	if err != nil {
		return CompactResult{}, err
	}
	return CompactResult{Boundary: boundary}, nil
}

func (m *DefaultManager) ManualSnip(ctx context.Context, req ManualSnipRequest) (SnipResult, error) {
	if m == nil || m.store == nil {
		return SnipResult{}, errors.New("context manager store is not available")
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.ProjectionID = strings.TrimSpace(req.ProjectionID)
	if req.SessionID == "" {
		return SnipResult{}, errors.New("manual snip session id is required")
	}
	if req.TurnID == "" {
		return SnipResult{}, errors.New("manual snip turn id is required")
	}
	now := m.now().UnixMilli()
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_snip"
	}
	snip, err := m.store.UpsertSnipBoundary(ctx, SnipBoundary{
		ID:           fmt.Sprintf("ctxmanual_snip_%s_%d", strings.ReplaceAll(req.TurnID, ":", "_"), now),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		SummaryRef:   fmt.Sprintf("runtime://turns/%s/context/manual-snip", req.TurnID),
		Reason:       reason,
		CreatedAt:    now,
	})
	if err != nil {
		return SnipResult{}, err
	}
	return SnipResult{SnipBoundary: snip}, nil
}

func ProjectionID(turnID string, step int) string {
	if step <= 0 {
		step = 1
	}
	return fmt.Sprintf("ctxproj_%s_step_%03d", strings.ReplaceAll(strings.TrimSpace(turnID), ":", "_"), step)
}
