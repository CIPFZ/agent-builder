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
	Width          int
	Height         int
	Title          string
	Subtitle       string
	Transcript     []transcriptEntry
	Input          inputRenderState
	Viewport       viewportRenderState
	Actions        messageActionsRenderState
	ExpandedTools  map[string]bool
	ExternalEditor bool
	HasPromptStash bool
	Approval       *approvalRenderState
	Dialog         *dialogRenderState
	Busy           bool
	Activity       string
	Diagnostics    diagnosticsState
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

type messageActionsRenderState struct {
	Active        bool
	SelectedIndex int
	Actions       []messageActionRenderItem
}

type messageActionRenderItem struct {
	Key   string
	Label string
}

type approvalRenderState struct {
	ToolName        string
	ToolInput       string
	ToolInputObject map[string]any
	Reason          string
	Category        string
	RuleSource      string
	DecisionReason  string
	AcceptFeedback  string
	SelectedIndex   int
}

type dialogRenderState struct {
	Title         string
	Subtitle      string
	Items         []dialogItem
	SelectedIndex int
	EmptyText     string
	FooterHint    string
	Kind          string
	Query         string
	QueryEnabled  bool
	MatchCount    int
	VisibleFrom   int
	VisibleTotal  int
}

const (
	headerLineCount         = 5
	externalEditorLineCount = 4
)

func newRenderSnapshot(m Model, width int) renderSnapshot {
	if width <= 0 {
		width = defaultRenderWidth
	}
	if width < minRenderWidth {
		width = minRenderWidth
	}
	transcript := append([]transcriptEntry(nil), m.transcript...)
	suggestions := append([]string(nil), m.suggestions...)
	expandedTools := make(map[string]bool, len(m.toolExpansion.expanded))
	for key, value := range m.toolExpansion.expanded {
		expandedTools[key] = value
	}
	snapshot := renderSnapshot{
		Width:      width,
		Height:     m.height,
		Title:      "MYCLAW",
		Subtitle:   "Agent Builder",
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
		Actions:        newMessageActionsRenderState(m),
		ExpandedTools:  expandedTools,
		ExternalEditor: m.externalEditor.Active,
		HasPromptStash: m.promptStash.HasStash,
		Busy:           m.busy,
		Activity:       m.activity.Label,
		Diagnostics:    m.diagnostics,
	}
	if m.approvalDialog.active() && m.approvalDialog.Request != nil {
		snapshot.Approval = &approvalRenderState{
			ToolName:        m.approvalDialog.Request.ToolName,
			ToolInput:       m.approvalDialog.Request.ToolInput,
			ToolInputObject: cloneAnyMap(m.approvalDialog.Request.ToolInputObject),
			Reason:          m.approvalDialog.Request.Reason,
			Category:        m.approvalDialog.Request.Category,
			RuleSource:      m.approvalDialog.Request.RuleSource,
			DecisionReason:  m.approvalDialog.Request.DecisionReason,
			AcceptFeedback:  m.approvalDialog.Request.AcceptFeedback,
			SelectedIndex:   m.approvalDialog.SelectedIndex,
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
			Kind:          m.dialog.Kind,
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
	if snapshot.Actions.Active {
		b.WriteString(r.renderMessageActionsBar(snapshot))
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
	b.WriteString(padLine(title, fmt.Sprintf("status: %s", status), width))
	b.WriteString("\n")
	modelLine := snapshot.Diagnostics.LLMLabel
	if modelLine == "" {
		modelLine = snapshot.Subtitle
	}
	activity := snapshot.Activity
	if activity == "" {
		activity = "ready"
	}
	b.WriteString(padLine("Model: "+modelLine, "Activity: "+activity, width))
	b.WriteString("\n")
	b.WriteString(padLine("Commands: /help  /clear  /model  /context", "Scroll: wheel / PgUp / PgDn", width))
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
	for i, entry := range snapshot.Transcript {
		if snapshot.Actions.Active && i == snapshot.Actions.SelectedIndex {
			b.WriteString("> ")
		}
		switch {
		case entry.Kind != "":
			r.renderSpecialBlock(&b, entry, width)
		case entry.Role == "user" && len(entry.Blocks) > 0:
			r.renderUserBlocks(&b, entry, width)
		case entry.Role == "assistant" && len(entry.Blocks) > 0:
			r.renderAssistantBlocks(&b, entry, width)
		case entry.Role == "tool" && len(entry.Blocks) > 0:
			r.renderTranscriptToolBlocks(&b, entry, width)
		case entry.Role == "user":
			r.renderRoleBlock(&b, "user", entry.Content, entry.Streaming, width)
		case entry.Role == "assistant":
			r.renderRoleBlock(&b, "assistant", entry.Content, entry.Streaming, width)
		case entry.Role == "tool":
			r.renderToolBlock(&b, entry, width, snapshot.ExpandedTools[toolExpansionKeyForRender(i, entry)])
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

func newMessageActionsRenderState(m Model) messageActionsRenderState {
	state := messageActionsRenderState{
		Active:        m.messageActions.Active,
		SelectedIndex: m.messageActions.SelectedIndex,
	}
	if !state.Active {
		return state
	}
	entry, ok := m.selectedMessageActionEntry()
	if !ok {
		return state
	}
	if entry.Role == "user" && entry.Kind == "" {
		state.Actions = append(state.Actions, messageActionRenderItem{Key: "enter", Label: "edit"})
	} else if isExpandableToolEntry(entry) {
		label := "expand"
		if m.selectedToolExpanded() {
			label = "collapse"
		}
		state.Actions = append(state.Actions, messageActionRenderItem{Key: "enter", Label: label})
	}
	state.Actions = append(state.Actions, messageActionRenderItem{Key: "c", Label: "copy"})
	if label, _, ok := primaryToolInputOf(entry); ok {
		state.Actions = append(state.Actions, messageActionRenderItem{Key: "p", Label: "copy " + label})
	}
	return state
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
	case messageKindContext:
		r.renderContextBlock(b, entry.Content, width)
	case messageKindSystem:
		r.renderLabeledBlock(b, "system", entry.Content, width)
	default:
		r.renderRoleBlock(b, entry.Role, entry.Content, entry.Streaming, width)
	}
}

func (r renderer) renderUserBlocks(b *strings.Builder, entry transcriptEntry, width int) {
	for _, block := range entry.Blocks {
		switch {
		case isImageMessageBlock(block):
			r.renderPromptBlock(b, "image "+imageBlockSummary(block), width)
		case block.Type == model.MessageBlockText:
			r.renderPromptBlock(b, block.Text, width)
		case block.Type == model.MessageBlockToolResult:
			label := "tool result"
			if block.IsError {
				label = "tool error"
			}
			r.renderPromptBlock(b, label+": "+block.Content, width)
		default:
			r.renderPromptBlock(b, blockLabel(block)+": "+messageBlockFallbackContent(block), width)
		}
	}
}

func (r renderer) renderAssistantBlocks(b *strings.Builder, entry transcriptEntry, width int) {
	for _, block := range entry.Blocks {
		switch block.Type {
		case model.MessageBlockThinking:
			r.renderIndentedBlock(b, "thinking", block.Text, width)
		case model.MessageBlockText:
			r.renderPlainBlock(b, block.Text, width)
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

func (r renderer) renderTranscriptToolBlocks(b *strings.Builder, entry transcriptEntry, width int) {
	for _, block := range entry.Blocks {
		if block.Type != model.MessageBlockToolResult {
			r.renderIndentedBlock(b, blockLabel(block), messageBlockFallbackContent(block), width)
			continue
		}
		label := "tool result"
		if block.IsError {
			label = "tool error"
		}
		r.renderLabeledBlock(b, label, block.Content, width)
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
	if isImageMessageBlock(block) {
		return imageBlockSummary(block)
	}
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

func blockLabel(block model.MessageBlock) string {
	if isImageMessageBlock(block) {
		return "image"
	}
	if strings.TrimSpace(string(block.Type)) != "" {
		return strings.ReplaceAll(string(block.Type), "_", " ")
	}
	return "block"
}

func isImageMessageBlock(block model.MessageBlock) bool {
	return block.Type == "image" || rawBlockType(block) == "image"
}

func rawBlockType(block model.MessageBlock) string {
	if block.Raw == nil {
		return ""
	}
	if value, ok := block.Raw["type"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func imageBlockSummary(block model.MessageBlock) string {
	source, ok := block.Raw["source"].(map[string]any)
	if !ok {
		return "[Image]"
	}
	if mediaType, ok := source["media_type"].(string); ok && strings.TrimSpace(mediaType) != "" {
		return "[Image: " + strings.TrimSpace(mediaType) + "]"
	}
	return "[Image]"
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
	semantic := describeApprovalSemantics(*approval)
	b.WriteString("  ")
	b.WriteString(semantic.Title)
	b.WriteString("\n")
	for _, detail := range semantic.Details {
		for _, line := range wrapCells(detail, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
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
	if approval.DecisionReason != "" {
		for _, line := range wrapCells("Decision: "+approval.DecisionReason, innerWidth) {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if approval.AcceptFeedback != "" {
		for _, line := range wrapCells("Feedback: "+approval.AcceptFeedback, innerWidth) {
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
	if snapshot.ExternalEditor {
		b.WriteString(centerLine("Save and close editor to continue...", width))
		b.WriteString("\n")
		b.WriteString(borderLine(width, '-'))
		b.WriteString("\n")
		b.WriteString(rightAlign("External editor active", width))
		b.WriteString("\n")
		return b.String()
	}
	inputLines := renderInputLinesWithCursor(snapshot.Input.Text, snapshot.Input.Cursor, width-2)
	for i, line := range inputLines {
		prefix := "> "
		if i > 0 {
			prefix = "  "
		}
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if snapshot.HasPromptStash {
		b.WriteString("  > Stashed (auto-restores after submit; Ctrl+S restores now)")
		b.WriteString("\n")
	}
	if snapshot.Dialog == nil && len(snapshot.Input.Suggestions) > 0 {
		b.WriteString(r.renderSuggestions(snapshot))
	}
	b.WriteString(borderLine(width, '-'))
	b.WriteString("\n")
	help := "Enter to send  |  Ctrl+S stash  |  Ctrl+G edit  |  Up/Down history  |  / commands"
	if snapshot.Approval != nil {
		help = "Enter select  |  Esc reject"
	}
	if snapshot.Dialog != nil {
		help = "Enter select  |  Esc close"
	}
	if snapshot.Actions.Active {
		help = "Message actions active"
	}
	b.WriteString(rightAlign(help, width))
	b.WriteString("\n")
	return b.String()
}

func (r renderer) renderMessageActionsBar(snapshot renderSnapshot) string {
	width := snapshot.Width
	var b strings.Builder
	b.WriteString(borderLine(width, '='))
	b.WriteString("\n")
	parts := make([]string, 0, len(snapshot.Actions.Actions)+2)
	for _, action := range snapshot.Actions.Actions {
		parts = append(parts, action.Key+" "+action.Label)
	}
	parts = append(parts, "Up/Down navigate", "esc back")
	line := "Message actions: " + strings.Join(parts, "  |  ")
	for _, wrapped := range wrapCells(line, width) {
		b.WriteString(wrapped)
		b.WriteString("\n")
	}
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
		if dialog.Kind == dialogKindHistorySearch && dialog.QueryEnabled && strings.TrimSpace(dialog.Query) != "" {
			empty = "No matching prompts"
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
	if role == "user" {
		r.renderPromptBlock(b, content, width)
		return
	}
	if role == "assistant" {
		r.renderPlainBlock(b, content, width)
		return
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

func (r renderer) renderPlainBlock(b *strings.Builder, content string, width int) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "(empty)"
	}
	for _, line := range wrapCells(content, width) {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (r renderer) renderPromptBlock(b *strings.Builder, content string, width int) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "(empty)"
	}
	available := width - 2
	if available < 20 {
		available = 20
	}
	lines := wrapCells(content, available)
	for i, line := range lines {
		if i == 0 {
			b.WriteString("> ")
		} else {
			b.WriteString("  ")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (r renderer) renderContextBlock(b *strings.Builder, content string, width int) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		r.renderPlainBlock(b, line, width)
	}
}

func (r renderer) renderToolBlock(b *strings.Builder, entry transcriptEntry, width int, expanded bool) {
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
		lines := tailNonEmptyLines(entry.ToolProgressOutput, collapsedToolOutputLines)
		if expanded {
			lines = nonEmptyLines(entry.ToolProgressOutput)
		}
		renderWrappedIndentedLines(b, lines, width)
		if !expanded {
			if hidden := hiddenLineCount(entry.ToolProgressOutput, collapsedToolOutputLines); hidden > 0 {
				b.WriteString(fmt.Sprintf("  ... %d more lines (enter expand)\n", hidden))
			}
		}
	}
	if entry.Content != "" && !isRunningPlaceholder(entry) {
		label := entry.Content
		if entry.ToolError {
			label = "Error: " + label
		}
		lines := nonEmptyLines(label)
		if !expanded && len(lines) > collapsedToolOutputLines {
			lines = lines[:collapsedToolOutputLines]
		}
		renderWrappedIndentedLines(b, lines, width)
		if !expanded {
			if hidden := hiddenLineCount(label, collapsedToolOutputLines); hidden > 0 {
				b.WriteString(fmt.Sprintf("  ... %d more lines (enter expand)\n", hidden))
			}
		}
	}
}

func toolExpansionKeyForRender(index int, entry transcriptEntry) string {
	if entry.ToolUseID != "" {
		return entry.ToolUseID
	}
	if entry.ToolName != "" || entry.Role == "tool" {
		return fmt.Sprintf("tool:%d", index)
	}
	return ""
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
	filtered := nonEmptyLines(text)
	if maxLines > 0 && len(filtered) > maxLines {
		return filtered[len(filtered)-maxLines:]
	}
	return filtered
}

func hiddenLineCount(text string, visibleLines int) int {
	lines := nonEmptyLines(text)
	if len(lines) <= visibleLines {
		return 0
	}
	return len(lines) - visibleLines
}

func nonEmptyLines(text string) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func renderWrappedIndentedLines(b *strings.Builder, lines []string, width int) {
	for _, line := range lines {
		for _, wrapped := range wrapCells(line, width-2) {
			b.WriteString("  ")
			b.WriteString(wrapped)
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

func (snapshot renderSnapshot) transcriptVisibleLines() int {
	if snapshot.Height <= 0 || snapshot.Viewport.ShowAllHistory {
		return 0
	}
	visible := snapshot.Height - headerLineCount - snapshot.promptVisibleLines()
	if visible < 3 {
		return 3
	}
	return visible
}

func (snapshot renderSnapshot) promptVisibleLines() int {
	if snapshot.ExternalEditor {
		return externalEditorLineCount
	}
	lines := 3 + len(renderInputLinesWithCursor(snapshot.Input.Text, snapshot.Input.Cursor, snapshot.Width-2))
	if snapshot.HasPromptStash {
		lines++
	}
	if snapshot.Dialog == nil && len(snapshot.Input.Suggestions) > 0 {
		lines += len(snapshot.Input.Suggestions)
	}
	return lines
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

func renderInputLinesWithCursor(input string, cursor int, width int) []string {
	runes := []rune(input)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	lines := buildInputVisualLines(input, width)
	rendered := make([]string, 0, len(lines))
	cursorLine := inputCursorLineIndex(lines, cursor)
	for index, line := range lines {
		lineRunes := []rune(line.Text)
		if index != cursorLine {
			rendered = append(rendered, line.Text)
			continue
		}
		cursorOffset := cursor - line.Start
		if cursorOffset < 0 {
			cursorOffset = 0
		}
		if cursorOffset > len(lineRunes) {
			cursorOffset = len(lineRunes)
		}
		var b strings.Builder
		b.WriteString(string(lineRunes[:cursorOffset]))
		if cursorOffset == len(lineRunes) {
			b.WriteString(CursorStyle.Render(" "))
		} else {
			b.WriteString(CursorStyle.Render(string(lineRunes[cursorOffset])))
			if cursorOffset+1 < len(lineRunes) {
				b.WriteString(string(lineRunes[cursorOffset+1:]))
			}
		}
		rendered = append(rendered, b.String())
	}
	if len(rendered) == 0 {
		return []string{CursorStyle.Render(" ")}
	}
	return rendered
}

func renderInputWithCursor(input string, cursor int, width int) string {
	return strings.Join(renderInputLinesWithCursor(input, cursor, width), "\n")
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
