package runtime

import (
	"fmt"
	"strings"
	"time"

	"myclaw/internal/compaction"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/session"
)

type SessionCompactionSnapshot struct {
	Analysis             compaction.Analysis
	LastCompactionReason string
	LastCompactedAt      time.Time
}

type SessionCompactionResult struct {
	Changed        bool
	Reason         string
	OriginalCount  int
	CompactedCount int
}

func (r *Runner) CompactionSnapshot(sessionID string) (SessionCompactionSnapshot, error) {
	sess, ok := r.sessions.GetByID(sessionID)
	if !ok {
		return SessionCompactionSnapshot{}, fmt.Errorf("session %q not found", sessionID)
	}
	messages, _ := r.sessions.Messages(sessionID)
	return SessionCompactionSnapshot{
		Analysis:             r.options.Compactor.Analyze(messages),
		LastCompactionReason: strings.TrimSpace(sess.Metadata.LastCompactionReason),
		LastCompactedAt:      sess.Metadata.LastCompactedAt,
	}, nil
}

func (r *Runner) CompactSession(sessionID, customInstructions string) (SessionCompactionResult, error) {
	sess, history, err := r.sessionHistory(sessionID)
	if err != nil {
		return SessionCompactionResult{}, err
	}

	var result compaction.Result
	if strings.TrimSpace(customInstructions) == "" {
		result = r.options.Compactor.CompactWithSessionMemory(
			history,
			r.latestSummaryMemory(sessionID),
			strings.TrimSpace(sess.Metadata.LastSummarizedMessageID),
		)
	} else {
		result = r.options.Compactor.Compact(history)
	}
	if err := r.applyManualCompaction(sess, result); err != nil {
		return SessionCompactionResult{}, err
	}
	return sessionCompactionResult(result), nil
}

func (r *Runner) MicrocompactSession(sessionID string) (SessionCompactionResult, error) {
	sess, history, err := r.sessionHistory(sessionID)
	if err != nil {
		return SessionCompactionResult{}, err
	}

	result := r.options.Compactor.Microcompact(history)
	if err := r.applyManualCompaction(sess, result); err != nil {
		return SessionCompactionResult{}, err
	}
	return sessionCompactionResult(result), nil
}

func (r *Runner) sessionHistory(sessionID string) (session.Session, []session.Message, error) {
	sess, ok := r.sessions.GetByID(sessionID)
	if !ok {
		return session.Session{}, nil, fmt.Errorf("session %q not found", sessionID)
	}
	history, ok := r.sessions.Messages(sessionID)
	if !ok || len(history) == 0 {
		return session.Session{}, nil, fmt.Errorf("session %q has no messages to compact", sessionID)
	}
	return sess, history, nil
}

func (r *Runner) applyManualCompaction(sess session.Session, result compaction.Result) error {
	if !result.Changed {
		return nil
	}

	if result.Reason == compaction.ReasonMicrocompact {
		if err := r.sessions.ReplaceMessages(sess.ID, cloneCompactedMessages(result.Messages)); err != nil {
			return err
		}
		return r.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
			metadata.LastCompactionReason = string(result.Reason)
			metadata.LastCompactedAt = time.Now().UTC()
		})
	}

	messages := cloneCompactedMessages(result.Messages)
	var boundary session.Message
	if result.BoundaryMessage != nil {
		boundary = *result.BoundaryMessage
	} else {
		boundary = newCompactBoundaryMessage(sess.ID)
		messages = append(messages, boundary)
	}
	if boundary.CreatedAt.IsZero() {
		boundary.CreatedAt = time.Now().UTC()
	}
	if err := r.sessions.ReplaceMessages(sess.ID, messages); err != nil {
		return err
	}

	if result.SummaryMessage != nil && r.options.MemoryService != nil {
		r.options.MemoryService.SaveCompactionSummary(sess, *result.SummaryMessage)
	}

	return r.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastCompactBoundaryID = boundary.ID
		metadata.LastCompactionReason = string(result.Reason)
		metadata.LastCompactedAt = boundary.CreatedAt
		if result.SummaryMessage != nil {
			metadata.LastCompactionSummaryID = result.SummaryMessage.ID
		}
		if result.SummarizedThroughID != "" {
			metadata.LastSummarizedMessageID = result.SummarizedThroughID
		}
	})
}

func (r *Runner) latestSummaryMemory(sessionID string) string {
	if r.options.MemoryService != nil {
		items := r.options.MemoryService.List(sessionID)
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].Type == memory.TypeSummary && strings.TrimSpace(items[i].Content) != "" {
				return items[i].Content
			}
		}
	}
	if messages, ok := r.sessions.Messages(sessionID); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			if (message.Role == "summary" || (message.Role == "user" && message.IsCompactSummary)) && strings.TrimSpace(message.Content) != "" {
				return message.Content
			}
		}
	}
	return ""
}

func sessionCompactionResult(result compaction.Result) SessionCompactionResult {
	return SessionCompactionResult{
		Changed:        result.Changed,
		Reason:         string(result.Reason),
		OriginalCount:  result.OriginalCount,
		CompactedCount: result.CompactedCount,
	}
}

func cloneCompactedMessages(messages []model.Message) []session.Message {
	cloned := make([]session.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func newCompactBoundaryMessage(sessionID string) session.Message {
	now := time.Now().UTC()
	return session.Message{
		ID:        "compact-" + now.Format("20060102150405"),
		SessionID: sessionID,
		Role:      "system",
		Subtype:   "compact_boundary",
		Content:   "Conversation compacted",
		CreatedAt: now,
	}
}
