package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/model"
)

type primaryToolInputSpec struct {
	Label string
	Key   string
}

var primaryToolInputs = map[string]primaryToolInputSpec{
	"Read":         {Label: "path", Key: "file_path"},
	"Edit":         {Label: "path", Key: "file_path"},
	"Write":        {Label: "path", Key: "file_path"},
	"NotebookEdit": {Label: "path", Key: "notebook_path"},
	"Bash":         {Label: "command", Key: "command"},
	"PowerShell":   {Label: "command", Key: "command"},
	"Grep":         {Label: "pattern", Key: "pattern"},
	"Glob":         {Label: "pattern", Key: "pattern"},
	"WebFetch":     {Label: "url", Key: "url"},
	"WebSearch":    {Label: "query", Key: "query"},
	"Task":         {Label: "prompt", Key: "prompt"},
	"Agent":        {Label: "prompt", Key: "prompt"},
}

func (s *messageActionsState) close() {
	s.Active = false
	s.SelectedIndex = -1
}

func (m *Model) enterMessageActions() bool {
	index, ok := m.lastNavigableTranscriptIndex()
	if !ok {
		return false
	}
	m.messageActions.Active = true
	m.messageActions.SelectedIndex = index
	m.clearSuggestions()
	m.scrollTranscriptBottom()
	return true
}

func (m *Model) handleMessageActionsKey(msg tea.KeyMsg) bool {
	if !m.messageActions.Active {
		return false
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.messageActions.close()
	case tea.KeyUp, tea.KeyCtrlP:
		m.navigateMessageAction(-1, false)
	case tea.KeyDown, tea.KeyCtrlN:
		m.navigateMessageAction(1, false)
	case tea.KeyShiftUp:
		m.navigateMessageAction(-1, true)
	case tea.KeyShiftDown:
		m.navigateMessageAction(1, true)
	case tea.KeyHome:
		m.navigateMessageActionToEdge(-1)
	case tea.KeyEnd:
		m.navigateMessageActionToEdge(1)
	case tea.KeyEnter:
		m.acceptMessageAction()
	case tea.KeyRunes:
		key := strings.ToLower(string(msg.Runes))
		switch key {
		case "c":
			m.copySelectedMessage()
		case "p":
			m.copySelectedPrimaryToolInput()
		case "j":
			m.navigateMessageAction(1, false)
		case "k":
			m.navigateMessageAction(-1, false)
		default:
			return true
		}
	default:
		return true
	}
	return true
}

func (m *Model) navigateMessageAction(direction int, usersOnly bool) {
	if len(m.transcript) == 0 {
		m.messageActions.close()
		return
	}
	start := m.messageActions.SelectedIndex
	for i := start + direction; i >= 0 && i < len(m.transcript); i += direction {
		if !isNavigableTranscriptEntry(m.transcript[i]) {
			continue
		}
		if usersOnly && m.transcript[i].Role != "user" {
			continue
		}
		m.messageActions.SelectedIndex = i
		return
	}
}

func (m *Model) navigateMessageActionToEdge(direction int) {
	if direction < 0 {
		for i := 0; i < len(m.transcript); i++ {
			if isNavigableTranscriptEntry(m.transcript[i]) {
				m.messageActions.SelectedIndex = i
				return
			}
		}
		return
	}
	for i := len(m.transcript) - 1; i >= 0; i-- {
		if isNavigableTranscriptEntry(m.transcript[i]) {
			m.messageActions.SelectedIndex = i
			return
		}
	}
}

func (m *Model) acceptMessageAction() {
	entry, ok := m.selectedMessageActionEntry()
	if !ok {
		m.messageActions.close()
		return
	}
	if isExpandableToolEntry(entry) {
		m.toggleSelectedToolExpansion()
		return
	}
	if entry.Role == "user" && strings.TrimSpace(entry.Content) != "" && entry.Kind == "" {
		m.input = stripSystemReminders(entry.Content)
		m.cursorPos = len([]rune(m.input))
		m.historyIndex = -1
		m.clearSuggestions()
		m.activity.Label = "Editing selected message"
		m.messageActions.close()
	}
}

func (m *Model) copySelectedMessage() {
	entry, ok := m.selectedMessageActionEntry()
	if !ok {
		return
	}
	text := copyTextOfTranscriptEntry(entry)
	if strings.TrimSpace(text) == "" {
		return
	}
	m.recordMessageActionCopy("message", text)
	m.messageActions.close()
}

func (m *Model) copySelectedPrimaryToolInput() {
	entry, ok := m.selectedMessageActionEntry()
	if !ok {
		return
	}
	label, text, ok := primaryToolInputOf(entry)
	if !ok || strings.TrimSpace(text) == "" {
		return
	}
	m.recordMessageActionCopy(label, text)
	m.messageActions.close()
}

func (m *Model) recordMessageActionCopy(label, text string) {
	m.messageActions.LastCopiedText = text
	m.messageActions.LastCopiedLabel = label
	m.activity.Label = fmt.Sprintf("Copied %s", label)
}

func (m Model) selectedMessageActionEntry() (transcriptEntry, bool) {
	index := m.messageActions.SelectedIndex
	if index < 0 || index >= len(m.transcript) {
		return transcriptEntry{}, false
	}
	entry := m.transcript[index]
	if !isNavigableTranscriptEntry(entry) {
		return transcriptEntry{}, false
	}
	return entry, true
}

func (m Model) lastNavigableTranscriptIndex() (int, bool) {
	for i := len(m.transcript) - 1; i >= 0; i-- {
		if isNavigableTranscriptEntry(m.transcript[i]) {
			return i, true
		}
	}
	return -1, false
}

func isNavigableTranscriptEntry(entry transcriptEntry) bool {
	if entry.Kind != "" {
		return strings.TrimSpace(copyTextOfTranscriptEntry(entry)) != ""
	}
	switch entry.Role {
	case "user":
		text := strings.TrimSpace(stripSystemReminders(entry.Content))
		return text != "" && !strings.HasPrefix(text, "<")
	case "assistant":
		if len(entry.Blocks) == 0 {
			return strings.TrimSpace(entry.Content) != ""
		}
		return strings.TrimSpace(copyTextOfTranscriptEntry(entry)) != "" || hasPrimaryToolInput(entry)
	case "tool":
		return strings.TrimSpace(copyTextOfTranscriptEntry(entry)) != ""
	default:
		return strings.TrimSpace(entry.Content) != ""
	}
}

func copyTextOfTranscriptEntry(entry transcriptEntry) string {
	switch {
	case entry.Kind == messageKindLocalCommand:
		return strings.TrimSpace(strings.Join(nonEmptyStrings(entry.LocalStdout, entry.LocalStderr), "\n\n"))
	case entry.Kind != "":
		return strings.TrimSpace(entry.Content)
	case entry.Role == "user":
		return stripSystemReminders(entry.Content)
	case entry.Role == "assistant":
		if len(entry.Blocks) == 0 {
			return entry.Content
		}
		return textFromBlocks(entry.Blocks)
	case entry.Role == "tool":
		if strings.TrimSpace(entry.Content) != "" {
			return entry.Content
		}
		return strings.TrimSpace(strings.Join(nonEmptyStrings(entry.ToolProgressOutput, entry.ToolProgressMessage), "\n\n"))
	default:
		return entry.Content
	}
}

func textFromBlocks(blocks []model.MessageBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case model.MessageBlockText:
			parts = appendNonEmpty(parts, block.Text)
		case model.MessageBlockToolResult:
			parts = appendNonEmpty(parts, block.Content)
		case model.MessageBlockToolUse:
			if _, text, ok := primaryToolInputFromBlock(block); ok {
				parts = appendNonEmpty(parts, text)
			}
		default:
			parts = appendNonEmpty(parts, messageBlockFallbackContent(block))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func hasPrimaryToolInput(entry transcriptEntry) bool {
	_, _, ok := primaryToolInputOf(entry)
	return ok
}

func primaryToolInputOf(entry transcriptEntry) (string, string, bool) {
	if entry.ToolName != "" {
		if label, value, ok := primaryToolInputFromName(entry.ToolName, entry.ToolInput, entry.ToolInputObject); ok {
			return label, value, true
		}
	}
	for _, block := range entry.Blocks {
		if block.Type != model.MessageBlockToolUse {
			continue
		}
		if label, value, ok := primaryToolInputFromBlock(block); ok {
			return label, value, true
		}
	}
	return "", "", false
}

func primaryToolInputFromBlock(block model.MessageBlock) (string, string, bool) {
	return primaryToolInputFromName(block.Name, block.Input, block.InputObject)
}

func primaryToolInputFromName(name, fallback string, input map[string]any) (string, string, bool) {
	spec, ok := primaryToolInputs[name]
	if !ok {
		return "", "", false
	}
	if input != nil {
		if value, ok := input[spec.Key].(string); ok && strings.TrimSpace(value) != "" {
			return spec.Label, value, true
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return spec.Label, fallback, true
	}
	return "", "", false
}

func stripSystemReminders(text string) string {
	const closeTag = "</system-reminder>"
	trimmed := strings.TrimLeft(text, " \t\r\n")
	for strings.HasPrefix(trimmed, "<system-reminder>") {
		end := strings.Index(trimmed, closeTag)
		if end < 0 {
			break
		}
		trimmed = strings.TrimLeft(trimmed[end+len(closeTag):], " \t\r\n")
	}
	return trimmed
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func appendNonEmpty(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	return append(values, strings.TrimSpace(value))
}
