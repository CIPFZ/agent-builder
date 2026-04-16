package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToolExpandCollapsesLongToolResultByDefault(t *testing.T) {
	tuiModel := toolExpandModelWithResult("toolu-1", longToolOutput(12))

	view := tuiModel.View()

	if !strings.Contains(view, "line-01") {
		t.Fatalf("collapsed view missing first line: %q", view)
	}
	if strings.Contains(view, "line-12") {
		t.Fatalf("collapsed view contains final line unexpectedly: %q", view)
	}
	if !strings.Contains(view, "more lines") || !strings.Contains(view, "expand") {
		t.Fatalf("collapsed view missing expand hint: %q", view)
	}
}

func TestToolExpandEnterTogglesSelectedToolResult(t *testing.T) {
	tuiModel := toolExpandModelWithResult("toolu-1", longToolOutput(12))
	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyEnter})

	expanded := tuiModel.View()
	if !strings.Contains(expanded, "line-12") {
		t.Fatalf("expanded view missing final line: %q", expanded)
	}
	if !tuiModel.toolExpansion.expanded["toolu-1"] {
		t.Fatalf("tool expansion state not set for toolu-1: %#v", tuiModel.toolExpansion)
	}
	if !tuiModel.messageActions.Active {
		t.Fatalf("message actions should stay active after expand toggle")
	}
	if !strings.Contains(expanded, "enter collapse") {
		t.Fatalf("expanded action bar missing collapse label: %q", expanded)
	}

	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyEnter})
	collapsed := tuiModel.View()
	if strings.Contains(collapsed, "line-12") {
		t.Fatalf("collapsed view still contains final line: %q", collapsed)
	}
	if tuiModel.toolExpansion.expanded["toolu-1"] {
		t.Fatalf("tool expansion state still set after collapse: %#v", tuiModel.toolExpansion)
	}
}

func TestToolExpandShowsFullRunningProgressOutput(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role:               "tool",
			ToolUseID:          "toolu-progress",
			ToolName:           "Bash",
			ToolInput:          "go test ./...",
			ToolStatus:         toolStatusRunning,
			ToolProgressOutput: longToolOutput(8),
		},
	}

	collapsed := tuiModel.View()
	if strings.Contains(collapsed, "line-01") {
		t.Fatalf("collapsed running progress should show tail only: %q", collapsed)
	}
	if !strings.Contains(collapsed, "line-08") {
		t.Fatalf("collapsed running progress missing tail: %q", collapsed)
	}

	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyEnter})
	expanded := tuiModel.View()
	if !strings.Contains(expanded, "line-01") || !strings.Contains(expanded, "line-08") {
		t.Fatalf("expanded running progress missing full output: %q", expanded)
	}
}

func TestToolExpandDoesNotReplaceUserEditAction(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{Role: "user", Content: "revise this prompt"},
	}

	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyShiftUp})
	tuiModel = updateToolExpandKey(tuiModel, tea.KeyMsg{Type: tea.KeyEnter})

	if tuiModel.input != "revise this prompt" {
		t.Fatalf("input = %q, want user message edit", tuiModel.input)
	}
	if tuiModel.messageActions.Active {
		t.Fatalf("message actions should close after user edit")
	}
}

func toolExpandModelWithResult(toolUseID, output string) Model {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role:       "tool",
			ToolUseID:  toolUseID,
			ToolName:   "Bash",
			ToolInput:  "go test ./...",
			ToolStatus: toolStatusSucceeded,
			Content:    output,
		},
	}
	return tuiModel
}

func longToolOutput(lines int) string {
	values := make([]string, 0, lines)
	for i := 1; i <= lines; i++ {
		values = append(values, fmt.Sprintf("line-%02d", i))
	}
	return strings.Join(values, "\n")
}

func updateToolExpandKey(tuiModel Model, msg tea.KeyMsg) Model {
	updated, _ := tuiModel.Update(msg)
	return updated.(Model)
}
