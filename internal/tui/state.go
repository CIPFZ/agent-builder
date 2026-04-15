package tui

import (
	"strings"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
)

type tuiState struct {
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

func newTUIState(cfg ...ModelConfig) tuiState {
	state := tuiState{
		transcript: make([]transcriptEntry, 0, 32),
		events:     []string{"Welcome to myclaw TUI"},
		inputState: newInputState(),
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
	s.input = ""
	s.busy = true
	return text, true
}

func (s *tuiState) clearSuggestions() {
	s.suggestions = nil
	s.selectedIndex = -1
}

func (s *tuiState) approvePending() (string, bool) {
	if s.pendingApproval == nil {
		return "", false
	}
	id := s.pendingApproval.ID
	s.pendingApproval = nil
	s.busy = true
	return id, true
}

func (s *tuiState) rejectPending() (string, bool) {
	if s.pendingApproval == nil {
		return "", false
	}
	id := s.pendingApproval.ID
	s.pendingApproval = nil
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
	s.diagnostics.LastEvent = event.Type
	s.diagnostics.EventCount++
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
			if event.Message.Role == "assistant" {
				if len(s.transcript) > 0 && s.transcript[len(s.transcript)-1].Role == "assistant" && s.transcript[len(s.transcript)-1].Streaming {
					s.transcript[len(s.transcript)-1].Content = event.Message.Content
					s.transcript[len(s.transcript)-1].Streaming = false
				} else {
					s.transcript = append(s.transcript, transcriptEntry{Role: "assistant", Content: event.Message.Content})
				}
				s.busy = false
			}
			if event.Message.Role == "tool" {
				s.transcript = append(s.transcript, transcriptEntry{Role: "tool", Content: event.Message.Content})
			}
		}
	case "tool.called":
		s.activity.Label = "Running tool: " + event.ToolName + " " + event.ToolInput
		s.transcript = append(s.transcript, transcriptEntry{Role: "tool", ToolName: event.ToolName, ToolInput: event.ToolInput, ToolStatus: "called", Content: "Calling " + event.ToolName + "..."})
	case "tool.result":
		s.activity.Label = "Tool finished: " + event.ToolName
		for i := len(s.transcript) - 1; i >= 0; i-- {
			if s.transcript[i].Role == "tool" && s.transcript[i].ToolStatus == "called" {
				s.transcript[i].ToolStatus = "result"
				if event.Message != nil {
					s.transcript[i].Content = event.Message.Content
				} else {
					s.transcript[i].Content = "(no output)"
				}
				break
			}
		}
	case "permission.required":
		s.pendingApproval = event.Approval
		if event.Approval != nil {
			s.activity.Label = "Awaiting approval: " + event.Approval.ToolName + " " + event.Approval.ToolInput
		}
		s.busy = false
	case "approval.updated":
		s.pendingApproval = nil
	case "run.error":
		if event.Error != "" {
			s.diagnostics.LastError = event.Error
			s.activity.Label = "Run error"
		}
	case "agent.lifecycle.start":
		s.busy = true
		if s.activity.Label == "" {
			s.activity.Label = "Running turn"
		}
	case "agent.lifecycle.end":
		s.busy = false
		s.activity.Label = "Idle"
	}
}
