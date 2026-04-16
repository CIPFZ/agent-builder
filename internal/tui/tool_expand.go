package tui

import "fmt"

const collapsedToolOutputLines = 5

func newToolExpansionState() toolExpansionState {
	return toolExpansionState{expanded: make(map[string]bool)}
}

func (s *toolExpansionState) clear() {
	s.expanded = make(map[string]bool)
}

func (s toolExpansionState) isExpanded(key string) bool {
	return key != "" && s.expanded[key]
}

func (s *toolExpansionState) toggle(key string) bool {
	if key == "" {
		return false
	}
	if s.expanded == nil {
		s.expanded = make(map[string]bool)
	}
	if s.expanded[key] {
		delete(s.expanded, key)
		return false
	}
	s.expanded[key] = true
	return true
}

func (m *Model) toggleSelectedToolExpansion() bool {
	entry, ok := m.selectedMessageActionEntry()
	if !ok || !isExpandableToolEntry(entry) {
		return false
	}
	key := m.toolExpansionKey(m.messageActions.SelectedIndex, entry)
	expanded := m.toolExpansion.toggle(key)
	if expanded {
		m.activity.Label = "Expanded tool output"
	} else {
		m.activity.Label = "Collapsed tool output"
	}
	return true
}

func (m Model) selectedToolExpanded() bool {
	entry, ok := m.selectedMessageActionEntry()
	if !ok || !isExpandableToolEntry(entry) {
		return false
	}
	return m.toolExpansion.isExpanded(m.toolExpansionKey(m.messageActions.SelectedIndex, entry))
}

func (m Model) toolExpansionKey(index int, entry transcriptEntry) string {
	if entry.ToolUseID != "" {
		return entry.ToolUseID
	}
	if entry.ToolName != "" || entry.Role == "tool" {
		return fmt.Sprintf("tool:%d", index)
	}
	return ""
}

func isExpandableToolEntry(entry transcriptEntry) bool {
	return entry.Role == "tool" && (entry.Content != "" || entry.ToolProgressOutput != "")
}
