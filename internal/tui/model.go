package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const mouseWheelScrollLines = 3

type Model struct {
	bridge Bridge
	tuiState
}

func NewModel(bridge Bridge, cfg ...ModelConfig) Model {
	model := Model{
		bridge:   bridge,
		tuiState: newTUIState(cfg...),
	}
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(typed.Width, typed.Height)
	case tea.MouseMsg:
		return m.handleMouse(typed)
	case tea.PasteMsg:
		if m.handlePasteText(typed.Content) {
			return m, nil
		}
	case tea.KeyMsg:
		return m.updateKey(typed)
	case RuntimeEventMsg:
		if m.store != nil {
			before := len(m.transcript)
			m.applyStoreSnapshot(m.store.snapshot())
			if len(m.transcript) > before {
				m.noteTranscriptAppended()
			}
			if typed.Event.Type == "permission.required" {
				m.dialog.close()
				m.approvalDialog.open(m.pendingApproval)
			}
			if typed.Event.Type == "approval.updated" {
				m.approvalDialog.close()
			}
			m.refreshTranscriptSearch()
			break
		}
		m.updateRuntimeEvent(typed.Event)
	case BridgeErrMsg:
		if m.store != nil {
			m.applyStoreSnapshot(m.store.applyBridgeError(typed.Err))
		} else {
			m.applyBridgeError(typed.Err)
		}
	case externalEditorFinishedMsg:
		m.applyExternalEditorFinished(typed)
	case globalSearchResultsMsg:
		m.applyGlobalSearchResults(typed)
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.scrollTranscriptUp(mouseWheelScrollLines)
	case tea.MouseWheelDown:
		m.scrollTranscriptDown(mouseWheelScrollLines)
	}
	return m, nil
}

func (m Model) View() tea.View {
	view := tea.NewView(m.viewContent())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.Cursor = m.inputCursor()
	return view
}

func (m Model) viewContent() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	return newRenderer().renderLayout(m, width)
}

func (m Model) inputCursor() *tea.Cursor {
	if m.externalEditor.Active || m.dialog.active() || m.approvalDialog.active() || m.messageActions.Active {
		return nil
	}
	width := m.width
	if width == 0 {
		width = defaultRenderWidth
	}
	if width < minRenderWidth {
		width = minRenderWidth
	}
	snapshot := newRenderSnapshot(m, width)
	lines := buildInputVisualLines(snapshot.Input.Text, width-2)
	lineIndex := inputCursorLineIndex(lines, snapshot.Input.Cursor)
	column := 0
	if lineIndex >= 0 && lineIndex < len(lines) {
		column = inputCursorColumn(lines[lineIndex], snapshot.Input.Text, snapshot.Input.Cursor)
	}
	r := newRenderer()
	y := strings.Count(r.renderHeader(snapshot)+r.renderTranscript(snapshot), "\n") + 1 + lineIndex
	return tea.NewCursor(2+column, y)
}
