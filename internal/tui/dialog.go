package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type dialogResult struct {
	Selected bool
	Item     dialogItem
	Action   string
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
		action := "enter"
		if msg.Type == tea.KeyTab {
			action = "tab"
		} else if msg.Type == tea.KeyShiftTab {
			action = "shift+tab"
		}
		return dialogResult{Selected: true, Item: item, Action: action}
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

func (m *Model) openKeybindingsDialog() {
	items := []dialogItem{
		{Label: "Ctrl+S", Description: "stash prompt / restore stashed prompt", Disabled: true},
		{Label: "Ctrl+G", Description: "open external editor for current prompt", Disabled: true},
		{Label: "Ctrl+X Ctrl+E", Description: "emacs-style external editor chord", Disabled: true},
		{Label: "Ctrl+R", Description: "open history search overlay", Disabled: true},
		{Label: "Ctrl+F", Description: "start transcript search", Disabled: true},
		{Label: "Ctrl+O", Description: "toggle transcript mode", Disabled: true},
		{Label: "Ctrl+E", Description: "toggle transcript full-history view", Disabled: true},
		{Label: "PgUp / PgDown", Description: "scroll transcript viewport", Disabled: true},
		{Label: "Shift+Up", Description: "open message actions on the latest message", Disabled: true},
		{Label: "j / k", Description: "navigate message actions like vim", Disabled: true},
		{Label: "Ctrl+Y", Description: "approve pending permission request", Disabled: true},
		{Label: "Ctrl+N", Description: "reject pending permission request", Disabled: true},
		{Label: "Esc", Description: "close dialog / exit focused transient mode", Disabled: true},
	}
	m.dialog.open(dialogSpec{
		Title:        "Keybindings",
		Subtitle:     "Active TUI shortcuts and modal controls",
		Items:        items,
		VisibleCount: len(items),
		FooterHint:   "Esc close",
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
	if provider, ok := m.bridge.(platformStatusBridge); ok {
		snapshot := provider.PlatformStatusSnapshot()
		if snapshot.SessionKey != "" {
			items = append(items, dialogItem{Label: "Session key", Description: snapshot.SessionKey, Disabled: true})
		}
		if snapshot.AgentID != "" {
			items = append(items, dialogItem{Label: "Agent ID", Description: snapshot.AgentID, Disabled: true})
		}
		sessionRole := "child session"
		if snapshot.IsMain {
			sessionRole = "main session"
		}
		items = append(items, dialogItem{Label: "Session role", Description: sessionRole, Disabled: true})
		for _, root := range snapshot.WorkspaceRoots {
			items = append(items, dialogItem{Label: "Workspace root", Description: root, Disabled: true})
		}
		if snapshot.ModelOverride != "" {
			items = append(items, dialogItem{Label: "Model override", Description: snapshot.ModelOverride, Disabled: true})
		}
		if snapshot.MCPServerCount > 0 || snapshot.MCPToolCount > 0 || snapshot.MCPPromptCount > 0 || snapshot.MCPResourceCount > 0 {
			items = append(items, dialogItem{
				Label: "MCP",
				Description: fmt.Sprintf(
					"%d servers | %d tools | %d prompts | %d resources",
					snapshot.MCPServerCount,
					snapshot.MCPToolCount,
					snapshot.MCPPromptCount,
					snapshot.MCPResourceCount,
				),
				Disabled: true,
			})
		}
	}
	if m.diagnostics.LastEvent != "" {
		items = append(items, dialogItem{Label: "Last event", Description: m.diagnostics.LastEvent, Disabled: true})
	}
	m.dialog.open(dialogSpec{
		Title:        "Session",
		Subtitle:     "Current TUI session details",
		Items:        items,
		VisibleCount: len(items),
		FooterHint:   "Esc close",
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
	case "keybindings":
		m.openKeybindingsDialog()
	case "open":
		m.openQuickOpenDialog()
	case "mcp":
		m.openMCPDialog()
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
