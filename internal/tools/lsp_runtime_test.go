package tools

import (
	"context"
	"strings"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

func TestLSPServerConfigNormalizesCoverageAndClassification(t *testing.T) {
	cfg := NormalizeLSPServerConfig(LSPServerConfig{
		Name:                 "  gopls  ",
		LanguageIDs:          []string{"go", "go", "gomod"},
		FilePatterns:         []string{"**/*.go", "**/*.go", "go.mod"},
		Command:              "gopls",
		Args:                 []string{"serve"},
		Env:                  map[string]string{"GOPATH": "/tmp/go"},
		CWD:                  " C:/repo ",
		WorkspaceRoot:        " C:/repo ",
		Enabled:              true,
		Capabilities:         []string{"definition", "diagnostics", "definition"},
		ReadOnlyCapabilities: []string{"definition", "diagnostics"},
		MutatingCapabilities: []string{"rename"},
	})

	if cfg.Name != "gopls" || cfg.Source != "lsp" || cfg.Version != "" {
		t.Fatalf("normalized identity = %#v", cfg)
	}
	if got := strings.Join(cfg.LanguageIDs, ","); got != "go,gomod" {
		t.Fatalf("language ids = %q, want stable dedupe", got)
	}
	if got := strings.Join(cfg.FilePatterns, ","); got != "**/*.go,go.mod" {
		t.Fatalf("file patterns = %q, want stable dedupe", got)
	}
	if got := strings.Join(cfg.Capabilities, ","); got != "definition,diagnostics" {
		t.Fatalf("capabilities = %q, want stable dedupe", got)
	}
	if got := strings.Join(cfg.ReadOnlyCapabilities, ","); got != "definition,diagnostics" {
		t.Fatalf("readonly capabilities = %q", got)
	}
	if got := strings.Join(cfg.MutatingCapabilities, ","); got != "rename" {
		t.Fatalf("mutating capabilities = %q", got)
	}
}

func TestLSPStateNormalizationPreservesSpecificRuntimeStates(t *testing.T) {
	for _, state := range []string{
		LSPStateDiscovered,
		LSPStateConfigured,
		LSPStateStarting,
		LSPStateActive,
		LSPStateDegraded,
		LSPStateFailed,
		LSPStateDisabled,
		LSPStateStopped,
	} {
		if got := NormalizeLSPState(state); got != state {
			t.Fatalf("NormalizeLSPState(%q) = %q", state, got)
		}
	}
	if got := NormalizeLSPState(ExtensionStateLoaded); got != LSPStateConfigured {
		t.Fatalf("loaded maps to %q, want configured", got)
	}
	if got := NormalizeLSPState(ExtensionStateUnloaded); got != LSPStateStopped {
		t.Fatalf("unloaded maps to %q, want stopped", got)
	}
}

func TestLSPReadOnlyToolContractsArePermissionClassified(t *testing.T) {
	registry := NewRegistry(NewLSPTools(nil, []LSPServerConfig{{Name: "gopls", Enabled: true}})...)

	contracts := registry.Contracts(ContractOptions{
		Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	for _, name := range []string{"lsp_symbol_search", "lsp_definition", "lsp_references", "lsp_diagnostics"} {
		contract, ok := findLSPContract(contracts, name)
		if !ok {
			t.Fatalf("contract %q missing from %#v", name, contracts)
		}
		if !contract.ReadOnly || contract.Destructive || !contract.ShouldDefer || !contract.AlwaysLoad {
			t.Fatalf("contract %q classification = %#v, want readonly deferred always-load", name, contract)
		}
		if contract.Source != "lsp" || contract.InputSchema["type"] != "object" {
			t.Fatalf("contract %q metadata = %#v", name, contract)
		}
	}

	denied := registry.Contracts(ContractOptions{
		Policy: permissions.Policy{
			Mode:  permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{ToolName: "lsp_definition", Action: permissions.ActionDeny}},
		},
	})
	if _, ok := findLSPContract(denied, "lsp_definition"); ok {
		t.Fatalf("denied lsp_definition remained exposed: %#v", denied)
	}
}

func TestLSPToolWithoutHandlerReturnsExplicitUnavailable(t *testing.T) {
	registry := NewRegistry(NewLSPTools(nil, []LSPServerConfig{{Name: "gopls", Enabled: true}})...)

	_, err := registry.InvokeWithContext(context.Background(), ToolUseContext{
		Session:  session.Session{ID: "s1"},
		ToolName: "lsp_definition",
		InputObject: map[string]any{
			"server": "gopls",
			"path":   "main.go",
			"line":   1,
			"column": 1,
		},
		Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	if err == nil || !strings.Contains(err.Error(), "LSP runtime unavailable") {
		t.Fatalf("lsp tool err = %v, want explicit unavailable", err)
	}
}

func findLSPContract(contracts []Contract, name string) (Contract, bool) {
	for _, contract := range contracts {
		if contract.Name == name {
			return contract, true
		}
	}
	return Contract{}, false
}
