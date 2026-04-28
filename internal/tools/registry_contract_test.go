package tools

import (
	"context"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

func TestRegistryContractsExposeStableToolIdentityAndClassification(t *testing.T) {
	registry := NewRegistry(&contractProbeTool{})

	contracts := registry.Contracts(ContractOptions{
		Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	if len(contracts) != 1 {
		t.Fatalf("expected one contract, got %d", len(contracts))
	}
	contract := contracts[0]
	if contract.Name != "Probe" {
		t.Fatalf("expected canonical name Probe, got %q", contract.Name)
	}
	if len(contract.Aliases) != 1 || contract.Aliases[0] != "probe.alias" {
		t.Fatalf("aliases were not preserved: %#v", contract.Aliases)
	}
	if contract.Source != "builtin" {
		t.Fatalf("expected default source builtin, got %q", contract.Source)
	}
	if !contract.ReadOnly || contract.Destructive {
		t.Fatalf("expected readonly non-destructive classification, got readonly=%v destructive=%v", contract.ReadOnly, contract.Destructive)
	}
	if !contract.ShouldDefer || !contract.AlwaysLoad {
		t.Fatalf("expected deferred always-load metadata, got defer=%v always=%v", contract.ShouldDefer, contract.AlwaysLoad)
	}
	if contract.InputSchema["type"] != "object" {
		t.Fatalf("schema was not exposed: %#v", contract.InputSchema)
	}

	contract.InputSchema["type"] = "mutated"
	contracts = registry.Contracts(ContractOptions{Policy: permissions.Policy{Mode: permissions.ModeDangerFullAccess}})
	if contracts[0].InputSchema["type"] != "object" {
		t.Fatalf("contract schema was not returned as an isolated copy: %#v", contracts[0].InputSchema)
	}
}

func TestRegistryContractsRespectPolicyAndDeferredExposure(t *testing.T) {
	registry := NewRegistry(&contractProbeTool{})

	contracts := registry.Contracts(ContractOptions{
		Policy: permissions.Policy{
			Mode:  permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{ToolName: "Probe", Action: permissions.ActionDeny}},
		},
	})
	if len(contracts) != 0 {
		t.Fatalf("expected denied tool contract to be hidden, got %#v", contracts)
	}

	contracts = registry.Contracts(ContractOptions{
		Policy:          permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		IncludeDeferred: false,
	})
	if len(contracts) != 1 {
		t.Fatalf("always-load deferred tool should remain visible, got %d", len(contracts))
	}
}

type contractProbeTool struct{}

func (t *contractProbeTool) Definition() Definition {
	return Definition{
		Name:        "Probe",
		Aliases:     []string{"probe.alias"},
		Description: "runtime contract probe",
		InputSchema: map[string]any{"type": "object"},
		ReadOnly:    true,
		ShouldDefer: true,
		AlwaysLoad:  true,
	}
}

func (t *contractProbeTool) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	return "ok", nil
}

func (t *contractProbeTool) IsEnabled() bool           { return true }
func (t *contractProbeTool) IsReadOnly(string) bool    { return true }
func (t *contractProbeTool) IsDestructive(string) bool { return false }
func (t *contractProbeTool) ShouldDefer() bool         { return true }
func (t *contractProbeTool) AlwaysLoad() bool          { return true }
func (t *contractProbeTool) PromptDescription() string { return "runtime contract probe" }
func (t *contractProbeTool) SearchHint() string        { return "probe search" }
