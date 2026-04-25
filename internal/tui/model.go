package tui

import tea "github.com/charmbracelet/bubbletea"

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
	case tea.KeyMsg:
		return m.updateKey(typed)
	case RuntimeEventMsg:
		if m.store != nil {
			before := len(m.transcript)
			m.applyStoreSnapshot(m.store.applyEvent(typed.Event))
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
	switch msg.Type {
	case tea.MouseWheelUp:
		m.scrollTranscriptUp(mouseWheelScrollLines)
	case tea.MouseWheelDown:
		m.scrollTranscriptDown(mouseWheelScrollLines)
	}
	return m, nil
}

func (m Model) View() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	return newRenderer().renderLayout(m, width)
}
