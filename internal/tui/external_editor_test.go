package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type fakePromptEditor struct {
	requests []externalEditorRequest
	content  string
	err      error
}

func (e *fakePromptEditor) edit(req externalEditorRequest) tea.Cmd {
	e.requests = append(e.requests, req)
	return func() tea.Msg {
		return externalEditorFinishedMsg{Content: e.content, Err: e.err}
	}
}

func TestExternalEditorCtrlGEditsPromptAndMovesCursorToEnd(t *testing.T) {
	editor := &fakePromptEditor{content: "edited prompt\n"}
	model := NewModel(&fakeBridge{}, ModelConfig{PromptEditor: editor.edit})
	model.input = "draft"
	model.cursorPos = 2
	model.suggestions = []string{"/debug"}
	model.selectedIndex = 0
	model.historyIndex = 1

	updated, cmd := model.Update(testKey(keyCtrlG))
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("cmd = nil, want external editor command")
	}
	if !model.externalEditor.Active {
		t.Fatalf("external editor state = %#v, want active", model.externalEditor)
	}
	if !contains(model.viewContent(), "Save and close editor to continue") {
		t.Fatalf("view missing external editor wait message: %q", model.viewContent())
	}
	if len(editor.requests) != 1 || editor.requests[0].Prompt != "draft" {
		t.Fatalf("requests = %#v, want prompt draft", editor.requests)
	}

	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if model.input != "edited prompt" {
		t.Fatalf("input = %q, want edited prompt", model.input)
	}
	if model.cursorPos != len([]rune("edited prompt")) {
		t.Fatalf("cursorPos = %d, want end", model.cursorPos)
	}
	if model.externalEditor.Active {
		t.Fatal("external editor still active after finished message")
	}
	if len(model.suggestions) != 0 || model.selectedIndex != -1 || model.historyIndex != -1 {
		t.Fatalf("suggestions/selected/history = %#v/%d/%d, want cleared", model.suggestions, model.selectedIndex, model.historyIndex)
	}
}

func TestExternalEditorCtrlXCtrlEChordOpensEditor(t *testing.T) {
	editor := &fakePromptEditor{content: "from chord"}
	model := NewModel(&fakeBridge{}, ModelConfig{PromptEditor: editor.edit})
	model.input = "draft"
	model.cursorPos = len([]rune(model.input))

	updated, cmd := model.Update(testKey(keyCtrlX))
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("first chord key cmd = %#v, want nil", cmd)
	}
	if !model.externalEditor.PendingCtrlX {
		t.Fatalf("external editor state = %#v, want pending Ctrl+X", model.externalEditor)
	}

	updated, cmd = model.Update(testKey(keyCtrlE))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil after Ctrl+X Ctrl+E, want editor command")
	}

	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.input != "from chord" {
		t.Fatalf("input = %q, want from chord", model.input)
	}
}

func TestExternalEditorExpandsAndRecollapsesPasteReferences(t *testing.T) {
	editor := &fakePromptEditor{content: "before pasted body after"}
	model := NewModel(&fakeBridge{}, ModelConfig{PromptEditor: editor.edit})
	ref := model.pastes.addText("pasted body", 0)
	model.input = "before " + ref
	model.cursorPos = len([]rune(model.input))

	updated, cmd := model.Update(testKey(keyCtrlG))
	model = updated.(Model)
	if len(editor.requests) != 1 || editor.requests[0].Prompt != "before pasted body" {
		t.Fatalf("requests = %#v, want expanded pasted body", editor.requests)
	}

	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.input != "before "+ref+" after" {
		t.Fatalf("input = %q, want recollapsed paste reference", model.input)
	}
}

func TestExternalEditorErrorLeavesInputUnchanged(t *testing.T) {
	editor := &fakePromptEditor{err: errors.New("editor failed")}
	model := NewModel(&fakeBridge{}, ModelConfig{PromptEditor: editor.edit})
	model.input = "draft"
	model.cursorPos = 3

	updated, cmd := model.Update(testKey(keyCtrlG))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if model.input != "draft" || model.cursorPos != 3 {
		t.Fatalf("input/cursor = %q/%d, want unchanged draft/3", model.input, model.cursorPos)
	}
	if model.externalEditor.Active || model.diagnostics.LastError != "editor failed" {
		t.Fatalf("external editor/last error = %#v/%q, want inactive/editor failed", model.externalEditor, model.diagnostics.LastError)
	}
}
