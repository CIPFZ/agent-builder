package app

import (
	"path/filepath"
	"strings"
	"testing"

	"myclaw/internal/config"
	"myclaw/internal/permissions"
)

func TestBootstrapRuntimeNormalizesClaudeCodePermissionModes(t *testing.T) {
	root := t.TempDir()

	bootstrap, err := bootstrapRuntime(root, config.Config{
		LLM: config.LLMConfig{
			Provider: "openai-compatible",
			Model:    "sonnet",
		},
		Permissions: config.PermissionConfig{
			Mode:           "bypass-permissions",
			SubagentMode:   "dont-ask",
			WorkspaceRoots: []string{filepath.Join(root, "workspace")},
		},
	}, bootstrapOptions{})
	if err != nil {
		t.Fatalf("bootstrapRuntime returned unexpected error: %v", err)
	}

	if bootstrap.Policy.Mode != permissions.ModeBypassPermissions {
		t.Fatalf("policy mode = %q, want %q", bootstrap.Policy.Mode, permissions.ModeBypassPermissions)
	}
	if bootstrap.Policy.SubagentMode != permissions.ModeDontAsk {
		t.Fatalf("policy subagent mode = %q, want %q", bootstrap.Policy.SubagentMode, permissions.ModeDontAsk)
	}
	if got := bootstrap.Runner.BaseMainLoopModelForSession(bootstrap.Sessions.GetOrCreateMain("main").ID); got != "claude-sonnet-4-5" {
		t.Fatalf("base main loop model = %q, want resolved sonnet alias", got)
	}
}

func TestBootstrapRuntimeUsesFallbackWorkspaceRootsForDaemonStyleHosts(t *testing.T) {
	root := t.TempDir()

	bootstrap, err := bootstrapRuntime(root, config.Config{
		Permissions: config.PermissionConfig{
			Mode: "workspace-write",
		},
	}, bootstrapOptions{
		FallbackWorkspaceRoots: []string{"configs/workspace"},
	})
	if err != nil {
		t.Fatalf("bootstrapRuntime returned unexpected error: %v", err)
	}

	if got, want := len(bootstrap.Policy.WorkspaceRoots), 1; got != want {
		t.Fatalf("workspace root count = %d, want %d (%#v)", got, want, bootstrap.Policy.WorkspaceRoots)
	}
	wantRoot := strings.ReplaceAll(filepath.Clean(filepath.Join(root, "configs", "workspace")), "\\", "/")
	if bootstrap.Policy.WorkspaceRoots[0] != wantRoot {
		t.Fatalf("workspace root = %q, want resolved fallback root", bootstrap.Policy.WorkspaceRoots[0])
	}
}
