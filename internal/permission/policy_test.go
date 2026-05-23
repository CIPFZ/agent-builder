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
