package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestContinuationMessagesKeepsLatestSummaryBoundaryAndTail(t *testing.T) {
	now := time.Now().UTC()
	messages := []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old user", CreatedAt: now},
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "new tail", CreatedAt: now.Add(3 * time.Second)},
	}

	got := ContinuationMessages(messages)
	if len(got) != 3 {
		t.Fatalf("message count = %d, want 3", len(got))
	}
	if got[0].Role != "summary" || got[0].ID != "summary-1" {
		t.Fatalf("first message = %#v, want latest pre-boundary summary", got[0])
	}
	if got[1].Content != "[compact_boundary]" {
		t.Fatalf("second message = %#v, want compact boundary", got[1])
	}
	if got[2].Content != "new tail" {
		t.Fatalf("third message = %#v, want post-boundary tail", got[2])
	}
}

func TestContinuationMessagesKeepsClaudeCompactSummaryBoundaryAndTail(t *testing.T) {
	now := time.Now().UTC()
	messages := []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old user", CreatedAt: now},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Subtype: "compact_boundary", Content: "Conversation compacted", CreatedAt: now.Add(time.Second)},
		{ID: "summary-1", SessionID: "sess-1", Role: "user", Content: "This session is being continued from a previous conversation that ran out of context.", IsCompactSummary: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "new tail", CreatedAt: now.Add(3 * time.Second)},
	}

	got := ContinuationMessages(messages)
	if len(got) != 3 {
		t.Fatalf("message count = %d, want 3", len(got))
	}
	if got[0].Subtype != "compact_boundary" || got[0].Content != "Conversation compacted" {
		t.Fatalf("first message = %#v, want Claude compact boundary", got[0])
	}
	if !got[1].IsCompactSummary || got[1].Role != "user" {
		t.Fatalf("second message = %#v, want Claude compact summary", got[1])
	}
	if got[2].Content != "new tail" {
		t.Fatalf("third message = %#v, want post-boundary tail", got[2])
	}

	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, messages)
	if boundary, ok := snapshot.CompactBoundary(); !ok || boundary.ID != "compact-1" {
		t.Fatalf("boundary = %#v, %v, want Claude compact boundary", boundary, ok)
	}
	if summary, ok := snapshot.CompactionSummary(); !ok || summary.ID != "summary-1" {
		t.Fatalf("summary = %#v, %v, want Claude compact summary", summary, ok)
	}
}

func TestContinuationMessagesUsesLatestBoundaryWhenTranscriptContainsMultipleCompactions(t *testing.T) {
	now := time.Now().UTC()
	messages := []Message{
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: older", CreatedAt: now},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(time.Second)},
		{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "mid tail", CreatedAt: now.Add(2 * time.Second)},
		{ID: "summary-2", SessionID: "sess-1", Role: "summary", Content: "Summary: newer", CreatedAt: now.Add(3 * time.Second)},
		{ID: "compact-2", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(4 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "user", Content: "latest tail", CreatedAt: now.Add(5 * time.Second)},
	}

	got := ContinuationMessages(messages)
	if len(got) != 3 {
		t.Fatalf("message count = %d, want 3", len(got))
	}
	if got[0].ID != "summary-2" {
		t.Fatalf("first message = %#v, want latest summary before last boundary", got[0])
	}
	if got[1].ID != "compact-2" {
		t.Fatalf("second message = %#v, want last compact boundary", got[1])
	}
	if got[2].ID != "msg-2" {
		t.Fatalf("third message = %#v, want post-boundary tail", got[2])
	}
}

func TestContinuationMessagesReturnsCloneWhenNoCompactBoundaryExists(t *testing.T) {
	now := time.Now().UTC()
	messages := []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "hello", CreatedAt: now},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "world", CreatedAt: now.Add(time.Second)},
	}

	got := ContinuationMessages(messages)
	if len(got) != len(messages) {
		t.Fatalf("message count = %d, want %d", len(got), len(messages))
	}
	got[0].Content = "mutated"
	if messages[0].Content != "hello" {
		t.Fatalf("original messages = %#v, want clone semantics", messages)
	}
}

func TestManagerContinuationMessagesUsesRecoveryView(t *testing.T) {
	manager := NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := manager.ReplaceMessages(sess.ID, []Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "old", CreatedAt: now},
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "tail", CreatedAt: now.Add(3 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}

	got, ok := manager.ContinuationMessages(sess.ID)
	if !ok {
		t.Fatalf("continuation messages for %q not found", sess.ID)
	}
	if len(got) != 3 {
		t.Fatalf("message count = %d, want 3", len(got))
	}
	if got[0].Role != "summary" || got[1].Content != "[compact_boundary]" || got[2].Content != "tail" {
		t.Fatalf("messages = %#v, want continuation-safe transcript", got)
	}
}

func TestManagerContinuationMessagesSynthesizesCompactionAnchorsFromMetadata(t *testing.T) {
	manager := NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := manager.ReplaceMessages(sess.ID, []Message{
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "tail", CreatedAt: now},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := manager.UpdateMetadata(sess.ID, func(metadata *SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	got, ok := manager.ContinuationMessages(sess.ID)
	if !ok {
		t.Fatalf("continuation messages for %q not found", sess.ID)
	}
	if len(got) != 3 {
		t.Fatalf("message count = %d, want synthesized summary + boundary + tail", len(got))
	}
	if got[0].ID != "summary-1" || got[0].Role != "summary" {
		t.Fatalf("first message = %#v, want synthesized summary", got[0])
	}
	if got[1].ID != "compact-1" || got[1].Content != "[compact_boundary]" {
		t.Fatalf("second message = %#v, want synthesized compact boundary", got[1])
	}
	if got[2].ID != "msg-2" || got[2].Content != "tail" {
		t.Fatalf("third message = %#v, want preserved tail", got[2])
	}
}

func TestRecoverySnapshotIncludesSessionMetadataAndBothTranscriptViews(t *testing.T) {
	now := time.Now().UTC()
	sess := Session{
		ID:      "sess-1",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
		Metadata: SessionMetadata{
			LastUserMessageID:       "msg-2",
			LastAssistantMessageID:  "msg-3",
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
			LastCompactionReason:    "message-limit",
			LastCompactedAt:         now.Add(2 * time.Second),
		},
	}
	messages := []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old", CreatedAt: now},
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "user", Content: "latest user", CreatedAt: now.Add(3 * time.Second)},
		{ID: "msg-3", SessionID: "sess-1", Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(4 * time.Second)},
	}

	snapshot := BuildRecoverySnapshot(sess, messages)
	if snapshot.Session.ID != sess.ID {
		t.Fatalf("snapshot session = %#v, want %q", snapshot.Session, sess.ID)
	}
	if len(snapshot.FullHistory) != 5 {
		t.Fatalf("full history count = %d, want 5", len(snapshot.FullHistory))
	}
	if len(snapshot.Continuation) != 4 {
		t.Fatalf("continuation count = %d, want 4", len(snapshot.Continuation))
	}
	if snapshot.Continuation[0].Role != "summary" {
		t.Fatalf("continuation = %#v, want summary at front", snapshot.Continuation)
	}
	if snapshot.Metadata.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("snapshot metadata = %#v, want compact boundary", snapshot.Metadata)
	}
}

func TestManagerRecoverySnapshotUsesPersistedMetadataAndMessages(t *testing.T) {
	manager := NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := manager.ReplaceMessages(sess.ID, []Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "old", CreatedAt: now},
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "tail", CreatedAt: now.Add(3 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := manager.UpdateMetadata(sess.ID, func(metadata *SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = "message-limit"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	snapshot, ok := manager.RecoverySnapshot(sess.ID)
	if !ok {
		t.Fatalf("snapshot for %q not found", sess.ID)
	}
	if snapshot.Session.ID != sess.ID {
		t.Fatalf("snapshot session = %#v, want %q", snapshot.Session, sess.ID)
	}
	if snapshot.Metadata.LastCompactionSummaryID != "summary-1" {
		t.Fatalf("snapshot metadata = %#v, want persisted summary id", snapshot.Metadata)
	}
	if len(snapshot.Continuation) != 3 {
		t.Fatalf("continuation count = %d, want 3", len(snapshot.Continuation))
	}
}

func TestRecoverySnapshotExposesDerivedResumeAnchors(t *testing.T) {
	now := time.Now().UTC()
	sess := Session{
		ID:      "sess-1",
		Key:     "agent:main:main",
		AgentID: "main",
		Metadata: SessionMetadata{
			LastUserMessageID:       "msg-2",
			LastAssistantMessageID:  "msg-3",
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
		},
	}
	snapshot := BuildRecoverySnapshot(sess, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "old", CreatedAt: now},
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "user", Content: "latest user", CreatedAt: now.Add(3 * time.Second)},
		{ID: "msg-3", SessionID: "sess-1", Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(4 * time.Second)},
	})

	user, ok := snapshot.LastUserMessage()
	if !ok || user.ID != "msg-2" {
		t.Fatalf("last user message = %#v, %v, want msg-2", user, ok)
	}
	assistant, ok := snapshot.LastAssistantMessage()
	if !ok || assistant.ID != "msg-3" {
		t.Fatalf("last assistant message = %#v, %v, want msg-3", assistant, ok)
	}
	boundary, ok := snapshot.CompactBoundary()
	if !ok || boundary.ID != "compact-1" {
		t.Fatalf("compact boundary = %#v, %v, want compact-1", boundary, ok)
	}
	summary, ok := snapshot.CompactionSummary()
	if !ok || summary.ID != "summary-1" {
		t.Fatalf("compaction summary = %#v, %v, want summary-1", summary, ok)
	}
	if !snapshot.HasCompaction() {
		t.Fatalf("snapshot = %#v, want compaction marker", snapshot)
	}
}

func TestRecoverySnapshotFallsBackToLatestContinuationMessagesWhenMetadataIDsAreMissing(t *testing.T) {
	now := time.Now().UTC()
	sess := Session{
		ID:      "sess-1",
		Key:     "agent:main:main",
		AgentID: "main",
	}
	snapshot := BuildRecoverySnapshot(sess, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "first", CreatedAt: now},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "second", CreatedAt: now.Add(time.Second)},
		{ID: "msg-3", SessionID: "sess-1", Role: "user", Content: "latest user", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-4", SessionID: "sess-1", Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(3 * time.Second)},
	})

	user, ok := snapshot.LastUserMessage()
	if !ok || user.ID != "msg-3" {
		t.Fatalf("last user message = %#v, %v, want msg-3", user, ok)
	}
	assistant, ok := snapshot.LastAssistantMessage()
	if !ok || assistant.ID != "msg-4" {
		t.Fatalf("last assistant message = %#v, %v, want msg-4", assistant, ok)
	}
}

func TestRecoverySnapshotRestoresInvokedSkillsFromAttachmentMessages(t *testing.T) {
	now := time.Now().UTC()
	attachmentPayload := struct {
		Type   string `json:"type"`
		Skills []struct {
			SkillName string `json:"skillName"`
			SkillPath string `json:"skillPath"`
			Content   string `json:"content"`
			AgentID   string `json:"agentId"`
			InvokedAt string `json:"invokedAt"`
		} `json:"skills"`
	}{
		Type: "invoked_skills",
		Skills: []struct {
			SkillName string `json:"skillName"`
			SkillPath string `json:"skillPath"`
			Content   string `json:"content"`
			AgentID   string `json:"agentId"`
			InvokedAt string `json:"invokedAt"`
		}{
			{
				SkillName: "research",
				SkillPath: "/skills/research/SKILL.md",
				Content:   "Use the skill to gather sources.",
				AgentID:   "agent-1",
				InvokedAt: now.Format(time.RFC3339Nano),
			},
		},
	}
	payload, err := json.Marshal(attachmentPayload)
	if err != nil {
		t.Fatalf("marshal attachment payload: %v", err)
	}

	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, []Message{
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: compacted", CreatedAt: now},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Subtype: "compact_boundary", Content: "Conversation compacted", CreatedAt: now.Add(time.Second)},
		{ID: "attachment-1", SessionID: "sess-1", Role: "attachment", Subtype: "invoked_skills", IsMeta: true, IsVisibleInTranscriptOnly: true, Content: string(payload), CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(3 * time.Second)},
	})

	skills := snapshot.RecoveredInvokedSkills()
	if len(skills) != 1 {
		t.Fatalf("skills = %#v, want one recovered invoked skill", skills)
	}
	if skills[0].SkillName != "research" || skills[0].SkillPath != "/skills/research/SKILL.md" || skills[0].AgentID != "agent-1" {
		t.Fatalf("skills[0] = %#v, want recovered invoked skill info", skills[0])
	}
	if skills[0].Content != "Use the skill to gather sources." {
		t.Fatalf("skills[0].Content = %q, want recovered content", skills[0].Content)
	}
}

func TestRecoverySnapshotContinuationStateTreatsTrailingAssistantAsReadyForNextPrompt(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "question", CreatedAt: now},
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "answer", CreatedAt: now.Add(time.Second)},
	})

	state := snapshot.ContinuationState()
	if !state.ReadyForPrompt {
		t.Fatalf("state = %#v, want ready for next prompt", state)
	}
	if state.Status != ContinuationStatusAwaitingUser {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusAwaitingUser)
	}
	if state.ResumeFromMessageID != "msg-2" || state.ResumeFromRole != "assistant" {
		t.Fatalf("state = %#v, want assistant anchor", state)
	}
}

func TestRecoverySnapshotContinuationStateTreatsTrailingUserAsUnfinishedTurn(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "question", CreatedAt: now},
	})

	state := snapshot.ContinuationState()
	if state.ReadyForPrompt {
		t.Fatalf("state = %#v, did not want ready for next prompt", state)
	}
	if state.Status != ContinuationStatusAwaitingAssistant {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusAwaitingAssistant)
	}
	if state.ResumeFromMessageID != "msg-1" || state.ResumeFromRole != "user" {
		t.Fatalf("state = %#v, want user anchor", state)
	}
}

func TestRecoverySnapshotContinuationStateTreatsTrailingToolAsUnfinishedTurn(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "user", Content: "question", CreatedAt: now},
		{ID: "msg-2", SessionID: "sess-1", Role: "tool", Content: "tool result", CreatedAt: now.Add(time.Second)},
	})

	state := snapshot.ContinuationState()
	if state.ReadyForPrompt {
		t.Fatalf("state = %#v, did not want ready for next prompt", state)
	}
	if state.Status != ContinuationStatusAwaitingAssistant {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusAwaitingAssistant)
	}
	if state.ResumeFromMessageID != "msg-2" || state.ResumeFromRole != "tool" {
		t.Fatalf("state = %#v, want tool anchor", state)
	}
}

func TestRecoverySnapshotContinuationStateTreatsPendingApprovalAsDistinctBlockedState(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{
		ID: "sess-1",
		Metadata: SessionMetadata{
			PendingApprovalID:     "approval-1",
			PendingApprovalStatus: "pending",
		},
	}, []Message{
		{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "waiting for approval", CreatedAt: now},
	})

	state := snapshot.ContinuationState()
	if state.ReadyForPrompt {
		t.Fatalf("state = %#v, did not want ready for next prompt", state)
	}
	if state.Status != ContinuationStatusAwaitingApproval {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusAwaitingApproval)
	}
	if state.ResumeFromMessageID != "approval-1" || state.ResumeFromRole != "approval" {
		t.Fatalf("state = %#v, want approval anchor", state)
	}
}

func TestRecoverySnapshotContinuationStateUsesCompactionBoundaryWhenOnlyCompactedViewRemains(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{
		ID: "sess-1",
		Metadata: SessionMetadata{
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
		},
	}, []Message{
		{ID: "summary-1", SessionID: "sess-1", Role: "summary", Content: "Summary: compacted", CreatedAt: now},
		{ID: "compact-1", SessionID: "sess-1", Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(time.Second)},
	})

	state := snapshot.ContinuationState()
	if !state.ReadyForPrompt {
		t.Fatalf("state = %#v, want compacted session to be ready for next prompt", state)
	}
	if state.Status != ContinuationStatusAwaitingUser {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusAwaitingUser)
	}
	if state.ResumeFromMessageID != "compact-1" || state.ResumeFromRole != "system" {
		t.Fatalf("state = %#v, want compact boundary anchor", state)
	}
	if !state.HasCompaction {
		t.Fatalf("state = %#v, want compaction marker", state)
	}
}

func TestRecoverySnapshotSynthesizesCompactionBoundaryFromMetadataWhenTranscriptDropsIt(t *testing.T) {
	now := time.Now().UTC()
	snapshot := BuildRecoverySnapshot(Session{
		ID: "sess-1",
		Metadata: SessionMetadata{
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
		},
	}, []Message{
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "tail after cleanup", CreatedAt: now},
	})

	boundary, ok := snapshot.CompactBoundary()
	if !ok {
		t.Fatal("expected compact boundary synthesized from metadata")
	}
	if boundary.ID != "compact-1" || boundary.Role != "system" || boundary.Content != "[compact_boundary]" {
		t.Fatalf("boundary = %#v, want synthesized compact boundary", boundary)
	}
	if !snapshot.HasCompaction() {
		t.Fatalf("snapshot = %#v, want compaction marker from metadata", snapshot)
	}
}

func TestRecoverySnapshotSynthesizesCompactionSummaryFromMetadataWhenTranscriptDropsIt(t *testing.T) {
	snapshot := BuildRecoverySnapshot(Session{
		ID: "sess-1",
		Metadata: SessionMetadata{
			LastCompactBoundaryID:   "compact-1",
			LastCompactionSummaryID: "summary-1",
		},
	}, []Message{
		{ID: "msg-2", SessionID: "sess-1", Role: "assistant", Content: "tail after cleanup"},
	})

	summary, ok := snapshot.CompactionSummary()
	if !ok {
		t.Fatal("expected compaction summary synthesized from metadata")
	}
	if summary.ID != "summary-1" || summary.Role != "summary" {
		t.Fatalf("summary = %#v, want synthesized summary anchor", summary)
	}
}

func TestRecoverySnapshotContinuationStateTreatsEmptyHistoryAsReady(t *testing.T) {
	snapshot := BuildRecoverySnapshot(Session{ID: "sess-1"}, nil)

	state := snapshot.ContinuationState()
	if !state.ReadyForPrompt {
		t.Fatalf("state = %#v, want ready for first prompt", state)
	}
	if state.Status != ContinuationStatusEmpty {
		t.Fatalf("status = %q, want %q", state.Status, ContinuationStatusEmpty)
	}
}
