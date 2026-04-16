package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func (s *tuiState) applyToolCalled(event runtime.RuntimeEvent) {
	entry := transcriptEntry{
		Role:            "tool",
		ToolUseID:       toolEventID(event),
		ToolName:        event.ToolName,
		ToolInput:       event.ToolInput,
		ToolInputObject: cloneAnyMap(event.ToolInputObject),
		ToolStatus:      toolStatusRunning,
		Content:         "Running " + event.ToolName + "...",
	}
	s.activity.Label = "Running tool: " + strings.TrimSpace(event.ToolName+" "+event.ToolInput)
	if index := s.findToolEntryIndex(entry.ToolUseID, entry.ToolName, entry.ToolInput, true); index >= 0 {
		s.transcript[index] = entry
		return
	}
	s.transcript = append(s.transcript, entry)
}

func (s *tuiState) applyToolProgress(progress *tools.ToolProgress) {
	if progress == nil {
		return
	}
	index := s.findToolEntryIndex(progress.ToolUseID, "", "", true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:                "tool",
			ToolUseID:           strings.TrimSpace(progress.ToolUseID),
			ToolStatus:          toolStatusRunning,
			ToolProgressType:    progress.Type,
			ToolProgressMessage: progress.Message,
			ToolProgressOutput:  progressOutput(progress),
		})
		return
	}
	entry := &s.transcript[index]
	entry.ToolProgressType = progress.Type
	entry.ToolProgressMessage = progress.Message
	if output := progressOutput(progress); output != "" {
		entry.ToolProgressOutput = output
	}
	if entry.ToolStatus == "" || entry.ToolStatus == "called" {
		entry.ToolStatus = toolStatusRunning
	}
}

func (s *tuiState) applyToolResult(event runtime.RuntimeEvent) {
	toolUseID, resultContent, isError := toolResultFromEvent(event)
	if toolUseID == "" {
		toolUseID = toolEventID(event)
	}
	status := toolStatusSucceeded
	if isError {
		status = toolStatusFailed
	}
	index := s.findToolEntryIndex(toolUseID, event.ToolName, event.ToolInput, true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:            "tool",
			ToolUseID:       toolUseID,
			ToolName:        event.ToolName,
			ToolInput:       event.ToolInput,
			ToolInputObject: cloneAnyMap(event.ToolInputObject),
			ToolStatus:      status,
			ToolError:       isError,
			Content:         resultContent,
		})
		return
	}
	entry := &s.transcript[index]
	if event.ToolName != "" {
		entry.ToolName = event.ToolName
	}
	if event.ToolInput != "" {
		entry.ToolInput = event.ToolInput
	}
	if event.ToolInputObject != nil {
		entry.ToolInputObject = cloneAnyMap(event.ToolInputObject)
	}
	if toolUseID != "" {
		entry.ToolUseID = toolUseID
	}
	entry.ToolStatus = status
	entry.ToolError = isError
	entry.Content = resultContent
}

func (s *tuiState) findToolEntryIndex(toolUseID, toolName, toolInput string, runningOnly bool) int {
	toolUseID = strings.TrimSpace(toolUseID)
	if toolUseID != "" {
		for i := len(s.transcript) - 1; i >= 0; i-- {
			entry := s.transcript[i]
			if entry.Role == "tool" && entry.ToolUseID == toolUseID && (!runningOnly || isRunningToolStatus(entry.ToolStatus)) {
				return i
			}
		}
	}
	for i := len(s.transcript) - 1; i >= 0; i-- {
		entry := s.transcript[i]
		if entry.Role != "tool" || (runningOnly && !isRunningToolStatus(entry.ToolStatus)) {
			continue
		}
		if toolName != "" && entry.ToolName != toolName {
			continue
		}
		if toolInput != "" && entry.ToolInput != toolInput {
			continue
		}
		return i
	}
	return -1
}

func isRunningToolStatus(status string) bool {
	return status == "" || status == "called" || status == toolStatusRunning
}

func toolEventID(event runtime.RuntimeEvent) string {
	if event.ToolUseID != "" {
		return event.ToolUseID
	}
	if event.Progress != nil && event.Progress.ToolUseID != "" {
		return event.Progress.ToolUseID
	}
	return ""
}

func toolResultFromEvent(event runtime.RuntimeEvent) (string, string, bool) {
	toolUseID := strings.TrimSpace(event.ToolUseID)
	content := ""
	isError := event.ToolError
	if event.Message != nil {
		blockID, blockContent, blockError := toolResultFromMessage(event.Message, toolUseID)
		if toolUseID == "" {
			toolUseID = blockID
		}
		if blockContent != "" {
			content = blockContent
		}
		isError = isError || blockError
		if content == "" {
			content = event.Message.Content
		}
	}
	if content == "" {
		content = "(no output)"
	}
	return toolUseID, trimToolResultPrefix(content, event.ToolName), isError
}

func toolResultFromMessage(message *session.Message, preferredToolUseID string) (string, string, bool) {
	if message == nil {
		return "", "", false
	}
	for _, block := range message.Blocks {
		if block.Type != model.MessageBlockToolResult {
			continue
		}
		if preferredToolUseID != "" && block.ToolUseID != "" && block.ToolUseID != preferredToolUseID {
			continue
		}
		return block.ToolUseID, block.Content, block.IsError
	}
	return "", "", false
}

func trimToolResultPrefix(content, toolName string) string {
	content = strings.TrimSpace(content)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return content
	}
	prefix := toolName + ":"
	if strings.HasPrefix(content, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(content, prefix))
	}
	return content
}

func progressOutput(progress *tools.ToolProgress) string {
	if progress == nil || progress.Data == nil {
		return ""
	}
	for _, key := range []string{"output", "fullOutput", "stdout", "stderr"} {
		if value, ok := progress.Data[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	if len(progress.Data) > 0 {
		encoded, err := json.Marshal(progress.Data)
		if err == nil {
			return string(encoded)
		}
	}
	return ""
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneToolProgress(input tools.ToolProgress) *tools.ToolProgress {
	cloned := input
	cloned.Data = cloneAnyMap(input.Data)
	return &cloned
}
