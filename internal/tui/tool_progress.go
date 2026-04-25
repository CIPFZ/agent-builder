package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"myclaw/internal/model"
)

func (s *tuiState) applyToolCalled(event clientEvent) {
	if event.Tool == nil {
		return
	}
	entry := transcriptEntry{
		Role:            "tool",
		ToolUseID:       toolEventID(event),
		ToolName:        event.Tool.ToolName,
		ToolInput:       event.Tool.ToolInput,
		ToolInputObject: cloneAnyMap(event.Tool.ToolInputObject),
		ToolStatus:      toolStatusRunning,
		Content:         "Running " + event.Tool.ToolName + "...",
	}
	s.activity.Label = "Running tool: " + strings.TrimSpace(event.Tool.ToolName+" "+event.Tool.ToolInput)
	if index := s.findToolEntryIndex(entry.ToolUseID, entry.ToolName, entry.ToolInput, true); index >= 0 {
		s.transcript[index] = entry
		return
	}
	s.transcript = append(s.transcript, entry)
}

func (s *tuiState) applyToolProgress(progress *clientToolEvent) {
	if progress == nil {
		return
	}
	index := s.findToolEntryIndex(progress.ToolUseID, "", "", true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:                "tool",
			ToolUseID:           strings.TrimSpace(progress.ToolUseID),
			ToolStatus:          toolStatusRunning,
			ToolProgressType:    progress.ProgressType,
			ToolProgressMessage: progress.ProgressMessage,
			ToolProgressOutput:  progressOutput(progress),
		})
		return
	}
	entry := &s.transcript[index]
	entry.ToolProgressType = progress.ProgressType
	entry.ToolProgressMessage = progress.ProgressMessage
	if output := progressOutput(progress); output != "" {
		entry.ToolProgressOutput = output
	}
	if entry.ToolStatus == "" || entry.ToolStatus == "called" {
		entry.ToolStatus = toolStatusRunning
	}
}

func (s *tuiState) applyToolResult(event clientEvent) {
	toolUseID, resultContent, isError := toolResultFromEvent(event)
	if toolUseID == "" {
		toolUseID = toolEventID(event)
	}
	status := toolStatusSucceeded
	if isError {
		status = toolStatusFailed
	}
	index := s.findToolEntryIndex(toolUseID, event.Tool.ToolName, event.Tool.ToolInput, true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:            "tool",
			ToolUseID:       toolUseID,
			ToolName:        event.Tool.ToolName,
			ToolInput:       event.Tool.ToolInput,
			ToolInputObject: cloneAnyMap(event.Tool.ToolInputObject),
			ToolStatus:      status,
			ToolError:       isError,
			Content:         resultContent,
		})
		return
	}
	entry := &s.transcript[index]
	if event.Tool != nil && event.Tool.ToolName != "" {
		entry.ToolName = event.Tool.ToolName
	}
	if event.Tool != nil && event.Tool.ToolInput != "" {
		entry.ToolInput = event.Tool.ToolInput
	}
	if event.Tool != nil && event.Tool.ToolInputObject != nil {
		entry.ToolInputObject = cloneAnyMap(event.Tool.ToolInputObject)
	}
	if toolUseID != "" {
		entry.ToolUseID = toolUseID
	}
	entry.ToolStatus = status
	entry.ToolError = isError
	entry.Content = resultContent
}

func (s *tuiState) findToolEntryIndex(toolUseID, toolName, toolInput string, runningOnly bool) int {
	return findToolEntryIndexIn(s.transcript, toolUseID, toolName, toolInput, runningOnly)
}

func isRunningToolStatus(status string) bool {
	return status == "" || status == "called" || status == toolStatusRunning
}

func toolEventID(event clientEvent) string {
	if event.Tool != nil && event.Tool.ToolUseID != "" {
		return event.Tool.ToolUseID
	}
	return ""
}

func toolResultFromEvent(event clientEvent) (string, string, bool) {
	toolUseID := strings.TrimSpace(toolEventID(event))
	content := ""
	isError := false
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
	toolName := ""
	if event.Tool != nil {
		toolName = event.Tool.ToolName
	}
	return toolUseID, trimToolResultPrefix(content, toolName), isError
}

func toolResultFromMessage(message *clientMessage, preferredToolUseID string) (string, string, bool) {
	if message == nil {
		return "", "", false
	}
	for _, block := range message.Blocks {
		if model.MessageBlockType(block.Type) != model.MessageBlockToolResult {
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

func progressOutput(progress *clientToolEvent) string {
	if progress == nil || progress.ProgressData == nil {
		return ""
	}
	for _, key := range []string{"output", "fullOutput", "stdout", "stderr"} {
		if value, ok := progress.ProgressData[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	if len(progress.ProgressData) > 0 {
		encoded, err := json.Marshal(progress.ProgressData)
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

