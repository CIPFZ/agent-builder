package contextmgr

import (
	"context"
	"fmt"

	"charm.land/fantasy"
)

func (m *DefaultManager) applyAutoCompact(ctx context.Context, projectionID string, req BuildInputRequest, messages []fantasy.Message, createdAt int64) (FullCompactConfig, []Warning, error) {
	cfg := req.AutoCompact
	full := req.FullCompact
	if !cfg.Enabled {
		return full, nil, nil
	}
	var warnings []Warning
	if cfg.HelperCall || cfg.RecursionGuard {
		warning, err := m.recordAutoCompactWarning(ctx, projectionID, req, "auto_compact_skipped", "Auto compact skipped for helper or guarded compact call.", "info", createdAt)
		if err != nil {
			return full, nil, err
		}
		return full, append(warnings, warning), nil
	}
	if cfg.MaxConsecutiveFailures > 0 && cfg.ConsecutiveFailures >= cfg.MaxConsecutiveFailures {
		warning, err := m.recordAutoCompactWarning(ctx, projectionID, req, "auto_compact_circuit_open", "Auto compact skipped after consecutive compact failures.", "error", createdAt)
		if err != nil {
			return full, nil, err
		}
		return full, append(warnings, warning), nil
	}
	if cfg.EffectiveContextWindowTokens <= 0 {
		return full, nil, nil
	}
	used := estimateMessageTokens(messages)
	remaining := cfg.EffectiveContextWindowTokens - cfg.OutputReserveTokens - used
	if cfg.BlockingBufferTokens > 0 && remaining <= cfg.BlockingBufferTokens {
		warning, err := m.recordAutoCompactWarning(ctx, projectionID, req, "context_blocking_threshold", "Projected context is inside the blocking threshold.", "error", createdAt)
		if err != nil {
			return full, nil, err
		}
		warnings = append(warnings, warning)
	}
	if cfg.WarningBufferTokens > 0 && remaining <= cfg.WarningBufferTokens {
		warning, err := m.recordAutoCompactWarning(ctx, projectionID, req, "context_warning_threshold", "Projected context is near the effective context limit.", "warning", createdAt)
		if err != nil {
			return full, nil, err
		}
		warnings = append(warnings, warning)
	}
	if cfg.AutoCompactBufferTokens > 0 && remaining <= cfg.AutoCompactBufferTokens {
		full.Enabled = true
		full.Trigger = "auto"
	}
	return full, warnings, nil
}

func (m *DefaultManager) recordAutoCompactWarning(ctx context.Context, projectionID string, req BuildInputRequest, code, message, severity string, createdAt int64) (Warning, error) {
	warning := Warning{
		ID:           fmt.Sprintf("ctxwarn_%s_%s", projectionID, code),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: projectionID,
		Code:         code,
		Message:      message,
		Severity:     severity,
		CreatedAt:    createdAt,
	}
	return m.store.UpsertWarning(ctx, warning)
}

func estimateMessageTokens(messages []fantasy.Message) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			total += estimateTokens(promptPartText(part))
		}
	}
	return total
}

func promptPartText(part fantasy.MessagePart) string {
	switch value := part.(type) {
	case fantasy.TextPart:
		return value.Text
	case fantasy.ToolResultPart:
		text, ok := toolResultText(value)
		if ok {
			return text
		}
	}
	return ""
}
