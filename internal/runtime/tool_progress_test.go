package runtime

import (
	"testing"

	"myclaw/internal/queryengine"
	"myclaw/internal/tools"
)

func TestFromQueryEventCarriesToolIdentityAndProgress(t *testing.T) {
	progress := &tools.ToolProgress{ToolUseID: "toolu-1", Type: "shell.progress", Message: "running"}
	event := fromQueryEvent(queryengine.Event{
		Type:              "tool.progress",
		RunID:             "run-1",
		ToolUseID:         "toolu-1",
		ProviderMessageID: "provider-1",
		ToolName:          "Bash",
		ToolInput:         "pwd",
		ToolError:         true,
		Progress:          progress,
	})

	if event.ToolUseID != "toolu-1" {
		t.Fatalf("ToolUseID = %q, want toolu-1", event.ToolUseID)
	}
	if event.ProviderMessageID != "provider-1" {
		t.Fatalf("ProviderMessageID = %q, want provider-1", event.ProviderMessageID)
	}
	if !event.ToolError {
		t.Fatal("ToolError = false, want true")
	}
	if event.Progress == nil || event.Progress.Message != "running" {
		t.Fatalf("Progress = %#v, want running progress", event.Progress)
	}
}
