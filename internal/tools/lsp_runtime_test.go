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

func TestLSPPermissionClassificationCoversExplicitCapabilityKinds(t *testing.T) {
	tests := []struct {
		name string
		cfg  LSPServerConfig
		want string
	}{
		{
			name: "read only",
			cfg:  LSPServerConfig{Name: "readonly", ReadOnlyCapabilities: []string{"definition", "diagnostics"}},
			want: LSPPermissionReadOnly,
		},
		{
			name: "mutating",
			cfg:  LSPServerConfig{Name: "mutating", MutatingCapabilities: []string{"rename"}},
			want: LSPPermissionMutating,
		},
		{
			name: "mixed",
			cfg:  LSPServerConfig{Name: "mixed", ReadOnlyCapabilities: []string{"definition"}, MutatingCapabilities: []string{"rename"}},
			want: LSPPermissionMixed,
		},
		{
			name: "empty defaults read only",
			cfg:  LSPServerConfig{Name: "empty"},
			want: LSPPermissionReadOnly,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LSPPermissionClassification(tc.cfg); got != tc.want {
				t.Fatalf("LSPPermissionClassification(%#v) = %q, want %q", tc.cfg, got, tc.want)
			}
		})
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

func TestLSPToolsRequireEnabledServersForContracts(t *testing.T) {
	disabledOnly := NewRegistry(NewLSPTools(nil, []LSPServerConfig{{Name: "gopls", Enabled: false}})...)
	if contracts := disabledOnly.Contracts(ContractOptions{Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess}}); len(contracts) != 0 {
		t.Fatalf("disabled-only LSP contracts = %#v, want none exposed", contracts)
	}

	mixed := NewRegistry(NewLSPTools(nil, []LSPServerConfig{
		{Name: "disabled", Enabled: false},
		{Name: "enabled", Enabled: true},
	})...)
	contracts := mixed.Contracts(ContractOptions{Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess}})
	if _, ok := findLSPContract(contracts, "lsp_definition"); !ok {
		t.Fatalf("mixed enabled/disabled LSP contracts = %#v, want enabled LSP tools", contracts)
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
