package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
)

type Bridge interface {
	SendUserMessage(string) error
	Approve(string) error
	Reject(string) error
}

type RuntimeEventMsg struct {
	Event runtime.RuntimeEvent
}

type BridgeErrMsg struct {
	Err error
}

type transcriptEntry struct {
	Role      string
	Content   string
	Streaming bool
}

type ModelConfig struct {
	SessionID string
	LLMLabel  string
	LogPath   string
}

type diagnosticsState struct {
	SessionID  string
	LLMLabel   string
	LogPath    string
	LastEvent  string
	LastError  string
	EventCount int
	LastMsg    string
}

type activityState struct {
	Label string
}

type Model struct {
	bridge          Bridge
	transcript      []transcriptEntry
	events          []string
	input           string
	busy            bool
	pendingApproval *approval.Request
	diagnostics     diagnosticsState
	activity        activityState
}

func NewModel(bridge Bridge, cfg ...ModelConfig) Model {
	model := Model{
		bridge:     bridge,
		transcript: make([]transcriptEntry, 0, 32),
		events:     []string{"Welcome to myclaw TUI"},
	}
	if len(cfg) > 0 {
		model.diagnostics.SessionID = cfg[0].SessionID
		model.diagnostics.LLMLabel = cfg[0].LLMLabel
		model.diagnostics.LogPath = cfg[0].LogPath
	}
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		m.diagnostics.LastMsg = "tea.KeyMsg"
		return m.updateKey(typed)
	case RuntimeEventMsg:
		m.diagnostics.LastMsg = "RuntimeEventMsg"
		return m.updateRuntimeEvent(typed.Event), nil
	case BridgeErrMsg:
		m.diagnostics.LastMsg = "BridgeErrMsg"
		if typed.Err != nil {
			m.events = append(m.events, "error: "+typed.Err.Error())
			m.busy = false
			m.diagnostics.LastError = typed.Err.Error()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input)
		if text == "" || m.pendingApproval != nil {
			return m, nil
		}
		if err := m.bridge.SendUserMessage(text); err != nil {
			m.events = append(m.events, "send failed: "+err.Error())
			return m, nil
		}
		m.transcript = append(m.transcript, transcriptEntry{Role: "user", Content: text})
		m.input = ""
		m.busy = true
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	case tea.KeyCtrlY:
		if m.pendingApproval == nil {
			return m, nil
		}
		if err := m.bridge.Approve(m.pendingApproval.ID); err != nil {
			m.events = append(m.events, "approve failed: "+err.Error())
			return m, nil
		}
		m.events = append(m.events, "approval approved: "+m.pendingApproval.ID)
		m.pendingApproval = nil
		m.busy = true
		return m, nil
	case tea.KeyCtrlN:
		if m.pendingApproval == nil {
			return m, nil
		}
		if err := m.bridge.Reject(m.pendingApproval.ID); err != nil {
			m.events = append(m.events, "reject failed: "+err.Error())
			return m, nil
		}
		m.events = append(m.events, "approval rejected: "+m.pendingApproval.ID)
		m.pendingApproval = nil
		m.busy = false
		return m, nil
	case tea.KeyRunes:
		m.input += string(msg.Runes)
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) updateRuntimeEvent(event runtime.RuntimeEvent) Model {
	m.diagnostics.LastEvent = event.Type
	m.diagnostics.EventCount++
	if event.Session.ID != "" && m.diagnostics.SessionID == "" {
		m.diagnostics.SessionID = event.Session.ID
	}
	switch event.Type {
	case "assistant.delta":
		if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Role != "assistant" || !m.transcript[len(m.transcript)-1].Streaming {
			m.transcript = append(m.transcript, transcriptEntry{Role: "assistant", Content: event.Delta, Streaming: true})
		} else {
			m.transcript[len(m.transcript)-1].Content += event.Delta
		}
	case "message.created":
		if event.Message != nil {
			if event.Message.Role == "assistant" {
				if len(m.transcript) > 0 && m.transcript[len(m.transcript)-1].Role == "assistant" && m.transcript[len(m.transcript)-1].Streaming {
					m.transcript[len(m.transcript)-1].Content = event.Message.Content
					m.transcript[len(m.transcript)-1].Streaming = false
				} else {
					m.transcript = append(m.transcript, transcriptEntry{Role: "assistant", Content: event.Message.Content})
				}
				m.busy = false
			}
			if event.Message.Role == "tool" {
				m.transcript = append(m.transcript, transcriptEntry{Role: "tool", Content: event.Message.Content})
			}
		}
	case "tool.called":
		m.events = append(m.events, fmt.Sprintf("tool called: %s %s", event.ToolName, event.ToolInput))
		m.activity.Label = strings.TrimSpace(fmt.Sprintf("Running tool: %s %s", event.ToolName, event.ToolInput))
	case "tool.result":
		m.events = append(m.events, fmt.Sprintf("tool result: %s", event.ToolName))
		m.activity.Label = strings.TrimSpace(fmt.Sprintf("Tool finished: %s", event.ToolName))
	case "permission.required":
		m.pendingApproval = event.Approval
		if event.Approval != nil {
			m.events = append(m.events, fmt.Sprintf("approval required: %s %s", event.Approval.ToolName, event.Approval.ToolInput))
			m.activity.Label = strings.TrimSpace(fmt.Sprintf("Awaiting approval: %s %s", event.Approval.ToolName, event.Approval.ToolInput))
		}
		m.busy = false
	case "run.error":
		if event.Error != "" {
			m.events = append(m.events, "run error: "+event.Error)
			m.diagnostics.LastError = event.Error
			m.activity.Label = "Run error"
		}
	case "agent.lifecycle.start":
		m.busy = true
		if strings.TrimSpace(m.activity.Label) == "" {
			m.activity.Label = "Running turn"
		}
	case "agent.lifecycle.end":
		m.busy = false
		m.activity.Label = "Idle"
	case "compact.warning":
		m.events = append(m.events, "compact warning")
		m.activity.Label = "Compaction: warning"
	case "compact.error":
		m.events = append(m.events, "compact error")
		m.activity.Label = "Compaction: error"
	case "compact.auto":
		m.events = append(m.events, "compact auto")
		m.activity.Label = "Compaction: auto"
	case "compact.blocked":
		m.events = append(m.events, "compact blocked")
		m.activity.Label = "Compaction: blocked"
	case "compact.boundary":
		m.events = append(m.events, "compact boundary")
		m.activity.Label = "Compaction: boundary"
	case "compact.replayed":
		m.events = append(m.events, "compact replayed")
		m.activity.Label = "Compaction: replayed"
	case "compact.memory_saved":
		m.events = append(m.events, "compact memory saved")
		m.activity.Label = "Compaction: memory saved"
	case "compact.cleaned":
		m.events = append(m.events, "compact cleaned")
		m.activity.Label = "Compaction: cleaned"
	}
	return m
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("myclaw TUI\n\n")
	b.WriteString("Diagnostics\n")
	b.WriteString("  Session: " + fallback(m.diagnostics.SessionID, "(pending)") + "\n")
	b.WriteString("  LLM: " + fallback(m.diagnostics.LLMLabel, "(unknown)") + "\n")
	b.WriteString("  Log: " + fallback(m.diagnostics.LogPath, "(disabled)") + "\n")
	b.WriteString("  Last Event: " + fallback(m.diagnostics.LastEvent, "(none)") + "\n")
	b.WriteString("  Last Msg: " + fallback(m.diagnostics.LastMsg, "(none)") + "\n")
	b.WriteString("  Last Error: " + fallback(m.diagnostics.LastError, "(none)") + "\n")
	b.WriteString(fmt.Sprintf("  Event Count: %d\n\n", m.diagnostics.EventCount))
	b.WriteString("Activity\n")
	b.WriteString("  " + fallback(m.activity.Label, "(idle)") + "\n\n")
	b.WriteString("Transcript\n")
	if len(m.transcript) == 0 {
		b.WriteString("  (empty)\n")
	} else {
		for _, entry := range m.transcript {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", entry.Role, entry.Content))
		}
	}
	b.WriteString("\nEvents\n")
	start := 0
	if len(m.events) > 8 {
		start = len(m.events) - 8
	}
	for _, event := range m.events[start:] {
		b.WriteString("  - " + event + "\n")
	}
	b.WriteString("\nInput\n")
	b.WriteString("> " + m.input + "\n")
	if m.pendingApproval != nil {
		b.WriteString(fmt.Sprintf("\nApproval pending: %s %s\n", m.pendingApproval.ToolName, m.pendingApproval.ToolInput))
		b.WriteString("Press Ctrl+Y to approve, Ctrl+N to reject.\n")
	}
	if m.busy {
		b.WriteString("\nStatus: busy\n")
	} else {
		b.WriteString("\nStatus: idle\n")
	}
	b.WriteString("Ctrl+C to quit.\n")
	return b.String()
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}
