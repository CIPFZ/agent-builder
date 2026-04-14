package model_test

import (
	"encoding/json"
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
