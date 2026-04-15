package tui

import tea "github.com/charmbracelet/bubbletea"

func newDialogState() dialogState {
	return dialogState{SelectedIndex: -1}
}

func (d dialogState) active() bool {
	return d.Title != ""
}

func (d *dialogState) open(spec dialogSpec) {
	d.Title = spec.Title
	d.Subtitle = spec.Subtitle
	d.Items = append([]dialogItem(nil), spec.Items...)
	d.EmptyText = spec.EmptyText
	if d.EmptyText == "" {
		d.EmptyText = "No items"
	}
	d.FooterHint = spec.FooterHint
	if d.FooterHint == "" {
		d.FooterHint = "Enter select | Esc close"
	}
	if len(d.Items) > 0 {
		d.SelectedIndex = 0
	} else {
		d.SelectedIndex = -1
	}
}

func (d *dialogState) close() {
	*d = newDialogState()
}

func (d *dialogState) moveUp() {
	if len(d.Items) == 0 {
		d.SelectedIndex = -1
		return
	}
	if d.SelectedIndex > 0 {
		d.SelectedIndex--
	}
}

func (d *dialogState) moveDown() {
	if len(d.Items) == 0 {
		d.SelectedIndex = -1
		return
	}
	if d.SelectedIndex < len(d.Items)-1 {
		d.SelectedIndex++
	}
}

func (d dialogState) Current() dialogItem {
	if d.SelectedIndex < 0 || d.SelectedIndex >= len(d.Items) {
		return dialogItem{}
	}
	return d.Items[d.SelectedIndex]
}

func (d *dialogState) handleKey(msg tea.KeyMsg) bool {
	if !d.active() {
		return false
	}
	switch msg.Type {
	case tea.KeyEscape, tea.KeyEnter:
		d.close()
	case tea.KeyUp:
		d.moveUp()
	case tea.KeyDown:
		d.moveDown()
	default:
		return true
	}
	return true
}

func (m *Model) openHelpDialog() {
	m.dialog.open(dialogSpec{
		Title:    "Commands",
		Subtitle: "Available local TUI commands",
		Items: []dialogItem{
			{Label: "/help", Description: "Show this command reference"},
			{Label: "/clear", Description: "Clear the visible conversation (pending)"},
			{Label: "/model", Description: "Show model options"},
			{Label: "/session", Description: "Show session details (pending)"},
			{Label: "/compact", Description: "Request compaction (pending)"},
			{Label: "/debug", Description: "Show diagnostics (pending)"},
		},
	})
}

func (m *Model) openModelDialog() {
	m.dialog.open(dialogSpec{
		Title:    "Model",
		Subtitle: "Current model configuration",
		Items: []dialogItem{
			{Label: "MiniMax-M2.7", Description: "Current configured display model"},
			{Label: "Model switching", Description: "Full picker will be implemented in a later module"},
		},
	})
}

func (m *Model) handleLocalCommand(text string) bool {
	switch text {
	case "/help":
		m.openHelpDialog()
	case "/model":
		m.openModelDialog()
	default:
		return false
	}
	m.input = ""
	m.cursorPos = 0
	m.historyIndex = -1
	m.clearSuggestions()
	return true
}
