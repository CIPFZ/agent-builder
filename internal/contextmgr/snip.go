package contextmgr

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

func (m *DefaultManager) applySnip(ctx context.Context, projectionID string, req BuildInputRequest, messages []fantasy.Message, createdAt int64) ([]fantasy.Message, []SnipBoundary, error) {
	cfg := req.Snip
	if !cfg.Enabled || len(messages) == 0 {
		return messages, nil, nil
	}
	if cfg.PreserveHeadMessages < 0 {
		cfg.PreserveHeadMessages = 0
	}
	if cfg.PreserveTailMessages <= 0 {
		cfg.PreserveTailMessages = 6
	}
	if cfg.PreserveHeadMessages+cfg.PreserveTailMessages >= len(messages) {
		return messages, nil, nil
	}
	reason := strings.TrimSpace(cfg.Reason)
	if reason == "" {
		reason = "manual_snip"
	}
	removeStart := cfg.PreserveHeadMessages
	removeEnd := len(messages) - cfg.PreserveTailMessages
	removedRefs := make([]string, 0, removeEnd-removeStart)
	for i := removeStart; i < removeEnd; i++ {
		removedRefs = append(removedRefs, fmt.Sprintf("projection-message:%s:%d", projectionID, i+1))
	}
	summary := strings.TrimSpace(cfg.Summary)
	if summary == "" {
		summary = fmt.Sprintf("[Snipped %d earlier messages from model input; scrollback remains in canonical history]", len(removedRefs))
	}
	projected := make([]fantasy.Message, 0, cfg.PreserveHeadMessages+1+cfg.PreserveTailMessages)
	projected = append(projected, cloneModelMessages(messages[:removeStart])...)
	projected = append(projected, fantasy.NewSystemMessage("<snip-boundary>\n"+summary+"\n</snip-boundary>"))
	projected = append(projected, cloneModelMessages(messages[removeEnd:])...)
	snip := SnipBoundary{
		ID:                 fmt.Sprintf("ctxsnip_%s", projectionID),
		SessionID:          req.SessionID,
		TurnID:             req.TurnID,
		ProjectionID:       projectionID,
		RemovedMessageRefs: removedRefs,
		PreservedHeadRef:   fmt.Sprintf("projection-message:%s:head:%d", projectionID, cfg.PreserveHeadMessages),
		PreservedTailRef:   fmt.Sprintf("projection-message:%s:tail:%d", projectionID, cfg.PreserveTailMessages),
		SummaryRef:         "runtime://context/projections/" + projectionID + "/snip-summary",
		Reason:             reason,
		CreatedAt:          createdAt,
	}
	stored, err := m.store.UpsertSnipBoundary(ctx, snip)
	if err != nil {
		return nil, nil, err
	}
	return projected, []SnipBoundary{stored}, nil
}
