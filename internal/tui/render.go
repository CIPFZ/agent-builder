package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#52B788")).Bold(true)

func (r renderer) renderLayout(m Model, width int) string {
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
						b.WriteString(UserMessageStyle.Render("❯ "+line) + "\n")
					} else {
						b.WriteString(UserMessageStyle.Render("  "+line) + "\n")
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
		runes    []rune
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
		runes     []rune
		startPos  int // global start position
		isCursor  bool
		cursorCol int // column within this visual line
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
