package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handlePromptStashKey(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyCtrlS {
		return false
	}
	if strings.TrimSpace(m.input) == "" {
		if m.promptStash.HasStash {
			m.restorePromptStash()
		}
		return true
	}
	m.stashPrompt()
	return true
}

func (m *Model) stashPrompt() {
	m.promptStash = promptStashState{
		HasStash: true,
		Input:    m.input,
		Cursor:   m.cursorPos,
		Pastes:   clonePasteState(m.pastes),
	}
	m.input = ""
	m.cursorPos = 0
	m.historyIndex = -1
	m.clearSuggestions()
	m.pastes = newPasteState()
}

func (m *Model) restorePromptStash() {
	if !m.promptStash.HasStash {
		return
	}
	stash := m.promptStash
	m.input = stash.Input
	m.cursorPos = stash.Cursor
	m.historyIndex = -1
	m.pastes = clonePasteState(stash.Pastes)
	m.promptStash = promptStashState{}
	m.updateSuggestions()
	m.normalizeCursor()
}

func clonePasteState(state pasteState) pasteState {
	cloned := pasteState{
		nextID:   state.nextID,
		contents: make(map[int]pasteContent, len(state.contents)),
	}
	if cloned.nextID == 0 {
		cloned.nextID = 1
	}
	for id, content := range state.contents {
		cloned.contents[id] = content
	}
	return cloned
}
