package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"myclaw/internal/approval"
	"myclaw/internal/session"
)

const dialogKindSessionResume = "session-resume"

func (m *Model) openSessionResumeDialog() {
	bridge, ok := m.bridge.(sessionResumeBridge)
	if !ok {
		m.dialog.open(dialogSpec{
			Kind:       dialogKindSessionResume,
			Title:      "Resume session",
			Subtitle:   "Session resume is not available for this bridge",
			EmptyText:  "No resumable sessions",
			FooterHint: "Esc close",
		})
		return
	}
	items := sessionResumeItems(bridge.SessionSnapshots())
	m.dialog.open(dialogSpec{
		Kind:         dialogKindSessionResume,
		Title:        "Resume session",
		Subtitle:     "Search and select a previous session",
		QueryEnabled: true,
		Items:        items,
		EmptyText:    "No conversations found to resume",
		FooterHint:   "Type to filter | Enter resume | Esc cancel",
		VisibleCount: 7,
	})
	m.clearSuggestions()
}

func sessionResumeItems(snapshots []sessionSnapshot) []dialogItem {
	sorted := append([]sessionSnapshot(nil), snapshots...)
	sort.SliceStable(sorted, func(i, j int) bool {
		leftActivity := sessionActivity(sorted[i])
		rightActivity := sessionActivity(sorted[j])
		if !leftActivity.Equal(rightActivity) {
			return leftActivity.After(rightActivity)
		}
		return sorted[i].Session.ID > sorted[j].Session.ID
	})
	items := make([]dialogItem, 0, len(sorted))
	for _, snapshot := range sorted {
		if snapshot.Session.ID == "" {
			continue
		}
		label := strings.TrimSpace(snapshot.FirstUserMessage)
		if label == "" {
			label = snapshot.Session.ID
		}
		description := fmt.Sprintf("%s | %d messages", snapshot.Session.ID, snapshot.MessageCount)
		if at := sessionActivity(snapshot); !at.IsZero() {
			description += " | " + at.Local().Format("2006-01-02 15:04")
		}
		if snapshot.LastMessage != "" && snapshot.LastMessage != label {
			description += " | " + truncateResumeText(snapshot.LastMessage, 80)
		}
		items = append(items, dialogItem{
			Label:       truncateResumeText(label, 80),
			Description: description,
			Value:       snapshot.Session.ID,
		})
	}
	return items
}

func sessionActivity(snapshot sessionSnapshot) time.Time {
	return snapshot.Session.Metadata.LastActivityAt
}

func truncateResumeText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func (m *Model) acceptSessionResumeItem(item dialogItem) {
	if item.Value == "" {
		return
	}
	bridge, ok := m.bridge.(sessionResumeBridge)
	if !ok {
		return
	}
	snapshot, ok := bridge.ResumeSession(item.Value)
	if !ok {
		m.events = appendBoundedEvent(m.events, "resume failed: "+item.Value, 200)
		return
	}
	m.restoreSessionSnapshot(snapshot)
}

func (m *Model) restoreSessionSnapshot(snapshot session.RecoverySnapshot) {
	messages := snapshot.Continuation
	m.transcript = make([]transcriptEntry, 0, len(messages))
	m.history = m.history[:0]
	for _, message := range messages {
		if entry, ok := transcriptEntryFromSessionMessage(message); ok {
			m.transcript = append(m.transcript, entry)
		}
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			m.history = append(m.history, strings.TrimSpace(message.Content))
		}
	}
	m.diagnostics.SessionID = snapshot.Session.ID
	m.diagnostics.LastEvent = "session.resumed"
	m.diagnostics.EventCount++
	m.events = appendBoundedEvent(m.events, "session.resumed: "+snapshot.Session.ID, 200)
	m.input = ""
	m.cursorPos = 0
	m.historyIndex = -1
	m.dialog.close()
	m.messageActions.close()
	m.toolExpansion.clear()
	m.pastes = newPasteState()
	m.viewport.Search = transcriptSearchState{}
	m.applyResumedContinuationState(snapshot)
	m.scrollTranscriptBottom()
}

func (m *Model) applyResumedContinuationState(snapshot session.RecoverySnapshot) {
	state := snapshot.ContinuationState()
	m.pendingApproval = nil
	m.approvalDialog.close()
	m.busy = !state.ReadyForPrompt
	m.activity.Label = "Idle"

	if state.Status == session.ContinuationStatusAwaitingAssistant {
		m.activity.Label = "Resuming assistant turn"
	}

	if approvalRequest, ok := pendingApprovalRequest(snapshot); ok {
		m.pendingApproval = approvalRequest
		m.approvalDialog.open(approvalRequest)
		m.busy = false
		m.activity.Label = "Awaiting approval: " + strings.TrimSpace(approvalRequest.ToolName+" "+approvalRequest.ToolInput)
	}
}

func pendingApprovalRequest(snapshot session.RecoverySnapshot) (*approval.Request, bool) {
	metadata := snapshot.Metadata
	if metadata.PendingApprovalID == "" && snapshot.Session.Metadata.PendingApprovalID != "" {
		metadata = snapshot.Session.Metadata
	}
	if metadata.PendingApprovalID == "" || metadata.PendingApprovalStatus != "pending" {
		return nil, false
	}
	request := &approval.Request{
		ID:                metadata.PendingApprovalID,
		SessionID:         snapshot.Session.ID,
		RunID:             metadata.PendingApprovalRunID,
		UserMessageID:     metadata.PendingApprovalUserMessageID,
		ToolName:          metadata.PendingApprovalToolName,
		ToolInput:         metadata.PendingApprovalToolInput,
		ToolInputObject:   cloneAnyMap(metadata.PendingApprovalToolInputObject),
		ToolUseID:         metadata.PendingApprovalToolUseID,
		ProviderMessageID: metadata.PendingApprovalProviderMsgID,
		Category:          metadata.PendingApprovalCategory,
		RuleSource:        metadata.PendingApprovalRuleSource,
		Reason:            metadata.PendingApprovalReason,
		DecisionReason:    metadata.PendingApprovalDecisionReason,
		AcceptFeedback:    metadata.PendingApprovalAcceptFeedback,
		Status:            approval.StatusPending,
	}
	if len(metadata.PendingApprovalContentBlocks) > 0 {
		request.ContentBlocks = make([]map[string]any, 0, len(metadata.PendingApprovalContentBlocks))
		for _, block := range metadata.PendingApprovalContentBlocks {
			request.ContentBlocks = append(request.ContentBlocks, cloneAnyMap(block))
		}
	}
	return request, true
}

func transcriptEntryFromSessionMessage(message session.Message) (transcriptEntry, bool) {
	if entry, ok := specialTranscriptEntryFromMessage(message); ok {
		return entry, true
	}
	switch message.Role {
	case "user", "assistant":
		return transcriptEntry{
			Role:    message.Role,
			Content: message.Content,
			Blocks:  cloneMessageBlocks(message.Blocks),
		}, true
	case "tool":
		return transcriptEntry{
			Role:    "tool",
			Content: message.Content,
			Blocks:  cloneMessageBlocks(message.Blocks),
		}, true
	case "system":
		return transcriptEntry{Kind: messageKindSystem, Role: "system", Content: message.Content}, true
	default:
		return transcriptEntry{}, false
	}
}
