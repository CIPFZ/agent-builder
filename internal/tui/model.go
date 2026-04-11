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

func (m Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Mouse wheel scrolling is handled by the terminal's scrollback buffer
	// No local scrolling needed - just let the terminal handle it
	return nil
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
		pad := (width - lipgloss.Width(msg)) / 2
		b.WriteString(strings.Repeat(" ", pad) + msg + "\n")
	} else {
		for _, e := range m.transcript {
			switch e.Role {
			case "tool":
				status := "◐"
				if e.ToolStatus == "result" {
					status = "✓"
				}
				b.WriteString(fmt.Sprintf("%s %s: %s\n", status, e.ToolName, e.ToolInput))
			case "user":
				content := e.Content
				if e.Streaming {
					content += "▊"
				}
				availableWidth := width - 6
				if availableWidth < 10 {
					availableWidth = 60
				}
				lines := m.wrapMessageContent(content, availableWidth)
				for i, line := range lines {
					if i == 0 {
						b.WriteString(UserMessageStyle.Render("❯ " + line) + "\n")
					} else {
						b.WriteString(UserMessageStyle.Render("  " + line) + "\n")
					}
				}
			case "assistant":
				content := e.Content
				if e.Streaming {
					content += "▊"
				}
				availableWidth := width - 2
				if availableWidth < 10 {
					availableWidth = 60
				}
				lines := m.wrapMessageContent(content, availableWidth)
				for _, line := range lines {
					b.WriteString(line + "\n")
				}
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

// lineStartPosition returns the position of the first character in the current visual line
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
func (m Model) wrapMessageContent(content string, width int) []string {
	var lines []string
	var currentLine strings.Builder
	lineWidth := 0

	for _, r := range content {
		if r == '\n' {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			lineWidth = 0
			continue
		}

		charWidth := lipgloss.Width(string(r))
		if lineWidth+charWidth > width && lineWidth > 0 {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			lineWidth = 0
		}
		currentLine.WriteRune(r)
		lineWidth += charWidth
	}

	if currentLine.Len() > 0 || len(lines) == 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
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
	// Step 1: Split input into explicit lines by \n, tracking global positions
	type ExplicitLine struct {
		runes   []rune
		startPos int // global start position in original runes
		endPos   int // global end position in original runes
	}

	var explicitLines []ExplicitLine
	globalPos := 0
	currentLine := []rune{}

	for _, r := range runes {
		if r == '\n' {
			explicitLines = append(explicitLines, ExplicitLine{
				runes:    currentLine,
				startPos: globalPos - len(currentLine),
				endPos:   globalPos,
			})
			globalPos++ // for the \n
			currentLine = nil
		} else {
			currentLine = append(currentLine, r)
			globalPos++
		}
	}
	// Add last line (or only line if no \n)
	if len(currentLine) > 0 || len(explicitLines) == 0 {
		explicitLines = append(explicitLines, ExplicitLine{
			runes:    currentLine,
			startPos: globalPos - len(currentLine),
			endPos:   globalPos,
		})
	}

	// Step 2: Wrap each explicit line into visual lines
	type VisualLine struct {
		runes      []rune
		startPos   int // global start position
		isCursor   bool
		cursorCol  int // column within this visual line
	}

	var visualLines []VisualLine

	for _, el := range explicitLines {
		lineRunes := el.runes
		lineStart := el.startPos
		lineLen := len(lineRunes)

		// Wrap this explicit line into visual lines
		visLineStart := 0
		for visLineStart < lineLen {
			// Find how many chars fit in this visual line
			visLineEnd := visLineStart
			visLineWidth := 0
			for visLineEnd < lineLen {
				charWidth := lipgloss.Width(string(lineRunes[visLineEnd]))
				if visLineWidth+charWidth > width {
					break
				}
				visLineWidth += charWidth
				visLineEnd++
			}

			// If nothing fits ( shouldn't happen with width > 0), force at least 1 char
			if visLineEnd == visLineStart && visLineStart < lineLen {
				visLineEnd = visLineStart + 1
			}

			// Determine cursor state for this visual line
			visLineRunes := lineRunes[visLineStart:visLineEnd]
			globalStart := lineStart + visLineStart
			globalEnd := lineStart + visLineEnd

			isCursor := cursorPos >= globalStart && cursorPos < globalEnd
			cursorCol := -1
			if isCursor {
				cursorCol = cursorPos - globalStart
			}

			visualLines = append(visualLines, VisualLine{
				runes:     visLineRunes,
				startPos:  globalStart,
				isCursor:  isCursor,
				cursorCol: cursorCol,
			})

			visLineStart = visLineEnd
		}
	}

	// Special case: if cursor is at the end of content and no line claimed it,
	// mark the last line as having cursor at end
	if cursorPos == len(runes) {
		cursorClaimed := false
		for _, vl := range visualLines {
			if vl.isCursor {
				cursorClaimed = true
				break
			}
		}
		if !cursorClaimed && len(visualLines) > 0 {
			lastIdx := len(visualLines) - 1
			visualLines[lastIdx].isCursor = true
			visualLines[lastIdx].cursorCol = len(visualLines[lastIdx].runes)
		}
	}

	// Step 3: Render visual lines
	var result strings.Builder
	totalLines := len(visualLines)

	for i, vl := range visualLines {
		isLastLine := (i == totalLines-1)
		if i > 0 {
			result.WriteString("\n")
		}

		// Check if cursor should be shown at end of this line
		cursorAtEnd := vl.isCursor && (vl.cursorCol >= len(vl.runes) || (isLastLine && cursorPos == len(runes)))

		if vl.isCursor && vl.cursorCol >= 0 && vl.cursorCol < len(vl.runes) {
			// Cursor within this line
			before := string(vl.runes[:vl.cursorCol])
			cursorChar := string(vl.runes[vl.cursorCol])
			after := string(vl.runes[vl.cursorCol+1:])
			result.WriteString(InputTextStyle.Render(before))
			result.WriteString(CursorStyle.Render(cursorChar))
			result.WriteString(InputTextStyle.Render(after))
		} else if cursorAtEnd {
			// Cursor at end of this line (including end of last line)
			result.WriteString(InputTextStyle.Render(string(vl.runes)))
			result.WriteString(CursorStyle.Render(" "))
		} else {
			// No cursor
			result.WriteString(InputTextStyle.Render(string(vl.runes)))
		}
	}

	return result.String()
}
