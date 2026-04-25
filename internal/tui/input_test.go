package tui

import (
	"strings"
	"testing"
)

func TestInputStateEditingRunesAndCursorNavigation(t *testing.T) {
	state := newInputState()

	if !state.handleEditingKey(testKeyRunes("hello"), 80, slashCommands) {
		t.Fatal("handle runes = false, want true")
	}
	state.handleEditingKey(testKey(keyLeft), 80, slashCommands)
	state.handleEditingKey(testKeyRunes("!"), 80, slashCommands)

	if state.input != "hell!o" {
		t.Fatalf("input = %q, want hell!o", state.input)
	}
	if state.cursorPos != 5 {
		t.Fatalf("cursorPos = %d, want 5", state.cursorPos)
	}

	state.handleEditingKey(testKey(keyBackspace), 80, slashCommands)
	if state.input != "hello" || state.cursorPos != 4 {
		t.Fatalf("after backspace input/cursor = %q/%d, want hello/4", state.input, state.cursorPos)
	}

	state.handleEditingKey(testKey(keyDelete), 80, slashCommands)
	if state.input != "hell" || state.cursorPos != 4 {
		t.Fatalf("after delete input/cursor = %q/%d, want hell/4", state.input, state.cursorPos)
	}
}

func TestInputStateSlashSuggestionsDoNotLeakIntoPlainText(t *testing.T) {
	state := newInputState()

	state.handleEditingKey(testKeyRunes("h"), 80, slashCommands)
	if len(state.suggestions) != 0 || state.selectedIndex != -1 {
		t.Fatalf("plain suggestions = %#v/%d, want cleared", state.suggestions, state.selectedIndex)
	}

	state = newInputState()
	state.handleEditingKey(testKeyRunes("/"), 80, slashCommands)
	if len(state.suggestions) == 0 || state.selectedIndex != 0 {
		t.Fatalf("slash suggestions = %#v/%d, want command suggestions", state.suggestions, state.selectedIndex)
	}

	state.handleEditingKey(testKeyRunes("mo"), 80, slashCommands)
	if len(state.suggestions) != 1 || state.suggestions[0] != "/model" {
		t.Fatalf("filtered suggestions = %#v, want [/model]", state.suggestions)
	}
}

func TestInputStateAcceptSuggestionMovesCursorToEnd(t *testing.T) {
	state := newInputState()
	state.handleEditingKey(testKeyRunes("/m"), 80, slashCommands)

	state.handleEditingKey(testKey(keyTab), 80, slashCommands)

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

	state.handleEditingKey(testKey(keyUp), 80, slashCommands)
	if state.input != "second" || state.cursorPos != len([]rune("second")) {
		t.Fatalf("after up input/cursor = %q/%d, want second/end", state.input, state.cursorPos)
	}

	state.handleEditingKey(testKey(keyUp), 80, slashCommands)
	if state.input != "first" {
		t.Fatalf("after second up input = %q, want first", state.input)
	}

	state.handleEditingKey(testKey(keyDown), 80, slashCommands)
	if state.input != "second" {
		t.Fatalf("after down input = %q, want second", state.input)
	}

	state.handleEditingKey(testKey(keyDown), 80, slashCommands)
	if state.input != "" || state.historyIndex != -1 {
		t.Fatalf("after final down input/historyIndex = %q/%d, want empty/-1", state.input, state.historyIndex)
	}
}

func TestInputStateVisualNavigationRespectsExplicitMultilineInput(t *testing.T) {
	state := newInputState()
	state.input = "abcd\nefghij"
	state.cursorPos = len([]rune("abcd\nef"))

	state.handleEditingKey(testKey(keyUp), 30, slashCommands)
	if state.cursorPos != 2 {
		t.Fatalf("cursor after up = %d, want 2", state.cursorPos)
	}

	state.handleEditingKey(testKey(keyDown), 30, slashCommands)
	if state.cursorPos != len([]rune("abcd\nef")) {
		t.Fatalf("cursor after down = %d, want %d", state.cursorPos, len([]rune("abcd\nef")))
	}
}

func TestInputStateHomeAndEndStayWithinVisualLine(t *testing.T) {
	state := newInputState()
	state.input = strings.Repeat("x", 30)
	state.cursorPos = 24

	state.handleEditingKey(testKey(keyHome), 24, slashCommands)
	if state.cursorPos != 22 {
		t.Fatalf("cursor after home = %d, want 22", state.cursorPos)
	}

	state.handleEditingKey(testKey(keyEnd), 24, slashCommands)
	if state.cursorPos != 30 {
		t.Fatalf("cursor after end = %d, want 30", state.cursorPos)
	}
}

func TestModelDelegatesEditingKeysButKeepsSubmitBoundary(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)

	updated, _ := model.Update(testKeyRunes("hello"))
	model = updated.(Model)
	if model.input != "hello" {
		t.Fatalf("input = %q, want hello", model.input)
	}

	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)
	if len(bridge.sent) != 1 || bridge.sent[0] != "hello" {
		t.Fatalf("sent = %#v, want [hello]", bridge.sent)
	}
	if model.input != "" || !model.busy {
		t.Fatalf("input/busy = %q/%v, want empty/true", model.input, model.busy)
	}
}

func TestCtrlKOpensQuickOpenDialog(t *testing.T) {
	model := NewModel(&fakeBridge{})

	updated, cmd := model.Update(testKey(keyCtrlK))
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("ctrl+k cmd = %v, want nil", cmd)
	}
	if !model.dialog.active() || model.dialog.Title != "Quick Open" {
		t.Fatalf("dialog = %#v, want quick open dialog", model.dialog)
	}
}
