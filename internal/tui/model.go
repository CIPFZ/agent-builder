package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
)

var greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#52B788")).Bold(true)

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
	ToolName  string
	ToolInput string
	ToolStatus string
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
	cursorPos       int  // cursor position in runes
	busy            bool
	pendingApproval *approval.Request
	diagnostics     diagnosticsState
	activity        activityState
	width           int
	height          int
	history         []string
	historyIndex    int
	suggestions     []string
	selectedIndex   int
}

var slashCommands = []string{"/help", "/clear", "/model", "/session", "/compact", "/debug"}

func NewModel(bridge Bridge, cfg ...ModelConfig) Model {
	model := Model{
		bridge:       bridge,
		transcript:   make([]transcriptEntry, 0, 32),
		events:       []string{"Welcome to myclaw TUI"},
		history:      make([]string, 0, 32),
		historyIndex: -1,
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
		// Enter sends the message
		m.clearSuggestions()
		text := strings.TrimSpace(m.input)
		if text == "" {
			return m, nil
		}
		if text != "" && (len(m.history) == 0 || m.history[len(m.history)-1] != text) {
			m.history = append(m.history, text)
		}
		m.historyIndex = -1
		m.cursorPos = 0
		m.bridge.SendUserMessage(text)
		m.transcript = append(m.transcript, transcriptEntry{Role: "user", Content: text})
		m.input = ""
		m.busy = true
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
		if m.pendingApproval != nil {
			m.bridge.Approve(m.pendingApproval.ID)
			m.pendingApproval = nil
			m.busy = true
		}
		return m, nil
	case tea.KeyCtrlN:
		if m.pendingApproval != nil {
			m.bridge.Reject(m.pendingApproval.ID)
			m.pendingApproval = nil
			m.busy = false
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
	m.suggestions = nil
	m.selectedIndex = -1
}

func (m *Model) updateRuntimeEvent(event runtime.RuntimeEvent) {
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
		m.activity.Label = fmt.Sprintf("Running tool: %s %s", event.ToolName, event.ToolInput)
		m.transcript = append(m.transcript, transcriptEntry{Role: "tool", ToolName: event.ToolName, ToolInput: event.ToolInput, ToolStatus: "called", Content: fmt.Sprintf("Calling %s...", event.ToolName)})
	case "tool.result":
		m.activity.Label = fmt.Sprintf("Tool finished: %s", event.ToolName)
		for i := len(m.transcript) - 1; i >= 0; i-- {
			if m.transcript[i].Role == "tool" && m.transcript[i].ToolStatus == "called" {
				m.transcript[i].ToolStatus = "result"
				if event.Message != nil {
					m.transcript[i].Content = event.Message.Content
				} else {
					m.transcript[i].Content = "(no output)"
				}
				break
			}
		}
	case "permission.required":
		m.pendingApproval = event.Approval
		if event.Approval != nil {
			m.activity.Label = fmt.Sprintf("Awaiting approval: %s %s", event.Approval.ToolName, event.Approval.ToolInput)
		}
		m.busy = false
	case "run.error":
		if event.Error != "" {
			m.diagnostics.LastError = event.Error
			m.activity.Label = "Run error"
		}
	case "agent.lifecycle.start":
		m.busy = true
		if m.activity.Label == "" {
			m.activity.Label = "Running turn"
		}
	case "agent.lifecycle.end":
		m.busy = false
		m.activity.Label = "Idle"
	}
}

func (m Model) View() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	return m.renderLayout(width)
}

func (m Model) renderLayout(width int) string {
	var b strings.Builder
	b.WriteString(m.renderHeader(width))
	b.WriteString(m.renderMessages(width))
	b.WriteString(m.renderFooter(width))
	return b.String()
}

func (m Model) renderHeader(width int) string {
	// Fixed layout with exact character positions
	// ╭─────────────────────────────── myclaw v0.1.0 ──────────────────────────────╮
	// │ Welcome back!                                      │ Tips for getting started │
	// │                                                    │ ─────────────────────── │
	// │ [ASCII ART]                                        │ /help - see commands    │
	// │                                                    │ /clear - clear conv     │
	// │ MiniMax-M2.7 · Agent Builder                      │ /model - switch model   │

	leftW := 56
	// Row format: │ content(pad leftW) │ content(pad rightW) │
	// Total = 2 (left │) + leftW + 3 ( │ ) + rightW + 2 ( │ + \n)
	// We want width = 2 + leftW + 3 + rightW + 2 => rightW = width - leftW - 7
	rightW := width - leftW - 7
	if rightW < 20 {
		rightW = 20
	}

	borderW := width - 2
	title := " myclaw v0.1.0 "
	titlePad := borderW - len(title)
	if titlePad < 0 {
		titlePad = 0
	}

	var b strings.Builder
	b.WriteString("╭" + title + strings.Repeat("─", titlePad) + "╮\n")

	// Pre-defined rows with exact content
	type row struct{ left, right string }
	rows := []row{
		{"Welcome back!", "Tips for getting started"},
		{"", "─────────────────────────"},
		{greenStyle.Render("███╗   ███╗██╗   ██╗ ██████╗██╗      █████╗ ██╗    ██╗"), "/help - see available commands"},
		{greenStyle.Render("████╗ ████║╚██╗ ██╔╝██╔════╝██║     ██╔══██╗██║    ██║"), "/clear - clear conversation"},
		{greenStyle.Render("██╔████╔██║ ╚████╔╝ ██║     ██║     ███████║██║ █╗ ██║"), "/model - switch models"},
		{greenStyle.Render("██║╚██╔╝██║  ╚██╔╝  ██║     ██║     ██╔══██║██║███╗██║"), ""},
		{greenStyle.Render("██║ ╚═╝ ██║   ██║   ╚██████╗███████╗██║  ██║╚███╔███╔╝"), ""},
		{greenStyle.Render("╚═╝     ╚═╝   ╚═╝    ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝"), ""},
		{"", ""},
		{"MiniMax-M2.7 · Agent Builder", ""},
	}

	for _, r := range rows {
		l := r.left
		rStr := r.right

		// Left side
		b.WriteString("│ ")
		b.WriteString(l)
		lWidth := lipgloss.Width(l)
		for lWidth < leftW {
			b.WriteString(" ")
			lWidth++
		}
		// Divider
		b.WriteString(" │ ")
		// Right side
		b.WriteString(rStr)
		rWidth := lipgloss.Width(rStr)
		for rWidth < rightW {
			b.WriteString(" ")
			rWidth++
		}
		b.WriteString(" │\n")
	}

	b.WriteString("╰" + strings.Repeat("─", borderW) + "╯\n")
	return b.String()
}

func (m Model) renderMessages(width int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("─", width) + "\n")

	if len(m.transcript) == 0 {
		msg := "(no messages yet - start a conversation!)"
		pad := (width - len(msg)) / 2
		b.WriteString(strings.Repeat(" ", pad) + msg + "\n")
	} else {
		start := 0
		if len(m.transcript) > 8 {
			start = len(m.transcript) - 8
		}
		for _, e := range m.transcript[start:] {
			switch e.Role {
			case "tool":
				status := "◐"
				if e.ToolStatus == "result" {
					status = "✓"
				}
				b.WriteString(fmt.Sprintf("%s %s: %s\n", status, e.ToolName, e.ToolInput))
			default:
				prefix := ""
				if e.Role == "user" {
					prefix = "user: "
				} else if e.Role == "assistant" {
					prefix = "assistant: "
				}
				content := e.Content
				if e.Streaming {
					content += "▊"
				}
				if len(content) > width-15 {
					content = content[:width-18] + "..."
				}
				b.WriteString(prefix + content + "\n")
			}
		}
	}

	if m.pendingApproval != nil {
		b.WriteString(fmt.Sprintf("\n⚠ Permission Required: %s %s\nCtrl+Y approve | Ctrl+N reject\n",
			m.pendingApproval.ToolName, m.pendingApproval.ToolInput))
	}
	return b.String()
}

func (m Model) renderFooter(width int) string {
	var b strings.Builder

	// Top border
	b.WriteString(strings.Repeat("─", width) + "\n")

	// Build input string with cursor at correct position
	inputWithCursor := m.buildInputWithCursor(width)

	// Show input with cursor
	b.WriteString("❯ " + inputWithCursor + "\n")

	// Bottom border
	b.WriteString(strings.Repeat("─", width) + "\n")

	// Help text on the right
	help := "Enter to send  |  ↑↓ history  |  / for commands"
	if m.pendingApproval != nil {
		help = "Ctrl+Y approve  |  Ctrl+N reject"
	}

	// Right-align help text
	helpWidth := lipgloss.Width(help)
	helpPad := width - helpWidth
	if helpPad < 0 {
		helpPad = 0
	}
	b.WriteString(strings.Repeat(" ", helpPad) + help + "\n")

	return b.String()
}

// lineStartPosition returns the position of the first character in the current line
func (m Model) lineStartPosition(runes []rune, pos int) int {
	for pos > 0 {
		if runes[pos-1] == '\n' {
			break
		}
		pos--
	}
	return pos
}

// lineEndPosition returns the position after the last character in the current line
// (before the \n or at the end of text)
func (m Model) lineEndPosition(runes []rune, pos int) int {
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

// buildInputWithCursor builds the input string with cursor at the correct visual position
// For multi-line input, it preserves the original line structure and renders cursor on the correct line
func (m Model) buildInputWithCursor(width int) string {
	runes := []rune(m.input)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos < 0 {
		pos = 0
	}

	// Calculate available width (subtract ❯  prefix)
	availableWidth := width - 2
	if availableWidth < 10 {
		availableWidth = 40
	}

	// Handle empty input - show cursor at position 0
	if len(runes) == 0 {
		return CursorStyle.Render(" ")
	}

	return m.renderMultiLineInput(runes, pos, availableWidth)
}

// renderMultiLineInput renders multi-line input with cursor at correct position
// It handles both explicit \n newlines AND automatic text wrapping
func (m Model) renderMultiLineInput(runes []rune, cursorPos int, width int) string {
	// Split input into explicit lines by \n
	var explicitLines [][]rune
	var currentLine []rune
	for _, r := range runes {
		if r == '\n' {
			explicitLines = append(explicitLines, currentLine)
			currentLine = nil
		} else {
			currentLine = append(currentLine, r)
		}
	}
	if len(currentLine) > 0 || len(explicitLines) == 0 {
		explicitLines = append(explicitLines, currentLine)
	}

	// Build visual lines with automatic wrapping
	type visualLine struct {
		runes      []rune
		isCursor   bool
		cursorCol  int // column within this visual line where cursor should be
		origPos    int // original rune position in input
	}

	var visualLines []visualLine

	for lineIdx, lineRunes := range explicitLines {
		var visLine []rune
		visLineWidth := 0
		lineStartOrigPos := 0
		if lineIdx > 0 {
			// Account for the \n character from previous line
			for i := 0; i < len(explicitLines[:lineIdx]); i++ {
				for j := 0; j < len(explicitLines[i]); j++ {
					lineStartOrigPos++
				}
				lineStartOrigPos++ // for \n
			}
		}

		origPos := 0
		for origPos <= len(lineRunes) {
			if origPos == len(lineRunes) {
				// End of explicit line
				if len(visLine) > 0 {
					isCursor := cursorPos >= lineStartOrigPos && cursorPos <= lineStartOrigPos+len(lineRunes)
					cursorCol := 0
					if isCursor {
						cursorCol = cursorPos - lineStartOrigPos
					}
					visualLines = append(visualLines, visualLine{runes: visLine, isCursor: isCursor, cursorCol: cursorCol, origPos: lineStartOrigPos})
				}
				break
			}

			char := string(lineRunes[origPos])
			charWidth := lipgloss.Width(char)

			// Check if adding this character exceeds width
			if visLineWidth+charWidth > width {
				// Need to wrap to new visual line
				isCursor := cursorPos >= lineStartOrigPos && cursorPos <= lineStartOrigPos+len(visLine)
				cursorCol := 0
				if isCursor {
					cursorCol = cursorPos - lineStartOrigPos
				}
				visualLines = append(visualLines, visualLine{runes: visLine, isCursor: isCursor, cursorCol: cursorCol, origPos: lineStartOrigPos})

				// Start new visual line
				visLine = []rune{lineRunes[origPos]}
				visLineWidth = charWidth
				lineStartOrigPos += len(visLine) - 1
				origPos++
			} else {
				visLine = append(visLine, lineRunes[origPos])
				visLineWidth += charWidth
				origPos++
			}
		}
	}

	// Render visual lines
	var result strings.Builder
	for i, vline := range visualLines {
		if i > 0 {
			result.WriteString("\n")
		}

		if vline.isCursor && vline.cursorCol >= 0 && vline.cursorCol < len(vline.runes) {
			// Cursor is within this line
			before := string(vline.runes[:vline.cursorCol])
			cursorChar := string(vline.runes[vline.cursorCol])
			after := string(vline.runes[vline.cursorCol+1:])
			result.WriteString(InputTextStyle.Render(before))
			result.WriteString(CursorStyle.Render(cursorChar))
			result.WriteString(InputTextStyle.Render(after))
		} else if vline.isCursor && vline.cursorCol >= len(vline.runes) {
			// Cursor at end of line
			result.WriteString(InputTextStyle.Render(string(vline.runes)))
			result.WriteString(CursorStyle.Render(" "))
		} else {
			// No cursor in this line
			result.WriteString(InputTextStyle.Render(string(vline.runes)))
		}
	}

	return result.String()
}
