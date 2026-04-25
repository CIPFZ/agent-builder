package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHistorySearchCtrlROpensPromptHistoryDialog(t *testing.T) {
	model := historySearchModel("first prompt", "second prompt")

	updated, cmd := model.Update(testKey(keyCtrlR))
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("ctrl+r cmd = %v, want nil", cmd)
	}
	if !model.dialog.active() || model.dialog.Title != "Search prompts" {
		t.Fatalf("dialog = %#v, want Search prompts dialog", model.dialog)
	}
	view := model.viewContent()
	if !contains(view, "Search prompts") || !contains(view, "second prompt") {
		t.Fatalf("view missing history search dialog/items: %q", view)
	}
}

func TestHistorySearchUsesRecentUniqueHistoryFirst(t *testing.T) {
	model := historySearchModel("alpha", "beta", "alpha", "gamma")
	model = updateHistorySearchKey(model, testKey(keyCtrlR))

	items := model.dialog.Picker.filteredItems()
	if len(items) != 3 {
		t.Fatalf("history items = %#v, want 3 unique prompts", items)
	}
	for i, want := range []string{"gamma", "alpha", "beta"} {
		if items[i].Value != want {
			t.Fatalf("item %d = %#v, want %q", i, items[i], want)
		}
	}
}

func TestHistorySearchFiltersAndAcceptsPrompt(t *testing.T) {
	model := historySearchModel("list files", "run tests", "commit changes")
	model.input = "draft"
	model.cursorPos = len(model.input)
	model = updateHistorySearchKey(model, testKey(keyCtrlR))
	model = updateHistorySearchKey(model, testKeyRunes("run"))

	view := model.viewContent()
	if !contains(view, "Search: run") || !contains(view, "run tests") || contains(view, "list files") {
		t.Fatalf("filtered view = %q, want only run tests", view)
	}

	model = updateHistorySearchKey(model, testKey(keyEnter))
	if model.dialog.active() {
		t.Fatalf("dialog still active after selecting history")
	}
	if model.input != "run tests" || model.cursorPos != len("run tests") {
		t.Fatalf("input/cursor = %q/%d, want selected prompt", model.input, model.cursorPos)
	}
}

func TestHistorySearchFuzzyMatchesAfterExactMatches(t *testing.T) {
	model := historySearchModel("run tests", "restore session", "refactor search")
	model = updateHistorySearchKey(model, testKey(keyCtrlR))
	model = updateHistorySearchKey(model, testKeyRunes("rs"))

	items := model.dialog.Picker.filteredItems()
	if len(items) != 3 {
		t.Fatalf("filtered items = %#v, want exact plus fuzzy matches", items)
	}
	for i, want := range []string{"refactor search", "restore session", "run tests"} {
		if items[i].Value != want {
			t.Fatalf("item %d = %#v, want %q", i, items[i], want)
		}
	}
}

func TestHistorySearchNoMatchingPrompts(t *testing.T) {
	model := historySearchModel("run tests")
	model = updateHistorySearchKey(model, testKey(keyCtrlR))
	model = updateHistorySearchKey(model, testKeyRunes("missing"))

	view := model.viewContent()
	if !contains(view, "No matching prompts") {
		t.Fatalf("no-match view = %q, want no matching prompts", view)
	}
}

func TestHistorySearchBackspaceEmptyQueryCancels(t *testing.T) {
	model := historySearchModel("run tests")
	model.input = "draft"
	model.cursorPos = 3
	model = updateHistorySearchKey(model, testKey(keyCtrlR))

	model = updateHistorySearchKey(model, testKey(keyBackspace))

	if model.dialog.active() {
		t.Fatalf("dialog still active after empty-query backspace")
	}
	if model.input != "draft" || model.cursorPos != 3 {
		t.Fatalf("input/cursor = %q/%d, want restored draft/3", model.input, model.cursorPos)
	}
}

func TestHistorySearchEscapeRestoresOriginalInput(t *testing.T) {
	model := historySearchModel("run tests")
	model.input = "draft"
	model.cursorPos = 2
	model = updateHistorySearchKey(model, testKey(keyCtrlR))
	model = updateHistorySearchKey(model, testKeyRunes("run"))

	model = updateHistorySearchKey(model, testKey(keyEscape))

	if model.dialog.active() {
		t.Fatalf("dialog still active after escape")
	}
	if model.input != "draft" || model.cursorPos != 2 {
		t.Fatalf("input/cursor = %q/%d, want original draft/2", model.input, model.cursorPos)
	}
}

func TestHistorySearchCtrlCClosesDialogWithoutQuitting(t *testing.T) {
	model := historySearchModel("run tests")
	model = updateHistorySearchKey(model, testKey(keyCtrlR))

	updated, cmd := model.Update(testKey(keyCtrlC))
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("ctrl+c in history search cmd = %v, want nil", cmd)
	}
	if model.dialog.active() {
		t.Fatalf("dialog still active after ctrl+c")
	}
}

func TestHistorySearchEmptyHistoryShowsEmptyState(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model = updateHistorySearchKey(model, testKey(keyCtrlR))

	view := model.viewContent()
	if !contains(view, "Search prompts") || !contains(view, "No history yet") {
		t.Fatalf("empty history view = %q, want empty state", view)
	}
}

func TestHistorySearchDoesNotOpenDuringTranscriptSearch(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model = updateHistorySearchKey(model, testKey(keyCtrlF))

	model = updateHistorySearchKey(model, testKey(keyCtrlR))

	if model.dialog.active() {
		t.Fatalf("history dialog opened while transcript search is active")
	}
	if !model.viewport.Search.Active {
		t.Fatalf("transcript search was not preserved")
	}
}

func historySearchModel(history ...string) Model {
	model := NewModel(&fakeBridge{})
	model.history = append(model.history, history...)
	return model
}

func updateHistorySearchKey(model Model, msg tea.KeyMsg) Model {
	updated, _ := model.Update(msg)
	return updated.(Model)
}
