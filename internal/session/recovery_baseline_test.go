package session

import (
	"testing"
	"time"

	"myclaw/internal/model"
)

func TestRecoverySnapshotBaselineSummarizesRestartContract(t *testing.T) {
	now := time.Now().UTC()
	sess := Session{ID: "session-1", AgentID: "main", Metadata: SessionMetadata{
		PendingApprovalID:        "approval-1",
		PendingApprovalStatus:    "pending",
		PendingApprovalToolUseID: "toolu-write",
		LastCompactBoundaryID:    "compact-1",
		LastCompactionSummaryID:  "summary-1",
		LastCompactionReason:     "message-limit",
	}}
	messages := []Message{
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: previous work", CreatedAt: now},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Subtype: "compact_boundary", Content: "[compact_boundary]", CreatedAt: now.Add(time.Second)},
		{ID: "assistant-tool", SessionID: sess.ID, Role: "assistant", Blocks: []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-write", Name: "Write"}}, CreatedAt: now.Add(2 * time.Second)},
		{ID: "tool-result", SessionID: sess.ID, Role: "tool", Blocks: []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-write", Content: "ok"}}, CreatedAt: now.Add(3 * time.Second)},
		{ID: "skills", SessionID: sess.ID, Role: "attachment", Subtype: "invoked_skills", Content: `{"type":"invoked_skills","skills":[{"skillName":"review","skillPath":"C:/skills/review/SKILL.md","content":"review content","agentId":"main","invokedAt":"2026-04-28T00:00:00Z"}]}`, CreatedAt: now.Add(4 * time.Second)},
	}

	baseline := BuildRecoverySnapshot(sess, messages).Baseline()
	if baseline.SessionID != sess.ID || baseline.Status != ContinuationStatusAwaitingApproval || baseline.ReadyForPrompt {
		t.Fatalf("baseline = %#v, want awaiting approval for session", baseline)
	}
	if baseline.PendingApprovalID != "approval-1" || baseline.PendingApprovalToolUseID != "toolu-write" {
		t.Fatalf("baseline pending approval = %#v", baseline)
	}
	if len(baseline.ToolUseIDs) != 1 || baseline.ToolUseIDs[0] != "toolu-write" {
		t.Fatalf("tool use ids = %#v", baseline.ToolUseIDs)
	}
	if len(baseline.ToolResultIDs) != 1 || baseline.ToolResultIDs[0] != "toolu-write" {
		t.Fatalf("tool result ids = %#v", baseline.ToolResultIDs)
	}
	if baseline.CompactBoundaryID != "compact-1" || baseline.CompactionSummaryID != "summary-1" || baseline.CompactionReason != "message-limit" {
		t.Fatalf("compaction baseline = %#v", baseline)
	}
	if len(baseline.InvokedSkills) != 1 || baseline.InvokedSkills[0].SkillName != "review" {
		t.Fatalf("invoked skills = %#v", baseline.InvokedSkills)
	}
}
