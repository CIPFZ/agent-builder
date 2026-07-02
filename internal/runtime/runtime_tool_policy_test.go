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

// Regression: two call sites (toRuntimeToolCall + applyConversationToolPolicy)
// must agree on the derived kind for the same tool. Previously they had drift
// around MCP/todo classification.
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
	// Successful terminal → groupable, except agent_task.
	if !runtimeToolGroupable("shell", "completed") {
		t.Error("completed shell should now be groupable")
	}
	if !runtimeToolGroupable("file_edit", "completed") {
		t.Error("completed file_edit should be groupable")
	}
	if runtimeToolGroupable("agent_task", "completed") {
		t.Error("agent_task should never group")
	}
	// Non-successful terminal states never group and expand by default.
	for _, status := range []string{"failed", "denied", "cancelled", "interrupted", "running", "waiting_permission"} {
		if runtimeToolGroupable("file_read", status) {
			t.Errorf("kind file_read status %q should not be groupable", status)
		}
	}
	for _, status := range []string{"failed", "denied", "cancelled", "interrupted", "running", "waiting_permission"} {
		if !runtimeToolDefaultExpanded(status) {
			t.Errorf("status %q should default-expand", status)
		}
	}
	// Successful terminal collapses by default.
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
	// Even though shell is now groupable, it is not quiet — we still want
	// the terminal excerpt visible when shell isn't grouped.
	if runtimeToolQuiet("shell", "completed") {
		t.Error("completed shell must not be quiet")
	}
	if runtimeToolQuiet("file_read", "failed") {
		t.Error("failed file_read must not be quiet")
	}
}

func TestApplyRuntimeToolPolicyPromotesShellExitToFailed(t *testing.T) {
	call := RuntimeToolCall{
		ID:       "tool-1",
		Name:     "bash",
		Source:   string(scheduler.ToolSourceShell),
		Status:   string(scheduler.ToolCallCompleted),
		ExitCode: 2,
	}
	out := applyRuntimeToolPolicy(call, runtimeToolPolicyContext{})
	if out.Status != string(scheduler.ToolCallFailed) {
		t.Fatalf("shell exit 2 → status %q, want failed", out.Status)
	}
	if out.Groupable {
		t.Fatalf("failed shell must not be groupable")
	}
	if !out.DefaultExpanded {
		t.Fatalf("failed shell must default-expand")
	}
}

func TestApplyRuntimeToolPolicyPermissionAndInterrupted(t *testing.T) {
	base := RuntimeToolCall{ID: "tool-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Status: string(scheduler.ToolCallRunning)}
	pending := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{PermissionStatus: "pending"})
	if pending.Status != string(scheduler.ToolCallWaitingPermission) {
		t.Fatalf("pending → %q", pending.Status)
	}
	denied := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{PermissionStatus: "denied"})
	if denied.Status != string(scheduler.ToolCallDenied) {
		t.Fatalf("denied → %q", denied.Status)
	}
	interrupted := applyRuntimeToolPolicy(base, runtimeToolPolicyContext{TurnTerminal: true, TurnError: "runtime restart"})
	if interrupted.Status != "interrupted" {
		t.Fatalf("terminal-turn open tool → %q", interrupted.Status)
	}
	if interrupted.Error == "" {
		t.Fatalf("interrupted tool must carry an error message")
	}
}

func TestAppendConversationToolItemsMixedKindAdjacentGrouping(t *testing.T) {
	// Two file_reads followed by two shells with no breaker in between →
	// one mixed group of four (new rule: mixed kinds allowed).
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-r1", Name: "view"},
				{Type: "tool_call", ToolCallID: "t-r2", Name: "view"},
				{Type: "tool_call", ToolCallID: "t-s1", Name: "bash"},
				{Type: "tool_call", ToolCallID: "t-s2", Name: "bash"},
			}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "done"}}, CreatedAt: 60, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-final", StartedAt: 1, FinishedAt: 100}},
		ToolCalls: []RuntimeToolCall{
			{ID: "t-r1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", StartedAt: 21, FinishedAt: 22},
			{ID: "t-r2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", StartedAt: 23, FinishedAt: 24},
			{ID: "t-s1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "completed", StartedAt: 25, FinishedAt: 26},
			{ID: "t-s2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "completed", StartedAt: 27, FinishedAt: 28},
		},
	})
	snapshot := projection.snapshot("session-1", "1")
	group := findConversationItem(t, snapshot.Items, "tool_group")
	if group.ID == "" {
		t.Fatalf("no tool_group emitted: %#v", snapshot.Items)
	}
	if len(group.ToolCallIDs) != 4 {
		t.Fatalf("mixed group size = %d, want 4: %#v", len(group.ToolCallIDs), group)
	}
	if group.Display.Kind != "generic" {
		t.Fatalf("mixed group kind = %q, want generic", group.Display.Kind)
	}
	// Counts have both kinds present.
	kinds := map[string]int{}
	for _, c := range group.Display.Counts {
		kinds[c.Kind] = c.Count
	}
	if kinds["file_read"] != 2 || kinds["shell"] != 2 {
		t.Fatalf("counts wrong: %#v", group.Display.Counts)
	}
}

func TestAppendConversationToolItemsAcrossAssistantMessages(t *testing.T) {
	// Two assistant messages in the same turn, each with a tool call and NO
	// intermediate text between them → should form one group.
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-a", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-1", Name: "view"},
			}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-b", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-2", Name: "view"},
			}, CreatedAt: 30, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "done"}}, CreatedAt: 60, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-final", StartedAt: 1, FinishedAt: 100}},
		ToolCalls: []RuntimeToolCall{
			{ID: "t-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-a", Name: "view", Source: "builtin", Status: "completed", StartedAt: 21, FinishedAt: 22},
			{ID: "t-2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-b", Name: "view", Source: "builtin", Status: "completed", StartedAt: 31, FinishedAt: 32},
		},
	})
	snapshot := projection.snapshot("session-1", "1")
	group := findConversationItem(t, snapshot.Items, "tool_group")
	if group.ID == "" || len(group.ToolCallIDs) != 2 {
		t.Fatalf("cross-message group missing: %#v", snapshot.Items)
	}
}

func TestAppendConversationToolItemsIntermediateTextBreaksGroup(t *testing.T) {
	// Two tool calls with a visible intermediate assistant text between them
	// → do NOT group.
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-a", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-1", Name: "view"},
			}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-b", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "text", Text: "hmm, let me check more"},
			}, CreatedAt: 25, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-c", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-2", Name: "view"},
			}, CreatedAt: 30, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "done"}}, CreatedAt: 60, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-final", StartedAt: 1, FinishedAt: 100}},
		ToolCalls: []RuntimeToolCall{
			{ID: "t-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-a", Name: "view", Source: "builtin", Status: "completed", StartedAt: 21, FinishedAt: 22},
			{ID: "t-2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-c", Name: "view", Source: "builtin", Status: "completed", StartedAt: 31, FinishedAt: 32},
		},
	})
	snapshot := projection.snapshot("session-1", "1")
	tools := 0
	for _, item := range snapshot.Items {
		if item.Kind == "tool_call" {
			tools++
		}
		if item.Kind == "tool_group" {
			t.Fatalf("intermediate text should break the group, got %#v", item)
		}
	}
	if tools != 2 {
		t.Fatalf("expected 2 tool_call items, got %d: %#v", tools, snapshot.Items)
	}
}

func TestAppendConversationToolItemsFailedIndependent(t *testing.T) {
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-ok1", Name: "view"},
				{Type: "tool_call", ToolCallID: "t-fail", Name: "bash"},
				{Type: "tool_call", ToolCallID: "t-ok2", Name: "view"},
			}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		ToolCalls: []RuntimeToolCall{
			{ID: "t-ok1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", StartedAt: 21, FinishedAt: 22},
			{ID: "t-fail", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "completed", ExitCode: 1, StartedAt: 23, FinishedAt: 24},
			{ID: "t-ok2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", StartedAt: 25, FinishedAt: 26},
		},
	})
	snapshot := projection.snapshot("session-1", "1")
	failed := findConversationItemByTool(t, snapshot.Items, "t-fail")
	if failed.Kind != "tool_call" || failed.Status != "failed" {
		t.Fatalf("failed shell not independent: %#v", failed)
	}
	if !failed.Display.DefaultExpanded {
		t.Fatalf("failed tool must default-expand: %#v", failed)
	}
	// The two ok reads shouldn't merge into one group across the failed
	// tool (adjacency broken).
	groupCount := 0
	for _, item := range snapshot.Items {
		if item.Kind == "tool_group" {
			groupCount++
		}
	}
	if groupCount != 0 {
		t.Fatalf("failed tool between groups should prevent grouping, got %d group(s)", groupCount)
	}
}

func TestExplorationSummaryLifecycle(t *testing.T) {
	// While the turn is running, status=exploring; when it completes,
	// status=done and ElapsedMS is non-zero.
	activity := RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "tool_call", ToolCallID: "t-1", Name: "view"},
			}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
		},
		Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		ToolCalls: []RuntimeToolCall{{ID: "t-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "running", StartedAt: 25}},
	}
	live := buildRuntimeOutputProjection(activity).snapshot("session-1", "1")
	expLive := findConversationItem(t, live.Items, "exploration_summary")
	if expLive.ID != "exploration-turn-1" {
		t.Fatalf("live exploration ID = %q", expLive.ID)
	}
	if expLive.Exploration == nil || expLive.Exploration.Status != "exploring" || expLive.Exploration.ToolTotal != 1 {
		t.Fatalf("live exploration = %#v", expLive.Exploration)
	}

	activity.Turns[0].Status = "completed"
	activity.Turns[0].FinishedAt = 100
	activity.ToolCalls[0].Status = "completed"
	activity.ToolCalls[0].FinishedAt = 30
	activity.Messages = append(activity.Messages, RuntimeMessage{ID: "assistant-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "done"}}, CreatedAt: 50, Finished: true, FinishReason: "end_turn"})
	activity.Turns[0].LatestAssistantMessageID = "assistant-final"
	done := buildRuntimeOutputProjection(activity).snapshot("session-1", "1")
	expDone := findConversationItem(t, done.Items, "exploration_summary")
	if expDone.Exploration == nil || expDone.Exploration.Status != "done" {
		t.Fatalf("done exploration = %#v", expDone)
	}
	if expDone.Exploration.ElapsedMS == 0 {
		t.Fatalf("done exploration should carry elapsed: %#v", expDone.Exploration)
	}
	if len(expDone.Exploration.ToolCounts) == 0 || expDone.Exploration.ToolCounts[0].Kind != "file_read" {
		t.Fatalf("done exploration counts: %#v", expDone.Exploration)
	}
}

func TestConversationItemSequenceStableOnInsert(t *testing.T) {
	// Build a snapshot, capture the user_message sequence, then rebuild with
	// an added tool call (different rank) and verify the user_message
	// sequence is unchanged.
	activity := RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "hello"}}, CreatedAt: 20, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-1", StartedAt: 1, FinishedAt: 30}},
	}
	before := buildRuntimeOutputProjection(activity).snapshot("session-1", "1")
	var userBefore RuntimeConversationItem
	for _, item := range before.Items {
		if item.Kind == "user_message" {
			userBefore = item
		}
	}
	// Insert a new tool call at a different rank.
	activity.Messages[1].Parts = append(activity.Messages[1].Parts, RuntimeMessagePart{Type: "tool_call", ToolCallID: "t-new", Name: "view"})
	activity.ToolCalls = []RuntimeToolCall{{ID: "t-new", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", StartedAt: 22, FinishedAt: 23}}
	after := buildRuntimeOutputProjection(activity).snapshot("session-1", "1")
	var userAfter RuntimeConversationItem
	for _, item := range after.Items {
		if item.Kind == "user_message" {
			userAfter = item
		}
	}
	if userBefore.Sequence != userAfter.Sequence {
		t.Fatalf("user_message sequence shifted: before=%d after=%d", userBefore.Sequence, userAfter.Sequence)
	}
}
