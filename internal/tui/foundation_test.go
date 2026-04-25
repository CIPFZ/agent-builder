package tui

import (
	"testing"

	"myclaw/internal/runtime"
)

func TestFoundationInputStateDefaults(t *testing.T) {
	state := newInputState()

	if state.historyIndex != -1 {
		t.Fatalf("historyIndex = %d, want -1", state.historyIndex)
	}
	if state.cursorPos != 0 {
		t.Fatalf("cursorPos = %d, want 0", state.cursorPos)
	}
	if len(state.history) != 0 {
		t.Fatalf("history len = %d, want 0", len(state.history))
	}
}

func TestFoundationRuntimeReducerTracksLastEvent(t *testing.T) {
	model := NewModel(&fakeBridge{})

	model.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "agent.lifecycle.start"}))

	if model.diagnostics.LastEvent != "agent.lifecycle.start" {
		t.Fatalf("LastEvent = %q, want agent.lifecycle.start", model.diagnostics.LastEvent)
	}
	if !model.busy {
		t.Fatal("busy = false, want true")
	}
}

func TestFoundationRendererPreservesCoreAnchors(t *testing.T) {
	model := NewModel(&fakeBridge{})
	renderer := newRenderer()

	view := renderer.renderLayout(model, 100)

	for _, want := range []string{"MYCLAW", "Commands: /help  /clear  /model  /context", "Enter to send"} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}
