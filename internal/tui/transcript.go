package tui

import (
	"fmt"

	"myclaw/internal/runtime"
)

func (m *Model) applyRuntimeEvent(event runtime.RuntimeEvent) {
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

func (m *Model) updateRuntimeEvent(event runtime.RuntimeEvent) {
	m.applyRuntimeEvent(event)
}
