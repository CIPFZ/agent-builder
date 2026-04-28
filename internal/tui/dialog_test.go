package tui

import (
	"fmt"
	"strings"
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

func TestDialogDefaultFooterUsesReadableNavigationHint(t *testing.T) {
	state := newDialogState()
	state.open(dialogSpec{Title: "Commands"})

	const want = "Up/Down navigate | Enter select | Esc close"
	if state.FooterHint != want {
		t.Fatalf("footer hint = %q, want %q", state.FooterHint, want)
	}
	if strings.Contains(state.FooterHint, "????") {
		t.Fatalf("footer hint contains corrupted navigation marker: %q", state.FooterHint)
	}
}

func TestSlashHelpAppendsTranscriptOutputWithoutSendingRuntimeMessage(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/help"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if model.dialog.active() {
		t.Fatalf("dialog active, want help output in transcript: %#v", model.dialog)
	}
	if len(model.transcript) == 0 || !contains(model.transcript[len(model.transcript)-1].Content, "Commands") {
		t.Fatalf("transcript = %#v, want help output", model.transcript)
	}
	if model.input != "" || model.busy {
		t.Fatalf("input/busy = %q/%v, want empty/false", model.input, model.busy)
	}
}

func TestDialogConsumesKeysBeforeInputEditing(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openModelDialog()
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
	model.openModelDialog()

	view := newRenderer().renderScreen(newRenderSnapshot(model, 80))

	for _, want := range []string{"Model", "Default", "Sonnet", "Opus", "Enter select", "Esc close"} {
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

func TestTypedSlashModelOpensDialogWithoutSendingRuntimeMessage(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)

	updated, _ := model.Update(testKeyRunes("/model"))
	model = updated.(Model)
	if len(model.suggestions) == 0 {
		t.Fatal("suggestions should be visible before enter")
	}

	updated, cmd := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %v, want nil for local /model command", cmd)
	}
	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() || model.dialog.Title != "Model" {
		t.Fatalf("dialog = %#v, want model dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Model", "Search: (type to filter)", "Default", "Sonnet", "Opus"} {
		if !contains(view, want) {
			t.Fatalf("model view missing %q: %q", want, view)
		}
	}
}

func TestModelDialogIsVisibleWithinTerminalHeight(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.setSize(80, 20)
	for i := 1; i <= 20; i++ {
		model.transcript = append(model.transcript, transcriptEntry{
			Role:    "assistant",
			Content: fmt.Sprintf("message-%02d", i),
		})
	}
	model.input = "/model"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	visible := firstLines(model.viewContent(), model.height)
	for _, want := range []string{"Model", "Search: (type to filter)", "Default", "Sonnet", "Opus"} {
		if !contains(visible, want) {
			t.Fatalf("visible view missing %q: %q", want, visible)
		}
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

func firstLines(text string, count int) string {
	lines := strings.Split(text, "\n")
	if count > len(lines) {
		count = len(lines)
	}
	return strings.Join(lines[:count], "\n")
}
