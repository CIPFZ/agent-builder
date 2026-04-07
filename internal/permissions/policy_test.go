package permissions_test

import (
	"testing"

	"myclaw/internal/permissions"
)

func TestPolicyEvaluateAskModeRequiresApproval(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/tmp/project",
	})

	if decision.Allowed {
		t.Fatal("expected ask mode to block immediate execution")
	}
	if !decision.RequiresApproval {
		t.Fatal("expected ask mode to require approval")
	}
	if decision.Category != permissions.CategoryApproval {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryApproval)
	}
}

func TestPolicyEvaluateWorkspaceModeAllowsInsideWorkspace(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{"/workspace/project"},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "go test ./...",
		WorkDir:  "/workspace/project/internal",
	})

	if !decision.Allowed {
		t.Fatalf("expected workspace mode to allow in-root work, got %#v", decision)
	}
	if decision.RequiresApproval {
		t.Fatalf("expected workspace mode to avoid approval in-root, got %#v", decision)
	}
}

func TestPolicyEvaluateWorkspaceModeRequiresApprovalOutsideWorkspace(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{"/workspace/project"},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "rm -rf /tmp/elsewhere",
		WorkDir:  "/tmp/elsewhere",
	})

	if decision.Allowed {
		t.Fatalf("expected out-of-root execution to stop for approval, got %#v", decision)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected out-of-root execution to require approval, got %#v", decision)
	}
	if decision.Category != permissions.CategoryWorkspaceBoundary {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryWorkspaceBoundary)
	}
}

func TestPolicyEvaluateDangerFullAccessAllowsEverywhere(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "sudo whoami",
		WorkDir:  "/",
	})

	if !decision.Allowed {
		t.Fatalf("expected danger-full-access to allow command, got %#v", decision)
	}
	if decision.RequiresApproval {
		t.Fatalf("expected danger-full-access to skip approval, got %#v", decision)
	}
}

func TestPolicyEvaluateAllowsNonSystemToolsWithoutApproval(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "text.upper",
		Command:  "hello world",
		WorkDir:  "/tmp/project",
	})

	if !decision.Allowed {
		t.Fatalf("decision = %#v, want non-system tool to be allowed", decision)
	}
	if decision.RequiresApproval {
		t.Fatalf("decision = %#v, want non-system tool to avoid approval", decision)
	}
}

func TestPolicyEvaluateAskModeRequiresApprovalForDestructiveTool(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName:     "file.delete",
		Command:      "delete build output",
		Destructive:  true,
		ReadOnly:     false,
		WorkDir:      "/workspace/project",
	})

	if decision.Allowed {
		t.Fatalf("expected destructive tool to avoid immediate execution, got %#v", decision)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected destructive tool to require approval, got %#v", decision)
	}
}

func TestPolicyEvaluateWorkspaceModeAllowsDestructiveTool(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName:     "file.delete",
		Command:      "delete temp file",
		Destructive:  true,
		ReadOnly:     false,
		WorkDir:      "/workspace/project",
	})

	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected workspace-write mode to allow destructive non-system tool, got %#v", decision)
	}
}

func TestPolicyEvaluatePlanModeRequiresApprovalForSystemRun(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		PlanMode:       true,
		WorkspaceRoots: []string{"/workspace/project"},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "go test ./...",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed {
		t.Fatalf("expected plan mode to avoid immediate execution, got %#v", decision)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected plan mode to require approval, got %#v", decision)
	}
	if decision.Category != permissions.CategoryPlanMode {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryPlanMode)
	}
}

func TestPolicyEvaluatePlanModeStillAllowsNonSystemTools(t *testing.T) {
	policy := permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		PlanMode: true,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "text.upper",
		Command:  "hello",
		WorkDir:  "/workspace/project",
	})

	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected non-system tool to stay allowed in plan mode, got %#v", decision)
	}
}

func TestPolicyEvaluatePlanModeRequiresApprovalForDestructiveTool(t *testing.T) {
	policy := permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		PlanMode: true,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName:     "file.delete",
		Command:      "delete generated file",
		Destructive:  true,
		ReadOnly:     false,
		WorkDir:      "/workspace/project",
	})

	if decision.Allowed {
		t.Fatalf("expected plan mode destructive tool to avoid immediate execution, got %#v", decision)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected plan mode destructive tool to require approval, got %#v", decision)
	}
	if decision.Category != permissions.CategoryPlanMode {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryPlanMode)
	}
}

func TestPolicyEvaluateIncludesRuleSourceForRuleDenial(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Source:   string(permissions.RuleSourceSession),
				Action:   permissions.ActionDeny,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected rule deny to hard deny, got %#v", decision)
	}
	if decision.RuleSource != string(permissions.RuleSourceSession) {
		t.Fatalf("decision rule source = %q, want %q", decision.RuleSource, permissions.RuleSourceSession)
	}
}

func TestPolicyEvaluateIncludesRuleSourceForRuleAllow(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Source:   string(permissions.RuleSourceProject),
				Action:   permissions.ActionAllow,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected rule allow to win, got %#v", decision)
	}
	if decision.RuleSource != string(permissions.RuleSourceProject) {
		t.Fatalf("decision rule source = %q, want %q", decision.RuleSource, permissions.RuleSourceProject)
	}
}

func TestPolicyDeriveForSubagentDefaultsToSaferMode(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeDangerFullAccess,
		WorkspaceRoots: []string{"C:/repo"},
	}

	derived := policy.DeriveForSubagent()

	if derived.Mode != permissions.ModeWorkspaceWrite {
		t.Fatalf("derived mode = %q, want %q", derived.Mode, permissions.ModeWorkspaceWrite)
	}
	if len(derived.WorkspaceRoots) != 1 || derived.WorkspaceRoots[0] != "C:/repo" {
		t.Fatalf("derived workspace roots = %#v, want copied roots", derived.WorkspaceRoots)
	}
}

func TestPolicyDeriveForSubagentUsesExplicitOverride(t *testing.T) {
	policy := permissions.Policy{
		Mode:         permissions.ModeDangerFullAccess,
		SubagentMode: permissions.ModeAsk,
	}

	derived := policy.DeriveForSubagent()

	if derived.Mode != permissions.ModeAsk {
		t.Fatalf("derived mode = %q, want %q", derived.Mode, permissions.ModeAsk)
	}
}
