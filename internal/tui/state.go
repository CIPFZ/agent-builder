package tui

import (
	"strings"

	"myclaw/internal/approval"
	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

type tuiState struct {
	transcript []transcriptEntry
	events     []string
	inputState
	dialog              dialogState
	approvalDialog      approvalDialogState
	lastDialogSelection *dialogItem
	busy                bool
	pendingApproval     *approval.Request
	diagnostics         diagnosticsState
	activity            activityState
	viewport            viewportState
	messageActions      messageActionsState
	toolExpansion       toolExpansionState
	width               int
	height              int
}

func newTUIState(cfg ...ModelConfig) tuiState {
	state := tuiState{
		transcript:     make([]transcriptEntry, 0, 32),
		events:         []string{"Welcome to myclaw TUI"},
		inputState:     newInputState(),
		dialog:         newDialogState(),
		approvalDialog: newApprovalDialogState(),
		toolExpansion:  newToolExpansionState(),
		viewport: viewportState{
			StickyBottom: true,
		},
	}
	if len(cfg) > 0 {
		state.diagnostics.SessionID = cfg[0].SessionID
		state.diagnostics.LLMLabel = cfg[0].LLMLabel
		state.diagnostics.LogPath = cfg[0].LogPath
	}
	return state
}

func (s *tuiState) setSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *tuiState) submitUserInput() (string, bool) {
	s.clearSuggestions()
	text := strings.TrimSpace(s.input)
	if text == "" {
		return "", false
	}
	if len(s.history) == 0 || s.history[len(s.history)-1] != text {
		s.history = append(s.history, text)
	}
	s.historyIndex = -1
	s.cursorPos = 0
	s.transcript = append(s.transcript, transcriptEntry{Role: "user", Content: text})
	s.scrollTranscriptBottom()
	s.input = ""
	s.busy = true
	return text, true
}

func (s *tuiState) clearSuggestions() {
	s.suggestions = nil
	s.selectedIndex = -1
}

func (s *tuiState) clearVisibleConversation() {
	s.transcript = s.transcript[:0]
	s.busy = false
	s.activity.Label = "Idle"
	s.pendingApproval = nil
	s.approvalDialog.close()
	s.dialog.close()
	s.messageActions.close()
	s.toolExpansion.clear()
	s.viewport.Search = transcriptSearchState{}
	s.scrollTranscriptBottom()
	s.events = []string{"conversation cleared"}
}

func (s *tuiState) approvePending() (string, bool) {
	if s.pendingApproval == nil {
		return "", false
	}
	id := s.pendingApproval.ID
	s.pendingApproval = nil
	s.approvalDialog.close()
	s.busy = true
	return id, true
}

func (s *tuiState) rejectPending() (string, bool) {
	if s.pendingApproval == nil {
		return "", false
	}
	id := s.pendingApproval.ID
	s.pendingApproval = nil
	s.approvalDialog.close()
	s.busy = false
	return id, true
}

func (s *tuiState) applyBridgeError(err error) {
	if err == nil {
		return
	}
	s.events = append(s.events, "error: "+err.Error())
	s.busy = false
	s.diagnostics.LastError = err.Error()
}

func (s *tuiState) applyRuntimeEvent(event runtime.RuntimeEvent) {
	transcriptLen := len(s.transcript)
	s.diagnostics.LastEvent = event.Type
	s.diagnostics.EventCount++
	if event.Type != "" && event.Type != "assistant.delta" {
		s.events = appendBoundedEvent(s.events, event.Type, 200)
	}
	if event.Session.ID != "" && s.diagnostics.SessionID == "" {
		s.diagnostics.SessionID = event.Session.ID
	}
	switch event.Type {
	case "assistant.delta":
		if len(s.transcript) == 0 || s.transcript[len(s.transcript)-1].Role != "assistant" || !s.transcript[len(s.transcript)-1].Streaming {
			s.transcript = append(s.transcript, transcriptEntry{Role: "assistant", Content: event.Delta, Streaming: true})
		} else {
			s.transcript[len(s.transcript)-1].Content += event.Delta
		}
	case "message.created":
		if event.Message != nil {
			if entry, ok := specialTranscriptEntryFromMessage(*event.Message); ok {
				s.transcript = append(s.transcript, entry)
				return
			}
			if event.Message.Role == "assistant" {
				if len(s.transcript) > 0 && s.transcript[len(s.transcript)-1].Role == "assistant" && s.transcript[len(s.transcript)-1].Streaming {
					s.transcript[len(s.transcript)-1].Content = event.Message.Content
					s.transcript[len(s.transcript)-1].Streaming = false
					s.transcript[len(s.transcript)-1].Blocks = cloneMessageBlocks(event.Message.Blocks)
				} else {
					s.transcript = append(s.transcript, transcriptEntry{Role: "assistant", Content: event.Message.Content, Blocks: cloneMessageBlocks(event.Message.Blocks)})
				}
				s.busy = false
			}
			if event.Message.Role == "tool" {
				s.transcript = append(s.transcript, transcriptEntry{Role: "tool", Content: event.Message.Content})
			}
			if event.Message.Role == "system" {
				s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindSystem, Role: "system", Content: event.Message.Content})
			}
		}
	case "tool.called":
		s.applyToolCalled(event)
	case "tool.progress":
		s.applyToolProgress(event.Progress)
	case "tool.result":
		s.activity.Label = "Tool finished: " + event.ToolName
		s.applyToolResult(event)
	case "permission.required":
		s.pendingApproval = event.Approval
		s.dialog.close()
		s.approvalDialog.open(event.Approval)
		if event.Approval != nil {
			s.activity.Label = "Awaiting approval: " + event.Approval.ToolName + " " + event.Approval.ToolInput
		}
		s.busy = false
	case "approval.updated":
		s.pendingApproval = nil
		s.approvalDialog.close()
	case "run.error":
		if event.Error != "" {
			s.diagnostics.LastError = event.Error
			s.activity.Label = "Run error"
			s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindError, Role: "system", Content: event.Error})
		}
	case "compact.boundary":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Conversation compacted"})
	case "compact.cleaned":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Compaction cleanup completed"})
	case "compact.memory_saved":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Session memory saved"})
	case "agent.lifecycle.start":
		s.busy = true
		if s.activity.Label == "" {
			s.activity.Label = "Running turn"
		}
	case "agent.lifecycle.end":
		s.busy = false
		s.activity.Label = "Idle"
	}
	if len(s.transcript) > transcriptLen {
		s.noteTranscriptAppended()
	}
}

func (s *tuiState) noteTranscriptAppended() {
	if s.viewport.StickyBottom || s.viewport.ScrollOffset == 0 {
		s.viewport.ScrollOffset = 0
		s.viewport.StickyBottom = true
		s.viewport.NewMessages = 0
		return
	}
	s.viewport.ScrollOffset++
	s.viewport.NewMessages++
}

func appendBoundedEvent(events []string, event string, limit int) []string {
	events = append(events, event)
	if limit > 0 && len(events) > limit {
		return append([]string(nil), events[len(events)-limit:]...)
	}
	return events
}

func specialTranscriptEntryFromMessage(message session.Message) (transcriptEntry, bool) {
	if message.IsCompactSummary {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "Conversation compacted into summary"
		}
		return transcriptEntry{Kind: messageKindCompact, Role: message.Role, Content: content}, true
	}
	if message.Role == "system" && (message.Subtype == "compact_boundary" || message.Content == "[compact_boundary]") {
		content := strings.TrimSpace(message.Content)
		if content == "" || content == "[compact_boundary]" {
			content = "Conversation compacted"
		}
		return transcriptEntry{Kind: messageKindCompact, Role: "system", Content: content}, true
	}
	stdout := extractTaggedContent(message.Content, "local-command-stdout")
	stderr := extractTaggedContent(message.Content, "local-command-stderr")
	if stdout == "" {
		stdout = extractTaggedContent(message.Content, "bash-stdout")
	}
	if stderr == "" {
		stderr = extractTaggedContent(message.Content, "bash-stderr")
	}
	if stdout != "" || stderr != "" {
		return transcriptEntry{
			Kind:        messageKindLocalCommand,
			Role:        message.Role,
			Content:     message.Content,
			LocalStdout: stdout,
			LocalStderr: stderr,
		}, true
	}
	return transcriptEntry{}, false
}

func extractTaggedContent(content, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	start := strings.Index(content, startTag)
	if start < 0 {
		return ""
	}
	start += len(startTag)
	end := strings.Index(content[start:], endTag)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(content[start : start+end])
}

func cloneMessageBlocks(blocks []model.MessageBlock) []model.MessageBlock {
	if len(blocks) == 0 {
		return nil
	}
	cloned := make([]model.MessageBlock, len(blocks))
	copy(cloned, blocks)
	for i := range cloned {
		cloned[i].InputObject = cloneAnyMap(blocks[i].InputObject)
		if blocks[i].Raw != nil {
			cloned[i].Raw = cloneAnyMap(blocks[i].Raw)
		}
	}
	return cloned
}
