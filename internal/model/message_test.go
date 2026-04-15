package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"myclaw/internal/model"
)

func TestMessageBlockMarshalToolUseUsesObjectAsCanonicalInput(t *testing.T) {
	block := model.MessageBlock{
		Type: model.MessageBlockToolUse,
		ID:   "toolu-1",
		Name: "structured.echo",
		InputObject: map[string]any{
			"command": "run",
			"cwd":     "/tmp",
		},
		Input: `{"command":"legacy","cwd":"/legacy"}`,
	}

	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	input, ok := payload["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want input object", payload)
	}
	if input["command"] != "run" || input["cwd"] != "/tmp" {
		t.Fatalf("input = %#v, want canonical object input", input)
	}
	if _, ok := payload["input_object"]; ok {
		t.Fatalf("payload = %#v, did not want input_object sidecar", payload)
	}
}

func TestMessageBlockUnmarshalToolUseObjectInputPreservesLegacyString(t *testing.T) {
	raw := []byte(`{"type":"tool_use","id":"toolu-1","name":"structured.echo","input":{"command":"run","cwd":"/tmp"}}`)

	var block model.MessageBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}

	if block.InputObject["command"] != "run" || block.InputObject["cwd"] != "/tmp" {
		t.Fatalf("input object = %#v, want object input", block.InputObject)
	}
	assertJSONInput(t, block.Input, map[string]any{
		"command": "run",
		"cwd":     "/tmp",
	})
}

func TestMessageBlockUnmarshalLegacyInputObjectSidecar(t *testing.T) {
	raw := []byte(`{"type":"tool_use","id":"toolu-1","name":"structured.echo","input":"{\"command\":\"run\",\"cwd\":\"/tmp\"}","input_object":{"command":"run","cwd":"/tmp"}}`)

	var block model.MessageBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}

	if block.InputObject["command"] != "run" || block.InputObject["cwd"] != "/tmp" {
		t.Fatalf("input object = %#v, want legacy sidecar object input", block.InputObject)
	}
	assertJSONInput(t, block.Input, map[string]any{
		"command": "run",
		"cwd":     "/tmp",
	})
}

func TestMessageBlockRawContentBlockRoundTripsUnknownBlockShape(t *testing.T) {
	block := model.MessageBlock{
		Type: "image",
		Raw: map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": "image/png",
				"data":       "abc123",
			},
		},
	}

	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	source, ok := payload["source"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want image source preserved", payload)
	}
	if source["media_type"] != "image/png" || source["data"] != "abc123" {
		t.Fatalf("source = %#v, want raw image fields preserved", source)
	}

	var restored model.MessageBlock
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	if restored.Raw["type"] != "image" {
		t.Fatalf("restored raw = %#v, want raw image block", restored.Raw)
	}
}

func TestCompactMetadataMarshalUsesClaudeCamelCaseWireShape(t *testing.T) {
	message := model.Message{
		ID:        "boundary-1",
		SessionID: "sess-1",
		Role:      "system",
		Subtype:   "compact_boundary",
		Content:   "Conversation compacted",
		CompactMetadata: &model.CompactMetadata{
			Trigger:                   "auto",
			PreTokens:                 123,
			UserContext:               "context",
			MessagesSummarized:        7,
			PreCompactDiscoveredTools: []string{"Bash"},
			PreservedSegment: &model.CompactPreservedSegment{
				HeadID:   "head-uuid",
				AnchorID: "anchor-uuid",
				TailID:   "tail-uuid",
			},
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	wire := string(data)

	for _, want := range []string{
		`"compactMetadata"`,
		`"preTokens"`,
		`"userContext"`,
		`"messagesSummarized"`,
		`"preCompactDiscoveredTools"`,
		`"preservedSegment"`,
		`"headUuid"`,
		`"anchorUuid"`,
		`"tailUuid"`,
	} {
		if !strings.Contains(wire, want) {
			t.Fatalf("wire JSON = %s, want %s", wire, want)
		}
	}

	for _, notWant := range []string{
		`"compact_metadata"`,
		`"pre_tokens"`,
		`"user_context"`,
		`"messages_summarized"`,
		`"pre_compact_discovered_tools"`,
		`"preserved_segment"`,
		`"head_id"`,
		`"anchor_id"`,
		`"tail_id"`,
	} {
		if strings.Contains(wire, notWant) {
			t.Fatalf("wire JSON = %s, did not want %s", wire, notWant)
		}
	}
}

func TestClaudeTranscriptMessageFromUserUsesNestedClaudeWireShape(t *testing.T) {
	parent := "parent-uuid"
	message := model.Message{
		ID:        "user-uuid",
		SessionID: "sess-1",
		Role:      "user",
		Content:   "hello",
	}

	transcript := model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{
		ParentUUID: &parent,
		Timestamp:  "2026-04-15T00:00:00Z",
	})
	data, err := json.Marshal(transcript)
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if payload["parentUuid"] != parent || payload["type"] != "user" || payload["uuid"] != "user-uuid" {
		t.Fatalf("payload = %#v, want Claude transcript parent/type/uuid", payload)
	}
	if _, ok := payload["role"]; ok {
		t.Fatalf("payload = %#v, did not want flat role", payload)
	}
	nested, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want nested message", payload)
	}
	if nested["role"] != "user" || nested["content"] != "hello" {
		t.Fatalf("nested message = %#v, want Claude user message", nested)
	}
}

func TestClaudeTranscriptMessageFromAssistantToolUsePreservesContentBlocks(t *testing.T) {
	message := model.Message{
		ID:                "assistant-uuid",
		SessionID:         "sess-1",
		Role:              "assistant",
		ProviderMessageID: "provider-msg-1",
		Blocks: []model.MessageBlock{
			{
				Type: model.MessageBlockToolUse,
				ID:   "toolu-1",
				Name: "system.run",
				InputObject: map[string]any{
					"command": "pwd",
				},
			},
		},
	}

	data, err := json.Marshal(model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{Timestamp: "2026-04-15T00:00:00Z"}))
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	nested := payload["message"].(map[string]any)
	if nested["id"] != "provider-msg-1" || nested["role"] != "assistant" || nested["type"] != "message" {
		t.Fatalf("nested message = %#v, want Claude assistant message", nested)
	}
	content, ok := nested["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("nested content = %#v, want one content block", nested["content"])
	}
	block := content[0].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "toolu-1" || block["name"] != "system.run" {
		t.Fatalf("block = %#v, want tool_use block", block)
	}
	input := block["input"].(map[string]any)
	if input["command"] != "pwd" {
		t.Fatalf("input = %#v, want structured command", input)
	}
}

func TestClaudeTranscriptMessageFromToolResultBecomesUserToolResult(t *testing.T) {
	sourceAssistant := "assistant-uuid"
	message := model.Message{
		ID:        "result-uuid",
		SessionID: "sess-1",
		Role:      "tool",
		Content:   "C:/repo",
		Blocks: []model.MessageBlock{
			{
				Type:      model.MessageBlockToolResult,
				ToolUseID: "toolu-1",
				Content:   "C:/repo",
			},
		},
	}

	data, err := json.Marshal(model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{
		SourceToolAssistantUUID: sourceAssistant,
		Timestamp:               "2026-04-15T00:00:00Z",
	}))
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if payload["type"] != "user" || payload["sourceToolAssistantUUID"] != sourceAssistant {
		t.Fatalf("payload = %#v, want Claude user tool_result transcript", payload)
	}
	nested := payload["message"].(map[string]any)
	content := nested["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "tool_result" || block["tool_use_id"] != "toolu-1" || block["content"] != "C:/repo" {
		t.Fatalf("block = %#v, want tool_result block", block)
	}
}

func TestClaudeTranscriptMessageFromCompactBoundaryUsesSystemShape(t *testing.T) {
	logicalParent := "pre-compact-uuid"
	message := model.Message{
		ID:              "boundary-uuid",
		SessionID:       "sess-1",
		Role:            "system",
		Subtype:         "compact_boundary",
		Content:         "Conversation compacted",
		LogicalParentID: logicalParent,
		CompactMetadata: &model.CompactMetadata{Trigger: "auto", PreTokens: 42},
	}

	data, err := json.Marshal(model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{Timestamp: "2026-04-15T00:00:00Z"}))
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	wire := string(data)
	for _, want := range []string{
		`"parentUuid":null`,
		`"logicalParentUuid":"pre-compact-uuid"`,
		`"type":"system"`,
		`"subtype":"compact_boundary"`,
		`"compactMetadata"`,
		`"preTokens":42`,
	} {
		if !strings.Contains(wire, want) {
			t.Fatalf("wire JSON = %s, want %s", wire, want)
		}
	}
}

func assertJSONInput(t *testing.T, got string, want map[string]any) {
	t.Helper()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("input = %q, want JSON object: %v", got, err)
	}
	for key, wantValue := range want {
		if parsed[key] != wantValue {
			t.Fatalf("input[%q] = %#v, want %#v in %#v", key, parsed[key], wantValue, parsed)
		}
	}
}
