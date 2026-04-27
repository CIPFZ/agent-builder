package tui

import (
	"fmt"
	"strings"

	"myclaw/internal/model"
)

type tuiState struct {
	store *clientStore

	transcript []transcriptEntry
	events     []string
	inputState
	dialog              dialogState
	taskPanel           taskPanelSnapshot
	approvalDialog      approvalDialogState
	lastDialogSelection *dialogItem
	busy                bool
	pendingApproval     *clientApproval
	diagnostics         diagnosticsState
	activity            activityState
	viewport            viewportState
	messageActions      messageActionsState
	toolExpansion       toolExpansionState
	pastes              pasteState
	promptStash         promptStashState
	externalEditor      externalEditorState
	quickOpen           quickOpenState
	globalSearch        globalSearchState
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
		pastes:         newPasteState(),
		viewport: viewportState{
			StickyBottom: true,
		},
	}
	if len(cfg) > 0 {
		state.diagnostics.SessionID = cfg[0].SessionID
		state.diagnostics.LLMLabel = cfg[0].LLMLabel
		state.diagnostics.LogPath = cfg[0].LogPath
		state.externalEditor.PromptEditor = cfg[0].PromptEditor
		state.quickOpen.OpenFile = cfg[0].OpenFile
		state.quickOpen.WorkspaceRoots = append([]string(nil), cfg[0].WorkspaceRoots...)
		state.globalSearch.OpenFileAtLine = cfg[0].OpenFileAtLine
		state.globalSearch.Search = cfg[0].WorkspaceSearch
		state.globalSearch.WorkspaceRoots = append([]string(nil), cfg[0].WorkspaceRoots...)
	}
	return state
}

func (s *tuiState) bindStore(store *clientStore) {
	if store == nil {
		return
	}
	s.store = store
	s.applyStoreSnapshot(store.snapshot())
}

func (s *tuiState) setSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *tuiState) submitUserInput() (string, bool) {
	s.clearSuggestions()
	displayText := strings.TrimSpace(s.input)
	if displayText == "" {
		return "", false
	}
	sendText := s.pastes.expandReferences(displayText)
	if len(s.history) == 0 || s.history[len(s.history)-1] != displayText {
		s.history = append(s.history, displayText)
	}
	s.historyIndex = -1
	s.cursorPos = 0
	s.input = ""
	s.pastes = newPasteState()
	if s.store != nil {
		s.applyStoreSnapshot(s.store.appendUserMessage(displayText))
	} else {
		s.transcript = append(s.transcript, transcriptEntry{Role: "user", Content: displayText})
		s.busy = true
	}
	s.scrollTranscriptBottom()
	return sendText, true
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
	s.pastes = newPasteState()
	s.promptStash = promptStashState{}
	s.externalEditor.Active = false
	s.externalEditor.PendingCtrlX = false
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

func (s *tuiState) clearPendingApproval() {
	s.pendingApproval = nil
	s.approvalDialog.close()
}

func (s *tuiState) applyBridgeError(err error) {
	if s.store != nil {
		s.applyStoreSnapshot(s.store.applyBridgeError(err))
		return
	}
	if err == nil {
		return
	}
	s.clearPendingApproval()
	s.events = append(s.events, "error: "+err.Error())
	s.busy = false
	s.diagnostics.LastError = err.Error()
}

func (s *tuiState) applyRuntimeEvent(event clientEvent) {
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
		message := ""
		if event.Message != nil {
			message = event.Message.Content
		}
		if message == "" && event.Tool != nil {
			message = event.Tool.ProgressMessage
		}
		if lastFinalAssistantAlreadyContains(s.transcript, message) {
			break
		}
		if len(s.transcript) == 0 || s.transcript[len(s.transcript)-1].Role != "assistant" || !s.transcript[len(s.transcript)-1].Streaming {
			s.transcript = append(s.transcript, transcriptEntry{Role: "assistant", Content: message, Streaming: true})
		} else {
			s.transcript[len(s.transcript)-1].Content += message
		}
	case "message.created":
		if event.Message != nil {
			if entry, ok := specialTranscriptEntryFromClientMessage(*event.Message); ok {
				s.transcript = append(s.transcript, entry)
				return
			}
			if event.Message.Role == "user" {
				if !transcriptHasMessageID(s.transcript, event.Message.ID) && !lastTranscriptMessageMatches(s.transcript, "user", event.Message.Content) {
					s.transcript = append(s.transcript, transcriptEntry{MessageID: event.Message.ID, Role: "user", Content: event.Message.Content, Blocks: cloneClientMessageBlocks(event.Message.Blocks)})
				}
			}
			if event.Message.Role == "assistant" {
				if len(s.transcript) > 0 && s.transcript[len(s.transcript)-1].Role == "assistant" && s.transcript[len(s.transcript)-1].Streaming {
					s.transcript[len(s.transcript)-1].MessageID = event.Message.ID
					s.transcript[len(s.transcript)-1].Content = event.Message.Content
					s.transcript[len(s.transcript)-1].Streaming = false
					s.transcript[len(s.transcript)-1].Blocks = cloneClientMessageBlocks(event.Message.Blocks)
				} else if !transcriptHasMessageID(s.transcript, event.Message.ID) && !transcriptMessageExistsSinceLastUser(s.transcript, "assistant", event.Message.Content) {
					s.transcript = append(s.transcript, transcriptEntry{MessageID: event.Message.ID, Role: "assistant", Content: event.Message.Content, Blocks: cloneClientMessageBlocks(event.Message.Blocks)})
				}
				s.busy = false
			}
			if event.Message.Role == "tool" {
				if entry, ok := transcriptEntryFromClientMessage(*event.Message); ok {
					s.transcript = append(s.transcript, entry)
				} else {
					s.transcript = append(s.transcript, transcriptEntry{Role: "tool", Content: event.Message.Content})
				}
			}
			if event.Message.Role == "system" {
				s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindSystem, Role: "system", Content: event.Message.Content})
			}
		}
	case "tool.called":
		s.applyToolCalled(event)
	case "tool.progress":
		s.applyToolProgress(event.Tool)
	case "tool.result":
		s.activity.Label = "Tool finished: " + event.Tool.ToolName
		s.applyToolResult(event)
	case "permission.required":
		if event.Tool != nil {
			s.pendingApproval = event.Tool.Approval
		}
		s.dialog.close()
		s.approvalDialog.open(s.pendingApproval)
		if s.pendingApproval != nil {
			s.activity.Label = "Awaiting approval: " + s.pendingApproval.ToolName + " " + s.pendingApproval.ToolInput
		}
		s.busy = false
	case "approval.updated":
		s.clearPendingApproval()
	case "run.error":
		s.clearPendingApproval()
		if event.Error != "" {
			s.diagnostics.LastError = event.Error
			s.activity.Label = "Run error"
			if !lastTranscriptSpecialMessageMatches(s.transcript, messageKindError, event.Error) {
				s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindError, Role: "system", Content: event.Error})
			}
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

func (s *tuiState) appendContextOutput(snapshot contextSnapshot) {
	lines := []string{"Context Usage"}
	if snapshot.Model != "" {
		lines = append(lines, snapshot.Model)
	}
	if snapshot.ContextWindowTokens > 0 {
		lines = append(lines, fmt.Sprintf("%d / %d tokens (%d%%)", snapshot.UsedTokens, snapshot.ContextWindowTokens, snapshot.UsagePercent))
	} else {
		lines = append(lines, fmt.Sprintf("%d tokens used", snapshot.UsedTokens))
	}
	lines = append(lines, snapshot.CategoryLines...)
	s.transcript = append(s.transcript, transcriptEntry{
		Kind:    messageKindContext,
		Role:    "system",
		Content: strings.Join(lines, "\n"),
	})
	s.noteTranscriptAppended()
}

func (s *tuiState) appendCommandOutput(title string, lines []string) {
	content := commandOutputContent(title, lines)
	if content == "" {
		return
	}
	s.dialog.close()
	s.messageActions.close()
	s.transcript = append(s.transcript, transcriptEntry{
		Kind:    messageKindContext,
		Role:    "system",
		Content: content,
	})
	s.noteTranscriptAppended()
}

func commandOutputContent(title string, lines []string) string {
	output := make([]string, 0, len(lines)+1)
	if strings.TrimSpace(title) != "" {
		output = append(output, strings.TrimSpace(title))
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			output = append(output, line)
		}
	}
	return strings.Join(output, "\n")
}

func appendBoundedEvent(events []string, event string, limit int) []string {
	events = append(events, event)
	if limit > 0 && len(events) > limit {
		return append([]string(nil), events[len(events)-limit:]...)
	}
	return events
}

func specialTranscriptEntryFromClientMessage(message clientMessage) (transcriptEntry, bool) {
	if message.Role == "summary" {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "Conversation compacted into summary"
		}
		return transcriptEntry{Kind: messageKindCompact, Role: message.Role, Content: content}, true
	}
	if message.Role == "system" && message.Content == "[compact_boundary]" {
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

func cloneClientMessageBlocks(blocks []clientMessageBlock) []model.MessageBlock {
	if len(blocks) == 0 {
		return nil
	}
	cloned := make([]model.MessageBlock, 0, len(blocks))
	for i := range cloned {
		_ = i
	}
	for _, block := range blocks {
		cloned = append(cloned, model.MessageBlock{
			Type:        model.MessageBlockType(block.Type),
			ID:          block.ID,
			ToolUseID:   block.ToolUseID,
			Text:        block.Text,
			Name:        block.Name,
			Input:       block.Input,
			InputObject: cloneAnyMap(block.InputObject),
			Content:     block.Content,
			IsError:     block.IsError,
		})
	}
	return cloned
}

func (s *tuiState) applyStoreSnapshot(snapshot clientStoreSnapshot) {
	s.transcript = snapshot.Transcript
	s.events = snapshot.Events
	s.diagnostics = snapshot.Diagnostics
	s.activity = snapshot.Activity
	s.busy = snapshot.Busy
	s.pendingApproval = snapshot.Approval
}
