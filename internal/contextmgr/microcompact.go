package contextmgr

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
)

const defaultMicrocompactKeepRecent = 3

func (m *DefaultManager) applyMicrocompact(ctx context.Context, projectionID string, req BuildInputRequest, messages []fantasy.Message, createdAt int64) ([]fantasy.Message, []ContentReplacement, []Boundary, error) {
	cfg := req.Microcompact
	if !cfg.Enabled || len(messages) == 0 {
		return messages, nil, nil, nil
	}
	if cfg.KeepRecent <= 0 {
		cfg.KeepRecent = defaultMicrocompactKeepRecent
	}
	candidates := compactableToolResults(messages)
	if len(candidates) <= cfg.KeepRecent {
		return messages, nil, nil, nil
	}
	trigger := microcompactTrigger(cfg, candidates, createdAt)
	if trigger == "" {
		return messages, nil, nil, nil
	}
	projected := cloneModelMessages(messages)
	cutoff := len(candidates) - cfg.KeepRecent
	var replacements []ContentReplacement
	var refs []string
	for _, candidate := range candidates[:cutoff] {
		replacement, err := m.microcompactToolResult(ctx, projectionID, req, &projected, candidate, trigger, createdAt)
		if err != nil {
			return nil, nil, nil, err
		}
		replacements = append(replacements, replacement)
		refs = append(refs, replacement.ToolCallID)
	}
	if len(replacements) == 0 {
		return projected, nil, nil, nil
	}
	boundary := Boundary{
		ID:           fmt.Sprintf("ctxbound_micro_%s", projectionID),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: projectionID,
		Kind:         "micro",
		Trigger:      trigger,
		Status:       ProjectionStatusCompleted,
		ToolCallRefs: refs,
		CreatedAt:    createdAt,
		CompletedAt:  createdAt,
	}
	storedBoundary, err := m.store.UpsertBoundary(ctx, boundary)
	if err != nil {
		return nil, nil, nil, err
	}
	return projected, replacements, []Boundary{storedBoundary}, nil
}

func compactableToolResults(messages []fantasy.Message) []toolResultCandidate {
	var out []toolResultCandidate
	for msgIndex, msg := range messages {
		if msg.Role != fantasy.MessageRoleTool {
			continue
		}
		for partIndex, part := range msg.Content {
			tr, ok := part.(fantasy.ToolResultPart)
			if !ok || strings.TrimSpace(tr.ToolCallID) == "" {
				continue
			}
			text, ok := toolResultText(tr)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, toolResultCandidate{
				messageIndex: msgIndex,
				partIndex:    partIndex,
				toolCallID:   tr.ToolCallID,
				text:         text,
				size:         utf8.RuneCountInString(text),
			})
		}
	}
	return out
}

func microcompactTrigger(cfg MicrocompactConfig, candidates []toolResultCandidate, nowMillis int64) string {
	if cfg.IdleIntervalMillis > 0 && cfg.LastAssistantAt > 0 && nowMillis-cfg.LastAssistantAt >= cfg.IdleIntervalMillis {
		return "idle_time"
	}
	if cfg.ToolResultCountLimit > 0 && len(candidates) > cfg.ToolResultCountLimit {
		return "tool_result_count"
	}
	if cfg.ToolResultTokenLimit > 0 {
		total := 0
		for _, candidate := range candidates {
			total += estimateTokens(candidate.text)
		}
		if total > cfg.ToolResultTokenLimit {
			return "tool_result_tokens"
		}
	}
	return ""
}

func (m *DefaultManager) microcompactToolResult(ctx context.Context, projectionID string, req BuildInputRequest, messages *[]fantasy.Message, candidate toolResultCandidate, trigger string, createdAt int64) (ContentReplacement, error) {
	if existing, err := m.store.GetContentReplacementByToolCall(ctx, candidate.toolCallID); err == nil && existing.ReplacementText != "" {
		replaceToolResultText(messages, candidate, existing.ReplacementText)
		return existing, nil
	} else if err != nil && err != sql.ErrNoRows {
		return ContentReplacement{}, err
	}
	ref := fmt.Sprintf("runtime://context/tool-results/%s/original", stableIDPart(candidate.toolCallID))
	replacementText := fmt.Sprintf("[Old tool result content compacted by projection microcompact; original output preserved at %s]", ref)
	replacement := ContentReplacement{
		ID:                         "ctxrepl_" + stableIDPart(candidate.toolCallID),
		SessionID:                  req.SessionID,
		TurnID:                     req.TurnID,
		ProjectionID:               projectionID,
		ToolCallID:                 candidate.toolCallID,
		Kind:                       "tool_result",
		OriginalRef:                ref,
		ReplacementText:            replacementText,
		OriginalSizeBytes:          int64(len(candidate.text)),
		OriginalEstimatedTokens:    estimateTokens(candidate.text),
		ReplacementEstimatedTokens: estimateTokens(replacementText),
		Reason:                     "microcompact_" + trigger,
		CreatedAt:                  createdAt,
	}
	stored, err := m.store.UpsertContentReplacement(ctx, replacement)
	if err != nil {
		return ContentReplacement{}, err
	}
	replaceToolResultText(messages, candidate, stored.ReplacementText)
	return stored, nil
}
