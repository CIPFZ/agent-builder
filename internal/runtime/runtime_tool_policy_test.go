package runtime

import (
	"testing"

	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeToolKindStaticRegistry(t *testing.T) {
	cases := []struct {
		name   string
		source scheduler.ToolSource
		want   string
	}{
		{"bash", scheduler.ToolSourceBuiltin, "shell"},
		{"job_output", scheduler.ToolSourceBuiltin, "shell"},
		{"view", scheduler.ToolSourceBuiltin, "file_read"},
		{"read", scheduler.ToolSourceBuiltin, "file_read"},
		{"write", scheduler.ToolSourceBuiltin, "file_write"},
		{"edit", scheduler.ToolSourceBuiltin, "file_edit"},
		{"multiedit", scheduler.ToolSourceBuiltin, "file_edit"},
		{"apply_patch", scheduler.ToolSourceBuiltin, "file_edit"},
		{"glob", scheduler.ToolSourceBuiltin, "file_search"},
		{"grep", scheduler.ToolSourceBuiltin, "file_search"},
		{"ls", scheduler.ToolSourceBuiltin, "file_search"},
		{"todos", scheduler.ToolSourceBuiltin, "todo"},
		{"agent", scheduler.ToolSourceBuiltin, "agent_task"},
		{"task_create", scheduler.ToolSourceBuiltin, "agent_task"},
	}
	for _, tc := range cases {
		got := runtimeToolKind(tc.name, tc.source, false, "")
		if got != tc.want {
			t.Errorf("runtimeToolKind(%q, %q) = %q, want %q", tc.name, tc.source, got, tc.want)
		}
	}
}

func TestRuntimeToolKindSourceFallback(t *testing.T) {
	if got := runtimeToolKind("mcp_github_get_issue", scheduler.ToolSourceMCP, false, ""); got != "generic" {
		t.Errorf("mcp default: %q", got)
	}
	if got := runtimeToolKind("customThing", "plugin", false, ""); got != "generic" {
		t.Errorf("plugin default: %q", got)
	}
	if got := runtimeToolKind("shell.bash", scheduler.ToolSourceShell, false, ""); got != "shell" {
		t.Errorf("shell source: %q", got)
	}
	if got := runtimeToolKind("random_cmd", scheduler.ToolSourceBuiltin, true, ""); got != "shell" {
		t.Errorf("hasCommand: %q", got)
	}
	if got := runtimeToolKind("random_cmd", scheduler.ToolSourceBuiltin, false, "execute"); got != "shell" {
		t.Errorf("execute risk: %q", got)
	}
}

// Regression: the scheduler and Runtime DTO mappers must agree on the
// semantic kind for the same tool.
func TestRuntimeToolKindTwoSitesAgree(t *testing.T) {
	schedulerCall := scheduler.ToolCall{Name: "bash", Source: scheduler.ToolSourceShell, Command: "ls"}
	runtimeCall := RuntimeToolCall{Name: "bash", Source: string(scheduler.ToolSourceShell), Command: "ls"}

	if a, b := runtimeToolPolicyDisplayKind(schedulerCall), runtimeToolPolicyKindForRuntime(runtimeCall); a != b {
		t.Fatalf("kind disagreement bash: %q vs %q", a, b)
	}
	for _, name := range []string{"view", "grep", "write", "edit", "todos", "agent"} {
		s := scheduler.ToolCall{Name: name, Source: scheduler.ToolSourceBuiltin}
		r := RuntimeToolCall{Name: name, Source: string(scheduler.ToolSourceBuiltin)}
		if a, b := runtimeToolPolicyDisplayKind(s), runtimeToolPolicyKindForRuntime(r); a != b {
			t.Fatalf("kind disagreement %s: %q vs %q", name, a, b)
		}
	}
}

func TestRuntimeToolPolicyGroupableRelaxedAndFailedIndependent(t *testing.T) {
	if !runtimeToolGroupable("shell", "completed") {
		t.Error("completed shell should now be groupable")
	}
	if !runtimeToolGroupable("file_edit", "completed") {
		t.Error("completed file_edit should be groupable")
	}
	if runtimeToolGroupable("agent_task", "completed") {
		t.Error("agent_task should never group")
	}
	for _, status := range []string{"failed", "denied", "cancelled", "interrupted", "running", "waiting_permission"} {
		if runtimeToolGroupable("file_read", status) {
			t.Errorf("kind file_read status %q should not be groupable", status)
		}
		if !runtimeToolDefaultExpanded(status) {
			t.Errorf("status %q should default-expand", status)
		}
	}
	if runtimeToolDefaultExpanded("completed") {
		t.Error("completed should not default-expand")
	}
}

func TestRuntimeToolPolicyQuietOnlyReadSearch(t *testing.T) {
	if !runtimeToolQuiet("file_read", "completed") {
		t.Error("completed file_read must be quiet")
	}
	if !runtimeToolQuiet("file_search", "completed") {
		t.Error("completed file_search must be quiet")
	}
	if runtimeToolQuiet("shell", "completed") {
		t.Error("completed shell must not be quiet")
	}
	if runtimeToolQuiet("file_read", "failed") {
		t.Error("failed file_read must not be quiet")
	}
}

func TestApplyRuntimeToolPolicyPromotesShellExitToFailed(t *testing.T) {
	call := RuntimeToolCall{ID: "tool-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Status: string(scheduler.ToolCallCompleted), ExitCode: 2}
	out := applyRuntimeToolPolicy(call, runtimeToolPolicyContext{})
	if out.Status != string(scheduler.ToolCallFailed) {
		t.Fatalf("shell exit 2 status %q, want failed", out.Status)
	}
	if out.Groupable {
		t.Fatal("failed shell must not be groupable")
	}
	if !out.DefaultExpanded {
		t.Fatal("failed shell must default-expand")
	}
}

func TestApplyRuntimeToolPolicyPermissionAndInterrupted(t *testing.T) {
	base := RuntimeToolCall{ID: "tool-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Status: string(scheduler.ToolCallRunning)}
	pending := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{PermissionStatus: "pending"})
	if pending.Status != string(scheduler.ToolCallWaitingPermission) {
		t.Fatalf("pending status = %q", pending.Status)
	}
	denied := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{PermissionStatus: "denied"})
	if denied.Status != string(scheduler.ToolCallDenied) {
		t.Fatalf("denied status = %q", denied.Status)
	}
	interrupted := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{TurnTerminal: true, TurnError: "runtime restart"})
	if interrupted.Status != "interrupted" || interrupted.Error == "" {
		t.Fatalf("interrupted tool = %#v", interrupted)
	}
}
