package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputStateEditingRunesAndCursorNavigation(t *testing.T) {
	state := newInputState()

	if !state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")}, 80, slashCommands) {
		t.Fatal("handle runes = false, want true")
	}
	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyLeft}, 80, slashCommands)
	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")}, 80, slashCommands)

	if state.input != "hell!o" {
		t.Fatalf("input = %q, want hell!o", state.input)
	}
	if state.cursorPos != 5 {
		t.Fatalf("cursorPos = %d, want 5", state.cursorPos)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyBackspace}, 80, slashCommands)
	if state.input != "hello" || state.cursorPos != 4 {
		t.Fatalf("after backspace input/cursor = %q/%d, want hello/4", state.input, state.cursorPos)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyDelete}, 80, slashCommands)
	if state.input != "hell" || state.cursorPos != 4 {
		t.Fatalf("after delete input/cursor = %q/%d, want hell/4", state.input, state.cursorPos)
	}
}

func TestInputStateSlashSuggestionsDoNotLeakIntoPlainText(t *testing.T) {
	state := newInputState()

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")}, 80, slashCommands)
	if len(state.suggestions) != 0 || state.selectedIndex != -1 {
		t.Fatalf("plain suggestions = %#v/%d, want cleared", state.suggestions, state.selectedIndex)
	}

	state = newInputState()
	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}, 80, slashCommands)
	if len(state.suggestions) == 0 || state.selectedIndex != 0 {
		t.Fatalf("slash suggestions = %#v/%d, want command suggestions", state.suggestions, state.selectedIndex)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mo")}, 80, slashCommands)
	if len(state.suggestions) != 1 || state.suggestions[0] != "/model" {
		t.Fatalf("filtered suggestions = %#v, want [/model]", state.suggestions)
	}
}

func TestInputStateAcceptSuggestionMovesCursorToEnd(t *testing.T) {
	state := newInputState()
	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/m")}, 80, slashCommands)

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyTab}, 80, slashCommands)

	if state.input != "/model" {
		t.Fatalf("input = %q, want /model", state.input)
	}
	if state.cursorPos != len([]rune("/model")) {
		t.Fatalf("cursorPos = %d, want end", state.cursorPos)
	}
	if len(state.suggestions) != 0 || state.selectedIndex != -1 {
		t.Fatalf("suggestions = %#v/%d, want cleared", state.suggestions, state.selectedIndex)
	}
}

func TestInputStateHistoryNavigationRestoresEntries(t *testing.T) {
	state := newInputState()
	state.history = []string{"first", "second"}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyUp}, 80, slashCommands)
	if state.input != "second" || state.cursorPos != len([]rune("second")) {
		t.Fatalf("after up input/cursor = %q/%d, want second/end", state.input, state.cursorPos)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyUp}, 80, slashCommands)
	if state.input != "first" {
		t.Fatalf("after second up input = %q, want first", state.input)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyDown}, 80, slashCommands)
	if state.input != "second" {
		t.Fatalf("after down input = %q, want second", state.input)
	}

	state.handleEditingKey(tea.KeyMsg{Type: tea.KeyDown}, 80, slashCommands)
	if state.input != "" || state.historyIndex != -1 {
		t.Fatalf("after final down input/historyIndex = %q/%d, want empty/-1", state.input, state.historyIndex)
	}
}

func TestModelDelegatesEditingKeysButKeepsSubmitBoundary(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	model = updated.(Model)
	if model.input != "hello" {
		t.Fatalf("input = %q, want hello", model.input)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(bridge.sent) != 1 || bridge.sent[0] != "hello" {
		t.Fatalf("sent = %#v, want [hello]", bridge.sent)
	}
	if model.input != "" || !model.busy {
		t.Fatalf("input/busy = %q/%v, want empty/true", model.input, model.busy)
	}
}
