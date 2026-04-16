package tui

const dialogKindHistorySearch = "history-search"

func (m *Model) openHistorySearchDialog() {
	items := m.historySearchItems()
	m.dialog.open(dialogSpec{
		Kind:           dialogKindHistorySearch,
		Title:          "Search prompts",
		Subtitle:       "Filter current prompt history",
		QueryEnabled:   true,
		Items:          items,
		EmptyText:      "No history yet",
		FooterHint:     "Type to filter | Enter use | Esc cancel",
		VisibleCount:   7,
		OriginalInput:  m.input,
		OriginalCursor: m.cursorPos,
	})
	m.clearSuggestions()
}

func (m *Model) acceptHistorySearchItem(item dialogItem) {
	if item.Value == "" {
		return
	}
	m.input = item.Value
	m.cursorPos = len([]rune(m.input))
	m.historyIndex = -1
	m.clearSuggestions()
}

func (m *Model) cancelHistorySearchDialog() {
	originalInput := m.dialog.OriginalInput
	originalCursor := m.dialog.OriginalCursor
	m.dialog.close()
	m.input = originalInput
	m.cursorPos = originalCursor
	m.historyIndex = -1
	m.clearSuggestions()
}

func (m Model) historySearchItems() []dialogItem {
	seen := make(map[string]bool, len(m.history))
	items := make([]dialogItem, 0, len(m.history))
	for i := len(m.history) - 1; i >= 0; i-- {
		prompt := m.history[i]
		if prompt == "" || seen[prompt] {
			continue
		}
		seen[prompt] = true
		items = append(items, dialogItem{Label: prompt, Value: prompt})
	}
	return items
}
