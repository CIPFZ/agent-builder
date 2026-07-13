package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplicationDataLayoutUsesStableProjectAndSessionIDs(t *testing.T) {
	app, err := newApplicationDataLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := app.Project("project-id")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(app.Root, "projects", "project-id", "objects"); project.ObjectsDir != want {
		t.Fatalf("ObjectsDir = %q, want %q", project.ObjectsDir, want)
	}
	session, err := project.Session("session-id")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(project.Root, "sessions", "session-id", "tmp"); session.TempDir != want {
		t.Fatalf("TempDir = %q, want %q", session.TempDir, want)
	}
}

func TestDataLayoutRejectsUnsafeIDs(t *testing.T) {
	app, err := newApplicationDataLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", ".", "..", "../escape", `C:\\escape`, "a/b", `a\\b`} {
		if _, err := app.Project(id); err == nil {
			t.Fatalf("Project(%q) succeeded", id)
		}
	}
}

func TestEnsureProjectDataLayoutCreatesOwnedDirectories(t *testing.T) {
	app, err := newApplicationDataLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := app.Project("project-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectDataLayout(project); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{project.MemoryDir, project.SkillsDir, project.MCPDir, project.ObjectsDir, project.SessionsDir, project.WorktreesDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("directory %q was not created", dir)
		}
	}
}
