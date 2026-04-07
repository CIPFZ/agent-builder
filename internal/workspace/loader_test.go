package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderLoadsKnownWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("agent rules"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte("persona"), 0o644); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "IGNORED.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write IGNORED.md: %v", err)
	}

	loader := NewLoader(root)
	ctx, err := loader.Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	if len(ctx.Files) != 2 {
		t.Fatalf("file count = %d, want 2", len(ctx.Files))
	}
	if ctx.Files[0].Name != "AGENTS.md" {
		t.Fatalf("file 0 = %q, want %q", ctx.Files[0].Name, "AGENTS.md")
	}
	if ctx.Files[1].Name != "SOUL.md" {
		t.Fatalf("file 1 = %q, want %q", ctx.Files[1].Name, "SOUL.md")
	}
}
