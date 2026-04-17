package model_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestClaudeTranscriptMessageIncludesSerializedSessionFields(t *testing.T) {
	parent := "parent-uuid"
	message := model.Message{
		ID:         "user-uuid",
		SessionID:  "sess-1",
		Role:       "user",
		Content:    "hello",
		CWD:        "C:/repo",
		UserType:   "external",
		Entrypoint: "cli",
		Version:    "1.2.3",
		GitBranch:  "main",
		Slug:       "plan-slug",
		AgentID:    "agent-1",
		TeamName:   "team",
		AgentName:  "worker",
		AgentColor: "blue",
		PromptID:   "prompt-1",
	}

	data, err := json.Marshal(model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{
		ParentUUID: &parent,
		Timestamp:  "2026-04-15T00:00:00Z",
	}))
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	wire := string(data)
	for _, want := range []string{
		`"cwd":"C:/repo"`,
		`"userType":"external"`,
		`"entrypoint":"cli"`,
		`"sessionId":"sess-1"`,
		`"version":"1.2.3"`,
		`"gitBranch":"main"`,
		`"slug":"plan-slug"`,
		`"agentId":"agent-1"`,
		`"teamName":"team"`,
		`"agentName":"worker"`,
		`"agentColor":"blue"`,
		`"promptId":"prompt-1"`,
	} {
		if !strings.Contains(wire, want) {
			t.Fatalf("wire JSON = %s, want %s", wire, want)
		}
	}
}

func TestClaudeTranscriptRoundTripPreservesSerializedSessionFields(t *testing.T) {
	transcript := model.ClaudeTranscriptMessage{
		ParentUUID:  nil,
		IsSidechain: false,
		CWD:         "C:/repo",
		UserType:    "external",
		Entrypoint:  "sdk-ts",
		SessionID:   "sess-1",
		Version:     "1.2.3",
		GitBranch:   "main",
		Slug:        "plan-slug",
		AgentID:     "agent-1",
		TeamName:    "team",
		AgentName:   "worker",
		AgentColor:  "blue",
		PromptID:    "prompt-1",
		Type:        "user",
		UUID:        "user-uuid",
		Timestamp:   "2026-04-15T00:00:00Z",
		Message:     &model.ClaudeAPIMessage{Role: "user", Content: "hello"},
	}

	message, err := model.MessageFromClaudeTranscript(transcript, "sess-1")
	if err != nil {
		t.Fatalf("convert transcript: %v", err)
	}
	if message.CWD != "C:/repo" || message.Entrypoint != "sdk-ts" || message.PromptID != "prompt-1" {
		t.Fatalf("message = %#v, want serialized session fields", message)
	}
	encoded := model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{Timestamp: "2026-04-15T00:00:00Z"})
	if encoded.CWD != "C:/repo" || encoded.Entrypoint != "sdk-ts" || encoded.PromptID != "prompt-1" {
		t.Fatalf("encoded = %#v, want serialized session fields preserved", encoded)
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

func TestClaudeTranscriptRoundTripPreservesToolRuntimeRoleAndSourceAssistantUUID(t *testing.T) {
	sourceAssistant := "assistant-uuid"
	original := model.Message{
		ID:        "tool-result-uuid",
		SessionID: "sess-1",
		Role:      "tool",
		Content:   "mcp__filesystem__read_resource: updated mcp output",
		Blocks: []model.MessageBlock{{
			Type:      model.MessageBlockToolResult,
			ToolUseID: "toolu-1",
			Content:   "tool output",
		}},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	transcript := model.NewClaudeTranscriptMessage(original, model.ClaudeTranscriptOptions{
		SourceToolAssistantUUID: sourceAssistant,
		Timestamp:               "2026-04-15T00:00:00Z",
	})
	if transcript.SourceToolAssistantUUID != sourceAssistant {
		t.Fatalf("transcript = %#v, want source assistant UUID", transcript)
	}

	roundTripped, err := model.MessageFromClaudeTranscript(transcript, "sess-1")
	if err != nil {
		t.Fatalf("round trip transcript: %v", err)
	}
	if roundTripped.Role != "tool" || len(roundTripped.Blocks) != 1 || roundTripped.Blocks[0].ToolUseID != "toolu-1" {
		t.Fatalf("round tripped message = %#v, want tool result runtime view", roundTripped)
	}
	if roundTripped.SourceToolAssistantUUID != sourceAssistant {
		t.Fatalf("round tripped message = %#v, want source assistant UUID preserved", roundTripped)
	}
	if roundTripped.Content != original.Content {
		t.Fatalf("round tripped content = %q, want runtime content %q", roundTripped.Content, original.Content)
	}
}

func TestClaudeTranscriptRoundTripPreservesAssistantAPIMessageMetadata(t *testing.T) {
	transcript := model.ClaudeTranscriptMessage{
		Type:      "assistant",
		UUID:      "assistant-uuid",
		Timestamp: "2026-04-15T00:00:00Z",
		Message: &model.ClaudeAPIMessage{
			ID:           "provider-msg",
			Model:        "claude-opus-4-6",
			Role:         "assistant",
			Type:         "message",
			StopReason:   "tool_use",
			StopSequence: "stop",
			Usage:        map[string]any{"input_tokens": float64(10)},
			Content:      []model.MessageBlock{{Type: model.MessageBlockText, Text: "hello"}},
		},
	}

	message, err := model.MessageFromClaudeTranscript(transcript, "sess-1")
	if err != nil {
		t.Fatalf("convert transcript: %v", err)
	}
	if message.ProviderMessageID != "provider-msg" || message.ProviderModel != "claude-opus-4-6" || message.StopReason != "tool_use" {
		t.Fatalf("message = %#v, want assistant provider metadata", message)
	}

	encoded := model.NewClaudeTranscriptMessage(message, model.ClaudeTranscriptOptions{Timestamp: "2026-04-15T00:00:00Z"})
	if encoded.Message == nil || encoded.Message.Model != "claude-opus-4-6" || encoded.Message.StopReason != "tool_use" {
		t.Fatalf("encoded transcript = %#v, want provider metadata preserved", encoded)
	}
}

func TestClaudeTranscriptToolResultParentsToSourceAssistantUUID(t *testing.T) {
	entries := model.NewClaudeTranscriptMessages([]model.Message{
		{
			ID:        "source-assistant-uuid",
			SessionID: "sess-1",
			Role:      "assistant",
			Content:   "calling tool",
			Blocks: []model.MessageBlock{{
				Type: model.MessageBlockToolUse,
				ID:   "toolu-1",
				Name: "Read",
			}},
			CreatedAt: time.Unix(1, 0).UTC(),
		},
		{
			ID:        "other-assistant-uuid",
			SessionID: "sess-1",
			Role:      "assistant",
			Content:   "parallel assistant",
			CreatedAt: time.Unix(2, 0).UTC(),
		},
		{
			ID:                      "tool-result-uuid",
			SessionID:               "sess-1",
			Role:                    "tool",
			Content:                 "Read: file contents",
			SourceToolAssistantUUID: "source-assistant-uuid",
			Blocks: []model.MessageBlock{{
				Type:      model.MessageBlockToolResult,
				ToolUseID: "toolu-1",
				Content:   "file contents",
			}},
			CreatedAt: time.Unix(3, 0).UTC(),
		},
	}, nil)

	if len(entries) != 3 || entries[2].ParentUUID == nil || *entries[2].ParentUUID != "source-assistant-uuid" {
		t.Fatalf("entries = %#v, want tool_result parentUuid to source assistant", entries)
	}
}

func TestClaudeTranscriptCompactBoundaryMovesParentToLogicalParent(t *testing.T) {
	parent := "summary-uuid"
	entry := model.NewClaudeTranscriptMessage(model.Message{
		ID:        "boundary-uuid",
		SessionID: "sess-1",
		Role:      "system",
		Subtype:   "compact_boundary",
		Content:   "Conversation compacted",
		CreatedAt: time.Unix(1, 0).UTC(),
	}, model.ClaudeTranscriptOptions{ParentUUID: &parent, Timestamp: "2026-04-15T00:00:00Z"})

	if entry.ParentUUID != nil {
		t.Fatalf("entry = %#v, want compact boundary parentUuid nil", entry)
	}
	if entry.LogicalParentUUID == nil || *entry.LogicalParentUUID != parent {
		t.Fatalf("entry = %#v, want original parentUuid preserved as logicalParentUuid", entry)
	}
}

func TestClaudeTranscriptLoadAppliesPreservedSegmentRelinks(t *testing.T) {
	entries := []model.ClaudeTranscriptMessage{
		{Type: "user", UUID: "old-root", Timestamp: "2026-04-15T00:00:01Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "old root"}},
		{ParentUUID: stringPtr("old-root"), Type: "assistant", UUID: "old-answer", Timestamp: "2026-04-15T00:00:02Z", Message: &model.ClaudeAPIMessage{ID: "provider-old", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "old answer"}}}},
		{Type: "system", UUID: "boundary", Timestamp: "2026-04-15T00:00:03Z", Subtype: "compact_boundary", Content: "Conversation compacted", CompactMetadata: &model.CompactMetadata{PreservedSegment: &model.CompactPreservedSegment{HeadID: "kept-head", AnchorID: "summary", TailID: "kept-tail"}}},
		{ParentUUID: stringPtr("boundary"), Type: "user", UUID: "summary", Timestamp: "2026-04-15T00:00:04Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "Summary: compacted"}, IsCompactSummary: true},
		{ParentUUID: stringPtr("old-answer"), Type: "user", UUID: "kept-head", Timestamp: "2026-04-15T00:00:05Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "kept prompt"}},
		{ParentUUID: stringPtr("kept-head"), Type: "assistant", UUID: "kept-tail", Timestamp: "2026-04-15T00:00:06Z", Message: &model.ClaudeAPIMessage{ID: "provider-kept", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "kept answer"}}}},
	}

	chain, err := model.LatestClaudeTranscriptChain(entries)
	if err != nil {
		t.Fatalf("latest chain: %v", err)
	}
	var ids []string
	for _, entry := range chain {
		ids = append(ids, entry.UUID)
	}
	want := []string{"boundary", "summary", "kept-head", "kept-tail"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("chain ids = %#v, want %#v", ids, want)
	}
}

func TestClaudeTranscriptPreservedSegmentZeroesAssistantUsage(t *testing.T) {
	entries := []model.ClaudeTranscriptMessage{
		{Type: "system", UUID: "boundary", Timestamp: "2026-04-15T00:00:03Z", Subtype: "compact_boundary", Content: "Conversation compacted", CompactMetadata: &model.CompactMetadata{PreservedSegment: &model.CompactPreservedSegment{HeadID: "kept-head", AnchorID: "summary", TailID: "kept-tail"}}},
		{ParentUUID: stringPtr("boundary"), Type: "user", UUID: "summary", Timestamp: "2026-04-15T00:00:04Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "Summary: compacted"}, IsCompactSummary: true},
		{Type: "user", UUID: "kept-head", Timestamp: "2026-04-15T00:00:05Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "kept prompt"}},
		{ParentUUID: stringPtr("kept-head"), Type: "assistant", UUID: "kept-tail", Timestamp: "2026-04-15T00:00:06Z", Message: &model.ClaudeAPIMessage{ID: "provider-kept", Role: "assistant", Type: "message", Usage: map[string]any{"input_tokens": float64(1234)}, Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "kept answer"}}}},
	}

	chain, err := model.LatestClaudeTranscriptChain(entries)
	if err != nil {
		t.Fatalf("latest chain: %v", err)
	}
	tail := chain[len(chain)-1]
	usage, ok := tail.Message.Usage.(map[string]any)
	if !ok || usage["input_tokens"] != 0 {
		t.Fatalf("usage = %#v, want stale usage zeroed", tail.Message.Usage)
	}
}

func TestClaudeTranscriptLoadSelectsTerminalUserAssistantLeaf(t *testing.T) {
	entries := []model.ClaudeTranscriptMessage{
		{Type: "user", UUID: "root", Timestamp: "2026-04-15T00:00:01Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "root"}},
		{ParentUUID: stringPtr("late-user"), Type: "assistant", UUID: "terminal-assistant", Timestamp: "2026-04-15T00:00:04Z", Message: &model.ClaudeAPIMessage{ID: "provider-terminal", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "terminal"}}}},
		{ParentUUID: stringPtr("root"), Type: "user", UUID: "late-user", Timestamp: "2026-04-15T00:00:02Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "late parent"}},
	}

	chain, err := model.LatestClaudeTranscriptChain(entries)
	if err != nil {
		t.Fatalf("latest chain: %v", err)
	}
	if chain[len(chain)-1].UUID != "terminal-assistant" {
		t.Fatalf("chain = %#v, want terminal user/assistant leaf", chain)
	}
}

func TestClaudeTranscriptLoadRecoversParallelToolResults(t *testing.T) {
	entries := []model.ClaudeTranscriptMessage{
		{Type: "user", UUID: "prompt", Timestamp: "2026-04-15T00:00:01Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "run tools"}},
		{ParentUUID: stringPtr("prompt"), Type: "assistant", UUID: "asst-a", Timestamp: "2026-04-15T00:00:02Z", Message: &model.ClaudeAPIMessage{ID: "provider-1", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-a", Name: "Read"}}}},
		{ParentUUID: stringPtr("asst-a"), Type: "assistant", UUID: "asst-b", Timestamp: "2026-04-15T00:00:03Z", Message: &model.ClaudeAPIMessage{ID: "provider-1", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-b", Name: "Grep"}}}},
		{ParentUUID: stringPtr("asst-a"), Type: "user", UUID: "tr-a", Timestamp: "2026-04-15T00:00:04Z", SourceToolAssistantUUID: "asst-a", Message: &model.ClaudeAPIMessage{Role: "user", Content: []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-a", Content: "a"}}}},
		{ParentUUID: stringPtr("asst-b"), Type: "user", UUID: "tr-b", Timestamp: "2026-04-15T00:00:05Z", SourceToolAssistantUUID: "asst-b", Message: &model.ClaudeAPIMessage{Role: "user", Content: []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-b", Content: "b"}}}},
		{ParentUUID: stringPtr("tr-a"), Type: "assistant", UUID: "final", Timestamp: "2026-04-15T00:00:06Z", Message: &model.ClaudeAPIMessage{ID: "provider-final", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "done"}}}},
	}

	chain, err := model.LatestClaudeTranscriptChain(entries)
	if err != nil {
		t.Fatalf("latest chain: %v", err)
	}
	var ids []string
	for _, entry := range chain {
		ids = append(ids, entry.UUID)
	}
	for _, want := range []string{"asst-b", "tr-b"} {
		if !containsString(ids, want) {
			t.Fatalf("chain ids = %#v, want recovered %s", ids, want)
		}
	}
}

func TestClaudeTranscriptParallelRecoveryKeepsToolUseBeforeToolResult(t *testing.T) {
	entries := []model.ClaudeTranscriptMessage{
		{Type: "user", UUID: "prompt", Timestamp: "2026-04-15T00:00:01Z", Message: &model.ClaudeAPIMessage{Role: "user", Content: "run tools"}},
		{ParentUUID: stringPtr("prompt"), Type: "assistant", UUID: "asst-a", Timestamp: "2026-04-15T00:00:02Z", Message: &model.ClaudeAPIMessage{ID: "provider-1", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-a", Name: "Read"}}}},
		{ParentUUID: stringPtr("asst-a"), Type: "assistant", UUID: "asst-b", Timestamp: "2026-04-15T00:00:05Z", Message: &model.ClaudeAPIMessage{ID: "provider-1", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockToolUse, ID: "toolu-b", Name: "Grep"}}}},
		{ParentUUID: stringPtr("asst-a"), Type: "user", UUID: "tr-a", Timestamp: "2026-04-15T00:00:06Z", SourceToolAssistantUUID: "asst-a", Message: &model.ClaudeAPIMessage{Role: "user", Content: []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-a", Content: "a"}}}},
		{ParentUUID: stringPtr("asst-b"), Type: "user", UUID: "tr-b", Timestamp: "2026-04-15T00:00:03Z", SourceToolAssistantUUID: "asst-b", Message: &model.ClaudeAPIMessage{Role: "user", Content: []model.MessageBlock{{Type: model.MessageBlockToolResult, ToolUseID: "toolu-b", Content: "b"}}}},
		{ParentUUID: stringPtr("tr-a"), Type: "assistant", UUID: "final", Timestamp: "2026-04-15T00:00:07Z", Message: &model.ClaudeAPIMessage{ID: "provider-final", Role: "assistant", Type: "message", Content: []model.MessageBlock{{Type: model.MessageBlockText, Text: "done"}}}},
	}

	chain, err := model.LatestClaudeTranscriptChain(entries)
	if err != nil {
		t.Fatalf("latest chain: %v", err)
	}
	var ids []string
	for _, entry := range chain {
		ids = append(ids, entry.UUID)
	}
	asstIndex := indexOfString(ids, "asst-b")
	resultIndex := indexOfString(ids, "tr-b")
	if asstIndex < 0 || resultIndex < 0 || resultIndex < asstIndex {
		t.Fatalf("chain ids = %#v, want recovered assistant before its tool_result", ids)
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

func stringPtr(value string) *string {
	return &value
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func indexOfString(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
