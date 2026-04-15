package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	defaultRenderWidth = 120
	minRenderWidth     = 60
)

type renderSnapshot struct {
	Width       int
	Title       string
	Subtitle    string
	Transcript  []transcriptEntry
	Input       inputRenderState
	Approval    *approvalRenderState
	Busy        bool
	Activity    string
	Diagnostics diagnosticsState
}

type inputRenderState struct {
	Text          string
	Cursor        int
	Suggestions   []string
	SelectedIndex int
}

type approvalRenderState struct {
	ToolName  string
	ToolInput string
	Reason    string
}

func newRenderSnapshot(m Model, width int) renderSnapshot {
	if width <= 0 {
		width = defaultRenderWidth
	}
	if width < minRenderWidth {
		width = minRenderWidth
	}
	transcript := append([]transcriptEntry(nil), m.transcript...)
	suggestions := append([]string(nil), m.suggestions...)
	snapshot := renderSnapshot{
		Width:      width,
		Title:      "myclaw",
		Subtitle:   "MiniMax-M2.7 Agent Builder",
		Transcript: transcript,
		Input: inputRenderState{
			Text:          m.input,
			Cursor:        m.cursorPos,
			Suggestions:   suggestions,
			SelectedIndex: m.selectedIndex,
		},
		Busy:        m.busy,
		Activity:    m.activity.Label,
		Diagnostics: m.diagnostics,
	}
	if m.pendingApproval != nil {
		snapshot.Approval = &approvalRenderState{
			ToolName:  m.pendingApproval.ToolName,
			ToolInput: m.pendingApproval.ToolInput,
			Reason:    m.pendingApproval.Reason,
		}
	}
	return snapshot
}

func (r renderer) renderLayout(m Model, width int) string {
	return r.renderScreen(newRenderSnapshot(m, width))
}

func (r renderer) renderScreen(snapshot renderSnapshot) string {
	var b strings.Builder
	b.WriteString(r.renderHeader(snapshot))
	b.WriteString(r.renderTranscript(snapshot))
	if snapshot.Approval != nil {
		b.WriteString(r.renderApproval(snapshot))
	}
	b.WriteString(r.renderPrompt(snapshot))
	return b.String()
}

func (r renderer) renderHeader(snapshot renderSnapshot) string {
	width := snapshot.Width
	var b strings.Builder
	b.WriteString(borderLine(width, '='))
	b.WriteString("\n")
	title := fmt.Sprintf(" %s v0.1.0 ", snapshot.Title)
	status := "idle"
	if snapshot.Busy {
		status = "running"
	}
	b.WriteString(padLine(title+snapshot.Subtitle, fmt.Sprintf("status: %s", status), width))
	b.WriteString("\n")
	b.WriteString(padLine("Welcome back!", "Tips for getting started", width))
	b.WriteString("\n")
	b.WriteString(padLine("  __  __  __   __  ___      __      __", "/help - see available commands", width))
	b.WriteString("\n")
	b.WriteString(padLine(" /  \\/  \\/ /  / / / _ | ___/ /___  / /", "/clear - clear conversation", width))
	b.WriteString("\n")
	b.WriteString(padLine("/ /\\_/ / / /__/ / / __ |/ _  / _ \\/ _ \\", "/model - switch models", width))
	b.WriteString("\n")
	b.WriteString(padLine("\\/  \\/_/ /____/_/ /_/ |_/\\_,_/\\___/_//_/", snapshot.activityLabel(), width))
	b.WriteString("\n")
	b.WriteString(borderLine(width, '='))
	b.WriteString("\n")
	return b.String()
}

func (r renderer) renderTranscript(snapshot renderSnapshot) string {
	width := snapshot.Width
	var b strings.Builder
	b.WriteString(borderLine(width, '-'))
	b.WriteString("\n")
	if len(snapshot.Transcript) == 0 {
		b.WriteString(centerLine("(no messages yet - start a conversation!)", width))
		b.WriteString("\n")
		return b.String()
	}
	for _, entry := range snapshot.Transcript {
		switch entry.Role {
		case "user":
			r.renderRoleBlock(&b, "user", entry.Content, entry.Streaming, width)
		case "assistant":
			r.renderRoleBlock(&b, "assistant", entry.Content, entry.Streaming, width)
		case "tool":
			r.renderToolBlock(&b, entry, width)
		default:
			r.renderRoleBlock(&b, entry.Role, entry.Content, entry.Streaming, width)
		}
	}
	return b.String()
}

func (r renderer) renderApproval(snapshot renderSnapshot) string {
	approval := snapshot.Approval
	if approval == nil {
		return ""
	}
	width := snapshot.Width
	var b strings.Builder
	b.WriteString(borderLine(width, '!'))
	b.WriteString("\n")
	b.WriteString("Permission Required")
	if approval.ToolName != "" || approval.ToolInput != "" {
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(approval.ToolName + " " + approval.ToolInput))
	}
	b.WriteString("\n")
	if approval.Reason != "" {
		for _, line := range wrapCells("Reason: "+approval.Reason, width-2) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("Ctrl+Y approve | Ctrl+N reject\n")
	return b.String()
}

func (r renderer) renderPrompt(snapshot renderSnapshot) string {
	width := snapshot.Width
	var b strings.Builder
	b.WriteString(borderLine(width, '-'))
	b.WriteString("\n")
	input := renderInputWithCursor(snapshot.Input.Text, snapshot.Input.Cursor, width-3)
	b.WriteString("> ")
	b.WriteString(input)
	b.WriteString("\n")
	if len(snapshot.Input.Suggestions) > 0 {
		b.WriteString(r.renderSuggestions(snapshot))
	}
	b.WriteString(borderLine(width, '-'))
	b.WriteString("\n")
	help := "Enter to send  |  Up/Down history  |  / for commands"
	if snapshot.Approval != nil {
		help = "Ctrl+Y approve  |  Ctrl+N reject"
	}
	b.WriteString(rightAlign(help, width))
	b.WriteString("\n")
	return b.String()
}

func (r renderer) renderSuggestions(snapshot renderSnapshot) string {
	var b strings.Builder
	for i, suggestion := range snapshot.Input.Suggestions {
		prefix := "  "
		if i == snapshot.Input.SelectedIndex {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(suggestion)
		b.WriteString("\n")
	}
	return b.String()
}

func (r renderer) renderRoleBlock(b *strings.Builder, role, content string, streaming bool, width int) {
	if streaming {
		content += " ..."
	}
	prefix := role + ": "
	available := width - lipgloss.Width(prefix)
	if available < 20 {
		available = 20
	}
	lines := wrapCells(content, available)
	for i, line := range lines {
		if i == 0 {
			b.WriteString(prefix)
		} else {
			b.WriteString(strings.Repeat(" ", lipgloss.Width(prefix)))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (r renderer) renderToolBlock(b *strings.Builder, entry transcriptEntry, width int) {
	status := "called"
	if entry.ToolStatus != "" {
		status = entry.ToolStatus
	}
	head := fmt.Sprintf("tool[%s] %s", status, entry.ToolName)
	if entry.ToolInput != "" {
		head += ": " + entry.ToolInput
	}
	b.WriteString(head)
	b.WriteString("\n")
	if entry.Content != "" && entry.Content != fmt.Sprintf("Calling %s...", entry.ToolName) {
		for _, line := range wrapCells(entry.Content, width-2) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func (snapshot renderSnapshot) activityLabel() string {
	if snapshot.Activity != "" {
		return snapshot.Activity
	}
	if snapshot.Diagnostics.LLMLabel != "" {
		return snapshot.Diagnostics.LLMLabel
	}
	return ""
}

func borderLine(width int, ch rune) string {
	if width < minRenderWidth {
		width = minRenderWidth
	}
	return strings.Repeat(string(ch), width)
}

func padLine(left, right string, width int) string {
	left = truncateCells(left, width)
	right = truncateCells(right, width)
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := width - leftWidth - rightWidth
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func centerLine(text string, width int) string {
	text = truncateCells(text, width)
	pad := (width - lipgloss.Width(text)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + text
}

func rightAlign(text string, width int) string {
	text = truncateCells(text, width)
	pad := width - lipgloss.Width(text)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + text
}

func wrapCells(content string, width int) []string {
	if width < 1 {
		width = 1
	}
	var lines []string
	var current strings.Builder
	lineWidth := 0
	for _, r := range content {
		if r == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			lineWidth = 0
			continue
		}
		cellWidth := lipgloss.Width(string(r))
		if lineWidth+cellWidth > width && current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
			lineWidth = 0
		}
		current.WriteRune(r)
		lineWidth += cellWidth
	}
	if current.Len() > 0 || len(lines) == 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func truncateCells(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		cellWidth := lipgloss.Width(string(r))
		if used+cellWidth > width {
			break
		}
		b.WriteRune(r)
		used += cellWidth
	}
	return b.String()
}

func renderInputWithCursor(input string, cursor int, width int) string {
	runes := []rune(input)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if len(runes) == 0 {
		return CursorStyle.Render(" ")
	}
	before := string(runes[:cursor])
	after := string(runes[cursor:])
	if cursor == len(runes) {
		return strings.Join(wrapCells(before, width), "\n") + CursorStyle.Render(" ")
	}
	afterRunes := []rune(after)
	cursorChar := string(afterRunes[0])
	rest := ""
	if len(afterRunes) > 1 {
		rest = string(afterRunes[1:])
	}
	return strings.Join(wrapCells(before+CursorStyle.Render(cursorChar)+rest, width), "\n")
}

func (m Model) renderLayout(width int) string {
	return newRenderer().renderLayout(m, width)
}

func (m Model) renderHeader(width int) string {
	return newRenderer().renderHeader(newRenderSnapshot(m, width))
}

func (m Model) renderMessages(width int) string {
	return newRenderer().renderTranscript(newRenderSnapshot(m, width))
}

func (m Model) renderFooter(width int) string {
	return newRenderer().renderPrompt(newRenderSnapshot(m, width))
}

func (m Model) wrapMessageContent(content string, width int) []string {
	return wrapCells(content, width)
}

func (m Model) buildInputWithCursor(width int) string {
	return renderInputWithCursor(m.input, m.cursorPos, width-2)
}

func (m Model) renderMultiLineInput(runes []rune, cursorPos int, width int) string {
	return renderInputWithCursor(string(runes), cursorPos, width)
}
