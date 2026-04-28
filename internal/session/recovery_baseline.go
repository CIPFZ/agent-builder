package session

import "myclaw/internal/model"

type RecoveryBaseline struct {
	SessionID                string
	Status                   ContinuationStatus
	ReadyForPrompt           bool
	ResumeFromMessageID      string
	ResumeFromRole           string
	PendingApprovalID        string
	PendingApprovalToolUseID string
	ToolUseIDs               []string
	ToolResultIDs            []string
	CompactBoundaryID        string
	CompactionSummaryID      string
	CompactionReason         string
	InvokedSkills            []RecoveredInvokedSkillInfo
}

func (s RecoverySnapshot) Baseline() RecoveryBaseline {
	state := s.ContinuationState()
	baseline := RecoveryBaseline{
		SessionID:                s.Session.ID,
		Status:                   state.Status,
		ReadyForPrompt:           state.ReadyForPrompt,
		ResumeFromMessageID:      state.ResumeFromMessageID,
		ResumeFromRole:           state.ResumeFromRole,
		PendingApprovalID:        s.Metadata.PendingApprovalID,
		PendingApprovalToolUseID: s.Metadata.PendingApprovalToolUseID,
		CompactionReason:         s.Metadata.LastCompactionReason,
		InvokedSkills:            s.RecoveredInvokedSkills(),
	}
	if boundary, ok := s.CompactBoundary(); ok {
		baseline.CompactBoundaryID = boundary.ID
	}
	if summary, ok := s.CompactionSummary(); ok {
		baseline.CompactionSummaryID = summary.ID
	}
	for _, message := range s.Continuation {
		for _, block := range message.Blocks {
			switch block.Type {
			case model.MessageBlockToolUse:
				if block.ID != "" {
					baseline.ToolUseIDs = appendUniqueString(baseline.ToolUseIDs, block.ID)
				}
			case model.MessageBlockToolResult:
				if block.ToolUseID != "" {
					baseline.ToolResultIDs = appendUniqueString(baseline.ToolResultIDs, block.ToolUseID)
				}
			}
		}
	}
	return baseline
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
