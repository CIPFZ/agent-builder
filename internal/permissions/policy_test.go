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

func TestPolicyEvaluateBypassPermissionsAllowsOutsideWorkspaceWithoutApproval(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeBypassPermissions,
		WorkspaceRoots: []string{"/workspace/project"},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/tmp/outside",
	})

	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected bypassPermissions to allow system action without approval, got %#v", decision)
	}
}

func TestPolicyEvaluateBypassPermissionsStillRespectsAskRule(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeBypassPermissions,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Source:   string(permissions.RuleSourceSession),
				Action:   permissions.ActionAsk,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || !decision.RequiresApproval {
		t.Fatalf("expected ask rule to survive bypassPermissions, got %#v", decision)
	}
	if decision.RuleSource != string(permissions.RuleSourceSession) {
		t.Fatalf("decision rule source = %q, want %q", decision.RuleSource, permissions.RuleSourceSession)
	}
}

func TestPolicyAutoModeUsesClassifierDecisionWhenAvailable(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAuto,
		AutoClassifier: func(req permissions.Request) (permissions.Decision, bool) {
			if req.AutoClassifierInput == "safe-read" {
				return permissions.Decision{
					Allowed: true,
					DecisionReason: permissions.DecisionReason{
						Type:       permissions.DecisionReasonClassifier,
						Classifier: "auto-mode",
						Reason:     "classifier allowed read",
					},
				}, true
			}
			return permissions.Decision{
				RequiresApproval: true,
				Category:         permissions.CategoryApproval,
				Reason:           "classifier requires confirmation",
				DecisionReason: permissions.DecisionReason{
					Type:       permissions.DecisionReasonClassifier,
					Classifier: "auto-mode",
					Reason:     "classifier requires confirmation",
				},
			}, true
		},
	}

	allowed := policy.Evaluate(permissions.Request{
		ToolName:            "system.run",
		Command:             "cat README.md",
		WorkDir:             "/outside",
		AutoClassifierInput: "safe-read",
	})
	if !allowed.Allowed || allowed.DecisionReason.Type != permissions.DecisionReasonClassifier {
		t.Fatalf("allowed decision = %#v, want classifier allow", allowed)
	}

	asked := policy.Evaluate(permissions.Request{
		ToolName:            "system.run",
		Command:             "rm -rf build",
		WorkDir:             "/outside",
		AutoClassifierInput: "destructive",
	})
	if !asked.RequiresApproval || asked.DecisionReason.Type != permissions.DecisionReasonClassifier {
		t.Fatalf("ask decision = %#v, want classifier ask", asked)
	}
}

func TestPolicyApplyPermissionUpdatesAddRulePrependsSessionRule(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
		Rules: []permissions.Rule{{
			ToolName: "system.run",
			Action:   permissions.ActionAsk,
			Source:   string(permissions.RuleSourceConfig),
		}},
	}

	updated := policy.ApplyPermissionUpdates([]permissions.PermissionUpdate{{
		Type:        permissions.PermissionUpdateAddRules,
		Destination: permissions.PermissionUpdateDestinationSession,
		Behavior:    permissions.ActionAllow,
		Rules: []permissions.PermissionRuleValue{{
			ToolName:    "system.run",
			RuleContent: "go test",
		}},
	}})

	if len(updated.Rules) != 2 {
		t.Fatalf("rules = %#v, want prepended session rule plus existing rule", updated.Rules)
	}
	if updated.Rules[0].Action != permissions.ActionAllow || updated.Rules[0].Source != string(permissions.RuleSourceSession) {
		t.Fatalf("first rule = %#v, want session allow rule", updated.Rules[0])
	}
	decision := updated.Evaluate(permissions.Request{ToolName: "system.run", Command: "go test ./...", WorkDir: ""})
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want added allow rule to match first", decision)
	}
}

func TestPolicyApplyPermissionUpdatesReplaceAndRemoveRules(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAsk,
		Rules: []permissions.Rule{
			{ToolName: "system.run", Action: permissions.ActionAsk, Source: string(permissions.RuleSourceConfig)},
			{ToolName: "text.read", Action: permissions.ActionDeny, Source: string(permissions.RuleSourceConfig)},
		},
	}

	updated := policy.ApplyPermissionUpdates([]permissions.PermissionUpdate{{
		Type:        permissions.PermissionUpdateReplaceRules,
		Destination: permissions.PermissionUpdateDestinationSession,
		Behavior:    permissions.ActionAsk,
		Rules: []permissions.PermissionRuleValue{{
			ToolName: "text.write",
		}},
	}, {
		Type:        permissions.PermissionUpdateRemoveRules,
		Destination: permissions.PermissionUpdateDestinationSession,
		Behavior:    permissions.ActionDeny,
		Rules: []permissions.PermissionRuleValue{{
			ToolName: "text.read",
		}},
	}})

	if len(updated.Rules) != 1 {
		t.Fatalf("rules = %#v, want replaced ask rules and removed deny rule", updated.Rules)
	}
	if updated.Rules[0].ToolName != "text.write" || updated.Rules[0].Action != permissions.ActionAsk {
		t.Fatalf("rules = %#v, want text.write ask rule", updated.Rules)
	}
}

func TestPolicyApplyPermissionUpdatesModeAndDirectories(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeAsk,
		WorkspaceRoots: []string{"/workspace/a"},
	}

	updated := policy.ApplyPermissionUpdates([]permissions.PermissionUpdate{{
		Type:        permissions.PermissionUpdateSetMode,
		Destination: permissions.PermissionUpdateDestinationSession,
		Mode:        permissions.ModeDangerFullAccess,
	}, {
		Type:        permissions.PermissionUpdateAddDirectories,
		Destination: permissions.PermissionUpdateDestinationSession,
		Directories: []string{"/workspace/b", "/workspace/a"},
	}, {
		Type:        permissions.PermissionUpdateRemoveDirectories,
		Destination: permissions.PermissionUpdateDestinationSession,
		Directories: []string{"/workspace/a"},
	}})

	if updated.Mode != permissions.ModeDangerFullAccess {
		t.Fatalf("mode = %q, want danger full access", updated.Mode)
	}
	if len(updated.WorkspaceRoots) != 1 || updated.WorkspaceRoots[0] != "/workspace/b" {
		t.Fatalf("workspace roots = %#v, want only added non-removed directory", updated.WorkspaceRoots)
	}
}

func TestDecisionReasonSerializeMatchesClaudeStructuredIOSemantics(t *testing.T) {
	if got := (permissions.DecisionReason{
		Type:     permissions.DecisionReasonHook,
		HookName: "PermissionRequest",
		Reason:   "hook says ask",
	}).Serialize(); got != "hook says ask" {
		t.Fatalf("hook decision reason = %q, want hook reason", got)
	}

	if got := (permissions.DecisionReason{
		Type: permissions.DecisionReasonMode,
		Mode: permissions.ModeAsk,
	}).Serialize(); got != "" {
		t.Fatalf("mode decision reason = %q, want empty serialized reason", got)
	}

	if got := (permissions.DecisionReason{
		Type:   permissions.DecisionReasonClassifier,
		Reason: "classifier says no",
	}).Serialize(); got != "classifier says no" {
		t.Fatalf("classifier decision reason = %q, want classifier reason", got)
	}
}

func TestDecisionReasonStructuredMatchesClaudeInternalShape(t *testing.T) {
	rule := permissions.Rule{
		ToolName: "system.run",
		Action:   permissions.ActionAsk,
		Source:   string(permissions.RuleSourceProject),
		Match: permissions.Match{
			CommandContains: []string{"go test"},
		},
	}
	got := (permissions.DecisionReason{
		Type: permissions.DecisionReasonRule,
		Rule: &rule,
	}).Structured()
	if got["type"] != "rule" {
		t.Fatalf("type = %#v, want rule", got["type"])
	}
	if got["rule"] == nil {
		t.Fatalf("structured rule = %#v, want rule payload", got)
	}

	hook := (permissions.DecisionReason{
		Type:       permissions.DecisionReasonHook,
		HookName:   "PermissionRequest",
		HookSource: "project",
		Reason:     "hook allowed",
	}).Structured()
	if hook["type"] != "hook" || hook["hookName"] != "PermissionRequest" || hook["hookSource"] != "project" || hook["reason"] != "hook allowed" {
		t.Fatalf("hook structured reason = %#v, want Claude hook shape", hook)
	}

	classifier := (permissions.DecisionReason{
		Type:       permissions.DecisionReasonClassifier,
		Classifier: "bash",
		Reason:     "classifier blocked",
	}).Structured()
	if classifier["type"] != "classifier" || classifier["classifier"] != "bash" || classifier["reason"] != "classifier blocked" {
		t.Fatalf("classifier structured reason = %#v, want Claude classifier shape", classifier)
	}
}

func TestPolicyEvaluateDontAskConvertsApprovalIntoDeny(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDontAsk,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected dontAsk mode to deny instead of asking, got %#v", decision)
	}
	if decision.Category != permissions.CategoryApproval {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryApproval)
	}
}

func TestPolicyEvaluateDefaultModeBehavesLikeExplicitApprovalMode(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDefault,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || !decision.RequiresApproval {
		t.Fatalf("expected default mode to require approval for system.run, got %#v", decision)
	}
	if decision.Category != permissions.CategoryApproval {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryApproval)
	}
}

func TestPolicyEvaluateAcceptEditsAllowsDestructiveNonSystemToolsButStillAsksForSystemRun(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeAcceptEdits,
	}

	editDecision := policy.Evaluate(permissions.Request{
		ToolName:    "file.edit",
		Command:     "edit app.go",
		Destructive: true,
		ReadOnly:    false,
		WorkDir:     "/workspace/project",
	})
	if !editDecision.Allowed || editDecision.RequiresApproval {
		t.Fatalf("expected acceptEdits to allow destructive non-system edit tools, got %#v", editDecision)
	}

	bashDecision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "rm generated.txt",
		WorkDir:  "/workspace/project",
	})
	if bashDecision.Allowed || !bashDecision.RequiresApproval {
		t.Fatalf("expected acceptEdits to still ask for system.run, got %#v", bashDecision)
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
		ToolName:    "file.delete",
		Command:     "delete build output",
		Destructive: true,
		ReadOnly:    false,
		WorkDir:     "/workspace/project",
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
		ToolName:    "file.delete",
		Command:     "delete temp file",
		Destructive: true,
		ReadOnly:    false,
		WorkDir:     "/workspace/project",
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
		ToolName:    "file.delete",
		Command:     "delete generated file",
		Destructive: true,
		ReadOnly:    false,
		WorkDir:     "/workspace/project",
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

func TestPolicyEvaluateIncludesRuleSourceForRuleAsk(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
		Rules: []permissions.Rule{
			{
				ToolName: "system.run",
				Source:   string(permissions.RuleSourceCommand),
				Action:   permissions.ActionAsk,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "system.run",
		Command:  "pwd",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || !decision.RequiresApproval {
		t.Fatalf("expected ask rule to require approval, got %#v", decision)
	}
	if decision.RuleSource != string(permissions.RuleSourceCommand) {
		t.Fatalf("decision rule source = %q, want %q", decision.RuleSource, permissions.RuleSourceCommand)
	}
	if decision.Category != permissions.CategoryApproval {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryApproval)
	}
}

func TestPolicyEvaluateMatchesServerPrefixToolRule(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
		Rules: []permissions.Rule{
			{
				ToolName: "mcp__filesystem",
				Action:   permissions.ActionDeny,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "mcp__filesystem__read_resource",
		Command:  "read",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected server-prefix deny to hard deny, got %#v", decision)
	}
	if decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryRuleDenied)
	}
}

func TestPolicyEvaluateMatchesServerWildcardToolRule(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
		Rules: []permissions.Rule{
			{
				ToolName: "mcp__filesystem__*",
				Action:   permissions.ActionDeny,
			},
		},
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "mcp__filesystem__read_resource",
		Command:  "read",
		WorkDir:  "/workspace/project",
	})

	if decision.Allowed || decision.RequiresApproval {
		t.Fatalf("expected server wildcard deny to hard deny, got %#v", decision)
	}
	if decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("decision category = %q, want %q", decision.Category, permissions.CategoryRuleDenied)
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
