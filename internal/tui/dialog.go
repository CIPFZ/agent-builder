package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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
	d.Kind = spec.Kind
	d.OriginalInput = spec.OriginalInput
	d.OriginalCursor = spec.OriginalCursor
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
	items := make([]dialogItem, 0, len(localSlashCommandSpecs))
	for _, command := range localSlashCommandSpecs {
		description := command.Description
		if command.ArgumentHint != "" {
			description += " " + command.ArgumentHint
		}
		if len(command.Aliases) > 0 {
			description += " (aliases: /" + strings.Join(command.Aliases, ", /") + ")"
		}
		items = append(items, dialogItem{Label: "/" + command.Name, Description: description})
	}
	m.dialog.open(dialogSpec{
		Title:        "Commands",
		Subtitle:     "Available local TUI commands",
		VisibleCount: len(slashCommands),
		Items:        items,
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

func (m *Model) openSessionDialog() {
	items := []dialogItem{
		{Label: "Session ID", Description: valueOrUnset(m.diagnostics.SessionID), Disabled: true},
		{Label: "Model", Description: valueOrUnset(m.diagnostics.LLMLabel), Disabled: true},
		{Label: "Log path", Description: valueOrUnset(m.diagnostics.LogPath), Disabled: true},
		{Label: "Events", Description: fmt.Sprintf("%d recorded", m.diagnostics.EventCount), Disabled: true},
	}
	if m.diagnostics.LastEvent != "" {
		items = append(items, dialogItem{Label: "Last event", Description: m.diagnostics.LastEvent, Disabled: true})
	}
	m.dialog.open(dialogSpec{
		Title:      "Session",
		Subtitle:   "Current TUI session details",
		Items:      items,
		FooterHint: "Esc close",
	})
}

func (m *Model) openDiagnosticsDialog() {
	items := []dialogItem{
		{Label: "Busy", Description: fmt.Sprintf("%t", m.busy), Disabled: true},
		{Label: "Activity", Description: valueOrUnset(m.activity.Label), Disabled: true},
		{Label: "Last event", Description: valueOrUnset(m.diagnostics.LastEvent), Disabled: true},
		{Label: "Last error", Description: valueOrUnset(m.diagnostics.LastError), Disabled: true},
		{Label: "Event count", Description: fmt.Sprintf("%d", m.diagnostics.EventCount), Disabled: true},
		{Label: "Transcript entries", Description: fmt.Sprintf("%d", len(m.transcript)), Disabled: true},
	}
	m.dialog.open(dialogSpec{
		Title:      "Diagnostics",
		Subtitle:   "Runtime and TUI state snapshot",
		Items:      items,
		FooterHint: "Esc close",
	})
}

func (m *Model) openCompactionDialog(customInstructions string) {
	items := []dialogItem{
		{Label: "Manual compaction", Description: "Runtime request path is not wired yet; showing status only", Disabled: true},
	}
	for _, event := range recentMatchingEvents(m.events, "compact.", 5) {
		items = append(items, dialogItem{Label: event, Disabled: true})
	}
	if len(items) == 1 {
		items = append(items, dialogItem{Label: "Recent compact events", Description: "none", Disabled: true})
	}
	subtitle := "Compaction status and recent compact events"
	if customInstructions != "" {
		subtitle += " - instructions: " + customInstructions
	}
	m.dialog.open(dialogSpec{
		Title:      "Compaction",
		Subtitle:   subtitle,
		Items:      items,
		FooterHint: "Esc close",
	})
}

func (m *Model) handleLocalCommand(text string) bool {
	command, ok := parseLocalSlashCommand(text)
	if !ok {
		return false
	}
	switch command.Spec.Name {
	case "help":
		m.openHelpDialog()
	case "clear":
		m.clearVisibleConversation()
	case "model":
		m.openModelDialog()
	case "session":
		m.openSessionDialog()
	case "tasks":
		m.openTasksDialog()
	case "resume":
		m.openSessionResumeDialog()
	case "compact":
		m.openCompactionDialog(command.Args)
	case "debug":
		m.openDiagnosticsDialog()
	default:
		return false
	}
	m.input = ""
	m.cursorPos = 0
	m.historyIndex = -1
	m.clearSuggestions()
	return true
}

func valueOrUnset(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}

func recentMatchingEvents(events []string, prefix string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	matches := make([]string, 0, limit)
	for i := len(events) - 1; i >= 0 && len(matches) < limit; i-- {
		if len(events[i]) >= len(prefix) && events[i][:len(prefix)] == prefix {
			matches = append(matches, events[i])
		}
	}
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}
