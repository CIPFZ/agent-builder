package permissions_test

import (
	"testing"

	"myclaw/internal/permissions"
)

func TestPolicyRuleDenyOverridesDangerMode(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionDeny,
				Match: permissions.Match{
					CommandContains: []string{"rm -rf"},
				},
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "rm -rf /tmp/demo",
		WorkDir:  "/tmp",
	})

	if decision.Allowed {
		t.Fatalf("expected deny rule to override full access, got %#v", decision)
	}
	if decision.RequiresApproval {
		t.Fatalf("expected deny rule to hard deny instead of approval, got %#v", decision)
	}
}

func TestPolicyRuleAllowByPathOverridesWorkspacePrompt(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
				Match: permissions.Match{
					WorkDirPrefixes: []string{"/workspace/safe"},
				},
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "go test ./...",
		WorkDir:  "/workspace/safe/project",
	})

	if !decision.Allowed {
		t.Fatalf("expected allow rule to permit safe path, got %#v", decision)
	}
}

func TestPolicyRuleDenyToolAlwaysBlocks(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		Rules: []permissions.Rule{
			{
				ToolName: "subagent.spawn",
				Action:   permissions.ActionDeny,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "subagent.spawn",
		Command:  "spawn background worker",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected hard deny for subagent.spawn rule, got %#v", decision)
	}
}

func TestPolicyDangerousCommandRequiresApprovalInsideWorkspace(t *testing.T) {
	policy := permissions.Policy{
		Mode:                     permissions.ModeWorkspaceWrite,
		WorkspaceRoots:           []string{"/workspace/project"},
		DangerousCommandPatterns: []string{"rm -rf"},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "rm -rf ./build",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed {
		t.Fatalf("expected dangerous command to stop for approval, got %#v", decision)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected dangerous command to require approval, got %#v", decision)
	}
}
