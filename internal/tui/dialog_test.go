package tui

import (
	"testing"
)

func TestDialogStateOpenNavigateAndClose(t *testing.T) {
	state := newDialogState()
	state.open(dialogSpec{
		Title: "Commands",
		Items: []dialogItem{
			{Label: "/help", Description: "show help"},
			{Label: "/model", Description: "switch model"},
		},
	})

	if !state.active() {
		t.Fatal("dialog inactive, want active")
	}
	if state.Current().Label != "/help" {
		t.Fatalf("current = %q, want /help", state.Current().Label)
	}

	state.moveDown()
	if state.Current().Label != "/model" {
		t.Fatalf("after down current = %q, want /model", state.Current().Label)
	}

	state.moveUp()
	if state.Current().Label != "/help" {
		t.Fatalf("after up current = %q, want /help", state.Current().Label)
	}

	state.close()
	if state.active() {
		t.Fatal("dialog active after close, want inactive")
	}
}

func TestSlashHelpOpensDialogWithoutSendingRuntimeMessage(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/help"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() {
		t.Fatal("dialog inactive, want help dialog open")
	}
	if model.input != "" || model.busy {
		t.Fatalf("input/busy = %q/%v, want empty/false", model.input, model.busy)
	}
}

func TestDialogConsumesKeysBeforeInputEditing(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openHelpDialog()
	model.input = "draft"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKeyRunes("x"))
	model = updated.(Model)

	if model.input != "draft" {
		t.Fatalf("input = %q, want unchanged draft", model.input)
	}

	updated, _ = model.Update(testKey(keyDown))
	model = updated.(Model)
	if model.dialog.SelectedIndex != 1 {
		t.Fatalf("selectedIndex = %d, want 1", model.dialog.SelectedIndex)
	}

	updated, _ = model.Update(testKey(keyEscape))
	model = updated.(Model)
	if model.dialog.active() {
		t.Fatal("dialog active after escape, want closed")
	}
}

func TestRendererIncludesDialogOverlay(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openHelpDialog()

	view := newRenderer().renderScreen(newRenderSnapshot(model, 80))

	for _, want := range []string{"Commands", "/help", "/model", "Enter select", "Esc close"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestSlashModelArgumentSetsSessionOverrideWithoutOpeningDialog(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/model opus"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if model.dialog.active() {
		t.Fatal("dialog active after /model opus, want closed")
	}
	if len(bridge.modelSets) != 1 || bridge.modelSets[0] != "opus" {
		t.Fatalf("modelSets = %#v, want [opus]", bridge.modelSets)
	}
	if model.input != "" {
		t.Fatalf("input = %q, want cleared", model.input)
	}
}

func TestSlashModelDefaultClearsSessionOverride(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/model default"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if bridge.modelClears != 1 {
		t.Fatalf("modelClears = %d, want 1", bridge.modelClears)
	}
	if len(bridge.modelSets) != 0 {
		t.Fatalf("modelSets = %#v, want none", bridge.modelSets)
	}
}
