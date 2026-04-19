package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestReadToolResolvesRelativePathsAgainstAgentWorktree(t *testing.T) {
	worktree := t.TempDir()
	target := filepath.Join(worktree, "notes.txt")
	if err := os.WriteFile(target, []byte("from worktree"), 0o644); err != nil {
		t.Fatalf("write %q: %v", target, err)
	}

	sessions := session.NewManager(nil)
	sess := sessions.CreateChild("researcher", "agent:researcher:child:1")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.AgentWorktreePath = worktree
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	sess, _ = sessions.GetByID(sess.ID)

	got, err := tools.NewReadTool().Invoke(context.Background(), sess, `{"file_path":"notes.txt"}`)
	if err != nil {
		t.Fatalf("invoke read tool: %v", err)
	}
	if got != "from worktree" {
		t.Fatalf("read output = %q, want worktree-relative file contents", got)
	}
}
