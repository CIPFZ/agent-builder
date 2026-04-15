package tui

import tea "github.com/charmbracelet/bubbletea"

type dialogResult struct {
	Selected bool
	Item     dialogItem
}

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
		d.FooterHint = "↑/↓ navigate | Enter select | Esc close"
	}
	d.Picker = newListPickerState(listPickerSpec{
		Items:        d.Items,
		QueryEnabled: spec.QueryEnabled,
		VisibleCount: spec.VisibleCount,
	})
	d.syncPickerSelection()
}

func (d *dialogState) close() {
	*d = newDialogState()
}

func (d *dialogState) moveUp() {
	d.Picker.moveUp()
	d.syncPickerSelection()
}

func (d *dialogState) moveDown() {
	d.Picker.moveDown()
	d.syncPickerSelection()
}

func (d dialogState) Current() dialogItem {
	return d.Picker.Current()
}

func (d *dialogState) handleKey(msg tea.KeyMsg) dialogResult {
	if !d.active() {
		return dialogResult{}
	}
	switch msg.Type {
	case tea.KeyEscape:
		d.close()
	case tea.KeyEnter, tea.KeyTab, tea.KeyShiftTab:
		item, ok := d.Picker.accept()
		if !ok {
			return dialogResult{}
		}
		d.close()
		return dialogResult{Selected: true, Item: item}
	case tea.KeyUp, tea.KeyCtrlP:
		d.moveUp()
	case tea.KeyDown, tea.KeyCtrlN:
		d.moveDown()
	case tea.KeyPgUp:
		d.Picker.pageUp()
		d.syncPickerSelection()
	case tea.KeyPgDown:
		d.Picker.pageDown()
		d.syncPickerSelection()
	case tea.KeyBackspace:
		d.Picker.backspaceQuery()
		d.syncPickerSelection()
	case tea.KeyRunes:
		d.Picker.insertQuery(string(msg.Runes))
		d.syncPickerSelection()
	default:
		return dialogResult{}
	}
	return dialogResult{}
}

func (d *dialogState) syncPickerSelection() {
	d.SelectedIndex = d.Picker.SelectedIndex
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
		Title:        "Model",
		Subtitle:     "Current model configuration",
		QueryEnabled: true,
		Items: []dialogItem{
			{Label: "MiniMax-M2.7", Value: "minimax-m2.7", Description: "Current configured display model", Disabled: true},
			{Label: "Model switching", Value: "model-switching", Description: "Full picker will be implemented in a later module"},
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
