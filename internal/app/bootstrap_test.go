package app

import (
	"net/http"
	"net/http/httptest"
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

func TestBootstrapRuntimePassesConfiguredMCPServersIntoRunner(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer authorization_uri="https://auth.example/authorize"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	bootstrap, err := bootstrapRuntime(root, config.Config{
		LLM: config.LLMConfig{
			Provider: "openai-compatible",
			Model:    "sonnet",
		},
		Permissions: config.PermissionConfig{
			Mode: "workspace-write",
		},
		MCP: config.MCPConfig{
			Enabled: true,
			Skills:  true,
			Servers: []config.MCPServerConfig{
				{
					Name:    "filesystem",
					Type:    "streamable_http",
					BaseURL: server.URL,
					Enabled: true,
				},
				{
					Name:    "disabled",
					Type:    "streamable_http",
					BaseURL: server.URL,
					Enabled: false,
				},
			},
		},
	}, bootstrapOptions{})
	if err != nil {
		t.Fatalf("bootstrapRuntime returned unexpected error: %v", err)
	}

	servers := bootstrap.Runner.MCPServers()
	if len(servers) != 1 {
		t.Fatalf("mcp servers = %#v, want one enabled configured server", servers)
	}
	if servers[0].Name != "filesystem" {
		t.Fatalf("server name = %q, want filesystem", servers[0].Name)
	}
	if servers[0].Endpoint != server.URL {
		t.Fatalf("server endpoint = %q, want bootstrap MCP base URL", servers[0].Endpoint)
	}
}

func TestBootstrapRuntimeCanDisableMCPStartup(t *testing.T) {
	root := t.TempDir()

	bootstrap, err := bootstrapRuntime(root, config.Config{
		LLM: config.LLMConfig{
			Provider: "openai-compatible",
			Model:    "sonnet",
		},
		Permissions: config.PermissionConfig{
			Mode: "workspace-write",
		},
		MCP: config.MCPConfig{
			Enabled: true,
			Servers: []config.MCPServerConfig{
				{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "fake-mcp.cmd",
					Enabled: true,
				},
			},
		},
	}, bootstrapOptions{
		DisableMCP: true,
	})
	if err != nil {
		t.Fatalf("bootstrapRuntime returned unexpected error: %v", err)
	}

	if servers := bootstrap.Runner.MCPServers(); len(servers) != 0 {
		t.Fatalf("mcp servers = %#v, want MCP disabled during bootstrap", servers)
	}
}
