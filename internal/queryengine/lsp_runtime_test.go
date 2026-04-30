package queryengine

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/session"
	storememory "myclaw/internal/store/memory"
	"myclaw/internal/tools"
)

func TestLSPInventoryProjectsConfiguredServers(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		LSPServers: []tools.LSPServerConfig{{
			Name:                 "gopls",
			LanguageIDs:          []string{"go"},
			FilePatterns:         []string{"**/*.go"},
			Command:              "gopls",
			Args:                 []string{"serve"},
			WorkspaceRoot:        "C:/repo",
			Enabled:              true,
			Capabilities:         []string{"definition", "diagnostics"},
			ReadOnlyCapabilities: []string{"definition", "diagnostics"},
			MutatingCapabilities: []string{"rename"},
		}},
	})

	inventory := engine.ExtensionInventory(sess.ID)
	if len(inventory.LSPBoundaries) != 1 {
		t.Fatalf("lsp boundaries = %#v, want configured server boundary", inventory.LSPBoundaries)
	}
	boundary := inventory.LSPBoundaries[0]
	if boundary.Name != "gopls" || boundary.Status != tools.LSPStateConfigured || boundary.LifecycleState != tools.LSPStateConfigured {
		t.Fatalf("lsp boundary = %#v, want configured gopls", boundary)
	}
	if !reflect.DeepEqual(boundary.LanguageIDs, []string{"go"}) || !reflect.DeepEqual(boundary.FilePatterns, []string{"**/*.go"}) {
		t.Fatalf("lsp coverage = %#v/%#v", boundary.LanguageIDs, boundary.FilePatterns)
	}
	if boundary.PermissionClassification != "read_only" || !reflect.DeepEqual(boundary.ReadOnlyCapabilities, []string{"definition", "diagnostics"}) {
		t.Fatalf("lsp permission projection = %#v", boundary)
	}
	if boundary.Command != "gopls serve" || boundary.WorkspaceRoot != "C:/repo" || !boundary.Enabled {
		t.Fatalf("lsp runtime projection = %#v", boundary)
	}
}

func TestLSPLifecycleDisableEnableAndRecovery(t *testing.T) {
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	options := Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		LSPServers: []tools.LSPServerConfig{{
			Name:        "gopls",
			LanguageIDs: []string{"go"},
			Enabled:     true,
		}},
	}
	engine := New(options)
	target := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeLSPBoundary, Source: "lsp", Name: "gopls"}
	if _, err := engine.DisableExtension(target); err != nil {
		t.Fatalf("disable lsp: %v", err)
	}

	recoveredManager := session.NewManager(store)
	recoveredSession, ok := recoveredManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("recovered session missing")
	}
	recovered := New(Config{
		Sessions:   recoveredManager,
		Client:     llm.NewMockClient(),
		LSPServers: options.LSPServers,
	})
	boundary, ok := findLSPBoundary(recovered.ExtensionInventory(recoveredSession.ID).LSPBoundaries, "gopls")
	if !ok || boundary.LifecycleState != tools.ExtensionStateDisabled || boundary.Status != tools.ExtensionStateDisabled {
		t.Fatalf("recovered disabled lsp = %#v, found=%v", boundary, ok)
	}

	if _, err := recovered.EnableExtension(target); err != nil {
		t.Fatalf("enable lsp: %v", err)
	}
	enabledManager := session.NewManager(store)
	enabledSession, ok := enabledManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("enabled session missing")
	}
	enabled := New(Config{
		Sessions:   enabledManager,
		Client:     llm.NewMockClient(),
		LSPServers: options.LSPServers,
	})
	boundary, ok = findLSPBoundary(enabled.ExtensionInventory(enabledSession.ID).LSPBoundaries, "gopls")
	if !ok || boundary.LifecycleState == tools.ExtensionStateDisabled {
		t.Fatalf("enabled recovered lsp = %#v, found=%v", boundary, ok)
	}
}

func TestLSPFailedAndDegradedOverlayPersists(t *testing.T) {
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	servers := []tools.LSPServerConfig{{Name: "gopls", Enabled: true}, {Name: "tsserver", Enabled: true}}
	engine := New(Config{Sessions: manager, Client: llm.NewMockClient(), LSPServers: servers})

	if _, err := engine.MarkExtensionDegraded(tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeLSPBoundary, Source: "lsp", Name: "gopls"}, "index stale"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}
	if _, err := engine.MarkExtensionFailed(tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeLSPBoundary, Source: "lsp", Name: "tsserver"}, "server crashed"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	recoveredManager := session.NewManager(store)
	recoveredSession, ok := recoveredManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("recovered session missing")
	}
	recovered := New(Config{Sessions: recoveredManager, Client: llm.NewMockClient(), LSPServers: servers})
	gopls, ok := findLSPBoundary(recovered.ExtensionInventory(recoveredSession.ID).LSPBoundaries, "gopls")
	if !ok || gopls.LifecycleState != tools.ExtensionStateDegraded || gopls.LastError != "index stale" {
		t.Fatalf("recovered gopls = %#v, found=%v", gopls, ok)
	}
	tsserver, ok := findLSPBoundary(recovered.ExtensionInventory(recoveredSession.ID).LSPBoundaries, "tsserver")
	if !ok || tsserver.LifecycleState != tools.ExtensionStateFailed || tsserver.LastError != "server crashed" {
		t.Fatalf("recovered tsserver = %#v, found=%v", tsserver, ok)
	}
}

func TestLSPInventoryRebuildsDeterministically(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	cfg := Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		LSPServers: []tools.LSPServerConfig{
			{Name: "tsserver", LanguageIDs: []string{"typescript"}, Enabled: true},
			{Name: "gopls", LanguageIDs: []string{"go"}, Enabled: true},
		},
	}
	first := New(cfg)
	second := New(cfg)
	if !reflect.DeepEqual(first.ExtensionInventory(sess.ID).LSPBoundaries, second.ExtensionInventory(sess.ID).LSPBoundaries) {
		t.Fatalf("lsp inventory changed across rebuild:\nfirst=%#v\nsecond=%#v", first.ExtensionInventory(sess.ID).LSPBoundaries, second.ExtensionInventory(sess.ID).LSPBoundaries)
	}
}

func TestLSPReloadUnsupportedIsExplicit(t *testing.T) {
	engine := New(Config{
		Client:     llm.NewMockClient(),
		LSPServers: []tools.LSPServerConfig{{Name: "gopls", Enabled: true}},
	})
	_, err := engine.ReloadExtension(context.Background(), tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeLSPBoundary, Source: "lsp", Name: "gopls"})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("reload lsp err = %v, want explicit unsupported", err)
	}
}

func TestDisabledLSPToolExecutionIsBlockedByLifecycle(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions:   manager,
		Client:     llm.NewMockClient(),
		LSPServers: []tools.LSPServerConfig{{Name: "gopls", Enabled: true}},
		LSPHandler: lspSuccessHandler{},
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:   tools.ExtensionTypeLSPBoundary,
			Source: "lsp",
			Name:   "gopls",
			State:  tools.ExtensionStateDisabled,
		}},
	})
	input, _ := json.Marshal(map[string]any{"server": "gopls", "path": "main.go", "line": 1, "column": 1})
	_, err := engine.tools.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  sess,
		ToolName: "lsp_definition",
		Input:    string(input),
		Policy:   engine.PermissionPolicyForSession(sess.ID),
	})
	if err == nil || !strings.Contains(err.Error(), "disabled by extension lifecycle state") {
		t.Fatalf("disabled lsp tool err = %v, want lifecycle disabled", err)
	}
}

func findLSPBoundary(boundaries []ExtensionBoundary, name string) (ExtensionBoundary, bool) {
	for _, boundary := range boundaries {
		if boundary.Name == name {
			return boundary, true
		}
	}
	return ExtensionBoundary{}, false
}

type lspSuccessHandler struct{}

func (lspSuccessHandler) HandleLSPRequest(context.Context, tools.LSPRequest) (tools.ToolResult, error) {
	return tools.ToolResult{Output: "ok"}, nil
}
