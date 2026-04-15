package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const syntheticClaudeModel = "<synthetic>"

type ClaudeTranscriptOptions struct {
	ParentUUID              *string
	LogicalParentUUID       *string
	SourceToolAssistantUUID string
	Timestamp               string
	IsSidechain             bool
}

type ClaudeTranscriptMessage struct {
	ParentUUID                *string           `json:"parentUuid"`
	LogicalParentUUID         *string           `json:"logicalParentUuid,omitempty"`
	IsSidechain               bool              `json:"isSidechain"`
	Type                      string            `json:"type"`
	UUID                      string            `json:"uuid"`
	Timestamp                 string            `json:"timestamp"`
	Message                   *ClaudeAPIMessage `json:"message,omitempty"`
	Subtype                   string            `json:"subtype,omitempty"`
	Content                   string            `json:"content,omitempty"`
	IsMeta                    bool              `json:"isMeta,omitempty"`
	IsVisibleInTranscriptOnly bool              `json:"isVisibleInTranscriptOnly,omitempty"`
	IsCompactSummary          bool              `json:"isCompactSummary,omitempty"`
	Level                     string            `json:"level,omitempty"`
	CompactMetadata           *CompactMetadata  `json:"compactMetadata,omitempty"`
	SourceToolAssistantUUID   string            `json:"sourceToolAssistantUUID,omitempty"`
	ToolUseResult             any               `json:"toolUseResult,omitempty"`
}

type ClaudeAPIMessage struct {
	ID                string `json:"id,omitempty"`
	Container         any    `json:"container,omitempty"`
	Model             string `json:"model,omitempty"`
	Role              string `json:"role"`
	StopReason        string `json:"stop_reason,omitempty"`
	StopSequence      string `json:"stop_sequence,omitempty"`
	Type              string `json:"type,omitempty"`
	Usage             any    `json:"usage,omitempty"`
	Content           any    `json:"content"`
	ContextManagement any    `json:"context_management,omitempty"`
}

func NewClaudeTranscriptMessage(message Message, opts ClaudeTranscriptOptions) ClaudeTranscriptMessage {
	timestamp := opts.Timestamp
	if timestamp == "" && !message.CreatedAt.IsZero() {
		timestamp = message.CreatedAt.Format(time.RFC3339Nano)
	}
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	logicalParent := opts.LogicalParentUUID
	if logicalParent == nil && message.LogicalParentID != "" {
		value := message.LogicalParentID
		logicalParent = &value
	}

	out := ClaudeTranscriptMessage{
		ParentUUID:                opts.ParentUUID,
		LogicalParentUUID:         logicalParent,
		IsSidechain:               opts.IsSidechain,
		Type:                      claudeTranscriptType(message),
		UUID:                      message.ID,
		Timestamp:                 timestamp,
		IsMeta:                    message.IsMeta,
		IsVisibleInTranscriptOnly: message.IsVisibleInTranscriptOnly,
		IsCompactSummary:          message.IsCompactSummary || message.Role == "summary",
		SourceToolAssistantUUID:   opts.SourceToolAssistantUUID,
	}

	switch out.Type {
	case "assistant":
		out.Message = &ClaudeAPIMessage{
			ID:                firstNonEmpty(message.ProviderMessageID, message.ID),
			Container:         nil,
			Model:             syntheticClaudeModel,
			Role:              "assistant",
			StopReason:        "stop_sequence",
			StopSequence:      "",
			Type:              "message",
			Content:           claudeAssistantContent(message),
			ContextManagement: nil,
		}
	case "user":
		out.Message = &ClaudeAPIMessage{
			Role:    "user",
			Content: claudeUserContent(message),
		}
	case "system":
		out.Subtype = message.Subtype
		out.Content = message.Content
		out.Level = message.Level
		out.CompactMetadata = message.CompactMetadata
		if message.Subtype == "compact_boundary" {
			out.ParentUUID = nil
		}
	default:
		out.Message = &ClaudeAPIMessage{
			Role:    "user",
			Content: message.Content,
		}
	}
	return out
}

func MessageFromClaudeTranscript(entry ClaudeTranscriptMessage, sessionID string) (Message, error) {
	createdAt, err := parseClaudeTimestamp(entry.Timestamp)
	if err != nil {
		return Message{}, err
	}

	message := Message{
		ID:                        entry.UUID,
		SessionID:                 sessionID,
		Role:                      entry.Type,
		Subtype:                   entry.Subtype,
		Content:                   entry.Content,
		IsMeta:                    entry.IsMeta,
		Level:                     entry.Level,
		CompactMetadata:           entry.CompactMetadata,
		IsCompactSummary:          entry.IsCompactSummary,
		IsVisibleInTranscriptOnly: entry.IsVisibleInTranscriptOnly,
		CreatedAt:                 createdAt,
	}
	if entry.LogicalParentUUID != nil {
		message.LogicalParentID = *entry.LogicalParentUUID
	}

	switch entry.Type {
	case "assistant":
		if entry.Message == nil {
			return message, nil
		}
		message.Role = "assistant"
		message.ProviderMessageID = entry.Message.ID
		content, blocks, err := parseClaudeMessageContent(entry.Message.Content)
		if err != nil {
			return Message{}, err
		}
		message.Content = content
		message.Blocks = blocks
	case "user":
		if entry.Message == nil {
			message.Role = "user"
			return message, nil
		}
		content, blocks, err := parseClaudeMessageContent(entry.Message.Content)
		if err != nil {
			return Message{}, err
		}
		message.Role = "user"
		if len(blocks) > 0 && blocks[0].Type == MessageBlockToolResult {
			message.Role = "tool"
		} else if entry.IsCompactSummary && strings.HasPrefix(content, "Summary:") {
			message.Role = "summary"
		}
		message.Content = content
		message.Blocks = blocks
	case "system":
		message.Role = "system"
	default:
		message.Role = entry.Type
	}

	return message, nil
}

func claudeTranscriptType(message Message) string {
	switch message.Role {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "summary":
		return "user"
	case "tool":
		return "user"
	default:
		return "user"
	}
}

func claudeAssistantContent(message Message) []MessageBlock {
	if len(message.Blocks) > 0 {
		return append([]MessageBlock(nil), message.Blocks...)
	}
	return []MessageBlock{{Type: MessageBlockText, Text: nonEmptyContent(message.Content)}}
}

func claudeUserContent(message Message) any {
	if len(message.Blocks) > 0 {
		return append([]MessageBlock(nil), message.Blocks...)
	}
	return nonEmptyContent(message.Content)
}

func nonEmptyContent(content string) string {
	if content == "" {
		return "(no content)"
	}
	return content
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseClaudeTimestamp(timestamp string) (time.Time, error) {
	if timestamp == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err == nil {
		return parsed, nil
	}
	parsed, err = time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("parse claude transcript timestamp %q: %w", timestamp, err)
}

func parseClaudeMessageContent(content any) (string, []MessageBlock, error) {
	switch value := content.(type) {
	case nil:
		return "", nil, nil
	case string:
		return value, nil, nil
	case []MessageBlock:
		blocks := append([]MessageBlock(nil), value...)
		return textFromMessageBlocks(blocks), blocks, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", nil, err
		}
		var contentString string
		if err := json.Unmarshal(data, &contentString); err == nil {
			return contentString, nil, nil
		}
		var blocks []MessageBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return "", nil, err
		}
		return textFromMessageBlocks(blocks), blocks, nil
	}
}

func textFromMessageBlocks(blocks []MessageBlock) string {
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case MessageBlockText:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case MessageBlockThinking:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case MessageBlockToolResult:
			if block.Content != "" {
				parts = append(parts, block.Content)
			}
		case MessageBlockToolUse:
			if block.Name != "" {
				parts = append(parts, block.Name)
			}
		default:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
