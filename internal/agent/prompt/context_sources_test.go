package prompt

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
)

func TestLoadContextSourcesPrecedenceAndDiscovery(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "project instructions")
	writeFile(t, filepath.Join(workspace, "CLAUDE.md"), "claude project")
	writeFile(t, filepath.Join(workspace, "AGENTS.local.md"), "local override")
	writeFile(t, filepath.Join(workspace, ".agents", "rules", "style.md"), "rule instructions")
	store := testStore(workspace, []string{"AGENTS.local.md", ".agents/rules/", "CLAUDE.md", "AGENTS.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	var loaded []string
	for _, source := range load.Sources {
		if source.State == ContextStateLoaded && source.Path != "" {
			loaded = append(loaded, filepath.Base(source.Path))
		}
	}
	want := []string{"AGENTS.md", "CLAUDE.md", "style.md", "AGENTS.local.md"}
	if !slices.Equal(loaded, want) {
		t.Fatalf("loaded order = %#v, want %#v", loaded, want)
	}
	if len(load.ContextFiles) != len(want) {
		t.Fatalf("context files = %#v", load.ContextFiles)
	}
}

func TestLoadContextSourcesMissingFileDoesNotFail(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := testStore(workspace, []string{"AGENTS.md"})
	load := LoadContextSources(context.Background(), store, nil, nil)
	if len(load.Sources) < 2 {
		t.Fatalf("sources = %#v", load.Sources)
	}
	missing := load.Sources[1]
	if missing.State != ContextStateUnavailable || missing.Reason != "missing" {
		t.Fatalf("missing source = %#v", missing)
	}
	if len(load.ContextFiles) != 0 {
		t.Fatalf("context files should be empty: %#v", load.ContextFiles)
	}
}

func TestLoadContextSourcesRejectsWorkspaceTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside.md")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, outside, "outside")
	store := testStore(workspace, []string{"../outside.md"})
	load := LoadContextSources(context.Background(), store, nil, nil)
	if len(load.Sources) < 2 {
		t.Fatalf("sources = %#v", load.Sources)
	}
	source := load.Sources[1]
	if source.State != ContextStateUnavailable || source.Reason != "outside_workspace" {
		t.Fatalf("source = %#v", source)
	}
	if source.Content != "" || len(load.ContextFiles) != 0 {
		t.Fatalf("outside content was loaded: %#v %#v", source, load.ContextFiles)
	}
}

func testStore(workspace string, contextPaths []string) *config.ConfigStore {
	cfg := config.NewRuntimeConfig(workspace, filepath.Join(workspace, ".crush"), false)
	cfg.Options.ContextPaths = contextPaths
	cfg.SetupAgents()
	return config.NewRuntimeStore(workspace, cfg)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
