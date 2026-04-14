package permissions_test

import (
	"testing"

	"myclaw/internal/permissions"
)

func TestSetupPolicyNormalizesAndDedupesWorkspaceRoots(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			` C:\repo\project\ `,
			`C:\repo\project`,
			`C:\repo\project\sub`,
		},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	if got, want := len(policy.WorkspaceRoots), 1; got != want {
		t.Fatalf("workspace root count = %d, want %d (%#v)", got, want, policy.WorkspaceRoots)
	}
	if policy.WorkspaceRoots[0] != "C:/repo/project" {
		t.Fatalf("workspace root 0 = %q, want normalized root", policy.WorkspaceRoots[0])
	}
}

func TestSetupPolicyRequiresWorkspaceRootsForWorkspaceMode(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
	})
	if err == nil {
		t.Fatal("expected workspace-write mode without roots to fail")
	}
}

func TestSetupPolicyRejectsUnknownModes(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.Mode("surprise-mode"),
	})
	if err == nil {
		t.Fatal("expected unknown mode to fail")
	}
}

func TestSetupPolicyNormalizesClaudeCodeExternalModes(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode:         permissions.Mode("bypass-permissions"),
		SubagentMode: permissions.Mode("dont-ask"),
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	if policy.Mode != permissions.ModeBypassPermissions {
		t.Fatalf("mode = %q, want %q", policy.Mode, permissions.ModeBypassPermissions)
	}
	if policy.SubagentMode != permissions.ModeDontAsk {
		t.Fatalf("subagent mode = %q, want %q", policy.SubagentMode, permissions.ModeDontAsk)
	}
}

func TestSetupPolicyAcceptsClaudeCodeUserAddressableModes(t *testing.T) {
	tests := []struct {
		name     string
		input    permissions.Mode
		wantMode permissions.Mode
		wantPlan bool
		wantAuto bool
	}{
		{name: "default", input: permissions.Mode("default"), wantMode: permissions.ModeDefault},
		{name: "accept edits", input: permissions.Mode("acceptEdits"), wantMode: permissions.ModeAcceptEdits},
		{name: "plan", input: permissions.Mode("plan"), wantMode: permissions.ModePlan, wantPlan: true},
		{name: "auto", input: permissions.Mode("auto"), wantMode: permissions.ModeAuto, wantAuto: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := permissions.SetupPolicy(permissions.Policy{
				Mode:           tc.input,
				WorkspaceRoots: []string{"/workspace/project"},
			})
			if err != nil {
				t.Fatalf("SetupPolicy returned unexpected error: %v", err)
			}
			if policy.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", policy.Mode, tc.wantMode)
			}
			if policy.PlanMode != tc.wantPlan {
				t.Fatalf("plan mode = %v, want %v", policy.PlanMode, tc.wantPlan)
			}
			if policy.AutoMode != tc.wantAuto {
				t.Fatalf("auto mode = %v, want %v", policy.AutoMode, tc.wantAuto)
			}
		})
	}
}

func TestSetupPolicyKeepsEmptySubagentModeUnset(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{"/workspace/project"},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}
	if policy.SubagentMode != "" {
		t.Fatalf("subagent mode = %q, want empty optional mode", policy.SubagentMode)
	}
}

func TestSetupPolicyRejectsSubagentEscalation(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:         permissions.ModeWorkspaceWrite,
		SubagentMode: permissions.ModeDangerFullAccess,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
	})
	if err == nil {
		t.Fatal("expected subagent escalation to fail")
	}
}

func TestSetupPolicyNormalizesSubagentModeAndDangerousPatterns(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode:         permissions.ModeDangerFullAccess,
		SubagentMode: permissions.ModeWorkspaceWrite,
		DangerousCommandPatterns: []string{
			" rm -rf ",
			"rm -rf",
			"sudo ",
		},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	if got, want := len(policy.DangerousCommandPatterns), 2; got != want {
		t.Fatalf("dangerous pattern count = %d, want %d (%#v)", got, want, policy.DangerousCommandPatterns)
	}
	if policy.DangerousCommandPatterns[0] != "rm -rf" {
		t.Fatalf("pattern 0 = %q, want trimmed deduped pattern", policy.DangerousCommandPatterns[0])
	}
	if policy.DangerousCommandPatterns[1] != "sudo" {
		t.Fatalf("pattern 1 = %q, want trimmed deduped pattern", policy.DangerousCommandPatterns[1])
	}
}

func TestSetupPolicyRejectsAutoModeWithAskPolicy(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:     permissions.ModeAsk,
		AutoMode: true,
	})
	if err == nil {
		t.Fatal("expected auto mode with ask policy to fail")
	}
}

func TestSetupPolicyRejectsPlanAndAutoModeTogether(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		PlanMode:       true,
		AutoMode:       true,
		WorkspaceRoots: []string{"/workspace/project"},
	})
	if err == nil {
		t.Fatal("expected plan mode and auto mode together to fail")
	}
}

func TestSetupPolicyRejectsDangerousAutoModeAllowRules(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		AutoMode: true,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
			},
		},
	})
	if err == nil {
		t.Fatal("expected auto mode blanket allow rule to fail")
	}
}

func TestSetupPolicyRejectsAutoModeAgentTaskAllowRule(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		AutoMode: true,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "agent.task",
				Action:   permissions.ActionAllow,
			},
		},
	})
	if err == nil {
		t.Fatal("expected auto mode agent.task allow rule to fail")
	}
}

func TestSetupPolicyRejectsAutoModeDangerousInterpreterAllowRule(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		AutoMode: true,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
				Match: permissions.Match{
					CommandContains: []string{"python -c"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected auto mode dangerous interpreter allow rule to fail")
	}
}

func TestSetupPolicyAllowsAutoModeScopedSafeCommandRule(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode:     permissions.ModeWorkspaceWrite,
		AutoMode: true,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
				Match: permissions.Match{
					CommandContains: []string{"go test"},
					WorkDirPrefixes: []string{"/workspace/project"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected scoped safe auto mode rule to pass, got %v", err)
	}
	if len(policy.Rules) != 1 {
		t.Fatalf("rule count = %d, want 1", len(policy.Rules))
	}
}

func TestSetupPolicyRejectsRuleWorkspaceOutsideConfiguredRoots(t *testing.T) {
	_, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
				Match: permissions.Match{
					WorkDirPrefixes: []string{"/tmp/outside"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected rule workdir outside configured roots to fail")
	}
}

func TestSetupPolicyAllowsRuleWorkspaceInsideConfiguredRoots(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Action:   permissions.ActionAllow,
				Match: permissions.Match{
					WorkDirPrefixes: []string{"/workspace/project/subdir"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected in-root workdir prefix to pass, got %v", err)
	}
	if got, want := policy.Rules[0].Match.WorkDirPrefixes[0], "/workspace/project/subdir"; got != want {
		t.Fatalf("rule workdir prefix = %q, want %q", got, want)
	}
}

func TestSetupPolicyMergesRuleLayersBySourcePrecedence(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		Rules: []permissions.Rule{
			{ToolName: "system.run", Action: permissions.ActionAllow, Source: "inline"},
		},
		RuleLayers: []permissions.RuleLayer{
			{
				Source: permissions.RuleSourceConfig,
				Rules: []permissions.Rule{
					{ToolName: "system.run", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceSession,
				Rules: []permissions.Rule{
					{ToolName: "system.run", Action: permissions.ActionDeny},
				},
			},
			{
				Source: permissions.RuleSourceProject,
				Rules: []permissions.Rule{
					{ToolName: "agent.task", Action: permissions.ActionDeny},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	if got, want := len(policy.Rules), 4; got != want {
		t.Fatalf("rule count = %d, want %d (%#v)", got, want, policy.Rules)
	}
	if policy.Rules[0].Source != "inline" {
		t.Fatalf("rule 0 source = %q, want inline", policy.Rules[0].Source)
	}
	if policy.Rules[1].Source != string(permissions.RuleSourceSession) {
		t.Fatalf("rule 1 source = %q, want session", policy.Rules[1].Source)
	}
	if policy.Rules[2].Source != string(permissions.RuleSourceProject) {
		t.Fatalf("rule 2 source = %q, want project", policy.Rules[2].Source)
	}
	if policy.Rules[3].Source != string(permissions.RuleSourceConfig) {
		t.Fatalf("rule 3 source = %q, want config", policy.Rules[3].Source)
	}
}

func TestSetupPolicyLayeredRulesAffectEvaluationInMergedOrder(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		RuleLayers: []permissions.RuleLayer{
			{
				Source: permissions.RuleSourceConfig,
				Rules: []permissions.Rule{
					{ToolName: "system.run", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceSession,
				Rules: []permissions.Rule{
					{ToolName: "system.run", Action: permissions.ActionDeny},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})
	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected higher-precedence session deny to win, got %#v", decision)
	}
}

func TestSetupPolicySortsExpandedRuleSourcesByPrecedence(t *testing.T) {
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode: permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{
			"/workspace/project",
		},
		RuleLayers: []permissions.RuleLayer{
			{
				Source: permissions.RuleSourceConfig,
				Rules: []permissions.Rule{
					{ToolName: "config.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceLocal,
				Rules: []permissions.Rule{
					{ToolName: "local.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceFlag,
				Rules: []permissions.Rule{
					{ToolName: "flag.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourcePolicy,
				Rules: []permissions.Rule{
					{ToolName: "policy.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceCLIArg,
				Rules: []permissions.Rule{
					{ToolName: "cli.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceCommand,
				Rules: []permissions.Rule{
					{ToolName: "command.tool", Action: permissions.ActionAllow},
				},
			},
			{
				Source: permissions.RuleSourceSession,
				Rules: []permissions.Rule{
					{ToolName: "session.tool", Action: permissions.ActionAllow},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SetupPolicy returned unexpected error: %v", err)
	}

	got := []string{
		policy.Rules[0].Source,
		policy.Rules[1].Source,
		policy.Rules[2].Source,
		policy.Rules[3].Source,
		policy.Rules[4].Source,
		policy.Rules[5].Source,
		policy.Rules[6].Source,
	}
	want := []string{
		string(permissions.RuleSourceSession),
		string(permissions.RuleSourceCommand),
		string(permissions.RuleSourceCLIArg),
		string(permissions.RuleSourcePolicy),
		string(permissions.RuleSourceFlag),
		string(permissions.RuleSourceLocal),
		string(permissions.RuleSourceConfig),
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rule source order = %#v, want %#v", got, want)
		}
	}
}

func TestPolicyDeriveForSubagentClearsInteractiveRunModes(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeDangerFullAccess,
		AutoMode:       true,
		PlanMode:       true,
		WorkspaceRoots: []string{"C:/repo"},
	}

	derived := policy.DeriveForSubagent()

	if derived.AutoMode {
		t.Fatal("expected derived subagent policy to disable auto mode")
	}
	if derived.PlanMode {
		t.Fatal("expected derived subagent policy to disable plan mode")
	}
}
