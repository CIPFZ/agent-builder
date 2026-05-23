package permission

import (
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
		{`{"command":"git reset --hard HEAD"}`, RiskDestructive},
		{`{"command":"Stop-Process -Id 1234"}`, RiskDestructive},
		{`{"command":"kill 1234"}`, RiskDestructive},
		{`{"command":"chmod -R 777 ."}`, RiskDestructive},
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
