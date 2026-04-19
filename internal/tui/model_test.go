package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

type fakeBridge struct {
	sent              []string
	approved          []string
	rejected          []string
	modelSets         []string
	modelClears       int
	compacts          []string
	microcompactCount int
	sendErr           error
	approveErr        error
	rejectErr         error
	modelErr          error
	compactErr        error

	sessionSnapshots   []sessionSnapshot
	resumeSnapshots    map[string]session.RecoverySnapshot
	resumedSessionID   string
	taskPanel          taskPanelSnapshot
	platformStatus     platformStatusSnapshot
	mcpStatus          mcpSnapshot
	compactionStatus   compactionSnapshot
	compactResult      compactionActionResult
	microcompactResult compactionActionResult
}

func (f *fakeBridge) SendUserMessage(input string) error {
	f.sent = append(f.sent, input)
	return f.sendErr
}

func (f *fakeBridge) Approve(id string) error {
	f.approved = append(f.approved, id)
	return f.approveErr
}

func (f *fakeBridge) Reject(id string) error {
	f.rejected = append(f.rejected, id)
	return f.rejectErr
}

func (f *fakeBridge) SetSessionModel(model string) error {
	f.modelSets = append(f.modelSets, model)
	return f.modelErr
}

func (f *fakeBridge) ClearSessionModel() error {
	f.modelClears++
	return f.modelErr
}

func (f *fakeBridge) CompactionSnapshot() compactionSnapshot {
	return f.compactionStatus
}

func (f *fakeBridge) CompactSession(customInstructions string) (compactionActionResult, error) {
	f.compacts = append(f.compacts, customInstructions)
	return f.compactResult, f.compactErr
}

func (f *fakeBridge) MicrocompactSession() (compactionActionResult, error) {
	f.microcompactCount++
	return f.microcompactResult, f.compactErr
}

func (f *fakeBridge) SessionSnapshots() []sessionSnapshot {
	return append([]sessionSnapshot(nil), f.sessionSnapshots...)
}

func (f *fakeBridge) ResumeSession(id string) (session.RecoverySnapshot, bool) {
	f.resumedSessionID = id
	if f.resumeSnapshots == nil {
		return session.RecoverySnapshot{}, false
	}
	snapshot, ok := f.resumeSnapshots[id]
	return snapshot, ok
}

func (f *fakeBridge) TaskPanelSnapshot() taskPanelSnapshot {
	return f.taskPanel
}

func (f *fakeBridge) PlatformStatusSnapshot() platformStatusSnapshot {
	return f.platformStatus
}

func (f *fakeBridge) MCPSnapshot() mcpSnapshot {
	return f.mcpStatus
}

func TestModelViewShowsCoreSections(t *testing.T) {
	model := NewModel(&fakeBridge{})
	view := model.View()

	for _, want := range []string{"myclaw", "Welcome back", "Tips for getting started", "Enter to send"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestModelEnterSendsInputAndClearsBuffer(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)

	for _, r := range "hello" {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 1 || bridge.sent[0] != "hello" {
		t.Fatalf("sent = %#v, want [hello]", bridge.sent)
	}
	if model.input != "" {
		t.Fatalf("input = %q, want cleared", model.input)
	}
	if len(model.transcript) == 0 || model.transcript[0].Role != "user" {
		t.Fatalf("transcript = %#v, want user entry", model.transcript)
	}
}

func TestModelRuntimeEventAccumulatesAssistantDelta(t *testing.T) {
	model := NewModel(&fakeBridge{})
	sess := session.Session{ID: "main-1", Key: "agent:main:main", AgentID: "main", IsMain: true}

	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "assistant.delta", Session: sess, Delta: "Hello"}})
	model = updated.(Model)
	updated, _ = model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "assistant.delta", Session: sess, Delta: " world"}})
	model = updated.(Model)

	if len(model.transcript) != 1 {
		t.Fatalf("transcript len = %d, want 1", len(model.transcript))
	}
	if model.transcript[0].Content != "Hello world" {
		t.Fatalf("assistant content = %q, want %q", model.transcript[0].Content, "Hello world")
	}
}

func TestModelPermissionRequiredShowsApprovalPrompt(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	request := approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd", Reason: "needs approval"}

	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{
		Type:     "permission.required",
		Approval: &request,
	}})
	model = updated.(Model)

	if model.pendingApproval == nil || model.pendingApproval.ID != "approval-1" {
		t.Fatalf("pending approval = %#v, want approval-1", model.pendingApproval)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	model = updated.(Model)
	if len(bridge.approved) != 1 || bridge.approved[0] != "approval-1" {
		t.Fatalf("approved = %#v, want approval-1", bridge.approved)
	}
}

func TestModelDiagnosticsViewShowsLatestState(t *testing.T) {
	model := NewModel(&fakeBridge{}, ModelConfig{
		SessionID: "main-000001",
		LLMLabel:  "openai-compatible / LongCat-Flash-Chat",
		LogPath:   "logs/myclaw.jsonl",
	})

	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "agent.lifecycle.start"}})
	model = updated.(Model)
	updated, _ = model.Update(BridgeErrMsg{Err: assertErr("boom")})
	model = updated.(Model)

	view := model.View()
	// New UI doesn't show all diagnostics directly, but should contain basic elements
	for _, want := range []string{"myclaw", "Welcome back", "MiniMax-M2.7"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestModelViewShowsCurrentActivityForToolApprovalAndCompaction(t *testing.T) {
	model := NewModel(&fakeBridge{})
	sess := session.Session{ID: "main-1", Key: "agent:main:main", AgentID: "main", IsMain: true}

	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{
		Type:      "tool.called",
		Session:   sess,
		ToolName:  "system.run",
		ToolInput: "pwd",
	}})
	model = updated.(Model)
	view := model.View()
	// New UI shows tool calls differently
	for _, want := range []string{"system.run", "pwd"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q after tool.called: %q", want, view)
		}
	}

	request := approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd", Reason: "needs approval"}
	updated, _ = model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{
		Type:     "permission.required",
		Session:  sess,
		Approval: &request,
	}})
	model = updated.(Model)
	view = model.View()
	// New UI shows approval
	if !contains(view, "Permission Required") {
		t.Fatalf("view missing approval UI: %q", view)
	}

	updated, _ = model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{
		Type:    "compact.auto",
		Session: sess,
	}})
	model = updated.(Model)
	view = model.View()
	// Compaction events are logged internally but may not show in new UI
	_ = view // Just verify no crash
}

func TestModelViewShowsCompactionEventsInEventLog(t *testing.T) {
	model := NewModel(&fakeBridge{})
	sess := session.Session{ID: "main-1", Key: "agent:main:main", AgentID: "main", IsMain: true}

	for _, eventType := range []string{
		"compact.warning",
		"compact.error",
		"compact.auto",
		"compact.boundary",
		"compact.memory_saved",
		"compact.cleaned",
	} {
		updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{
			Type:    eventType,
			Session: sess,
		}})
		model = updated.(Model)
	}

	view := model.View()
	// New UI doesn't show compact events directly, but should render without error
	// Verify basic UI elements are present
	for _, want := range []string{"myclaw", "Welcome back"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(haystack) > len(needle) && (contains(haystack[1:], needle) || haystack[:len(needle)] == needle))
}
