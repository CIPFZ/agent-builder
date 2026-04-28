package runtime

import "testing"

func TestRuntimeEventPayloadStabilityForToolApprovalAndCommandFamilies(t *testing.T) {
	if EventToolCalled != "tool.called" || EventPermissionRequired != "permission.required" || EventCommandCompleted != "command.completed" {
		t.Fatalf("event constants drifted: %q %q %q", EventToolCalled, EventPermissionRequired, EventCommandCompleted)
	}
	event := RuntimeEvent{
		Type:              EventToolResult,
		RunID:             "run-1",
		ToolUseID:         "toolu-1",
		ProviderMessageID: "msg-1",
		ToolName:          "Write",
		ToolInput:         `{"file_path":"README.md"}`,
		ToolInputObject:   map[string]any{"file_path": "README.md"},
		ToolError:         true,
		Meta:              map[string]any{"exit_code": 1},
	}
	payload := event.Payload()
	for _, key := range []string{"type", "run_id", "tool_use_id", "provider_message_id", "tool_name", "tool_input", "tool_input_object", "tool_error", "meta"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %#v", key, payload)
		}
	}
	payload["tool_input_object"].(map[string]any)["file_path"] = "mutated"
	if event.ToolInputObject["file_path"] != "README.md" {
		t.Fatalf("payload did not clone tool input object: %#v", event.ToolInputObject)
	}
}

func TestRuntimeApprovalEventNameMatchesGatewayAndTUIContract(t *testing.T) {
	if EventApprovalResolved != "approval.updated" {
		t.Fatalf("approval event = %q, want approval.updated", EventApprovalResolved)
	}
}

func TestRuntimeEventPayloadClonesStructuredContent(t *testing.T) {
	event := RuntimeEvent{Type: EventToolResult, StructuredContent: map[string]any{"items": []any{map[string]any{"name": "stable"}}}}
	payload := event.Payload()
	items := payload["structured_content"].(map[string]any)["items"].([]any)
	items[0].(map[string]any)["name"] = "mutated"
	original := event.StructuredContent.(map[string]any)["items"].([]any)[0].(map[string]any)["name"]
	if original != "stable" {
		t.Fatalf("structured content mutated through payload: %#v", event.StructuredContent)
	}
}
