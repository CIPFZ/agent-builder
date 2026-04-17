package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/model"
	"myclaw/internal/session"
)

func TestSlashResumeOpensSearchableSessionDialog(t *testing.T) {
	bridge := &fakeBridge{
		sessionSnapshots: []sessionSnapshot{
			{
				Session: session.Session{
					ID:      "main-000001",
					Key:     "agent:main:main",
					AgentID: "main",
					IsMain:  true,
					Metadata: session.SessionMetadata{
						LastActivityAt: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
					},
				},
				MessageCount:     2,
				FirstUserMessage: "Implement TUI resume",
			},
		},
	}
	model := NewModel(bridge)
	model.input = "/resume"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Kind != dialogKindSessionResume {
		t.Fatalf("dialog = %#v, want active resume dialog", model.dialog)
	}
	if !model.dialog.Picker.QueryEnabled {
		t.Fatal("resume dialog query disabled, want searchable")
	}
	if got := model.dialog.Picker.Current(); got.Value != "main-000001" || got.Label != "Implement TUI resume" {
		t.Fatalf("current item = %#v, want resume session item", got)
	}
	if model.input != "" {
		t.Fatalf("input = %q, want cleared after opening dialog", model.input)
	}
}

func TestSessionResumeSelectionRestoresTranscriptAndPromptHistory(t *testing.T) {
	bridge := &fakeBridge{
		sessionSnapshots: []sessionSnapshot{
			{Session: session.Session{ID: "main-000001", Key: "agent:main:main", AgentID: "main", IsMain: true, Metadata: session.SessionMetadata{LastActivityAt: time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC)}}, FirstUserMessage: "older", MessageCount: 1},
			{Session: session.Session{ID: "main-000002", Key: "agent:main:main", AgentID: "main", IsMain: true, Metadata: session.SessionMetadata{LastActivityAt: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)}}, FirstUserMessage: "target", MessageCount: 3},
		},
		resumeSnapshots: map[string]session.RecoverySnapshot{
			"main-000002": {
				Session: session.Session{ID: "main-000002", Key: "agent:main:main", AgentID: "main", IsMain: true},
				Continuation: []session.Message{
					{ID: "msg-1", SessionID: "main-000002", Role: "user", Content: "target"},
					{ID: "msg-2", SessionID: "main-000002", Role: "assistant", Content: "done", Blocks: []model.MessageBlock{{Type: model.MessageBlockText, Text: "done"}}},
					{ID: "msg-3", SessionID: "main-000002", Role: "user", Content: "next prompt"},
				},
			},
		},
	}
	model := NewModel(bridge)
	model.transcript = append(model.transcript, transcriptEntry{Role: "user", Content: "stale"})
	model.busy = true
	model.input = "/resume"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if bridge.resumedSessionID != "main-000002" {
		t.Fatalf("resumedSessionID = %q, want main-000002", bridge.resumedSessionID)
	}
	if len(model.transcript) != 3 {
		t.Fatalf("transcript len = %d, want 3", len(model.transcript))
	}
	if model.transcript[0].Role != "user" || model.transcript[0].Content != "target" {
		t.Fatalf("first transcript = %#v, want restored user message", model.transcript[0])
	}
	if model.transcript[1].Role != "assistant" || len(model.transcript[1].Blocks) != 1 {
		t.Fatalf("assistant transcript = %#v, want restored message blocks", model.transcript[1])
	}
	if len(model.history) != 2 || model.history[0] != "target" || model.history[1] != "next prompt" {
		t.Fatalf("history = %#v, want restored user prompts", model.history)
	}
	if model.diagnostics.SessionID != "main-000002" {
		t.Fatalf("diagnostics session = %q, want main-000002", model.diagnostics.SessionID)
	}
	if !model.busy || model.input != "" || model.dialog.active() {
		t.Fatalf("busy/input/dialog = %v/%q/%v, want awaiting-assistant empty closed", model.busy, model.input, model.dialog.active())
	}
	if model.activity.Label != "Resuming assistant turn" {
		t.Fatalf("activity = %q, want resumed assistant turn", model.activity.Label)
	}
}

func TestSessionResumeSelectionRestoresPendingApprovalState(t *testing.T) {
	bridge := &fakeBridge{
		sessionSnapshots: []sessionSnapshot{
			{Session: session.Session{ID: "main-000002", Key: "agent:main:main", AgentID: "main", IsMain: true, Metadata: session.SessionMetadata{LastActivityAt: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)}}, FirstUserMessage: "target", MessageCount: 3},
		},
		resumeSnapshots: map[string]session.RecoverySnapshot{
			"main-000002": {
				Session: session.Session{
					ID:      "main-000002",
					Key:     "agent:main:main",
					AgentID: "main",
					IsMain:  true,
					Metadata: session.SessionMetadata{
						PendingApprovalID:        "approval-1",
						PendingApprovalStatus:    "pending",
						PendingApprovalToolName:  "system.run",
						PendingApprovalToolInput: "pwd",
						PendingApprovalReason:    "needs approval",
					},
				},
				Continuation: []session.Message{
					{ID: "msg-1", SessionID: "main-000002", Role: "assistant", Content: "waiting for approval"},
				},
			},
		},
	}
	model := NewModel(bridge)
	model.input = "/resume"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.pendingApproval == nil || model.pendingApproval.ID != "approval-1" {
		t.Fatalf("pending approval = %#v, want approval-1 restored from snapshot metadata", model.pendingApproval)
	}
	if !model.approvalDialog.active() {
		t.Fatal("approval dialog inactive after resume, want reopened")
	}
	if model.activity.Label != "Awaiting approval: system.run pwd" {
		t.Fatalf("activity = %q, want awaiting approval label", model.activity.Label)
	}
	if model.busy {
		t.Fatal("busy = true, want false while approval is pending")
	}
}

func TestSessionResumeSelectionUsesContinuationTranscriptForCompactedSessions(t *testing.T) {
	bridge := &fakeBridge{
		sessionSnapshots: []sessionSnapshot{
			{Session: session.Session{ID: "main-000002", Key: "agent:main:main", AgentID: "main", IsMain: true, Metadata: session.SessionMetadata{LastActivityAt: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)}}, FirstUserMessage: "new question", MessageCount: 5},
		},
		resumeSnapshots: map[string]session.RecoverySnapshot{
			"main-000002": session.BuildRecoverySnapshot(session.Session{
				ID:      "main-000002",
				Key:     "agent:main:main",
				AgentID: "main",
				IsMain:  true,
				Metadata: session.SessionMetadata{
					LastCompactBoundaryID:   "compact-1",
					LastCompactionSummaryID: "summary-1",
				},
			}, []session.Message{
				{ID: "msg-old", SessionID: "main-000002", Role: "user", Content: "stale pre-compact"},
				{ID: "summary-1", SessionID: "main-000002", Role: "summary", Content: "Summary: compacted"},
				{ID: "compact-1", SessionID: "main-000002", Role: "system", Content: "[compact_boundary]"},
				{ID: "msg-2", SessionID: "main-000002", Role: "assistant", Content: "post-compact answer"},
				{ID: "msg-3", SessionID: "main-000002", Role: "user", Content: "new question"},
			}),
		},
	}
	model := NewModel(bridge)
	model.input = "/resume"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(model.transcript) != 4 {
		t.Fatalf("transcript len = %d, want compacted continuation only", len(model.transcript))
	}
	if model.transcript[0].Content == "stale pre-compact" {
		t.Fatalf("first transcript = %#v, did not want pre-compact history restored", model.transcript[0])
	}
	if model.transcript[0].Kind != messageKindCompact {
		t.Fatalf("first transcript = %#v, want compaction marker from continuation snapshot", model.transcript[0])
	}
	if len(model.history) != 1 || model.history[0] != "new question" {
		t.Fatalf("history = %#v, want only continuation user prompts", model.history)
	}
}

func TestRuntimeBridgeResumeSessionChangesSendTarget(t *testing.T) {
	sessions := session.NewManager(nil)
	first := sessions.GetOrCreateMain("main")
	second := sessions.CreateChild("main", "agent:main:resume")
	_, _ = sessions.AppendMessage(second.ID, "user", "previous")
	bridge := NewRuntimeBridge(sessions, nil, "main")

	snapshot, ok := bridge.ResumeSession(second.ID)
	if !ok {
		t.Fatal("ResumeSession ok = false, want true")
	}
	if len(snapshot.Continuation) != 1 || snapshot.Continuation[0].Content != "previous" {
		t.Fatalf("snapshot = %#v, want restored previous message in continuation", snapshot)
	}

	if err := bridge.SendUserMessage("after resume"); err != nil {
		t.Fatalf("SendUserMessage after resume: %v", err)
	}
	firstMessages, _ := sessions.Messages(first.ID)
	secondMessages, _ := sessions.Messages(second.ID)
	if len(firstMessages) != 0 {
		t.Fatalf("first session messages = %#v, want unchanged", firstMessages)
	}
	if len(secondMessages) != 2 || secondMessages[1].Content != "after resume" {
		t.Fatalf("second session messages = %#v, want appended follow-up", secondMessages)
	}
}

func TestRuntimeBridgeResumeSessionReturnsSemanticRecoverySnapshot(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-old", SessionID: sess.ID, Role: "user", Content: "stale pre-compact"},
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted"},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]"},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "post-compact answer"},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	bridge := NewRuntimeBridge(sessions, nil, "main")
	snapshot, ok := bridge.ResumeSession(sess.ID)
	if !ok {
		t.Fatal("ResumeSession ok = false, want true")
	}
	if len(snapshot.FullHistory) != 4 {
		t.Fatalf("full history len = %d, want 4", len(snapshot.FullHistory))
	}
	if len(snapshot.Continuation) != 3 {
		t.Fatalf("continuation len = %d, want compacted semantic view", len(snapshot.Continuation))
	}
	if snapshot.Continuation[0].ID != "summary-1" || snapshot.Continuation[1].ID != "compact-1" {
		t.Fatalf("continuation = %#v, want summary + boundary semantic restore", snapshot.Continuation)
	}
}

func TestRuntimeBridgeSessionSnapshotsIncludeTitlesAndActivity(t *testing.T) {
	sessions := session.NewManager(nil)
	first := sessions.GetOrCreateMain("main")
	second := sessions.CreateChild("main", "agent:main:later")
	_, _ = sessions.AppendMessage(first.ID, "user", "first prompt")
	_, _ = sessions.AppendMessage(first.ID, "assistant", "first answer")
	_, _ = sessions.AppendMessage(second.ID, "user", "second prompt")
	bridge := NewRuntimeBridge(sessions, nil, "main")

	items := sessionResumeItems(bridge.SessionSnapshots())

	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if items[0].Value != second.ID || items[0].Label != "second prompt" {
		t.Fatalf("first item = %#v, want most recent second session", items[0])
	}
	if items[1].Value != first.ID || items[1].Label != "first prompt" {
		t.Fatalf("second item = %#v, want older first session", items[1])
	}
}
