package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestListPickerFiltersAndResetsFocus(t *testing.T) {
	picker := newListPickerState(listPickerSpec{
		Items: []dialogItem{
			{Label: "Alpha", Value: "alpha", Description: "first"},
			{Label: "Beta", Value: "beta", Description: "second"},
			{Label: "Gamma", Value: "gamma", Description: "third"},
		},
		VisibleCount: 2,
	})
	picker.moveDown()

	picker.setQuery("sec")

	if picker.Query != "sec" {
		t.Fatalf("Query = %q, want sec", picker.Query)
	}
	if picker.Current().Value != "beta" {
		t.Fatalf("current = %#v, want beta", picker.Current())
	}
	if picker.SelectedIndex != 0 || picker.VisibleFromIndex != 0 {
		t.Fatalf("selection/viewport = %d/%d, want 0/0", picker.SelectedIndex, picker.VisibleFromIndex)
	}
	if got := picker.MatchCount(); got != 1 {
		t.Fatalf("MatchCount = %d, want 1", got)
	}
}

func TestListPickerNavigationMaintainsViewport(t *testing.T) {
	picker := newListPickerState(listPickerSpec{
		Items: []dialogItem{
			{Label: "one", Value: "one"},
			{Label: "two", Value: "two"},
			{Label: "three", Value: "three"},
			{Label: "four", Value: "four"},
		},
		VisibleCount: 2,
	})

	picker.moveDown()
	picker.moveDown()
	if picker.Current().Value != "three" {
		t.Fatalf("current = %#v, want three", picker.Current())
	}
	if picker.VisibleFromIndex != 1 {
		t.Fatalf("VisibleFromIndex = %d, want 1", picker.VisibleFromIndex)
	}

	picker.pageDown()
	if picker.Current().Value != "four" {
		t.Fatalf("after pageDown current = %#v, want four", picker.Current())
	}

	picker.pageUp()
	if picker.Current().Value != "two" {
		t.Fatalf("after pageUp current = %#v, want two", picker.Current())
	}
}

func TestListPickerSkipsDisabledSelection(t *testing.T) {
	picker := newListPickerState(listPickerSpec{
		Items: []dialogItem{
			{Label: "Current", Value: "current", Disabled: true},
			{Label: "Switch", Value: "switch"},
		},
	})

	if selected, ok := picker.accept(); ok || selected.Value != "" {
		t.Fatalf("accept disabled = %#v/%v, want no selection", selected, ok)
	}

	picker.moveDown()
	if selected, ok := picker.accept(); !ok || selected.Value != "switch" {
		t.Fatalf("accept enabled = %#v/%v, want switch/true", selected, ok)
	}
}

func TestModelPickerSelectionIsCapturedBeforeClose(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openModelDialog()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.dialog.active() {
		t.Fatal("dialog active after selection, want closed")
	}
	if model.lastDialogSelection == nil || model.lastDialogSelection.Value != "model-switching" {
		t.Fatalf("last selection = %#v, want model-switching", model.lastDialogSelection)
	}
}

func TestModelPickerQueryConsumesRunesAndFilters(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openModelDialog()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("switch")})
	model = updated.(Model)

	if model.input != "" {
		t.Fatalf("input = %q, want unchanged empty input", model.input)
	}
	if model.dialog.Picker.Query != "switch" {
		t.Fatalf("picker query = %q, want switch", model.dialog.Picker.Query)
	}
	if model.dialog.Current().Value != "model-switching" {
		t.Fatalf("current = %#v, want model-switching", model.dialog.Current())
	}
}

func TestRendererShowsPickerViewportAndQuery(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.dialog.open(dialogSpec{
		Title:        "Pick",
		QueryEnabled: true,
		VisibleCount: 2,
		Items: []dialogItem{
			{Label: "one", Value: "one"},
			{Label: "two", Value: "two"},
			{Label: "three", Value: "three"},
		},
	})
	model.dialog.moveDown()
	model.dialog.moveDown()
	model.dialog.Picker.setQuery("th")

	view := newRenderer().renderScreen(newRenderSnapshot(model, 80))

	for _, want := range []string{"Search: th", "1 match", "> three"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if contains(view, "one -") || contains(view, "two -") {
		t.Fatalf("view shows filtered-out rows: %q", view)
	}
}
