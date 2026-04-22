package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSlashContextOpensContextDialogWithUsageSummary(t *testing.T) {
	bridge := &fakeBridge{
		contextStatus: contextSnapshot{
			Model:               "MiniMax-M2.7",
			UsedTokens:          120000,
			ContextWindowTokens: 968000,
			UsagePercent:        12,
			CategoryLines: []string{
				"Messages: 82000 tokens (8.5%)",
				"System prompt: 12000 tokens (1.2%)",
				"Autocompact buffer: 13000 tokens (1.3%)",
				"Free space: 843000 tokens (87.1%)",
			},
		},
	}
	model := NewModel(bridge)
	model.input = "/context"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.dialog.active() {
		t.Fatalf("dialog = %#v, want no dialog for /context output", model.dialog)
	}
	view := model.View()
	for _, want := range []string{
		"Context Usage",
		"MiniMax-M2.7",
		"120000 / 968000 tokens (12%)",
		"Messages: 82000 tokens",
		"Autocompact buffer: 13000 tokens",
		"Free space: 843000 tokens",
	} {
		if !contains(view, want) {
			t.Fatalf("context view missing %q: %q", want, view)
		}
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].Kind != messageKindContext {
		t.Fatalf("transcript = %#v, want context output entry", model.transcript)
	}
}
