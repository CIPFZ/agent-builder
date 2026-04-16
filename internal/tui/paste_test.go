package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelPasteShortTextSanitizesAndInsertsAtCursor(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.input = "hello world"
	model.cursorPos = len([]rune("hello "))
	model.suggestions = []string{"/help"}
	model.selectedIndex = 0
	model.historyIndex = 1

	updated, _ := model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("a\rb\t\x1b[31mred\x1b[0m"),
		Paste: true,
	})
	model = updated.(Model)

	if model.input != "hello a\nb    redworld" {
		t.Fatalf("input = %q, want sanitized paste inserted at cursor", model.input)
	}
	if model.cursorPos != len([]rune("hello a\nb    red")) {
		t.Fatalf("cursorPos = %d, want after pasted text", model.cursorPos)
	}
	if len(model.suggestions) != 0 || model.selectedIndex != -1 {
		t.Fatalf("suggestions = %#v/%d, want cleared", model.suggestions, model.selectedIndex)
	}
	if model.historyIndex != -1 {
		t.Fatalf("historyIndex = %d, want -1", model.historyIndex)
	}
}

func TestModelPasteLongTextStoresReferenceAndSubmitExpandsForBridge(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.setSize(80, 30)
	model.input = "before  after"
	model.cursorPos = len([]rune("before "))
	longPaste := strings.Repeat("x", pasteThreshold+1)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(longPaste), Paste: true})
	model = updated.(Model)

	const placeholder = "[Pasted text #1]"
	if model.input != "before "+placeholder+" after" {
		t.Fatalf("input = %q, want placeholder inserted", model.input)
	}
	if got := model.pastes.contents[1].Content; got != longPaste {
		t.Fatalf("stored paste len = %d, want %d", len(got), len(longPaste))
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.sent) != 1 || bridge.sent[0] != "before "+longPaste+" after" {
		t.Fatalf("sent = %#v, want expanded paste content", bridge.sent)
	}
	if len(model.transcript) != 1 || model.transcript[0].Content != "before "+placeholder+" after" {
		t.Fatalf("transcript = %#v, want display text with placeholder", model.transcript)
	}
	if len(model.history) != 1 || model.history[0] != "before "+placeholder+" after" {
		t.Fatalf("history = %#v, want display text with placeholder", model.history)
	}
	if len(model.pastes.contents) != 0 || model.pastes.nextID != 1 {
		t.Fatalf("paste state after submit = %#v, want reset for next prompt", model.pastes)
	}
}

func TestModelPasteTooManyLinesStoresReferenceWithLineCount(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.setSize(80, 30)
	pasted := "a\nb\nc\nd"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	model = updated.(Model)

	if model.input != "[Pasted text #1 +3 lines]" {
		t.Fatalf("input = %q, want pasted text line-count placeholder", model.input)
	}
	if got := model.pastes.contents[1].Content; got != pasted {
		t.Fatalf("stored paste = %q, want original sanitized content", got)
	}
}

func TestClearCommandClearsPasteState(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.pastes = newPasteState()
	model.pastes.addText("large pasted content", 0)
	model.input = "/clear"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(model.pastes.contents) != 0 || model.pastes.nextID != 1 {
		t.Fatalf("paste state = %#v, want reset", model.pastes)
	}
}
