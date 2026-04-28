package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderLoadsKnownWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("project instructions"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
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

	if len(ctx.Files) != 3 {
		t.Fatalf("file count = %d, want 3", len(ctx.Files))
	}
	if ctx.Files[0].Name != "CLAUDE.md" {
		t.Fatalf("file 0 = %q, want %q", ctx.Files[0].Name, "CLAUDE.md")
	}
	if ctx.Files[1].Name != "AGENTS.md" {
		t.Fatalf("file 1 = %q, want %q", ctx.Files[1].Name, "AGENTS.md")
	}
	if ctx.Files[2].Name != "SOUL.md" {
		t.Fatalf("file 2 = %q, want %q", ctx.Files[2].Name, "SOUL.md")
	}
}

func TestLoaderLoadsInstructionFilesFromGitRootToCurrentDirectoryInPriorityOrder(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	currentDir := filepath.Join(projectRoot, "services", "api")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(projectRoot, ".claude"),
		filepath.Join(projectRoot, ".claude", "rules"),
		filepath.Join(projectRoot, "services", ".claude"),
		filepath.Join(currentDir, ".claude"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writes := map[string]string{
		filepath.Join(projectRoot, "CLAUDE.md"):                        "project root",
		filepath.Join(projectRoot, ".claude", "CLAUDE.md"):             "project dot claude",
		filepath.Join(projectRoot, ".claude", "rules", "root-rule.md"): "project root rule",
		filepath.Join(projectRoot, "CLAUDE.local.md"):                  "project local",
		filepath.Join(projectRoot, "services", "CLAUDE.md"):            "services project",
		filepath.Join(projectRoot, "services", ".claude", "CLAUDE.md"): "services dot claude",
		filepath.Join(currentDir, "CLAUDE.md"):                         "api project",
		filepath.Join(currentDir, ".claude", "CLAUDE.md"):              "api dot claude",
		filepath.Join(currentDir, "CLAUDE.local.md"):                   "api local",
	}
	for path, content := range writes {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	loader := NewLoader(currentDir)
	ctx, err := loader.Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	got := make([]string, 0, len(ctx.Files))
	for _, file := range ctx.Files {
		got = append(got, file.Content)
	}
	want := []string{
		"project root",
		"project dot claude",
		"project root rule",
		"project local",
		"services project",
		"services dot claude",
		"api project",
		"api dot claude",
		"api local",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("instruction order = %#v, want %#v", got, want)
	}
}

func TestLoaderLoadsRecursiveRulesBeforeLocalInstructions(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	rulesDir := filepath.Join(projectRoot, ".claude", "rules", "nested")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	writes := map[string]string{
		filepath.Join(projectRoot, "CLAUDE.md"):                "project",
		filepath.Join(projectRoot, ".claude", "rules", "a.md"): "rule a",
		filepath.Join(rulesDir, "b.md"):                        "rule b",
		filepath.Join(projectRoot, "CLAUDE.local.md"):          "local",
	}
	for path, content := range writes {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	loader := NewLoader(projectRoot)
	ctx, err := loader.Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	got := make([]string, 0, len(ctx.Files))
	for _, file := range ctx.Files {
		got = append(got, file.Content)
	}
	want := []string{"project", "rule a", "rule b", "local"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("instruction order = %#v, want %#v", got, want)
	}
}

func TestLoaderLoadsManagedAndUserInstructionsBeforeProjectInstructions(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	userDir := filepath.Join(root, "user")
	managedDir := filepath.Join(root, "managed")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	for _, dir := range []string{userDir, managedDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writes := map[string]string{
		filepath.Join(managedDir, "CLAUDE.md"):  "managed",
		filepath.Join(userDir, "CLAUDE.md"):     "user",
		filepath.Join(projectRoot, "CLAUDE.md"): "project",
	}
	for path, content := range writes {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	loader := NewLoader(projectRoot, WithManagedInstructionDir(managedDir), WithUserInstructionDir(userDir))
	ctx, err := loader.Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	got := make([]string, 0, len(ctx.Files))
	for _, file := range ctx.Files {
		got = append(got, file.Content)
	}
	want := []string{"managed", "user", "project"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("instruction order = %#v, want %#v", got, want)
	}
}

func TestLoaderRecordsDeterministicInstructionFingerprints(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	currentDir := filepath.Join(projectRoot, "packages", "api")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(currentDir, ".claude", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	writes := map[string]string{
		filepath.Join(projectRoot, "CLAUDE.md"):                    "root",
		filepath.Join(currentDir, "CLAUDE.md"):                     "api",
		filepath.Join(currentDir, ".claude", "rules", "z-rule.md"): "z rule",
		filepath.Join(currentDir, ".claude", "rules", "a-rule.md"): "a rule",
		filepath.Join(currentDir, "CLAUDE.local.md"):               "local",
	}
	for path, content := range writes {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	first, err := NewLoader(currentDir).Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	second, err := NewLoader(currentDir).Load()
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}

	if first.Fingerprint == "" {
		t.Fatal("workspace fingerprint is empty")
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint changed across identical loads: %q != %q", first.Fingerprint, second.Fingerprint)
	}
	gotNames := make([]string, 0, len(first.Files))
	for _, file := range first.Files {
		gotNames = append(gotNames, filepath.ToSlash(strings.TrimPrefix(file.Path, projectRoot+string(os.PathSeparator))))
		if file.Hash == "" || file.Size <= 0 || file.ModTime.IsZero() || file.Fingerprint == "" {
			t.Fatalf("file metadata not populated for %#v", file)
		}
	}
	wantNames := []string{
		"CLAUDE.md",
		"packages/api/CLAUDE.md",
		"packages/api/.claude/rules/a-rule.md",
		"packages/api/.claude/rules/z-rule.md",
		"packages/api/CLAUDE.local.md",
	}
	if strings.Join(gotNames, "|") != strings.Join(wantNames, "|") {
		t.Fatalf("instruction path order = %#v, want %#v", gotNames, wantNames)
	}

	if err := os.WriteFile(filepath.Join(currentDir, "CLAUDE.md"), []byte("api changed"), 0o644); err != nil {
		t.Fatalf("rewrite api CLAUDE.md: %v", err)
	}
	changed, err := NewLoader(currentDir).Load()
	if err != nil {
		t.Fatalf("reload changed workspace: %v", err)
	}
	if changed.Fingerprint == first.Fingerprint {
		t.Fatalf("fingerprint did not change after instruction content changed: %q", changed.Fingerprint)
	}
}

func TestLoaderResolvesIncludeFilesBeforeIncludingFileAndPreventsCycles(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "includes"), 0o755); err != nil {
		t.Fatalf("mkdir includes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "includes", "shared.md"), []byte("shared rule"), 0o644); err != nil {
		t.Fatalf("write shared: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "cycle.md"), []byte("@./CLAUDE.md\ncycle leaf"), 0o644); err != nil {
		t.Fatalf("write cycle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "CLAUDE.md"), []byte("@./includes/shared.md\n@./cycle.md\nproject rule"), 0o644); err != nil {
		t.Fatalf("write claude: %v", err)
	}

	loader := NewLoader(projectRoot)
	ctx, err := loader.Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	got := make([]string, 0, len(ctx.Files))
	for _, file := range ctx.Files {
		got = append(got, file.Content)
	}
	want := []string{"shared rule", "cycle leaf", "project rule"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("instruction order = %#v, want %#v", got, want)
	}
}
