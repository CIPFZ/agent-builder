package session

type RecoverySnapshot struct {
	Session      Session
	Metadata     SessionMetadata
	FullHistory  []Message
	Continuation []Message
}

type ContinuationStatus string

const (
	ContinuationStatusEmpty             ContinuationStatus = "empty"
	ContinuationStatusAwaitingUser      ContinuationStatus = "awaiting_user"
	ContinuationStatusAwaitingAssistant ContinuationStatus = "awaiting_assistant"
	ContinuationStatusAwaitingApproval  ContinuationStatus = "awaiting_approval"
)

type ContinuationState struct {
	Status             ContinuationStatus
	ReadyForPrompt     bool
	ResumeFromMessageID string
	ResumeFromRole     string
	HasCompaction      bool
}

func BuildRecoverySnapshot(sess Session, messages []Message) RecoverySnapshot {
	return RecoverySnapshot{
		Session:      sess,
		Metadata:     sess.Metadata,
		FullHistory:  cloneMessages(messages),
		Continuation: synthesizedContinuationMessages(sess, messages),
	}
}

func (s RecoverySnapshot) LastUserMessage() (Message, bool) {
	if msg, ok := messageByID(s.FullHistory, s.Metadata.LastUserMessageID); ok {
		return msg, true
	}
	return latestMessageByRole(s.Continuation, "user")
}

func (s RecoverySnapshot) LastAssistantMessage() (Message, bool) {
	if msg, ok := messageByID(s.FullHistory, s.Metadata.LastAssistantMessageID); ok {
		return msg, true
	}
	return latestMessageByRole(s.Continuation, "assistant")
}

func (s RecoverySnapshot) CompactBoundary() (Message, bool) {
	if msg, ok := messageByID(s.FullHistory, s.Metadata.LastCompactBoundaryID); ok {
		return msg, true
	}
	for i := len(s.Continuation) - 1; i >= 0; i-- {
		if isCompactBoundary(s.Continuation[i]) {
			return s.Continuation[i], true
		}
	}
	if s.Metadata.LastCompactBoundaryID != "" {
		return Message{
			ID:        s.Metadata.LastCompactBoundaryID,
			SessionID: s.Session.ID,
			Role:      "system",
			Content:   "[compact_boundary]",
		}, true
	}
	return Message{}, false
}

func (s RecoverySnapshot) CompactionSummary() (Message, bool) {
	if msg, ok := messageByID(s.FullHistory, s.Metadata.LastCompactionSummaryID); ok {
		return msg, true
	}
	for i := len(s.Continuation) - 1; i >= 0; i-- {
		if s.Continuation[i].Role == "summary" {
			return s.Continuation[i], true
		}
	}
	if s.Metadata.LastCompactionSummaryID != "" {
		return Message{
			ID:        s.Metadata.LastCompactionSummaryID,
			SessionID: s.Session.ID,
			Role:      "summary",
		}, true
	}
	return Message{}, false
}

func (s RecoverySnapshot) HasCompaction() bool {
	_, ok := s.CompactBoundary()
	return ok
}

func (s RecoverySnapshot) ContinuationState() ContinuationState {
	state := ContinuationState{
		Status:         ContinuationStatusEmpty,
		ReadyForPrompt: true,
		HasCompaction:  s.HasCompaction(),
	}
	if s.Metadata.PendingApprovalID != "" && s.Metadata.PendingApprovalStatus == "pending" {
		state.Status = ContinuationStatusAwaitingApproval
		state.ReadyForPrompt = false
		state.ResumeFromMessageID = s.Metadata.PendingApprovalID
		state.ResumeFromRole = "approval"
		return state
	}

	last, ok := latestContinuationAnchor(s.Continuation)
	if !ok {
		if boundary, ok := s.CompactBoundary(); ok {
			state.Status = ContinuationStatusAwaitingUser
			state.ReadyForPrompt = true
			state.ResumeFromMessageID = boundary.ID
			state.ResumeFromRole = boundary.Role
		}
		return state
	}

	state.ResumeFromMessageID = last.ID
	state.ResumeFromRole = last.Role
	switch last.Role {
	case "user":
		state.Status = ContinuationStatusAwaitingAssistant
		state.ReadyForPrompt = false
	case "tool":
		state.Status = ContinuationStatusAwaitingAssistant
		state.ReadyForPrompt = false
	default:
		state.Status = ContinuationStatusAwaitingUser
		state.ReadyForPrompt = true
	}
	return state
}

func ContinuationMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	lastBoundary := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if isCompactBoundary(messages[i]) {
			lastBoundary = i
			break
		}
	}
	if lastBoundary < 0 {
		return cloneMessages(messages)
	}

	start := lastBoundary
	for i := lastBoundary - 1; i >= 0; i-- {
		if messages[i].Role == "summary" {
			start = i
			break
		}
	}
	return cloneMessages(messages[start:])
}

func synthesizedContinuationMessages(sess Session, messages []Message) []Message {
	continuation := ContinuationMessages(messages)
	if len(continuation) == 0 {
		return synthesizeCompactionAnchors(sess, continuation)
	}
	return synthesizeCompactionAnchors(sess, continuation)
}

func isCompactBoundary(message Message) bool {
	return message.Role == "system" && message.Content == "[compact_boundary]"
}

func cloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}

func messageByID(messages []Message, messageID string) (Message, bool) {
	if messageID == "" {
		return Message{}, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ID == messageID {
			return messages[i], true
		}
	}
	return Message{}, false
}

func latestMessageByRole(messages []Message, role string) (Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return messages[i], true
		}
	}
	return Message{}, false
}

func latestContinuationAnchor(messages []Message) (Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		switch messages[i].Role {
		case "user", "assistant", "tool":
			return messages[i], true
		}
	}
	return Message{}, false
}

func synthesizeCompactionAnchors(sess Session, continuation []Message) []Message {
	if sess.Metadata.LastCompactBoundaryID == "" && sess.Metadata.LastCompactionSummaryID == "" {
		return continuation
	}
	hasSummary := false
	hasBoundary := false
	for _, message := range continuation {
		if message.Role == "summary" {
			hasSummary = true
		}
		if isCompactBoundary(message) {
			hasBoundary = true
		}
	}
	if hasSummary && hasBoundary {
		return continuation
	}

	prefix := make([]Message, 0, 2)
	if !hasSummary && sess.Metadata.LastCompactionSummaryID != "" {
		prefix = append(prefix, Message{
			ID:        sess.Metadata.LastCompactionSummaryID,
			SessionID: sess.ID,
			Role:      "summary",
		})
	}
	if !hasBoundary && sess.Metadata.LastCompactBoundaryID != "" {
		prefix = append(prefix, Message{
			ID:        sess.Metadata.LastCompactBoundaryID,
			SessionID: sess.ID,
			Role:      "system",
			Content:   "[compact_boundary]",
		})
	}
	if len(prefix) == 0 {
		return continuation
	}
	out := make([]Message, 0, len(prefix)+len(continuation))
	out = append(out, prefix...)
	out = append(out, continuation...)
	return out
}
