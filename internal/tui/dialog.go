package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	runtimecommands "myclaw/internal/commands"
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
		d.FooterHint = "Up/Down navigate | Enter select | Esc close"
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
	event := keyEventType(msg)
	switch event {
	case keyEscape:
		d.close()
	case keyEnter, keyTab, keyShiftTab:
		item, ok := d.Picker.accept()
		if !ok {
			return dialogResult{}
		}
		d.close()
		action := "enter"
		if event == keyTab {
			action = "tab"
		} else if event == keyShiftTab {
			action = "shift+tab"
		}
		return dialogResult{Selected: true, Item: item, Action: action}
	case keyUp, keyCtrlP:
		d.moveUp()
	case keyDown, keyCtrlN:
		d.moveDown()
	case keyPgUp:
		d.Picker.pageUp()
		d.syncPickerSelection()
	case keyPgDown:
		d.Picker.pageDown()
		d.syncPickerSelection()
	case keyBackspace:
		d.Picker.backspaceQuery()
		d.syncPickerSelection()
	case keyRunes:
		d.Picker.insertQuery(string(keyEventRunes(msg)))
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
		items = append(items, dialogItem{Label: "/" + command.Name, Description: slashCommandDescription(command)})
	}
	m.appendDisplayItems("Commands", "Available local TUI commands", items, "")
}

func slashCommandDescription(command slashCommandSpec) string {
	description := command.Description
	if command.ArgumentHint != "" {
		description += " " + command.ArgumentHint
	}
	if len(command.Aliases) > 0 {
		description += " (aliases: /" + strings.Join(command.Aliases, ", /") + ")"
	}
	return description
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
	m.appendDisplayItems("Keybindings", "Active TUI shortcuts and modal controls", items, "")
}

func (m *Model) openModelDialog() {
	snapshot := platformStatusSnapshot{}
	if provider, ok := m.bridge.(platformStatusBridge); ok {
		snapshot = provider.PlatformStatusSnapshot()
	}
	subtitle := "Select the model for this session"
	if snapshot.ResolvedModel != "" {
		subtitle = "Current runtime model: " + snapshot.ResolvedModel
	}
	m.dialog.open(dialogSpec{
		Title:        "Model",
		Subtitle:     subtitle,
		QueryEnabled: true,
		Kind:         dialogKindModel,
		Items:        modelDialogItems(snapshot),
	})
}

func (m *Model) appendContextOutput() {
	m.appendContextOutputSnapshot(m.bridge.ContextSnapshot())
}

func (m *Model) appendContextOutputSnapshot(snapshot contextSnapshot) {
	m.tuiState.appendContextOutput(snapshot)
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
		if snapshot.BaseModel != "" {
			items = append(items, dialogItem{Label: "Base model", Description: snapshot.BaseModel, Disabled: true})
		}
		if snapshot.ModelOverride != "" {
			items = append(items, dialogItem{Label: "Model override", Description: snapshot.ModelOverride, Disabled: true})
		}
		if snapshot.ResolvedModel != "" {
			items = append(items, dialogItem{Label: "Resolved model", Description: snapshot.ResolvedModel, Disabled: true})
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
	m.appendDisplayItems("Session", "Current TUI session details", items, "")
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
	m.appendDisplayItems("Diagnostics", "Runtime and TUI state snapshot", items, "")
}

func (m *Model) openCompactionDialog(customInstructions string) {
	snapshot := m.bridge.CompactionSnapshot()
	manualDescription := "Rewrite the transcript into a compact summary and keep the current tail"
	manualValue := "compact:"
	if customInstructions != "" {
		manualDescription = "Run manual compaction with custom instructions"
		manualValue = "compact:" + strings.TrimSpace(customInstructions)
	}
	items := []dialogItem{
		{Label: "Manual compaction", Value: manualValue, Description: manualDescription},
		{Label: "Microcompact tool output", Value: "microcompact", Description: "Trim older expandable tool output without rewriting the conversation"},
	}
	if snapshot.EstimatedTokens > 0 {
		items = append(items, dialogItem{Label: "Estimated tokens", Description: fmt.Sprintf("%d tokens", snapshot.EstimatedTokens), Disabled: true})
	}
	if snapshot.WarningThreshold > 0 || snapshot.ErrorThreshold > 0 || snapshot.AutoCompactThreshold > 0 || snapshot.BlockingThreshold > 0 {
		items = append(items, dialogItem{
			Label: "Thresholds",
			Description: fmt.Sprintf(
				"warn %d | error %d | auto %d | block %d",
				snapshot.WarningThreshold,
				snapshot.ErrorThreshold,
				snapshot.AutoCompactThreshold,
				snapshot.BlockingThreshold,
			),
			Disabled: true,
		})
	}
	if snapshot.LastCompactionReason != "" {
		items = append(items, dialogItem{Label: "Last reason", Description: snapshot.LastCompactionReason, Disabled: true})
	}
	if snapshot.LastCompactedAtLabel != "" {
		items = append(items, dialogItem{Label: "Last compacted", Description: snapshot.LastCompactedAtLabel, Disabled: true})
	}
	for _, event := range recentMatchingEvents(m.events, "compact.", 5) {
		items = append(items, dialogItem{Label: event, Disabled: true})
	}
	if len(items) == 2 {
		items = append(items, dialogItem{Label: "Recent compact events", Description: "none", Disabled: true})
	}
	subtitle := "Compact the current conversation while keeping the session usable"
	if customInstructions != "" {
		subtitle += " - instructions: " + customInstructions
	}
	m.dialog.open(dialogSpec{
		Kind:         dialogKindCompaction,
		Title:        "Compaction",
		Subtitle:     subtitle,
		Items:        items,
		VisibleCount: len(items),
		FooterHint:   "Enter run | Esc close",
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
	case "search":
		m.openGlobalSearchDialog()
	case "mcp":
		m.openMCPDialog()
	case "clear":
		m.clearVisibleConversation()
	case "model":
		if command.Args != "" {
			m.applyModelSelection(command.Args)
		} else {
			m.openModelDialog()
		}
	case "context":
		m.appendContextOutput()
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
		result, err := runtimecommands.NewDefaultRegistry().Execute(m.runtimeCommandContext(), text)
		if err != nil {
			return false
		}
		if result.ShouldQuery {
			m.input = result.NormalizedInput
			m.cursorPos = len([]rune(m.input))
			m.clearSuggestions()
			return false
		}
		m.appendCommandOutput(commandTitle(result.CommandName), strings.Split(result.Output, "\n"))
	}
	m.input = ""
	m.cursorPos = 0
	m.historyIndex = -1
	m.clearSuggestions()
	return true
}

func (m Model) runtimeCommandContext() runtimecommands.Context {
	ctx := runtimecommands.Context{
		PermissionMode:       "default",
		Model:                m.diagnostics.LLMLabel,
		HasMemory:            true,
		HasResumableSessions: true,
		HasTasks:             true,
		HasMCP:               true,
	}
	if provider, ok := m.bridge.(platformStatusBridge); ok {
		snapshot := provider.PlatformStatusSnapshot()
		if strings.TrimSpace(snapshot.ResolvedModel) != "" {
			ctx.Model = snapshot.ResolvedModel
		}
	}
	return ctx
}

func commandTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Command"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
func (m *Model) appendDisplayItems(title, subtitle string, items []dialogItem, empty string) {
	lines := displayItemLines(subtitle, items, empty)
	m.appendCommandOutput(title, lines)
}

func displayItemLines(subtitle string, items []dialogItem, empty string) []string {
	lines := make([]string, 0, len(items)+1)
	if strings.TrimSpace(subtitle) != "" {
		lines = append(lines, subtitle)
	}
	if len(items) == 0 {
		if strings.TrimSpace(empty) == "" {
			empty = "No items"
		}
		lines = append(lines, empty)
		return lines
	}
	for _, item := range items {
		line := strings.TrimSpace(item.Label)
		description := strings.TrimSpace(item.Description)
		if description != "" {
			if line == "" {
				line = description
			} else {
				line += ": " + description
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
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
