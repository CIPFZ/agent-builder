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
