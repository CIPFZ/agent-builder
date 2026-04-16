package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptStashCtrlSStashesNonEmptyPromptAndRendersNotice(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.input = "draft prompt"
	model.cursorPos = len([]rune("draft"))
	model.historyIndex = 0
	model.suggestions = []string{"/clear"}
	model.selectedIndex = 0

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)

	if model.input != "" {
		t.Fatalf("input = %q, want cleared", model.input)
	}
	if model.cursorPos != 0 {
		t.Fatalf("cursorPos = %d, want 0", model.cursorPos)
	}
	if !model.promptStash.HasStash {
		t.Fatal("prompt stash inactive, want active")
	}
	if model.promptStash.Input != "draft prompt" || model.promptStash.Cursor != len([]rune("draft")) {
		t.Fatalf("prompt stash = %#v, want original input and cursor", model.promptStash)
	}
	if model.historyIndex != -1 || len(model.suggestions) != 0 || model.selectedIndex != -1 {
		t.Fatalf("input navigation state = history %d suggestions %#v selected %d, want reset", model.historyIndex, model.suggestions, model.selectedIndex)
	}
	view := model.View()
	for _, want := range []string{"Stashed", "Ctrl+S"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestPromptStashCtrlSRestoresWhenInputEmpty(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.input = "restore me"
	model.cursorPos = len([]rune("restore"))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)

	if model.promptStash.HasStash {
		t.Fatal("prompt stash still active, want cleared after restore")
	}
	if model.input != "restore me" {
		t.Fatalf("input = %q, want restored prompt", model.input)
	}
	if model.cursorPos != len([]rune("restore")) {
		t.Fatalf("cursorPos = %d, want restored cursor", model.cursorPos)
	}
}

func TestPromptStashAutoRestoresAfterSubmittingNonSlashPrompt(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "stashed prompt"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	model.input = "send now"
	model.cursorPos = len([]rune(model.input))
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 1 || bridge.sent[0] != "send now" {
		t.Fatalf("sent = %#v, want submitted prompt only", bridge.sent)
	}
	if model.promptStash.HasStash {
		t.Fatal("prompt stash still active, want consumed after auto-restore")
	}
	if model.input != "stashed prompt" {
		t.Fatalf("input = %q, want stashed prompt restored after submit", model.input)
	}
	if model.cursorPos != len([]rune("stashed prompt")) {
		t.Fatalf("cursorPos = %d, want restored cursor", model.cursorPos)
	}
}

func TestPromptStashPreservesPasteReferencesForSubmit(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	longPaste := strings.Repeat("x", pasteThreshold+1)
	ref := model.pastes.addText(longPaste, 0)
	model.input = "before " + ref
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	if len(model.pastes.contents) != 0 {
		t.Fatalf("active paste state = %#v, want cleared while prompt is stashed", model.pastes)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 1 || bridge.sent[0] != "before "+longPaste {
		t.Fatalf("sent = %#v, want restored paste reference expanded", bridge.sent)
	}
	if len(model.history) != 1 || model.history[0] != "before "+ref {
		t.Fatalf("history = %#v, want display prompt with paste ref", model.history)
	}
}

func TestPromptStashCtrlSNoOpWhenEmptyAndNoStash(t *testing.T) {
	model := NewModel(&fakeBridge{})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)

	if model.promptStash.HasStash {
		t.Fatal("prompt stash active, want no-op")
	}
	if model.input != "" || model.cursorPos != 0 {
		t.Fatalf("input state = %q cursor %d, want unchanged empty input", model.input, model.cursorPos)
	}
}

func TestClearCommandClearsPromptStash(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.input = "keep for later"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	model.input = "/clear"
	model.cursorPos = len([]rune(model.input))
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.promptStash.HasStash {
		t.Fatalf("prompt stash = %#v, want cleared by /clear", model.promptStash)
	}
}
