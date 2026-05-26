package prompt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
)

func TestLoadContextSourcesPrecedenceAndDiscovery(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "project instructions")
	writeFile(t, filepath.Join(workspace, "CLAUDE.md"), "claude project")
	writeFile(t, filepath.Join(workspace, "AGENTS.local.md"), "local override")
	writeFile(t, filepath.Join(workspace, ".claude", "CLAUDE.md"), "dot claude")
	writeFile(t, filepath.Join(workspace, ".claude", "rules", "style.md"), "rule instructions")
	store := testStore(workspace, []string{"AGENTS.local.md", ".claude/rules/", "CLAUDE.md", "AGENTS.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	var loaded []string
	for _, source := range load.Sources {
		if source.State == ContextStateLoaded && source.Path != "" {
			loaded = append(loaded, filepath.Base(source.Path))
		}
	}
	want := []string{"AGENTS.md", "CLAUDE.md", "CLAUDE.md", "style.md", "AGENTS.local.md"}
	if !slices.Equal(loaded, want) {
		t.Fatalf("loaded order = %#v, want %#v", loaded, want)
	}
	if len(load.ContextFiles) != len(want) {
		t.Fatalf("context files = %#v", load.ContextFiles)
	}
}

func TestLoadContextSourcesDiscoversUpwardInstructions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "repo", "pkg", "feature")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "repo", "AGENTS.md"), "root agents")
	writeFile(t, filepath.Join(root, "repo", "pkg", "CLAUDE.md"), "pkg claude")
	writeFile(t, filepath.Join(root, "repo", "pkg", ".claude", "rules", "go.md"), "go rules")
	store := testStore(workspace, nil)

	load := LoadContextSources(context.Background(), store, nil, nil)
	assertLoadedPath(t, load, filepath.Join(root, "repo", "AGENTS.md"), ContextSourceProjectAgents)
	assertLoadedPath(t, load, filepath.Join(root, "repo", "pkg", "CLAUDE.md"), ContextSourceProjectClaude)
	assertLoadedPath(t, load, filepath.Join(root, "repo", "pkg", ".claude", "rules", "go.md"), ContextSourceClaudeRule)
}

func TestLoadContextSourcesIncludeSuccessAndMissingDiagnostic(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "see @./docs/included.md and @./missing.md")
	writeFile(t, filepath.Join(workspace, "docs", "included.md"), "included instructions")
	store := testStore(workspace, []string{"AGENTS.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	assertLoadedPath(t, load, filepath.Join(workspace, "docs", "included.md"), ContextSourceFile)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool {
		return source.Reason == "missing" && strings.HasSuffix(filepath.ToSlash(source.Path), "/missing.md") && source.ParentID != ""
	}) {
		t.Fatalf("missing include diagnostic not found: %#v", load.Sources)
	}
}

func TestLoadContextSourcesIncludeCycleDetection(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "@./a.md")
	writeFile(t, filepath.Join(workspace, "a.md"), "@./AGENTS.md")
	store := testStore(workspace, []string{"AGENTS.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool {
		return source.Reason == "include_cycle"
	}) {
		t.Fatalf("include cycle diagnostic missing: %#v", load.Sources)
	}
}

func TestLoadContextSourcesIncludeDepthLimit(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "@./1.md")
	for i := 1; i <= maxContextIncludeDepth+2; i++ {
		next := ""
		if i <= maxContextIncludeDepth+1 {
			next = fmt.Sprintf("@./%d.md", i+1)
		}
		writeFile(t, filepath.Join(workspace, fmt.Sprintf("%d.md", i)), next)
	}
	store := testStore(workspace, []string{"AGENTS.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool {
		return source.Reason == "include_depth_exceeded"
	}) {
		t.Fatalf("include depth diagnostic missing: %#v", load.Sources)
	}
}

func TestLoadContextSourcesGuardsLargeAndBinaryFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	largePath := filepath.Join(workspace, "large.md")
	binaryPath := filepath.Join(workspace, "binary.md")
	writeFile(t, largePath, strings.Repeat("x", maxContextFileBytes+1))
	if err := os.WriteFile(binaryPath, []byte{0xff, 0x00, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	store := testStore(workspace, []string{"large.md", "binary.md"})

	load := LoadContextSources(context.Background(), store, nil, nil)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool { return source.Reason == "file_too_large" }) {
		t.Fatalf("large file diagnostic missing: %#v", load.Sources)
	}
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool { return source.Reason == "binary_or_non_utf8" }) {
		t.Fatalf("binary diagnostic missing: %#v", load.Sources)
	}
}

func TestLoadContextSourcesFrontmatterRulePathMatching(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	match := filepath.Join(workspace, ".claude", "rules", "match.md")
	skip := filepath.Join(workspace, ".claude", "rules", "skip.md")
	writeFile(t, match, "---\npaths: .claude/rules/*.md\n---\nmatched")
	writeFile(t, skip, "---\npaths: docs/**\n---\nskipped")
	store := testStore(workspace, nil)

	load := LoadContextSources(context.Background(), store, nil, nil)
	assertLoadedPath(t, load, match, ContextSourceClaudeRule)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool {
		return filepath.Clean(source.Path) == filepath.Clean(filepath.ToSlash(skip)) && source.State == ContextStateDisabled && source.Reason == "frontmatter_path_not_matched"
	}) {
		t.Fatalf("frontmatter skip diagnostic missing: %#v", load.Sources)
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

func assertLoadedPath(t *testing.T, load ContextLoadResult, path string, kind ContextSourceKind) {
	t.Helper()
	wantPath := filepath.ToSlash(path)
	if !slices.ContainsFunc(load.Sources, func(source ContextSource) bool {
		return source.Path == wantPath && source.Kind == kind && source.State == ContextStateLoaded
	}) {
		t.Fatalf("loaded source %s %s not found: %#v", kind, wantPath, load.Sources)
	}
}
