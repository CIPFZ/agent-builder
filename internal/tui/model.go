package tui

import tea "github.com/charmbracelet/bubbletea"

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
		return m, m.handleMouse(typed)
	case tea.KeyMsg:
		return m.updateKey(typed)
	case RuntimeEventMsg:
		m.updateRuntimeEvent(typed.Event)
	case BridgeErrMsg:
		m.applyBridgeError(typed.Err)
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Mouse wheel scrolling is handled by the terminal's scrollback buffer
	// No local scrolling needed - just let the terminal handle it
	return nil
}

func (m Model) View() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	return newRenderer().renderLayout(m, width)
}
