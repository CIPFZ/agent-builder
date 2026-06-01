package permission

import (
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestStaticPolicyEvaluate(t *testing.T) {
	t.Parallel()

	readCall := scheduler.ToolCall{Name: "view", InputSummary: `{"file":"README.md"}`}
	writeCall := scheduler.ToolCall{Name: "write", InputSummary: `{"file":"README.md"}`}

	if got := NewPermissionPolicy(PolicyModeAutoRead).Evaluate(readCall); got.Decision != PolicyAllow || got.Risk != RiskRead {
		t.Fatalf("auto_read read result = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeAutoRead).Evaluate(writeCall); got.Decision != PolicyAsk || got.Risk != RiskWrite {
		t.Fatalf("auto_read write result = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModePlan).Evaluate(writeCall); got.Decision != PolicyDeny {
		t.Fatalf("plan write result = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeDenyAll).Evaluate(readCall); got.Decision != PolicyDeny {
		t.Fatalf("deny_all result = %#v", got)
	}
}

func TestStaticPolicyModeMatrix(t *testing.T) {
	t.Parallel()

	calls := map[Risk]scheduler.ToolCall{
		RiskRead:        {Name: "view", InputSummary: `{"file":"README.md"}`},
		RiskWrite:       {Name: "edit", InputSummary: `{"file":"README.md"}`},
		RiskExecute:     {Name: "bash", InputSummary: `{"command":"go test ./..."}`},
		RiskNetwork:     {Name: "fetch", InputSummary: `{"url":"https://example.com"}`},
		RiskSecret:      {Name: "view", InputSummary: `{"file":"token"}`},
		RiskDestructive: {Name: "bash", InputSummary: `{"command":"git reset --hard"}`},
	}

	for risk, call := range calls {
		if got := NewPermissionPolicy(PolicyModePlan).Evaluate(call); risk == RiskRead && got.Decision != PolicyAllow {
			t.Fatalf("plan read = %#v", got)
		} else if risk != RiskRead && got.Decision != PolicyDeny {
			t.Fatalf("plan %s = %#v, want deny", risk, got)
		}
	}
	if got := NewPermissionPolicy(PolicyModeAutoRead).Evaluate(calls[RiskRead]); got.Decision != PolicyAllow {
		t.Fatalf("auto_read read = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeAutoRead).Evaluate(calls[RiskExecute]); got.Decision != PolicyAsk {
		t.Fatalf("auto_read execute = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeDenyAll).Evaluate(calls[RiskRead]); got.Decision != PolicyDeny {
		t.Fatalf("deny_all read = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeAsk).Evaluate(calls[RiskRead]); got.Decision != PolicyAsk {
		t.Fatalf("ask read = %#v", got)
	}
	for risk, call := range calls {
		if got := NewPermissionPolicy(PolicyModeFullAccess).Evaluate(call); got.Decision != PolicyAllow || got.Mode != PolicyModeFullAccess {
			t.Fatalf("full_access %s = %#v, want allow", risk, got)
		}
	}
}

func TestClassifyRiskPlanModeBlockedTools(t *testing.T) {
	t.Parallel()

	cases := map[string]Risk{
		"todos":       RiskWrite,
		"job_kill":    RiskDestructive,
		"download":    RiskNetwork,
		"lsp_restart": RiskExecute,
		"multiedit":   RiskWrite,
	}
	for name, want := range cases {
		if got := ClassifyRisk(name, "{}"); got != want {
			t.Fatalf("ClassifyRisk(%q) = %s, want %s", name, got, want)
		}
		if result := NewPermissionPolicy(PolicyModePlan).Evaluate(scheduler.ToolCall{Name: name}); result.Decision != PolicyDeny {
			t.Fatalf("plan policy for %q = %#v, want deny", name, result)
		}
	}
}

func TestPlanModeOnlyAllowsKnownReadOnlyTools(t *testing.T) {
	t.Parallel()

	allowed := []string{"view", "ls", "grep", "glob", "rg", "diagnostics", "references", "crush_info", "crush_logs", "job_output", "list_mcp_resources", "read_mcp_resource", "context_activation"}
	for _, name := range allowed {
		result := NewPermissionPolicy(PolicyModePlan).Evaluate(scheduler.ToolCall{Name: name, Source: scheduler.ToolSourceBuiltin})
		if result.Decision != PolicyAllow || result.Risk != RiskRead {
			t.Fatalf("plan policy for read-only %q = %#v, want allow/read", name, result)
		}
	}

	unknown := NewPermissionPolicy(PolicyModePlan).Evaluate(scheduler.ToolCall{Name: "maybe_mutates", Source: scheduler.ToolSourceBuiltin})
	if unknown.Decision != PolicyDeny || unknown.Risk != RiskExecute {
		t.Fatalf("plan policy for unknown tool = %#v, want deny/execute", unknown)
	}
}

func TestClassifyToolCallRiskUsesSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call scheduler.ToolCall
		want Risk
	}{
		{
			name: "shell source is execute even for unknown name",
			call: scheduler.ToolCall{Name: "run", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"go test ./..."}`},
			want: RiskExecute,
		},
		{
			name: "shell destructive input is destructive",
			call: scheduler.ToolCall{Name: "run", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"git reset --hard"}`},
			want: RiskDestructive,
		},
		{
			name: "mcp source is network",
			call: scheduler.ToolCall{Name: "search", Source: scheduler.ToolSourceMCP, InputSummary: `{}`},
			want: RiskNetwork,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyToolCallRisk(tt.call); got != tt.want {
				t.Fatalf("ClassifyToolCallRisk = %s, want %s", got, tt.want)
			}
			if result := NewPermissionPolicy(PolicyModePlan).Evaluate(tt.call); result.Decision != PolicyDeny {
				t.Fatalf("plan policy = %#v, want deny", result)
			}
		})
	}
}

func TestClassifyShellCommandRiskBaseline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		command string
		want    Risk
	}{
		{`{"command":"go test ./..."}`, RiskExecute},
		{`{"command":"rm -rf /tmp/build"}`, RiskDestructive},
		{`{"command":"del /s build"}`, RiskDestructive},
		{`{"command":"Remove-Item -Recurse .\\build"}`, RiskDestructive},
		{`{"command":"Remove-Item .\\build -Recurse -Force"}`, RiskDestructive},
		{`{"command":"git reset --hard HEAD"}`, RiskDestructive},
		{`{"command":"Stop-Process -Id 1234"}`, RiskDestructive},
		{`{"command":"kill 1234"}`, RiskDestructive},
		{`{"command":"chmod -R 777 ."}`, RiskDestructive},
		{`{"command":"Out-File result.txt"}`, RiskDestructive},
		{`{"command":"echo secret > .env"}`, RiskDestructive},
	}
	for _, tt := range cases {
		if got := ClassifyShellCommandRisk(tt.command); got != tt.want {
			t.Fatalf("ClassifyShellCommandRisk(%s) = %s, want %s", tt.command, got, tt.want)
		}
	}
}

func TestShellPolicyModes(t *testing.T) {
	t.Parallel()

	call := scheduler.ToolCall{Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"go test ./..."}`}
	if got := NewPermissionPolicy(PolicyModePlan).Evaluate(call); got.Decision != PolicyDeny || got.Risk != RiskExecute {
		t.Fatalf("plan shell = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeDenyAll).Evaluate(call); got.Decision != PolicyDeny {
		t.Fatalf("deny_all shell = %#v", got)
	}
	if got := NewPermissionPolicy(PolicyModeAutoRead).Evaluate(call); got.Decision != PolicyAsk || got.Risk != RiskExecute {
		t.Fatalf("auto_read shell = %#v", got)
	}
}

func TestJobToolsRisk(t *testing.T) {
	t.Parallel()

	if got := ClassifyToolCallRisk(scheduler.ToolCall{Name: "job_output", Source: scheduler.ToolSourceShell}); got != RiskRead {
		t.Fatalf("job_output risk = %s, want read", got)
	}
	if got := ClassifyToolCallRisk(scheduler.ToolCall{Name: "job_kill", Source: scheduler.ToolSourceShell}); got != RiskDestructive {
		t.Fatalf("job_kill risk = %s, want destructive", got)
	}
	if result := NewPermissionPolicy(PolicyModePlan).Evaluate(scheduler.ToolCall{Name: "job_kill", Source: scheduler.ToolSourceShell}); result.Decision != PolicyDeny {
		t.Fatalf("plan job_kill = %#v, want deny", result)
	}
}

func TestScopedPolicyRulePrecedenceAndDiagnostics(t *testing.T) {
	t.Parallel()

	policy := NewScopedPermissionPolicy(PolicyModeAutoRead, "default", []PolicyRule{
		{ID: "allow-read", Decision: PolicyAllow, BuiltinTool: "view", Source: "user", Reason: "view allowed"},
		{ID: "deny-cap", Decision: PolicyDeny, CapabilityID: "builtin:view", Source: "workspace", Reason: "capability denied"},
	})
	result := policy.Evaluate(scheduler.ToolCall{Name: "view", Source: scheduler.ToolSourceBuiltin, CapabilityID: "builtin:view"})
	if result.Decision != PolicyDeny {
		t.Fatalf("decision = %#v, want deny by higher-precedence deny rule", result)
	}
	if result.RuleID != "deny-cap" || result.RuleSource != "workspace" || result.RuleScopeKind != "capability" || result.RuleScopeValue != "builtin:view" {
		t.Fatalf("matched rule diagnostics = %#v", result)
	}
	if result.Mode != PolicyModeAutoRead || result.TargetSummary != "builtin:view" {
		t.Fatalf("mode/target diagnostics = %#v", result)
	}
}

func TestScopedPolicyRulesOverrideModeBaseline(t *testing.T) {
	t.Parallel()

	planAllow := NewScopedPermissionPolicy(PolicyModePlan, "", []PolicyRule{
		{ID: "allow-grep", Decision: PolicyAllow, ShellPrefix: "grep ", Reason: "grep command allowed in plan"},
	})
	result := planAllow.Evaluate(scheduler.ToolCall{Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"grep -R TODO ."}`})
	if result.Decision != PolicyAllow || result.RuleID != "allow-grep" {
		t.Fatalf("scoped allow in plan = %#v", result)
	}

	askDeny := NewScopedPermissionPolicy(PolicyModeAsk, "", []PolicyRule{
		{ID: "deny-secret-path", Decision: PolicyDeny, PathPrefix: "C:\\repo\\.env", Reason: "secret path blocked"},
	})
	result = askDeny.Evaluate(scheduler.ToolCall{Name: "view", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"C:\\repo\\.env\\prod"}`})
	if result.Decision != PolicyDeny || result.RuleID != "deny-secret-path" {
		t.Fatalf("path deny in ask = %#v", result)
	}

	fullAccessDeny := NewScopedPermissionPolicy(PolicyModeFullAccess, "", []PolicyRule{
		{ID: "deny-secret-path", Decision: PolicyDeny, PathPrefix: "C:\\repo\\.env", Reason: "secret path blocked"},
	})
	result = fullAccessDeny.Evaluate(scheduler.ToolCall{Name: "view", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"C:\\repo\\.env\\prod"}`})
	if result.Decision != PolicyDeny || result.RuleID != "deny-secret-path" || result.Mode != PolicyModeFullAccess {
		t.Fatalf("path deny in full_access = %#v", result)
	}
}

func TestScopedPolicyMCPAndSkillScopes(t *testing.T) {
	t.Parallel()

	policy := NewScopedPermissionPolicy(PolicyModeAutoRead, "", []PolicyRule{
		{ID: "ask-mcp-tool", Decision: PolicyAsk, MCPServer: "github", MCPTool: "create_issue", Source: "project"},
		{ID: "deny-skill", Decision: PolicyDeny, Skill: "deployment", Source: "managed"},
	})
	mcp := policy.Evaluate(scheduler.ToolCall{Name: "create_issue", Source: scheduler.ToolSourceMCP, CapabilityID: "mcp:github:create_issue"})
	if mcp.Decision != PolicyAsk || mcp.RuleID != "ask-mcp-tool" || mcp.RuleScopeKind != "mcp_tool" {
		t.Fatalf("mcp scoped result = %#v", mcp)
	}
	skill := policy.Evaluate(scheduler.ToolCall{Name: "deployment", Source: scheduler.ToolSourceUnknown, CapabilityID: "skill:deployment"})
	if skill.Decision != PolicyDeny || skill.RuleID != "deny-skill" || skill.RuleScopeKind != "skill" {
		t.Fatalf("skill scoped result = %#v", skill)
	}
}

func TestPolicyProfilesHeadlessAskFailClosed(t *testing.T) {
	t.Parallel()

	call := scheduler.ToolCall{Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"go test ./..."}`}
	headless := NewScopedPermissionPolicy(PolicyModeAutoRead, "headless", nil).Evaluate(call)
	if headless.Decision != PolicyDeny || !headless.Headless || headless.Profile != string(PolicyProfileHeadless) {
		t.Fatalf("headless ask = %#v, want deny/headless", headless)
	}
	if !strings.Contains(headless.Reason, "fail closed") {
		t.Fatalf("headless reason = %q", headless.Reason)
	}

	task := NewScopedPermissionPolicy(PolicyModeAutoRead, "subagent", []PolicyRule{
		{ID: "allow-tests", Decision: PolicyAllow, PolicyProfile: "task", ShellPrefix: "go test", Source: "task-profile"},
	}).Evaluate(call)
	if task.Decision != PolicyAllow || !task.Headless || task.Profile != string(PolicyProfileTask) || task.RuleID != "allow-tests" {
		t.Fatalf("task deterministic allow = %#v", task)
	}

	deny := NewScopedPermissionPolicy(PolicyModeAutoRead, "replay-safe", []PolicyRule{
		{ID: "deny-shell", Decision: PolicyDeny, PolicyProfile: "recovery", ShellPrefix: "go test", Source: "replay"},
	}).Evaluate(call)
	if deny.Decision != PolicyDeny || !deny.Headless || deny.Profile != string(PolicyProfileRecovery) || deny.RuleID != "deny-shell" {
		t.Fatalf("recovery deterministic deny = %#v", deny)
	}
}

func TestScopedPolicyPrecedenceAndPathShellRules(t *testing.T) {
	t.Parallel()

	policy := NewScopedPermissionPolicy(PolicyModeAutoRead, "default", []PolicyRule{
		{ID: "allow-cwd", Decision: PolicyAllow, CWDPrefix: `C:\repo`, Source: "workspace"},
		{ID: "ask-path", Decision: PolicyAsk, PathPrefix: `C:\repo\protected`, Source: "project"},
		{ID: "deny-shell-regex", Decision: PolicyDeny, ShellRegex: `(?i)^git\s+reset`, Source: "user"},
	})
	path := policy.Evaluate(scheduler.ToolCall{Name: "view", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"C:\\repo\\protected\\secret.txt","working_dir":"C:\\repo"}`})
	if path.Decision != PolicyAsk || path.RuleID != "ask-path" || path.RuleScopeKind != "path_prefix" {
		t.Fatalf("path precedence = %#v, want ask path rule before cwd allow", path)
	}
	shell := policy.Evaluate(scheduler.ToolCall{Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"git reset --hard HEAD"}`})
	if shell.Decision != PolicyDeny || shell.RuleID != "deny-shell-regex" || shell.RuleScopeKind != "shell_regex" {
		t.Fatalf("shell regex rule = %#v", shell)
	}
}

func TestClassifyShellCommandRiskHardening(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
		reason  string
	}{
		{"bash recursive", `{"command":"rm -r build"}`, "recursive delete"},
		{"bash forced", `{"command":"rm -f build.log"}`, "forced delete"},
		{"bash checkout", `{"command":"git checkout -- ."}`, "git checkout"},
		{"bash restore", `{"command":"git restore --source HEAD -- ."}`, "git restore"},
		{"bash clean", `{"command":"git clean -fdx"}`, "git clean"},
		{"compound newline", "{\"command\":\"echo ok\\nrm -rf build\"}", "recursive forced delete"},
		{"compound pipe", `{"command":"echo ok | taskkill /PID 123 /F"}`, "process termination"},
		{"cmd rmdir", `{"command":"cmd /c rmdir /s /q build"}`, "cmd delete"},
		{"cmd del", `{"command":"del /s /q build\\*"}`, "cmd delete"},
		{"powershell remove", `{"command":"Remove-Item .\\build -Recurse -Force"}`, "PowerShell recursive forced delete"},
		{"powershell alias rm", `{"command":"rm .\\build -Recurse -Force"}`, "PowerShell recursive forced delete"},
		{"powershell alias del", `{"command":"del .\\build -Recurse"}`, "PowerShell recursive delete"},
		{"powershell literal path", `{"command":"Remove-Item -LiteralPath '.\\build folder' -Force"}`, "PowerShell forced delete"},
		{"process kill", `{"command":"taskkill /F /PID 123"}`, "process termination"},
		{"chmod", `{"command":"chmod 600 secret.txt"}`, "ownership or permission change"},
		{"chown", `{"command":"chown me file.txt"}`, "ownership or permission change"},
		{"redirection", `{"Command":"echo token > .env"}`, "redirection overwrite"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyShellCommand(tt.command)
			if result.Risk != RiskDestructive {
				t.Fatalf("risk = %#v, want destructive", result)
			}
			if !strings.Contains(result.Reason, tt.reason) {
				t.Fatalf("reason = %q, want contains %q", result.Reason, tt.reason)
			}
			if result.TargetSummary == "" {
				t.Fatalf("target summary missing: %#v", result)
			}
		})
	}
}

func TestClassifyShellCommandReadOnlyAndAmbiguous(t *testing.T) {
	t.Parallel()

	readOnly := ClassifyShellCommand(`{"command":"git status --short"}`)
	if readOnly.Risk != RiskExecute {
		t.Fatalf("git status risk = %#v, want execute", readOnly)
	}
	ambiguous := ClassifyShellCommand(`{"command":"$cmd"}`)
	if ambiguous.Risk != RiskExecute || ambiguous.Reason == "" {
		t.Fatalf("ambiguous command = %#v, want execute with reason", ambiguous)
	}
}
