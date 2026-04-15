package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Normalize cursor position
	runes := []rune(m.input)
	if m.cursorPos < 0 {
		m.cursorPos = 0
	}
	if m.cursorPos > len(runes) {
		m.cursorPos = len(runes)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		if m.pendingApproval != nil {
			m.clearSuggestions()
			return m, nil
		}
		text, ok := m.submitUserInput()
		if !ok {
			return m, nil
		}
		m.bridge.SendUserMessage(text)
		return m, nil
	case tea.KeyLeft:
		if m.cursorPos > 0 {
			// Check if we're on a line boundary (after \n)
			if m.cursorPos > 0 && runes[m.cursorPos-1] == '\n' {
				// Don't move past the \n, stay on current line's first character
				// Actually, move to before the \n (end of previous line)
				m.cursorPos--
			} else {
				m.cursorPos--
			}
		}
		return m, nil
	case tea.KeyRight:
		if m.cursorPos < len(runes) {
			// Don't move past \n, stay on current line
			if runes[m.cursorPos] != '\n' {
				m.cursorPos++
			}
		}
		return m, nil
	case tea.KeyHome:
		// Move to beginning of current line
		m.cursorPos = m.lineStartPosition(runes, m.cursorPos)
		return m, nil
	case tea.KeyEnd:
		// Move to end of current line (before \n or end of text)
		m.cursorPos = m.lineEndPosition(runes, m.cursorPos)
		return m, nil
	case tea.KeyUp:
		if len(m.suggestions) > 0 && m.selectedIndex > 0 {
			m.selectedIndex--
		} else if len(m.history) > 0 {
			if m.historyIndex == -1 {
				m.historyIndex = len(m.history) - 1
			} else if m.historyIndex > 0 {
				m.historyIndex--
			}
			m.input = m.history[m.historyIndex]
			m.cursorPos = len([]rune(m.input))
		} else {
			// Multi-line visual navigation
			m.cursorPos = m.moveCursorUp(runes, m.cursorPos)
		}
		return m, nil
	case tea.KeyDown:
		if len(m.suggestions) > 0 && m.selectedIndex < len(m.suggestions)-1 {
			m.selectedIndex++
		} else if m.historyIndex != -1 {
			if m.historyIndex < len(m.history)-1 {
				m.historyIndex++
				m.input = m.history[m.historyIndex]
			} else {
				m.historyIndex = -1
				m.input = ""
			}
			m.cursorPos = len([]rune(m.input))
		} else {
			// Multi-line visual navigation
			m.cursorPos = m.moveCursorDown(runes, m.cursorPos)
		}
		return m, nil
	case tea.KeyTab:
		m.acceptSuggestion()
		return m, nil
	case tea.KeySpace:
		// Insert space at cursor position
		m.input = string(runes[:m.cursorPos]) + " " + string(runes[m.cursorPos:])
		m.cursorPos++
		m.historyIndex = -1
		m.updateSuggestions()
		return m, nil
	case tea.KeyEscape:
		m.clearSuggestions()
		return m, nil
	case tea.KeyBackspace:
		if m.cursorPos > 0 {
			m.input = string(runes[:m.cursorPos-1]) + string(runes[m.cursorPos:])
			m.cursorPos--
		}
		m.historyIndex = -1
		m.updateSuggestions()
		return m, nil
	case tea.KeyDelete:
		if m.cursorPos < len(runes) {
			m.input = string(runes[:m.cursorPos]) + string(runes[m.cursorPos+1:])
		}
		m.historyIndex = -1
		m.updateSuggestions()
		return m, nil
	case tea.KeyCtrlY:
		if id, ok := m.approvePending(); ok {
			m.bridge.Approve(id)
		}
		return m, nil
	case tea.KeyCtrlN:
		if id, ok := m.rejectPending(); ok {
			m.bridge.Reject(id)
		}
		return m, nil
	case tea.KeyRunes:
		// Insert at cursor position
		m.input = string(runes[:m.cursorPos]) + string(msg.Runes) + string(runes[m.cursorPos:])
		m.cursorPos += len(msg.Runes)
		m.historyIndex = -1
		m.updateSuggestions()
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) updateSuggestions() {
	if !strings.HasPrefix(m.input, "/") || m.input == "/" {
		m.suggestions = slashCommands
		m.selectedIndex = 0
		return
	}
	input := strings.ToLower(m.input)
	var matches []string
	for _, cmd := range slashCommands {
		if strings.HasPrefix(strings.ToLower(cmd), input) {
			matches = append(matches, cmd)
		}
	}
	m.suggestions = matches
	if len(m.suggestions) > 0 {
		m.selectedIndex = 0
	} else {
		m.selectedIndex = -1
	}
}

func (m Model) acceptSuggestion() {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.suggestions) {
		m.input = m.suggestions[m.selectedIndex]
		m.suggestions = nil
		m.selectedIndex = -1
	}
}

func (m *Model) clearSuggestions() {
	m.tuiState.clearSuggestions()
}

func (m Model) lineStartPosition(runes []rune, pos int) int {
	if len(runes) == 0 {
		return 0
	}
	width := m.width - 2
	if width < 20 {
		width = 80
	}

	visLineStart := 0
	visLineWidth := 0

	for i := 0; i < len(runes); i++ {
		charWidth := lipgloss.Width(string(runes[i]))

		if visLineWidth+charWidth > width && visLineWidth > 0 {
			// Would exceed width - this char starts a new visual line
			if i <= pos {
				visLineStart = i
				visLineWidth = charWidth
			} else {
				// pos is in previous visual line
				return visLineStart
			}
		} else {
			visLineWidth += charWidth
		}
	}

	return visLineStart
}

// lineEndPosition returns the position after the last character in the current visual line
func (m Model) lineEndPosition(runes []rune, pos int) int {
	if len(runes) == 0 {
		return 0
	}
	width := m.width - 2
	if width < 20 {
		width = 80
	}

	visLineWidth := 0

	for i := 0; i < len(runes); i++ {
		charWidth := lipgloss.Width(string(runes[i]))

		if visLineWidth+charWidth > width && visLineWidth > 0 {
			// Would exceed width - previous line ends before this char
			if i <= pos {
				visLineWidth = charWidth
			} else {
				return i
			}
		} else {
			visLineWidth += charWidth
		}
	}

	return len(runes)
}

// moveCursorUp moves cursor to the same column in the previous visual line
func (m Model) moveCursorUp(runes []rune, cursorPos int) int {
	if len(runes) == 0 || cursorPos == 0 {
		return 0
	}

	width := m.width - 2
	if width < 20 {
		width = 80
	}

	// Find current visual line boundaries and column
	currentLineStart := m.lineStartPosition(runes, cursorPos)
	currentCol := cursorPos - currentLineStart

	if currentLineStart == 0 {
		// Already at first visual line, stay at position 0
		return 0
	}

	// Find the start of previous visual line
	prevLineStart := 0
	visLineWidth := 0
	for i := 0; i < currentLineStart; i++ {
		charWidth := lipgloss.Width(string(runes[i]))
		if visLineWidth+charWidth > width && visLineWidth > 0 {
			prevLineStart = i
		}
		visLineWidth += charWidth
	}

	// Calculate target position in previous line
	targetPos := prevLineStart + currentCol
	if targetPos > currentLineStart-1 {
		targetPos = currentLineStart - 1
	}

	return targetPos
}

// moveCursorDown moves cursor to the same column in the next visual line
func (m Model) moveCursorDown(runes []rune, cursorPos int) int {
	if len(runes) == 0 {
		return 0
	}

	width := m.width - 2
	if width < 20 {
		width = 80
	}

	// Find current visual line boundaries
	currentLineStart := m.lineStartPosition(runes, cursorPos)
	currentLineEnd := m.lineEndPosition(runes, cursorPos)
	currentCol := cursorPos - currentLineStart

	// Find the end of current visual line
	visLineWidth := 0
	for i := currentLineStart; i < len(runes); i++ {
		charWidth := lipgloss.Width(string(runes[i]))
		if visLineWidth+charWidth > width && visLineWidth > 0 {
			currentLineEnd = i
			break
		}
		visLineWidth += charWidth
	}

	if currentLineEnd >= len(runes) {
		// Already at last visual line, stay at end
		return len(runes)
	}

	// Find start of next visual line (which is currentLineEnd)
	nextLineStart := currentLineEnd

	// Calculate target position in next line
	targetPos := nextLineStart + currentCol
	if targetPos >= len(runes) {
		targetPos = len(runes)
	}

	return targetPos
}

// wrapMessageContent wraps message content by visual width, preserving line breaks
