package contextmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

var ErrCompactInputTooLong = errors.New("compact summary input too long")

func (m *DefaultManager) applyFullCompact(ctx context.Context, projectionID string, req BuildInputRequest, messages []fantasy.Message, createdAt int64) ([]fantasy.Message, []Boundary, error) {
	cfg := req.FullCompact
	if !cfg.Enabled {
		return messages, nil, nil
	}
	if m.summarizer == nil {
		return nil, nil, errors.New("full compact summarizer is not available")
	}
	if cfg.PreserveTailMessages <= 0 {
		cfg.PreserveTailMessages = 6
	}
	if cfg.MaxSummaryRetries <= 0 {
		cfg.MaxSummaryRetries = 2
	}
	trigger := strings.TrimSpace(cfg.Trigger)
	if trigger == "" {
		trigger = "manual"
	}
	summaryInput := cloneModelMessages(messages)
	var summary CompactSummaryResult
	var err error
	for attempt := 0; attempt <= cfg.MaxSummaryRetries; attempt++ {
		summary, err = m.summarizer.SummarizeCompact(ctx, CompactSummaryRequest{
			SessionID: req.SessionID,
			TurnID:    req.TurnID,
			Messages:  summaryInput,
		})
		if err == nil {
			break
		}
		if !errors.Is(err, ErrCompactInputTooLong) || len(summaryInput) <= cfg.PreserveTailMessages {
			return nil, nil, err
		}
		summaryInput = dropOldestSummaryRound(summaryInput, cfg.PreserveTailMessages)
	}
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return nil, nil, errors.New("compact summary is empty")
	}
	tail := preservedTail(messages, cfg.PreserveTailMessages)
	projected := make([]fantasy.Message, 0, 1+len(tail)+len(cfg.ReinjectedMessages))
	projected = append(projected, fantasy.NewSystemMessage("<compact-summary>\n"+summary.Summary+"\n</compact-summary>"))
	projected = append(projected, tail...)
	projected = append(projected, cloneModelMessages(cfg.ReinjectedMessages)...)
	messageRefs := make([]string, 0, len(messages)-len(tail))
	omitted := len(messages) - len(tail)
	for i := 0; i < omitted; i++ {
		messageRefs = append(messageRefs, fmt.Sprintf("projection-message:%s:%d", projectionID, i+1))
	}
	reinjectedRefs := make([]string, 0, len(cfg.ReinjectedMessages))
	for i := range cfg.ReinjectedMessages {
		reinjectedRefs = append(reinjectedRefs, fmt.Sprintf("reinjected:%s:%d", projectionID, i+1))
	}
	boundary := Boundary{
		ID:             fmt.Sprintf("ctxbound_full_%s", projectionID),
		SessionID:      req.SessionID,
		TurnID:         req.TurnID,
		ProjectionID:   projectionID,
		Kind:           "full",
		Trigger:        trigger,
		Status:         ProjectionStatusCompleted,
		SummaryRef:     firstNonEmptyString(summary.Ref, "runtime://context/projections/"+projectionID+"/compact-summary"),
		MessageRefs:    messageRefs,
		ReinjectedRefs: reinjectedRefs,
		CreatedAt:      createdAt,
		CompletedAt:    createdAt,
	}
	storedBoundary, err := m.store.UpsertBoundary(ctx, boundary)
	if err != nil {
		return nil, nil, err
	}
	return projected, []Boundary{storedBoundary}, nil
}

func preservedTail(messages []fantasy.Message, keep int) []fantasy.Message {
	if keep <= 0 || len(messages) == 0 {
		return nil
	}
	if keep >= len(messages) {
		return cloneModelMessages(messages)
	}
	return cloneModelMessages(messages[len(messages)-keep:])
}

func dropOldestSummaryRound(messages []fantasy.Message, protectedTail int) []fantasy.Message {
	if len(messages) <= protectedTail {
		return messages
	}
	return cloneModelMessages(messages[1:])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
