package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"myclaw/internal/tools"
)

func TestLoadClaudeSkillDirectoriesLoadsBundledAndPluginSources(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	root := t.TempDir()
	bundled := filepath.Join(root, "bundled")
	plugin := filepath.Join(root, "plugin")
	for _, dir := range []string{
		filepath.Join(bundled, "review"),
		filepath.Join(plugin, "deploy"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir skill: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(bundled, "review", "SKILL.md"), []byte("---\ndescription: Bundled review\n---\nReview bundled content."), 0o644); err != nil {
		t.Fatalf("write bundled skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "deploy", "SKILL.md"), []byte("---\ndescription: Plugin deploy\n---\nDeploy plugin content."), 0o644); err != nil {
		t.Fatalf("write plugin skill: %v", err)
	}

	loaded := tools.LoadClaudeSkillDirectories(tools.SkillDiscoveryOptions{
		IncludeBundled: true,
		IncludePlugins: true,
		BundledDirs:    []string{bundled},
		PluginDirs:     []string{plugin},
	})

	if len(loaded) != 2 {
		t.Fatalf("loaded = %#v, want bundled and plugin skills", loaded)
	}
	byName := map[string]tools.SkillCommand{}
	for _, skill := range loaded {
		byName[skill.Name] = skill
	}
	if byName["review"].LoadedFrom != "bundled" || byName["review"].Source != "bundled" {
		t.Fatalf("bundled skill = %#v, want bundled source metadata", byName["review"])
	}
	if byName["deploy"].LoadedFrom != "plugin" || byName["deploy"].Source != "plugin" {
		t.Fatalf("plugin skill = %#v, want plugin source metadata", byName["deploy"])
	}
}
