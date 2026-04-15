package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
)

type Model struct {
	bridge     Bridge
	transcript []transcriptEntry
	events     []string
	inputState
	busy            bool
	pendingApproval *approval.Request
	diagnostics     diagnosticsState
	activity        activityState
	width           int
	height          int
}

var slashCommands = []string{"/help", "/clear", "/model", "/session", "/compact", "/debug"}

func NewModel(bridge Bridge, cfg ...ModelConfig) Model {
	model := Model{
		bridge:     bridge,
		transcript: make([]transcriptEntry, 0, 32),
		events:     []string{"Welcome to myclaw TUI"},
		inputState: newInputState(),
	}
	if len(cfg) > 0 {
		model.diagnostics.SessionID = cfg[0].SessionID
		model.diagnostics.LLMLabel = cfg[0].LLMLabel
		model.diagnostics.LogPath = cfg[0].LogPath
	}
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
	case tea.MouseMsg:
		return m, m.handleMouse(typed)
	case tea.KeyMsg:
		return m.updateKey(typed)
	case RuntimeEventMsg:
		m.updateRuntimeEvent(typed.Event)
	case BridgeErrMsg:
		if typed.Err != nil {
			m.events = append(m.events, "error: "+typed.Err.Error())
			m.busy = false
			m.diagnostics.LastError = typed.Err.Error()
		}
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
