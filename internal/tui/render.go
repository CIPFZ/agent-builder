package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"myclaw/internal/model"
)

const (
	defaultRenderWidth = 120
	minRenderWidth     = 60
)

type renderSnapshot struct {
	Width       int
	Height      int
	Title       string
	Subtitle    string
	Transcript  []transcriptEntry
	Input       inputRenderState
	Viewport    viewportRenderState
	Approval    *approvalRenderState
	Dialog      *dialogRenderState
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

type viewportRenderState struct {
	ScrollOffset   int
	TranscriptMode bool
	ShowAllHistory bool
	NewMessages    int
	Search         transcriptSearchRenderState
}

type transcriptSearchRenderState struct {
	Active        bool
	Query         string
	MatchCount    int
	SelectedIndex int
}

type approvalRenderState struct {
	ToolName      string
	ToolInput     string
	Reason        string
	Category      string
	RuleSource    string
	SelectedIndex int
}

type dialogRenderState struct {
	Title         string
	Subtitle      string
	Items         []dialogItem
	SelectedIndex int
	EmptyText     string
	FooterHint    string
	Query         string
	QueryEnabled  bool
	MatchCount    int
	VisibleFrom   int
	VisibleTotal  int
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
		Height:     m.height,
		Title:      "myclaw",
		Subtitle:   "MiniMax-M2.7 Agent Builder",
		Transcript: transcript,
		Input: inputRenderState{
			Text:          m.input,
			Cursor:        m.cursorPos,
			Suggestions:   suggestions,
			SelectedIndex: m.selectedIndex,
		},
		Viewport: viewportRenderState{
			ScrollOffset:   m.viewport.ScrollOffset,
			TranscriptMode: m.viewport.TranscriptMode,
			ShowAllHistory: m.viewport.ShowAllHistory,
			NewMessages:    m.viewport.NewMessages,
			Search: transcriptSearchRenderState{
				Active:        m.viewport.Search.Active,
				Query:         m.viewport.Search.Query,
				MatchCount:    m.viewport.Search.MatchCount,
				SelectedIndex: m.viewport.Search.SelectedIndex,
			},
		},
		Busy:        m.busy,
		Activity:    m.activity.Label,
		Diagnostics: m.diagnostics,
	}
	if m.approvalDialog.active() && m.approvalDialog.Request != nil {
		snapshot.Approval = &approvalRenderState{
			ToolName:      m.approvalDialog.Request.ToolName,
			ToolInput:     m.approvalDialog.Request.ToolInput,
			Reason:        m.approvalDialog.Request.Reason,
			Category:      m.approvalDialog.Request.Category,
			RuleSource:    m.approvalDialog.Request.RuleSource,
			SelectedIndex: m.approvalDialog.SelectedIndex,
		}
	}
	if snapshot.Approval == nil && m.dialog.active() {
		items := m.dialog.Picker.VisibleItems()
		snapshot.Dialog = &dialogRenderState{
			Title:         m.dialog.Title,
			Subtitle:      m.dialog.Subtitle,
			Items:         items,
			SelectedIndex: m.dialog.Picker.SelectedIndex - m.dialog.Picker.VisibleFromIndex,
			EmptyText:     m.dialog.EmptyText,
			FooterHint:    m.dialog.FooterHint,
			Query:         m.dialog.Picker.Query,
			QueryEnabled:  m.dialog.Picker.QueryEnabled,
			MatchCount:    m.dialog.Picker.MatchCount(),
			VisibleFrom:   m.dialog.Picker.VisibleFromIndex,
			VisibleTotal:  len(m.dialog.Picker.filteredItems()),
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
	b.WriteString(r.renderPrompt(snapshot))
	if snapshot.Approval != nil {
		b.WriteString(r.renderApprovalDialog(snapshot))
	}
	if snapshot.Dialog != nil {
		b.WriteString(r.renderDialog(snapshot))
	}
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
		if statusLine := snapshot.viewportStatusLine(width, 0); statusLine != "" {
			b.WriteString(statusLine)
			b.WriteString("\n")
		}
		return b.String()
	}
	lines := r.renderTranscriptLines(snapshot)
	visibleLines := snapshot.transcriptVisibleLines()
	if visibleLines > 0 && len(lines) > visibleLines {
		effectiveOffset := clampViewportOffset(len(lines), visibleLines, snapshot.Viewport.ScrollOffset)
		statusLine := snapshot.viewportStatusLine(width, effectiveOffset)
		if statusLine != "" && visibleLines > 1 {
			visibleLines--
			effectiveOffset = clampViewportOffset(len(lines), visibleLines, snapshot.Viewport.ScrollOffset)
			statusLine = snapshot.viewportStatusLine(width, effectiveOffset)
		}
		lines = sliceTranscriptViewport(lines, visibleLines, effectiveOffset)
		for _, line := range lines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		if statusLine != "" {
			b.WriteString(statusLine)
			b.WriteString("\n")
		}
		return b.String()
	}
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if statusLine := snapshot.viewportStatusLine(width, 0); statusLine != "" {
		b.WriteString(statusLine)
		b.WriteString("\n")
	}
	return b.String()
}

func (r renderer) renderTranscriptLines(snapshot renderSnapshot) []string {
	width := snapshot.Width
	var b strings.Builder
	for _, entry := range snapshot.Transcript {
		switch {
		case entry.Kind != "":
			r.renderSpecialBlock(&b, entry, width)
		case entry.Role == "assistant" && len(entry.Blocks) > 0:
			r.renderAssistantBlocks(&b, entry, width)
		case entry.Role == "user":
			r.renderRoleBlock(&b, "user", entry.Content, entry.Streaming, width)
		case entry.Role == "assistant":
			r.renderRoleBlock(&b, "assistant", entry.Content, entry.Streaming, width)
		case entry.Role == "tool":
			r.renderToolBlock(&b, entry, width)
		default:
			r.renderRoleBlock(&b, entry.Role, entry.Content, entry.Streaming, width)
		}
	}
	output := strings.TrimSuffix(b.String(), "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func (r renderer) renderSpecialBlock(b *strings.Builder, entry transcriptEntry, width int) {
	switch entry.Kind {
	case messageKindError:
		r.renderLabeledBlock(b, "error", entry.Content, width)
	case messageKindCompact:
		content := entry.Content
		if strings.TrimSpace(content) == "" {
			content = "Conversation compacted"
		}
		r.renderLabeledBlock(b, "compact", content, width)
	case messageKindLocalCommand:
		b.WriteString("local command\n")
		if entry.LocalStdout != "" {
			r.renderIndentedBlock(b, "stdout", entry.LocalStdout, width)
		}
		if entry.LocalStderr != "" {
			r.renderIndentedBlock(b, "stderr", entry.LocalStderr, width)
		}
	case messageKindSystem:
		r.renderLabeledBlock(b, "system", entry.Content, width)
	default:
		r.renderRoleBlock(b, entry.Role, entry.Content, entry.Streaming, width)
	}
}

func (r renderer) renderAssistantBlocks(b *strings.Builder, entry transcriptEntry, width int) {
	b.WriteString("assistant\n")
	for _, block := range entry.Blocks {
		switch block.Type {
		case model.MessageBlockThinking:
			r.renderIndentedBlock(b, "thinking", block.Text, width)
		case model.MessageBlockText:
			r.renderIndentedBlock(b, "text", block.Text, width)
		case model.MessageBlockToolUse:
			name := block.Name
			if name == "" {
				name = "(unnamed tool)"
			}
			summary := messageBlockInputSummary(block)
			if summary != "" {
				name += ": " + summary
			}
			r.renderIndentedBlock(b, "tool use", name, width)
		case model.MessageBlockToolResult:
			label := "tool result"
			if block.IsError {
				label = "tool error"
			}
			r.renderIndentedBlock(b, label, block.Content, width)
		default:
			r.renderIndentedBlock(b, string(block.Type), messageBlockFallbackContent(block), width)
		}
	}
}

func (r renderer) renderLabeledBlock(b *strings.Builder, label, content string, width int) {
	b.WriteString(label)
	b.WriteString("\n")
	r.renderIndentedBlock(b, "", content, width)
}

func (r renderer) renderIndentedBlock(b *strings.Builder, label, content string, width int) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "(empty)"
	}
	prefix := "  "
	if label != "" {
		prefix += label + ": "
	}
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

func messageBlockInputSummary(block model.MessageBlock) string {
	if len(block.InputObject) > 0 {
		parts := make([]string, 0, len(block.InputObject))
		for key, value := range block.InputObject {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
		sort.Strings(parts)
		return strings.Join(parts, ", ")
	}
	return block.Input
}

func messageBlockFallbackContent(block model.MessageBlock) string {
	for _, value := range []string{block.Text, block.Content, block.Name, block.Input} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	if len(block.Raw) == 0 {
		return "(empty)"
	}
	parts := make([]string, 0, len(block.Raw))
	for key, value := range block.Raw {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func (r renderer) renderApprovalDialog(snapshot renderSnapshot) string {
	approval := snapshot.Approval
	if approval == nil {
		return ""
	}
	width := snapshot.Width
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}
	var b strings.Builder
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	b.WriteString("  Permission Required")
	if approval.ToolName != "" || approval.ToolInput != "" {
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(approval.ToolName + " " + approval.ToolInput))
	}
	b.WriteString("\n")
	if approval.Reason != "" {
		for _, line := range wrapCells("Reason: "+approval.Reason, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if approval.Category != "" {
		for _, line := range wrapCells("Category: "+approval.Category, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if approval.RuleSource != "" {
		for _, line := range wrapCells("Rule: "+approval.RuleSource, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	options := []dialogItem{
		{Label: "Approve once", Description: "Run this tool call"},
		{Label: "Reject", Description: "Cancel this tool call"},
	}
	for i, option := range options {
		prefix := "  "
		if i == approval.SelectedIndex {
			prefix = "> "
		}
		line := option.Label
		if option.Description != "" {
			line += " - " + option.Description
		}
		for j, wrapped := range wrapCells(line, innerWidth) {
			if j == 0 {
				b.WriteString(prefix)
			} else {
				b.WriteString("  ")
			}
			b.WriteString(wrapped)
			b.WriteString("\n")
		}
	}
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	b.WriteString(rightAlign("Enter select  |  Esc reject  |  Ctrl+Y approve  |  Ctrl+N reject", width))
	b.WriteString("\n")
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
	if snapshot.Dialog == nil && len(snapshot.Input.Suggestions) > 0 {
		b.WriteString(r.renderSuggestions(snapshot))
	}
	b.WriteString(borderLine(width, '-'))
	b.WriteString("\n")
	help := "Enter to send  |  Up/Down history  |  / for commands"
	if snapshot.Approval != nil {
		help = "Enter select  |  Esc reject"
	}
	if snapshot.Dialog != nil {
		help = "Enter select  |  Esc close"
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

func (r renderer) renderDialog(snapshot renderSnapshot) string {
	dialog := snapshot.Dialog
	if dialog == nil {
		return ""
	}
	width := snapshot.Width
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}
	var b strings.Builder
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(truncateCells(dialog.Title, innerWidth))
	b.WriteString("\n")
	if dialog.Subtitle != "" {
		for _, line := range wrapCells(dialog.Subtitle, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	if dialog.QueryEnabled {
		b.WriteString("  Search: ")
		if dialog.Query == "" {
			b.WriteString("(type to filter)")
		} else {
			b.WriteString(truncateCells(dialog.Query, innerWidth-8))
		}
		b.WriteString("\n")
	}
	if len(dialog.Items) == 0 {
		empty := dialog.EmptyText
		if empty == "" {
			empty = "(empty)"
		}
		b.WriteString("  ")
		b.WriteString(empty)
		b.WriteString("\n")
	} else {
		for i, item := range dialog.Items {
			prefix := "  "
			if i == dialog.SelectedIndex {
				prefix = "> "
			}
			line := item.Label
			if item.Disabled {
				line += " (disabled)"
			}
			if item.Description != "" {
				line += " - " + item.Description
			}
			for j, wrapped := range wrapCells(line, innerWidth) {
				if j == 0 {
					b.WriteString(prefix)
				} else {
					b.WriteString("  ")
				}
				b.WriteString(wrapped)
				b.WriteString("\n")
			}
		}
	}
	hint := dialog.FooterHint
	if hint == "" {
		hint = "Enter select  |  Esc close"
	}
	if dialog.QueryEnabled {
		matchLabel := fmt.Sprintf("%d matches", dialog.MatchCount)
		if dialog.MatchCount == 1 {
			matchLabel = "1 match"
		}
		b.WriteString("  ")
		b.WriteString(matchLabel)
		if dialog.VisibleTotal > len(dialog.Items) {
			b.WriteString(fmt.Sprintf(" · showing %d-%d", dialog.VisibleFrom+1, dialog.VisibleFrom+len(dialog.Items)))
		}
		b.WriteString("\n")
	}
	b.WriteString(borderLine(width, '#'))
	b.WriteString("\n")
	b.WriteString(rightAlign(hint, width))
	b.WriteString("\n")
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
	status := toolStatusRunning
	if entry.ToolStatus != "" {
		status = entry.ToolStatus
	}
	head := fmt.Sprintf("tool[%s]", status)
	if entry.ToolName != "" {
		head += " " + entry.ToolName
	}
	if input := toolInputSummary(entry); input != "" {
		head += ": " + input
	}
	b.WriteString(head)
	b.WriteString("\n")
	if entry.ToolProgressMessage != "" && status == toolStatusRunning {
		for _, line := range wrapCells(entry.ToolProgressMessage, width-2) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if entry.ToolProgressOutput != "" && status == toolStatusRunning {
		for _, line := range tailNonEmptyLines(entry.ToolProgressOutput, 5) {
			for _, wrapped := range wrapCells(line, width-2) {
				b.WriteString("  ")
				b.WriteString(wrapped)
				b.WriteString("\n")
			}
		}
	}
	if entry.Content != "" && !isRunningPlaceholder(entry) {
		label := entry.Content
		if entry.ToolError {
			label = "Error: " + label
		}
		for _, line := range wrapCells(label, width-2) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func toolInputSummary(entry transcriptEntry) string {
	if entry.ToolInput != "" {
		return entry.ToolInput
	}
	if entry.ToolInputObject == nil {
		return ""
	}
	parts := make([]string, 0, len(entry.ToolInputObject))
	for key, value := range entry.ToolInputObject {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func isRunningPlaceholder(entry transcriptEntry) bool {
	return entry.Content == "Running "+entry.ToolName+"..." || entry.Content == "Calling "+entry.ToolName+"..."
}

func tailNonEmptyLines(text string, maxLines int) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	if maxLines > 0 && len(filtered) > maxLines {
		return filtered[len(filtered)-maxLines:]
	}
	return filtered
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

func (snapshot renderSnapshot) transcriptVisibleLines() int {
	if snapshot.Height <= 0 || snapshot.Viewport.ShowAllHistory {
		return 0
	}
	promptLines := 4
	if snapshot.Dialog == nil && len(snapshot.Input.Suggestions) > 0 {
		promptLines += len(snapshot.Input.Suggestions)
	}
	const headerLines = 8
	visible := snapshot.Height - headerLines - promptLines
	if visible < 3 {
		return 3
	}
	return visible
}

func (snapshot renderSnapshot) viewportStatusLine(width int, effectiveOffset int) string {
	var parts []string
	if snapshot.Viewport.Search.Active {
		parts = append(parts, snapshot.transcriptSearchStatus())
	}
	if snapshot.Viewport.TranscriptMode {
		parts = append(parts, "Transcript")
	}
	if effectiveOffset > 0 {
		parts = append(parts, fmt.Sprintf("scrolled %d lines up", effectiveOffset))
	}
	if snapshot.Viewport.NewMessages > 0 {
		label := "message"
		if snapshot.Viewport.NewMessages > 1 {
			label = "messages"
		}
		parts = append(parts, fmt.Sprintf("%d new %s", snapshot.Viewport.NewMessages, label))
	}
	if len(parts) == 0 {
		return ""
	}
	return rightAlign(strings.Join(parts, "  |  "), width)
}

func (snapshot renderSnapshot) transcriptSearchStatus() string {
	query := snapshot.Viewport.Search.Query
	if snapshot.Viewport.Search.MatchCount == 0 {
		return fmt.Sprintf("Search: %s 0/0", query)
	}
	return fmt.Sprintf(
		"Search: %s %d/%d",
		query,
		snapshot.Viewport.Search.SelectedIndex+1,
		snapshot.Viewport.Search.MatchCount,
	)
}

func sliceTranscriptViewport(lines []string, visibleLines, scrollOffset int) []string {
	if visibleLines <= 0 || len(lines) <= visibleLines {
		return lines
	}
	scrollOffset = clampViewportOffset(len(lines), visibleLines, scrollOffset)
	end := len(lines) - scrollOffset
	start := end - visibleLines
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

func clampViewportOffset(totalLines, visibleLines, scrollOffset int) int {
	if visibleLines <= 0 || totalLines <= visibleLines {
		return 0
	}
	maxOffset := totalLines - visibleLines
	if scrollOffset > maxOffset {
		return maxOffset
	}
	if scrollOffset < 0 {
		return 0
	}
	return scrollOffset
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
