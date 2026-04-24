package runtime

import (
	"testing"

	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestResolveWorkDirPrefersSessionWorktree(t *testing.T) {
	sess := session.Session{
		Key: "session:key",
		Metadata: session.SessionMetadata{
			AgentWorktreePath: "C:/repo/.worktrees/child",
		},
	}
	if got := resolveWorkDir(sess, workspace.NewLoader("C:/repo")); got != "C:/repo/.worktrees/child" {
		t.Fatalf("resolveWorkDir = %q, want worktree path", got)
	}
}

func TestFromQueryEventPreservesStructuredShellPayload(t *testing.T) {
	event := fromQueryEvent(queryengine.Event{
		Type:              "tool.result",
		RunID:             "run-shell",
		ToolUseID:         "toolu-shell",
		ProviderMessageID: "msg-shell",
		ToolName:          "Bash",
		ToolInput:         `{"command":"pwd"}`,
		StructuredContent: map[string]any{"command": "pwd", "exit_code": 0},
		Meta:              map[string]any{"working_directory": "C:/repo/.worktrees/child"},
	})
	if event.StructuredContent == nil {
		t.Fatalf("event = %#v, want structured content", event)
	}
	if event.Meta["working_directory"] != "C:/repo/.worktrees/child" {
		t.Fatalf("meta = %#v, want working_directory preserved", event.Meta)
	}
}
