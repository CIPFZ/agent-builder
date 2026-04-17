package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/model"
)

func TestMessageActionsShiftUpEntersSelectionOnLastNavigableMessage(t *testing.T) {
	tuiModel := messageActionsModel()

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})

	if !tuiModel.messageActions.Active {
		t.Fatalf("message actions inactive after shift+up")
	}
	if tuiModel.messageActions.SelectedIndex != 2 {
		t.Fatalf("selected index = %d, want last navigable message index 2", tuiModel.messageActions.SelectedIndex)
	}
	view := tuiModel.View()
	for _, want := range []string{"Message actions", "c copy", "p copy command", "Bash"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestMessageActionsNavigateAndCopySelectedMessage(t *testing.T) {
	tuiModel := messageActionsModel()
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyUp})

	if tuiModel.messageActions.SelectedIndex != 1 {
		t.Fatalf("selected index = %d, want assistant text entry", tuiModel.messageActions.SelectedIndex)
	}

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	if tuiModel.messageActions.Active {
		t.Fatalf("message actions still active after copy")
	}
	if tuiModel.messageActions.LastCopiedText != "assistant answer" {
		t.Fatalf("last copied text = %q, want assistant answer", tuiModel.messageActions.LastCopiedText)
	}
	if !strings.Contains(tuiModel.View(), "Copied message") {
		t.Fatalf("view missing copy status: %q", tuiModel.View())
	}
}

func TestMessageActionsCopyPrimaryToolInput(t *testing.T) {
	tuiModel := messageActionsModel()
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	if tuiModel.messageActions.LastCopiedText != "go test ./..." {
		t.Fatalf("last copied text = %q, want tool primary input", tuiModel.messageActions.LastCopiedText)
	}
	if !strings.Contains(tuiModel.View(), "Copied command") {
		t.Fatalf("view missing copy primary status: %q", tuiModel.View())
	}
}

func TestMessageActionsEnterEditsUserMessage(t *testing.T) {
	tuiModel := messageActionsModel()
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})

	if tuiModel.messageActions.SelectedIndex != 0 {
		t.Fatalf("selected index = %d, want user entry", tuiModel.messageActions.SelectedIndex)
	}

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyEnter})

	if tuiModel.messageActions.Active {
		t.Fatalf("message actions still active after edit")
	}
	if tuiModel.input != "user prompt" || tuiModel.cursorPos != len("user prompt") {
		t.Fatalf("input/cursor = %q/%d, want edited prompt", tuiModel.input, tuiModel.cursorPos)
	}
}

func TestMessageActionsCopyRichUserBlockMessage(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role: "user",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockText, Text: "Please inspect this screenshot"},
				{
					Type: "image",
					Raw: map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/png",
						},
					},
				},
			},
		},
	}

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	if tuiModel.messageActions.LastCopiedText != "Please inspect this screenshot\n\n[Image: image/png]" {
		t.Fatalf("last copied text = %q, want rich user block text", tuiModel.messageActions.LastCopiedText)
	}
}

func TestMessageActionsEscapeAndCtrlCExitWithoutQuitting(t *testing.T) {
	tuiModel := messageActionsModel()
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})

	updated, cmd := tuiModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tuiModel = updated.(Model)
	if cmd != nil {
		t.Fatalf("esc cmd = %v, want nil", cmd)
	}
	if tuiModel.messageActions.Active {
		t.Fatalf("message actions still active after esc")
	}

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	updated, cmd = tuiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	tuiModel = updated.(Model)
	if cmd != nil {
		t.Fatalf("ctrl+c cmd = %v, want nil", cmd)
	}
	if tuiModel.messageActions.Active {
		t.Fatalf("message actions still active after ctrl+c")
	}
}

func TestMessageActionsDoesNotOpenDuringTranscriptSearch(t *testing.T) {
	tuiModel := messageActionsModel()
	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyCtrlF})

	tuiModel = updateMessageActionKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})

	if tuiModel.messageActions.Active {
		t.Fatalf("message actions opened while transcript search is active")
	}
	if !tuiModel.viewport.Search.Active {
		t.Fatalf("transcript search was not preserved")
	}
}

func messageActionsModel() Model {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{Role: "user", Content: "user prompt"},
		{Role: "assistant", Content: "assistant answer"},
		{
			Role:    "assistant",
			Content: "assistant-tool",
			Blocks: []model.MessageBlock{
				{
					Type: model.MessageBlockToolUse,
					Name: "Bash",
					InputObject: map[string]any{
						"command": "go test ./...",
					},
				},
			},
		},
	}
	return tuiModel
}

func updateMessageActionKey(tuiModel Model, msg tea.KeyMsg) Model {
	updated, _ := tuiModel.Update(msg)
	return updated.(Model)
}
