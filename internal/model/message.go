package model

import (
	"encoding/json"
	"time"
)

type MessageBlockType string

const (
	MessageBlockText       MessageBlockType = "text"
	MessageBlockThinking   MessageBlockType = "thinking"
	MessageBlockToolUse    MessageBlockType = "tool_use"
	MessageBlockToolResult MessageBlockType = "tool_result"
)

type MessageBlock struct {
	Type        MessageBlockType `json:"type"`
	ID          string           `json:"id,omitempty"`
	ToolUseID   string           `json:"tool_use_id,omitempty"`
	Text        string           `json:"text,omitempty"`
	Name        string           `json:"name,omitempty"`
	Input       string           `json:"input,omitempty"`
	InputObject map[string]any   `json:"input_object,omitempty"`
	Content     string           `json:"content,omitempty"`
	IsError     bool             `json:"is_error,omitempty"`
	Raw         map[string]any   `json:"-"`
}

func (b MessageBlock) MarshalJSON() ([]byte, error) {
	if b.Raw != nil {
		raw := make(map[string]any, len(b.Raw)+1)
		for key, value := range b.Raw {
			raw[key] = value
		}
		if _, ok := raw["type"]; !ok && b.Type != "" {
			raw["type"] = string(b.Type)
		}
		return json.Marshal(raw)
	}

	type wireBlock struct {
		Type      MessageBlockType `json:"type"`
		ID        string           `json:"id,omitempty"`
		ToolUseID string           `json:"tool_use_id,omitempty"`
		Text      string           `json:"text,omitempty"`
		Name      string           `json:"name,omitempty"`
		Input     any              `json:"input,omitempty"`
		Content   string           `json:"content,omitempty"`
		IsError   bool             `json:"is_error,omitempty"`
	}

	var input any
	if b.InputObject != nil {
		input = b.InputObject
	} else if b.Input != "" {
		input = b.Input
	}
	return json.Marshal(wireBlock{
		Type:      b.Type,
		ID:        b.ID,
		ToolUseID: b.ToolUseID,
		Text:      b.Text,
		Name:      b.Name,
		Input:     input,
		Content:   b.Content,
		IsError:   b.IsError,
	})
}

func (b *MessageBlock) UnmarshalJSON(data []byte) error {
	type wireBlock struct {
		Type        MessageBlockType `json:"type"`
		ID          string           `json:"id,omitempty"`
		ToolUseID   string           `json:"tool_use_id,omitempty"`
		Text        string           `json:"text,omitempty"`
		Name        string           `json:"name,omitempty"`
		Input       json.RawMessage  `json:"input,omitempty"`
		InputObject json.RawMessage  `json:"input_object,omitempty"`
		Content     string           `json:"content,omitempty"`
		IsError     bool             `json:"is_error,omitempty"`
	}

	var wire wireBlock
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	b.Type = wire.Type
	b.ID = wire.ID
	b.ToolUseID = wire.ToolUseID
	b.Text = wire.Text
	b.Name = wire.Name
	b.Content = wire.Content
	b.IsError = wire.IsError
	b.Input = ""
	b.InputObject = nil

	if len(wire.Input) > 0 && string(wire.Input) != "null" {
		var inputString string
		if err := json.Unmarshal(wire.Input, &inputString); err == nil {
			b.Input = inputString
		} else {
			var inputObject map[string]any
			if err := json.Unmarshal(wire.Input, &inputObject); err != nil {
				return err
			}
			if inputObject != nil {
				b.InputObject = inputObject
				encoded, err := json.Marshal(inputObject)
				if err != nil {
					return err
				}
				b.Input = string(encoded)
			}
		}
	}
	if len(wire.InputObject) > 0 && string(wire.InputObject) != "null" {
		var inputObject map[string]any
		if err := json.Unmarshal(wire.InputObject, &inputObject); err != nil {
			return err
		}
		if inputObject != nil {
			b.InputObject = inputObject
			if b.Input == "" {
				encoded, err := json.Marshal(inputObject)
				if err != nil {
					return err
				}
				b.Input = string(encoded)
			}
		}
	}
	if b.Type != MessageBlockText &&
		b.Type != MessageBlockThinking &&
		b.Type != MessageBlockToolUse &&
		b.Type != MessageBlockToolResult {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		b.Raw = raw
	}
	return nil
}

type Message struct {
	ID                        string           `json:"id"`
	SessionID                 string           `json:"session_id"`
	Role                      string           `json:"role"`
	Subtype                   string           `json:"subtype,omitempty"`
	Content                   string           `json:"content"`
	ProviderMessageID         string           `json:"provider_message_id,omitempty"`
	Blocks                    []MessageBlock   `json:"blocks,omitempty"`
	IsMeta                    bool             `json:"is_meta,omitempty"`
	Level                     string           `json:"level,omitempty"`
	LogicalParentID           string           `json:"logical_parent_id,omitempty"`
	CompactMetadata           *CompactMetadata `json:"compactMetadata,omitempty"`
	IsCompactSummary          bool             `json:"is_compact_summary,omitempty"`
	IsVisibleInTranscriptOnly bool             `json:"is_visible_in_transcript_only,omitempty"`
	CreatedAt                 time.Time        `json:"created_at"`
}

type CompactMetadata struct {
	Trigger                   string                   `json:"trigger,omitempty"`
	PreTokens                 int                      `json:"preTokens,omitempty"`
	UserContext               string                   `json:"userContext,omitempty"`
	MessagesSummarized        int                      `json:"messagesSummarized,omitempty"`
	PreCompactDiscoveredTools []string                 `json:"preCompactDiscoveredTools,omitempty"`
	PreservedSegment          *CompactPreservedSegment `json:"preservedSegment,omitempty"`
}

type CompactPreservedSegment struct {
	HeadID   string `json:"headUuid,omitempty"`
	AnchorID string `json:"anchorUuid,omitempty"`
	TailID   string `json:"tailUuid,omitempty"`
}
