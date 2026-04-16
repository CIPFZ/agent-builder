package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/runtime"
)

func TestSlashClearClearsVisibleConversationWithoutSendingRuntimeMessage(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.transcript = []transcriptEntry{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	model.events = append(model.events, "old event")
	model.busy = true
	model.input = "/clear"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if len(model.transcript) != 0 {
		t.Fatalf("transcript = %#v, want cleared", model.transcript)
	}
	if model.busy {
		t.Fatal("busy = true, want false after local clear")
	}
	if model.input != "" || model.cursorPos != 0 {
		t.Fatalf("input/cursor = %q/%d, want cleared", model.input, model.cursorPos)
	}
	if len(model.events) == 0 || model.events[len(model.events)-1] != "conversation cleared" {
		t.Fatalf("events = %#v, want trailing conversation cleared", model.events)
	}
}

func TestSlashSessionOpensSessionDialogWithRuntimeMetadata(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge, ModelConfig{
		SessionID: "main-000001",
		LLMLabel:  "openai-compatible / LongCat-Flash-Chat",
		LogPath:   "logs/myclaw.jsonl",
	})
	model.input = "/session"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() || model.dialog.Title != "Session" {
		t.Fatalf("dialog = %#v, want session dialog", model.dialog)
	}
	view := model.View()
	for _, want := range []string{"Session", "main-000001", "LongCat-Flash-Chat", "logs/myclaw.jsonl"} {
		if !contains(view, want) {
			t.Fatalf("session view missing %q: %q", want, view)
		}
	}
}

func TestSlashDebugOpensDiagnosticsDialogWithLatestState(t *testing.T) {
	model := NewModel(&fakeBridge{})
	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "agent.lifecycle.start"}})
	model = updated.(Model)
	updated, _ = model.Update(BridgeErrMsg{Err: assertErr("boom")})
	model = updated.(Model)
	model.input = "/debug"
	model.cursorPos = len([]rune(model.input))

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Diagnostics" {
		t.Fatalf("dialog = %#v, want diagnostics dialog", model.dialog)
	}
	view := model.View()
	for _, want := range []string{"Diagnostics", "Last event", "agent.lifecycle.start", "Last error", "boom", "Event count", "1"} {
		if !contains(view, want) {
			t.Fatalf("debug view missing %q: %q", want, view)
		}
	}
}

func TestSlashCompactOpensCompactionDialogWithRecentEvents(t *testing.T) {
	model := NewModel(&fakeBridge{})
	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "compact.warning"}})
	model = updated.(Model)
	updated, _ = model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "compact.cleaned"}})
	model = updated.(Model)
	model.input = "/compact"
	model.cursorPos = len([]rune(model.input))

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Compaction" {
		t.Fatalf("dialog = %#v, want compaction dialog", model.dialog)
	}
	view := model.View()
	for _, want := range []string{"Compaction", "compact.warning", "compact.cleaned", "Manual compaction"} {
		if !contains(view, want) {
			t.Fatalf("compact view missing %q: %q", want, view)
		}
	}
}

func TestHelpDialogListsOnlyImplementedLocalCommandDescriptions(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openHelpDialog()
	view := model.View()

	for _, want := range []string{"/clear", "Clear visible conversation", "/session", "Show session details", "/compact", "Show compaction status", "/debug", "Show diagnostics"} {
		if !contains(view, want) {
			t.Fatalf("help view missing %q: %q", want, view)
		}
	}
	if contains(view, "pending") {
		t.Fatalf("help view still contains pending marker: %q", view)
	}
}

func TestSlashClearAliasesUseLocalClearCommand(t *testing.T) {
	for _, input := range []string{"/reset", "/new"} {
		bridge := &fakeBridge{}
		model := NewModel(bridge)
		model.transcript = []transcriptEntry{{Role: "user", Content: "hello"}}
		model.input = input
		model.cursorPos = len([]rune(model.input))

		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)

		if len(bridge.sent) != 0 {
			t.Fatalf("%s sent = %#v, want no runtime message", input, bridge.sent)
		}
		if len(model.transcript) != 0 {
			t.Fatalf("%s transcript = %#v, want cleared", input, model.transcript)
		}
	}
}

func TestSlashCompactAcceptsOptionalInstructionsAsLocalCommand(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/compact summarize tool output only"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() || model.dialog.Title != "Compaction" {
		t.Fatalf("dialog = %#v, want compaction dialog", model.dialog)
	}
	if !contains(model.dialog.Subtitle, "summarize tool output only") {
		t.Fatalf("compact subtitle = %q, want custom instructions", model.dialog.Subtitle)
	}
}
