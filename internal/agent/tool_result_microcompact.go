package agent

import (
	"log/slog"
	"time"

	"github.com/charmbracelet/crush/internal/message"
)

const clearedContent = "[Old tool result content cleared]"

var compactableTools = map[string]bool{
	"bash":          true,
	"grep":          true,
	"glob":          true,
	"view":          true,
	"fetch":         true,
	"agentic_fetch": true,
	"web_fetch":     true,
	"web_search":    true,
	"ls":            true,
	"sourcegraph":   true,
}

// Microcompact clears old tool results when the conversation has been idle.
// Follows Claude Code's time-based microcompact design: uses the last assistant
// message's timestamp (not a separately tracked time) to calculate idle gap.
type Microcompact struct {
	compactInterval    time.Duration
	keepLastAssistants int
}

// NewMicrocompact creates a new Microcompact.
func NewMicrocompact(compactInterval time.Duration, keepLastAssistants int) *Microcompact {
	return &Microcompact{
		compactInterval:    compactInterval,
		keepLastAssistants: keepLastAssistants,
	}
}

// Compact clears old tool results if the conversation has been idle longer
// than the compact interval. Uses the last assistant message's CreatedAt
// timestamp to calculate the idle gap (same approach as Claude Code's
// evaluateTimeBasedTrigger).
func (m *Microcompact) Compact(messages []message.Message) []message.Message {
	slog.Debug("Microcompact: called", "msg_count", len(messages))

	// Find last assistant message and calculate gap from its timestamp.
	lastAssistant := m.findLastAssistant(messages)
	if lastAssistant == nil {
		slog.Debug("Microcompact: no assistant found, skipping")
		return messages
	}

	gapDuration := time.Duration(time.Now().Unix()-lastAssistant.CreatedAt) * time.Second
	slog.Debug("Microcompact: gap check",
		"gap", gapDuration.String(),
		"interval", m.compactInterval.String(),
		"last_created_at", lastAssistant.CreatedAt,
	)
	if gapDuration < m.compactInterval {
		slog.Debug("Microcompact: gap too small, skipping")
		return messages
	}

	// Count total assistant messages to determine if compaction is needed.
	totalAssistants := 0
	for _, msg := range messages {
		if msg.Role == message.Assistant {
			totalAssistants++
		}
	}

	// If we have fewer assistants than we want to keep, don't clear anything.
	if totalAssistants <= m.keepLastAssistants {
		slog.Debug("Microcompact: not enough assistants, skipping",
			"total", totalAssistants, "keep_last", m.keepLastAssistants)
		return messages
	}

	slog.Info("Microcompact: clearing old tool results",
		"total_assistants", totalAssistants,
		"keep_last", m.keepLastAssistants,
		"gap", gapDuration.String(),
	)

	// Find cutoff point: messages before this index belong to assistants
	// that should be compacted.
	cutoff := len(messages)
	kept := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == message.Assistant {
			kept++
			if kept > m.keepLastAssistants {
				cutoff = i
				break
			}
		}
	}

	for i := 0; i < cutoff; i++ {
		if messages[i].Role != message.Tool {
			continue
		}
		messages[i] = m.clearMessage(messages[i])
	}

	return messages
}

// findLastAssistant returns a pointer to the last assistant message in the slice.
func (m *Microcompact) findLastAssistant(messages []message.Message) *message.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == message.Assistant {
			return &messages[i]
		}
	}
	return nil
}

func (m *Microcompact) clearMessage(msg message.Message) message.Message {
	var newParts []message.ContentPart
	for _, part := range msg.Parts {
		tr, ok := part.(message.ToolResult)
		if !ok {
			newParts = append(newParts, part)
			continue
		}
		if compactableTools[tr.Name] {
			tr.Content = clearedContent
		}
		newParts = append(newParts, tr)
	}
	msg.Parts = newParts
	return msg
}
