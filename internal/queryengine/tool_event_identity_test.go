package queryengine

import (
	"testing"

	"myclaw/internal/model"
	"myclaw/internal/session"
)

func TestToolResultEventInfersToolIdentityAndErrorFromMessageBlocks(t *testing.T) {
	msg := session.Message{
		Role:    "tool",
		Content: "Bash: exit status 1",
		Blocks: []model.MessageBlock{{
			Type:      model.MessageBlockToolResult,
			ToolUseID: "toolu-bash",
			Content:   "exit status 1",
			IsError:   true,
		}},
	}

	toolUseID, toolError := toolResultIdentity(&msg)

	if toolUseID != "toolu-bash" {
		t.Fatalf("toolUseID = %q, want toolu-bash", toolUseID)
	}
	if !toolError {
		t.Fatal("toolError = false, want true")
	}
}
