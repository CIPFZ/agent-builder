package contextmgr

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/config"
)

const (
	defaultMaxSingleToolResultChars = config.DefaultToolResultMaxChars
	defaultMessageToolResultBudget  = config.DefaultToolResultMessageBudget
)

type toolResultCandidate struct {
	messageIndex int
	partIndex    int
	toolCallID   string
	text         string
	size         int
	reason       string
}

func (m *DefaultManager) applyToolResultBudget(ctx context.Context, projectionID string, req BuildInputRequest, messages []fantasy.Message, createdAt int64) ([]fantasy.Message, []ContentReplacement, error) {
	if len(messages) == 0 {
		return messages, nil, nil
	}
	cfg := req.ToolResultBudget
	if cfg.MaxSingleResultChars <= 0 {
		cfg.MaxSingleResultChars = defaultMaxSingleToolResultChars
	}
	if cfg.MessageBudgetChars <= 0 {
		cfg.MessageBudgetChars = defaultMessageToolResultBudget
	}
	projected := cloneModelMessages(messages)
	var replacements []ContentReplacement
	replaced := make(map[string]struct{})
	for msgIndex, msg := range projected {
		var candidates []toolResultCandidate
		total := 0
		for partIndex, part := range msg.Content {
			tr, ok := part.(fantasy.ToolResultPart)
			if !ok || strings.TrimSpace(tr.ToolCallID) == "" {
				continue
			}
			text, ok := toolResultText(tr)
			if !ok {
				continue
			}
			size := utf8.RuneCountInString(text)
			total += size
			candidates = append(candidates, toolResultCandidate{
				messageIndex: msgIndex,
				partIndex:    partIndex,
				toolCallID:   tr.ToolCallID,
				text:         text,
				size:         size,
			})
		}
		for _, candidate := range candidates {
			if candidate.size <= cfg.MaxSingleResultChars {
				continue
			}
			candidate.reason = "single_result_budget"
			replacement, err := m.replaceToolResult(ctx, projectionID, req, &projected, candidate, createdAt)
			if err != nil {
				return nil, nil, err
			}
			replacements = append(replacements, replacement)
			replaced[candidate.toolCallID] = struct{}{}
			total -= maxInt(0, candidate.size-utf8.RuneCountInString(replacement.ReplacementText))
		}
		if total <= cfg.MessageBudgetChars {
			continue
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].size > candidates[j].size
		})
		for _, candidate := range candidates {
			if total <= cfg.MessageBudgetChars {
				break
			}
			if _, ok := replaced[candidate.toolCallID]; ok {
				continue
			}
			candidate.reason = "message_tool_result_budget"
			replacement, err := m.replaceToolResult(ctx, projectionID, req, &projected, candidate, createdAt)
			if err != nil {
				return nil, nil, err
			}
			replacements = append(replacements, replacement)
			replaced[candidate.toolCallID] = struct{}{}
			total -= maxInt(0, candidate.size-utf8.RuneCountInString(replacement.ReplacementText))
		}
	}
	return projected, replacements, nil
}

func (m *DefaultManager) replaceToolResult(ctx context.Context, projectionID string, req BuildInputRequest, messages *[]fantasy.Message, candidate toolResultCandidate, createdAt int64) (ContentReplacement, error) {
	const replacementKind = "tool_result_budget"
	if existing, err := m.store.GetContentReplacement(ctx, req.SessionID, candidate.toolCallID, replacementKind); err == nil && existing.ReplacementText != "" {
		replaceToolResultText(messages, candidate, existing.ReplacementText)
		return existing, nil
	} else if err != nil && err != sql.ErrNoRows {
		return ContentReplacement{}, err
	}
	ref := extractRuntimeObjectRef(candidate.text)
	if ref == "" {
		ref, _ = m.store.RuntimeObjectRefForToolCall(ctx, req.SessionID, candidate.toolCallID)
	}
	if ref == "" {
		return ContentReplacement{}, fmt.Errorf("tool result %s exceeds the provider budget but has no readable runtime object ref", candidate.toolCallID)
	}
	exists, err := m.store.RuntimeObjectRefExists(ctx, req.SessionID, ref)
	if err != nil {
		return ContentReplacement{}, err
	}
	if !exists {
		return ContentReplacement{}, fmt.Errorf("tool result %s references missing runtime object %s", candidate.toolCallID, ref)
	}
	replacementText := buildToolResultReplacement(candidate.text, ref)
	replacement := ContentReplacement{
		ID:                         "ctxrepl_" + stableIDPart(req.SessionID) + "_" + stableIDPart(candidate.toolCallID) + "_" + replacementKind,
		SessionID:                  req.SessionID,
		TurnID:                     req.TurnID,
		ProjectionID:               projectionID,
		ToolCallID:                 candidate.toolCallID,
		Kind:                       replacementKind,
		OriginalRef:                ref,
		ReplacementText:            replacementText,
		OriginalSizeBytes:          int64(len(candidate.text)),
		OriginalEstimatedTokens:    estimateTokens(candidate.text),
		ReplacementEstimatedTokens: estimateTokens(replacementText),
		Reason:                     candidate.reason,
		CreatedAt:                  createdAt,
	}
	stored, err := m.store.UpsertContentReplacement(ctx, replacement)
	if err != nil {
		return ContentReplacement{}, err
	}
	replaceToolResultText(messages, candidate, stored.ReplacementText)
	return stored, nil
}

func extractRuntimeObjectRef(text string) string {
	const prefix = "runtime://objects/"
	start := strings.Index(text, prefix)
	if start < 0 {
		return ""
	}
	end := start + len(prefix)
	for end < len(text) {
		switch text[end] {
		case ' ', '\t', '\r', '\n', '<', '>', '"', '\'':
			return text[start:end]
		default:
			end++
		}
	}
	return text[start:end]
}

func toolResultText(part fantasy.ToolResultPart) (string, bool) {
	switch output := part.Output.(type) {
	case fantasy.ToolResultOutputContentText:
		return output.Text, true
	default:
		return "", false
	}
}

func replaceToolResultText(messages *[]fantasy.Message, candidate toolResultCandidate, replacement string) {
	if candidate.messageIndex < 0 || candidate.messageIndex >= len(*messages) {
		return
	}
	msg := &(*messages)[candidate.messageIndex]
	if candidate.partIndex < 0 || candidate.partIndex >= len(msg.Content) {
		return
	}
	part, ok := msg.Content[candidate.partIndex].(fantasy.ToolResultPart)
	if !ok {
		return
	}
	part.Output = fantasy.ToolResultOutputContentText{Text: replacement}
	msg.Content[candidate.partIndex] = part
}

func buildToolResultReplacement(original, ref string) string {
	preview := safePreview(original, 1200, 800)
	return fmt.Sprintf("<persisted-output>\nThis tool result was too large (%d bytes). The original output is preserved at: %s\nUse a narrower read/view request if exact details are needed.\n\nPreview:\n%s\n</persisted-output>", len(original), ref, preview)
}

func safePreview(text string, headRunes, tailRunes int) string {
	runes := []rune(text)
	if len(runes) <= headRunes+tailRunes {
		return text
	}
	return string(runes[:headRunes]) + "\n...\n" + string(runes[len(runes)-tailRunes:])
}

func cloneModelMessages(in []fantasy.Message) []fantasy.Message {
	out := append([]fantasy.Message(nil), in...)
	for i := range out {
		out[i].Content = append([]fantasy.MessagePart(nil), out[i].Content...)
	}
	return out
}

func stableIDPart(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_")
	return replacer.Replace(value)
}

func estimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	tokens := (utf8.RuneCountInString(text) + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
