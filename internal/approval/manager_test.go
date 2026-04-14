package approval_test

import (
	"testing"

	"myclaw/internal/approval"
)

func TestManagerListBySessionAndStatusFiltersResults(t *testing.T) {
	manager := approval.NewManager()
	first := manager.Create("session-1", "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	second := manager.Create("session-1", "run-2", "msg-2", "system.run", "rm -rf ./build", "dangerous command", "dangerous-command", "")
	_, _ = manager.UpdateStatus(second.ID, approval.StatusRejected)
	manager.Create("session-2", "run-3", "msg-3", "system.run", "pwd", "approval required", "approval", "")

	pending := manager.ListBySessionAndStatus("session-1", approval.StatusPending)
	if len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("pending approvals = %#v, want only first request", pending)
	}

	rejected := manager.ListBySessionAndStatus("session-1", approval.StatusRejected)
	if len(rejected) != 1 || rejected[0].ID != second.ID {
		t.Fatalf("rejected approvals = %#v, want only second request", rejected)
	}
}

func TestManagerClearSessionDefaultsToTerminalStatusesOnly(t *testing.T) {
	manager := approval.NewManager()
	pending := manager.Create("session-1", "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	rejected := manager.Create("session-1", "run-2", "msg-2", "system.run", "rm -rf ./build", "dangerous command", "dangerous-command", "")
	approved := manager.Create("session-1", "run-3", "msg-3", "system.run", "echo ok", "approval required", "approval", "")
	_, _ = manager.UpdateStatus(rejected.ID, approval.StatusRejected)
	_, _ = manager.UpdateStatus(approved.ID, approval.StatusApproved)

	cleared := manager.ClearBySessionAndStatus("session-1", "")

	if cleared != 2 {
		t.Fatalf("cleared count = %d, want 2", cleared)
	}
	remaining := manager.ListBySession("session-1")
	if len(remaining) != 1 || remaining[0].ID != pending.ID {
		t.Fatalf("remaining approvals = %#v, want only pending approval", remaining)
	}
}

func TestManagerCreateStoresCategory(t *testing.T) {
	manager := approval.NewManager()
	request := manager.Create("session-1", "run-1", "msg-1", "system.run", "pwd", "approval required", "workspace-boundary", "")

	if request.Category != "workspace-boundary" {
		t.Fatalf("approval category = %q, want workspace-boundary", request.Category)
	}
}

func TestManagerCreateStoresRuleSource(t *testing.T) {
	manager := approval.NewManager()
	request := manager.Create("session-1", "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "session")

	if request.RuleSource != "session" {
		t.Fatalf("approval rule source = %q, want session", request.RuleSource)
	}
}

func TestManagerUpdatePromptMetadataStoresClonedApprovalFeedbackBlocks(t *testing.T) {
	manager := approval.NewManager()
	request := manager.Create("session-1", "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	contentBlocks := []map[string]any{{
		"type": "text",
		"text": "reviewer note",
	}}

	updated, err := manager.UpdatePromptMetadata(request.ID, "approved with context", contentBlocks)
	if err != nil {
		t.Fatalf("update prompt metadata: %v", err)
	}
	contentBlocks[0]["text"] = "mutated"

	if updated.AcceptFeedback != "approved with context" {
		t.Fatalf("accept feedback = %q, want stored feedback", updated.AcceptFeedback)
	}
	if len(updated.ContentBlocks) != 1 || updated.ContentBlocks[0]["text"] != "reviewer note" {
		t.Fatalf("content blocks = %#v, want cloned block", updated.ContentBlocks)
	}
	restored, ok := manager.Get(request.ID)
	if !ok {
		t.Fatal("expected stored approval")
	}
	if len(restored.ContentBlocks) != 1 || restored.ContentBlocks[0]["text"] != "reviewer note" {
		t.Fatalf("stored content blocks = %#v, want cloned block", restored.ContentBlocks)
	}
}
