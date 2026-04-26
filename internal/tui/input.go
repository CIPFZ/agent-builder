package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.approvalDialog.active() {
		if keyEventType(msg) == keyCtrlC {
			return m, tea.Quit
		}
		result := m.approvalDialog.handleKey(msg)
		switch result.Action {
		case approvalDialogApprove:
			if id, ok := m.approvePending(); ok {
				m.bridge.Approve(id)
			}
		case approvalDialogReject:
			if id, ok := m.rejectPending(); ok {
				m.bridge.Reject(id)
			}
		}
		return m, nil
	}
	if m.dialog.active() {
		dialogKind := m.dialog.Kind
		if m.dialog.Kind == dialogKindHistorySearch {
			switch keyEventType(msg) {
			case keyCtrlC, keyEscape:
				m.cancelHistorySearchDialog()
				return m, nil
			case keyBackspace:
				if m.dialog.Picker.Query == "" {
					m.cancelHistorySearchDialog()
					return m, nil
				}
			}
		} else if keyEventType(msg) == keyCtrlC {
			return m, tea.Quit
		}
		result := m.dialog.handleKey(msg)
		if dialogKind == dialogKindQuickOpen && m.dialog.active() {
			event := keyEventType(msg)
			preserveSelection := event != keyRunes && event != keyBackspace
			m.updateQuickOpenDialog(preserveSelection)
		}
		if dialogKind == dialogKindGlobalSearch && m.dialog.active() {
			event := keyEventType(msg)
			preserveSelection := event != keyRunes && event != keyBackspace
			cmd := m.triggerGlobalSearch()
			m.updateGlobalSearchDialog(preserveSelection)
			return m, cmd
		}
		if result.Selected {
			item := result.Item
			m.lastDialogSelection = &item
			switch dialogKind {
			case dialogKindHistorySearch:
				m.acceptHistorySearchItem(item)
			case dialogKindQuickOpen:
				m.acceptQuickOpenItem(item, result.Action)
			case dialogKindGlobalSearch:
				m.acceptGlobalSearchItem(item, result.Action)
			case dialogKindMCPList:
				m.acceptMCPItem(item)
			case dialogKindModel:
				m.applyModelSelection(item.Value)
			case dialogKindCompaction:
				m.acceptCompactionItem(item)
			case dialogKindSessionResume:
				m.acceptSessionResumeItem(item)
			case dialogKindTasks:
				m.acceptTaskItem(item)
			}
		}
		return m, nil
	}
	if m.handleMessageActionsKey(msg) {
		return m, nil
	}
	if m.handleTranscriptSearchKey(msg) {
		return m, nil
	}
	if cmd, handled := m.handleExternalEditorKey(msg); handled {
		return m, cmd
	}
	if m.handlePromptStashKey(msg) {
		return m, nil
	}
	if m.viewport.TranscriptMode {
		switch keyEventType(msg) {
		case keyEscape, keyCtrlC:
			m.exitTranscriptMode()
			return m, nil
		}
	}
	if m.handleViewportKey(msg) {
		return m, nil
	}
	if isLargeInput(msg) || containsBracketedPaste(msg) {
		if m.handlePasteKey(msg) {
			return m, nil
		}
	}
	switch keyEventType(msg) {
	case keyCtrlC:
		return m, tea.Quit
	case keyEnter:
		if m.pendingApproval != nil {
			m.clearSuggestions()
			return m, nil
		}
		displayText := strings.TrimSpace(m.input)
		if m.handleLocalCommand(strings.TrimSpace(m.input)) {
			return m, nil
		}
		shouldRestoreStash := m.promptStash.HasStash && !strings.HasPrefix(displayText, "/")
		text, ok := m.submitUserInput()
		if !ok {
			return m, nil
		}
		if shouldRestoreStash {
			m.restorePromptStash()
		}
		return m, sendUserMessageCmd(m.bridge, text)
	case keyCtrlY:
		if id, ok := m.approvePending(); ok {
			m.bridge.Approve(id)
		}
		return m, nil
	case keyCtrlN:
		if id, ok := m.rejectPending(); ok {
			m.bridge.Reject(id)
		}
		return m, nil
	case keyCtrlR:
		if m.viewport.Search.Active {
			return m, nil
		}
		m.openHistorySearchDialog()
		return m, nil
	case keyCtrlK:
		m.openQuickOpenDialog()
		return m, nil
	case keyShiftUp:
		if m.viewport.Search.Active {
			return m, nil
		}
		m.enterMessageActions()
		return m, nil
	default:
		if m.inputState.handleEditingKey(msg, m.width, slashCommands) {
			return m, nil
		}
		return m, nil
	}
}

func (m *Model) handlePasteKey(msg tea.KeyMsg) bool {
	if keyEventType(msg) != keyRunes {
		return false
	}
	return m.handlePasteText(string(keyEventRunes(msg)))
}

func (m *Model) handlePasteText(text string) bool {
	text = sanitizePastedText(text)
	if text == "" {
		m.clearSuggestions()
		return true
	}
	numLines := pastedTextRefNumLines(text)
	inserted := text
	if len([]rune(text)) > pasteThreshold || numLines > m.maxVisiblePasteLines() {
		inserted = m.pastes.addText(text, numLines)
	}
	m.inputState.insertRunes([]rune(inserted), slashCommands)
	return true
}

func (m Model) maxVisiblePasteLines() int {
	if m.height == 0 {
		return 2
	}
	maxLines := m.height - 10
	if maxLines > 2 {
		return 2
	}
	if maxLines < 0 {
		return 0
	}
	return maxLines
}

func isLargeInput(msg tea.KeyMsg) bool {
	return keyEventType(msg) == keyRunes && len(keyEventRunes(msg)) > pasteThreshold
}

func containsBracketedPaste(msg tea.KeyMsg) bool {
	if keyEventType(msg) != keyRunes {
		return false
	}
	text := string(keyEventRunes(msg))
	return strings.Contains(text, "\x1b[200~") && strings.Contains(text, "\x1b[201~")
}

func (m *Model) handleViewportKey(msg tea.KeyMsg) bool {
	pageLines := m.transcriptPageLines()
	switch keyEventType(msg) {
	case keyCtrlO:
		m.toggleTranscriptMode()
	case keyCtrlF:
		m.startTranscriptSearch()
	case keyCtrlE:
		m.toggleTranscriptHistory()
	case keyPgUp:
		m.scrollTranscriptUp(pageLines)
	case keyPgDown:
		m.scrollTranscriptDown(pageLines)
	case keyHome:
		if !m.viewport.TranscriptMode && m.input != "" {
			return false
		}
		m.scrollTranscriptTop()
	case keyEnd:
		if !m.viewport.TranscriptMode && m.input != "" {
			return false
		}
		m.scrollTranscriptBottom()
	default:
		return false
	}
	return true
}

func (m Model) transcriptPageLines() int {
	snapshot := newRenderSnapshot(m, m.width)
	visible := snapshot.transcriptVisibleLines()
	if visible <= 0 {
		return viewportDefaultPageLines
	}
	return visible
}

func (s *inputState) handleEditingKey(msg tea.KeyMsg, width int, commands []string) bool {
	s.normalizeCursor()
	switch keyEventType(msg) {
	case keyLeft:
		s.moveLeft()
	case keyRight:
		s.moveRight()
	case keyHome:
		s.cursorPos = s.lineStartPosition(width)
	case keyEnd:
		s.cursorPos = s.lineEndPosition(width)
	case keyUp:
		s.moveUp(width)
	case keyDown:
		s.moveDown(width)
	case keyTab:
		s.acceptSuggestion()
	case keySpace:
		s.insertRunes([]rune(" "), commands)
	case keyEscape:
		s.clearSuggestions()
	case keyBackspace:
		s.backspace(commands)
	case keyDelete:
		s.deleteAtCursor(commands)
	case keyRunes:
		s.insertRunes(keyEventRunes(msg), commands)
	default:
		return false
	}
	return true
}

func (s *inputState) normalizeCursor() {
	runes := []rune(s.input)
	if s.cursorPos < 0 {
		s.cursorPos = 0
	}
	if s.cursorPos > len(runes) {
		s.cursorPos = len(runes)
	}
}

func (s *inputState) insertRunes(runes []rune, commands []string) {
	current := []rune(s.input)
	s.input = string(current[:s.cursorPos]) + string(runes) + string(current[s.cursorPos:])
	s.cursorPos += len(runes)
	s.historyIndex = -1
	s.updateSuggestions(commands)
}

func (s *inputState) backspace(commands []string) {
	current := []rune(s.input)
	if s.cursorPos > 0 {
		s.input = string(current[:s.cursorPos-1]) + string(current[s.cursorPos:])
		s.cursorPos--
	}
	s.historyIndex = -1
	s.updateSuggestions(commands)
}

func (s *inputState) deleteAtCursor(commands []string) {
	current := []rune(s.input)
	if s.cursorPos < len(current) {
		s.input = string(current[:s.cursorPos]) + string(current[s.cursorPos+1:])
	}
	s.historyIndex = -1
	s.updateSuggestions(commands)
}

func (s *inputState) moveLeft() {
	if s.cursorPos > 0 {
		s.cursorPos--
	}
}

func (s *inputState) moveRight() {
	if s.cursorPos < len([]rune(s.input)) {
		s.cursorPos++
	}
}

func (s *inputState) moveUp(width int) {
	if len(s.suggestions) > 0 && s.selectedIndex > 0 {
		s.selectedIndex--
		return
	}
	if s.historyIndex == -1 {
		lineIndex, lineCount := s.cursorVisualLine(width)
		if lineCount > 1 {
			if lineIndex > 0 {
				s.cursorPos = s.moveCursorUp(width)
			}
			return
		}
	}
	if s.historyIndex != -1 || len(s.history) > 0 {
		s.moveHistoryUp()
		return
	}
}

func (s *inputState) moveHistoryUp() {
	if len(s.history) > 0 {
		if s.historyIndex == -1 {
			s.historyIndex = len(s.history) - 1
		} else if s.historyIndex > 0 {
			s.historyIndex--
		}
		s.input = s.history[s.historyIndex]
		s.cursorPos = len([]rune(s.input))
		s.clearSuggestions()
		return
	}
}

func (s *inputState) moveDown(width int) {
	if len(s.suggestions) > 0 && s.selectedIndex < len(s.suggestions)-1 {
		s.selectedIndex++
		return
	}
	if s.historyIndex == -1 {
		lineIndex, lineCount := s.cursorVisualLine(width)
		if lineCount > 1 {
			if lineIndex < lineCount-1 {
				s.cursorPos = s.moveCursorDown(width)
			}
			return
		}
	}
	if s.historyIndex != -1 {
		if s.historyIndex < len(s.history)-1 {
			s.historyIndex++
			s.input = s.history[s.historyIndex]
		} else {
			s.historyIndex = -1
			s.input = ""
		}
		s.cursorPos = len([]rune(s.input))
		s.clearSuggestions()
		return
	}
}

func (s inputState) cursorVisualLine(width int) (int, int) {
	lines := buildInputVisualLines(s.input, s.visualWidth(width))
	if len(lines) == 0 {
		return 0, 0
	}
	return inputCursorLineIndex(lines, s.cursorPos), len(lines)
}

func (s *inputState) updateSuggestions(commands []string) {
	if !strings.HasPrefix(s.input, "/") {
		s.clearSuggestions()
		return
	}
	if s.input == "/" {
		s.suggestions = append([]string(nil), commands...)
		s.selectedIndex = 0
		return
	}
	input := strings.ToLower(s.input)
	var matches []string
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd), input) {
			matches = append(matches, cmd)
		}
	}
	s.suggestions = matches
	if len(s.suggestions) > 0 {
		s.selectedIndex = 0
	} else {
		s.selectedIndex = -1
	}
}

func (s *inputState) acceptSuggestion() {
	if s.selectedIndex >= 0 && s.selectedIndex < len(s.suggestions) {
		s.input = s.suggestions[s.selectedIndex]
		s.cursorPos = len([]rune(s.input))
		s.historyIndex = -1
		s.clearSuggestions()
	}
}

func (s *inputState) clearSuggestions() {
	s.suggestions = nil
	s.selectedIndex = -1
}

func (m *Model) clearSuggestions() {
	m.tuiState.clearSuggestions()
}

func (m Model) updateSuggestions() {
	m.inputState.updateSuggestions(slashCommands)
}

func (m Model) acceptSuggestion() {
	m.inputState.acceptSuggestion()
}

func (s inputState) visualWidth(width int) int {
	visualWidth := width - 2
	if visualWidth < 20 {
		return 80
	}
	return visualWidth
}

func (s inputState) lineStartPosition(width int) int {
	lines := buildInputVisualLines(s.input, s.visualWidth(width))
	if len(lines) == 0 {
		return 0
	}
	return lines[inputCursorLineIndex(lines, s.cursorPos)].Start
}

func (s inputState) lineEndPosition(width int) int {
	lines := buildInputVisualLines(s.input, s.visualWidth(width))
	if len(lines) == 0 {
		return 0
	}
	return lines[inputCursorLineIndex(lines, s.cursorPos)].End
}

func (s inputState) moveCursorUp(width int) int {
	lines := buildInputVisualLines(s.input, s.visualWidth(width))
	if len(lines) == 0 || s.cursorPos == 0 {
		return 0
	}
	currentLineIndex := inputCursorLineIndex(lines, s.cursorPos)
	if currentLineIndex == 0 {
		return 0
	}
	currentColumn := inputCursorColumn(lines[currentLineIndex], s.input, s.cursorPos)
	return inputCursorPositionForColumn(lines[currentLineIndex-1], s.input, currentColumn)
}

func (s inputState) moveCursorDown(width int) int {
	lines := buildInputVisualLines(s.input, s.visualWidth(width))
	if len(lines) == 0 {
		return 0
	}
	currentLineIndex := inputCursorLineIndex(lines, s.cursorPos)
	if currentLineIndex >= len(lines)-1 {
		return len([]rune(s.input))
	}
	currentColumn := inputCursorColumn(lines[currentLineIndex], s.input, s.cursorPos)
	return inputCursorPositionForColumn(lines[currentLineIndex+1], s.input, currentColumn)
}

func (m Model) lineStartPosition(runes []rune, pos int) int {
	state := m.inputState
	state.input = string(runes)
	state.cursorPos = pos
	return state.lineStartPosition(m.width)
}

func (m Model) lineEndPosition(runes []rune, pos int) int {
	state := m.inputState
	state.input = string(runes)
	state.cursorPos = pos
	return state.lineEndPosition(m.width)
}

func (m Model) moveCursorUp(runes []rune, cursorPos int) int {
	state := m.inputState
	state.input = string(runes)
	state.cursorPos = cursorPos
	return state.moveCursorUp(m.width)
}

func (m Model) moveCursorDown(runes []rune, cursorPos int) int {
	state := m.inputState
	state.input = string(runes)
	state.cursorPos = cursorPos
	return state.moveCursorDown(m.width)
}
