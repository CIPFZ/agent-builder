package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestRuntimeOutputProjectionLinksClientRequestStepsAndToolResults(t *testing.T) {
	activity := RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "msg-user-1", SessionID: "session-1", Role: "user", Content: "same", ClientRequestID: "client-1", Metadata: map[string]string{"clientRequestId": "client-1"}, CreatedAt: 20, UpdatedAt: 20},
			{ID: "msg-user-2", SessionID: "session-1", Role: "user", Content: "same", ClientRequestID: "client-2", Metadata: map[string]string{"clientRequestId": "client-2"}, CreatedAt: 30, UpdatedAt: 30},
			{ID: "msg-assistant-2a", SessionID: "session-1", Role: "assistant", Content: "checking", Parts: []RuntimeMessagePart{
				{Type: "reasoning", Thinking: "Need a file"},
				{Type: "text", Text: "Checking"},
				{Type: "tool_call", ToolCallID: "tool-1", Name: "view", Input: `{"file":"a.go"}`},
			}, CreatedAt: 40, UpdatedAt: 41, Finished: true},
			{ID: "msg-tool-2a", SessionID: "session-1", Role: "tool", Parts: []RuntimeMessagePart{
				{Type: "tool_result", ToolCallID: "tool-1", Name: "view", Content: "package main", DeliveredToModel: true},
			}, CreatedAt: 42, UpdatedAt: 42},
			{ID: "msg-assistant-2b", SessionID: "session-1", Role: "assistant", Content: "done", Parts: []RuntimeMessagePart{
				{Type: "text", Text: "Done"},
			}, CreatedAt: 43, UpdatedAt: 44, Finished: true},
		},
		Turns: []RuntimeTurn{
			{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "msg-user-1", StartedAt: 10, FinishedAt: 25},
			{ID: "turn-2", SessionID: "session-1", Status: "completed", UserMessageID: "msg-user-2", LatestAssistantMessageID: "msg-assistant-2b", StartedAt: 15, FinishedAt: 50},
		},
		ToolCalls: []RuntimeToolCall{{
			ID: "tool-1", SessionID: "session-1", TurnID: "turn-2", MessageID: "msg-assistant-2a", Name: "view", Source: "builtin", Status: "completed", StartedAt: 41, FinishedAt: 42,
		}},
		Permissions: []RuntimePermissionRequest{{
			ID: "perm-1", SessionID: "session-1", TurnID: "turn-2", ToolCallID: "tool-2", ToolName: "bash", Action: "run", Status: "pending", CreatedAt: 45,
		}},
	}

	snapshot := buildRuntimeOutputProjection(activity).snapshot("session-1", "9")
	if len(snapshot.Messages) != 5 || snapshot.Messages[0].ClientRequestID != "client-1" || snapshot.Messages[1].ClientRequestID != "client-2" {
		t.Fatalf("messages not stable/client-linked: %#v", snapshot.Messages)
	}
	if len(snapshot.AssistantSteps) != 2 {
		t.Fatalf("assistant steps = %#v", snapshot.AssistantSteps)
	}
	if snapshot.AssistantSteps[0].TurnID != "turn-2" || snapshot.AssistantSteps[0].Index != 0 || snapshot.AssistantSteps[0].ToolCallIDs[0] != "tool-1" {
		t.Fatalf("first step = %#v", snapshot.AssistantSteps[0])
	}
	if len(snapshot.ToolResults) != 1 || snapshot.ToolResults[0].ToolCallID != "tool-1" || snapshot.ToolResults[0].TurnID != "turn-2" {
		t.Fatalf("tool results = %#v", snapshot.ToolResults)
	}
	if len(snapshot.ToolCalls) != 1 || snapshot.ToolCalls[0].AssistantStepID != snapshot.AssistantSteps[0].ID || snapshot.ToolCalls[0].LatestResultID != snapshot.ToolResults[0].ID {
		t.Fatalf("tool call result linkage = %#v result=%#v", snapshot.ToolCalls, snapshot.ToolResults)
	}
	if len(snapshot.Permissions) != 1 || snapshot.Permissions[0].Status != "pending" {
		t.Fatalf("permissions = %#v", snapshot.Permissions)
	}
}

func TestRuntimeOutputEventsUseStableMonotonicSubSequences(t *testing.T) {
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{{
			ID: "msg-assistant", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "tool_call", ToolCallID: "tool-1", Name: "bash"}}, CreatedAt: 10, UpdatedAt: 11,
		}, {
			ID: "msg-tool", SessionID: "session-1", Role: "tool", Parts: []RuntimeMessagePart{{Type: "tool_result", ToolCallID: "tool-1", Name: "bash", Content: "ok"}}, CreatedAt: 12,
		}},
		Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", StartedAt: 1}},
		ToolCalls: []RuntimeToolCall{{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "msg-assistant", Name: "bash", Source: "builtin", Status: "completed", StartedAt: 10, FinishedAt: 12}},
	})
	events := projection.eventsFromRuntimeEvents([]RuntimeEvent{{
		ID: "event-1", Sequence: 7, SessionID: "session-1", TurnID: "turn-1", MessageID: "msg-tool", ToolCallID: "tool-1", Type: "tool.call.output", CreatedAt: "2026-06-28T00:00:00Z",
	}})
	// tool.call event now also refreshes the turn's exploration_summary and
	// the tool_call conversation item, so we expect ≥3 sub-events, all with
	// strictly monotonic Sequences.
	if len(events) < 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].ToolCall == nil || events[1].ToolResult == nil {
		t.Fatalf("first two events order/linkage = %#v", events)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatalf("non-monotonic sub-sequence at %d: %#v", i, events)
		}
	}
	sawExploration := false
	for _, ev := range events {
		if ev.Item != nil && ev.Item.Kind == "exploration_summary" {
			sawExploration = true
		}
	}
	if !sawExploration {
		t.Fatalf("expected exploration_summary item event, got %#v", events)
	}
}

func TestRuntimeConversationProjectionSimpleFinalAnswer(t *testing.T) {
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10, UpdatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "hello"}}, CreatedAt: 20, UpdatedAt: 21, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-1", StartedAt: 1, FinishedAt: 30}},
	}).snapshot("session-1", "1")
	if snapshot.Version != 1 {
		t.Fatalf("version = %d", snapshot.Version)
	}
	assertConversationKinds(t, snapshot.Items, "user_message", "assistant_message")
	final := findConversationItem(t, snapshot.Items, "assistant_message")
	if final.Phase != "final" || final.Content != "hello" {
		t.Fatalf("final = %#v", final)
	}
}

func TestRuntimeConversationProjectionThinkingAndToolOnlyHidden(t *testing.T) {
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "check", CreatedAt: 10},
			{ID: "assistant-thinking", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "reasoning", Thinking: "plan"}}, CreatedAt: 20, UpdatedAt: 21, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-tool-only", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "tool_call", ToolCallID: "tool-1", Name: "view"}}, CreatedAt: 22, UpdatedAt: 23, Finished: true, FinishReason: "tool_use"},
		},
		Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		ToolCalls: []RuntimeToolCall{{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-tool-only", Name: "view", Source: "builtin", Status: "running", StartedAt: 24}},
	}).snapshot("session-1", "1")
	assertConversationKinds(t, snapshot.Items, "user_message", "exploration_summary", "assistant_thinking", "tool_call", "turn_progress")
	if item := findConversationItem(t, snapshot.Items, "assistant_message"); item.ID != "" {
		t.Fatalf("tool-only assistant produced message item: %#v", item)
	}
	exploration := findConversationItem(t, snapshot.Items, "exploration_summary")
	if exploration.Status != "exploring" || exploration.Exploration == nil || exploration.Exploration.ToolTotal != 1 {
		t.Fatalf("exploration summary = %#v", exploration)
	}
}

func TestRuntimeConversationProjectionCompletedThinkingHidden(t *testing.T) {
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-old", SessionID: "session-1", Role: "user", Content: "old", CreatedAt: 10},
			{ID: "assistant-old-thinking", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "reasoning", Thinking: "old plan"}}, CreatedAt: 20, UpdatedAt: 21, Finished: true, FinishReason: "tool_use"},
			{ID: "assistant-old-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "old done"}}, CreatedAt: 30, UpdatedAt: 31, Finished: true, FinishReason: "end_turn"},
			{ID: "user-new", SessionID: "session-1", Role: "user", Content: "new", CreatedAt: 40},
			{ID: "assistant-new-thinking", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "reasoning", Thinking: "new plan"}}, CreatedAt: 50, UpdatedAt: 51, Finished: true, FinishReason: "tool_use"},
		},
		Turns: []RuntimeTurn{
			{ID: "turn-old", SessionID: "session-1", Status: "completed", UserMessageID: "user-old", LatestAssistantMessageID: "assistant-old-final", StartedAt: 1, FinishedAt: 35},
			{ID: "turn-new", SessionID: "session-1", Status: "running", UserMessageID: "user-new", StartedAt: 40},
		},
	}).snapshot("session-1", "1")

	var thinking []RuntimeConversationItem
	for _, item := range snapshot.Items {
		if item.Kind == "assistant_thinking" {
			thinking = append(thinking, item)
		}
	}
	if len(thinking) != 1 {
		t.Fatalf("expected only active turn thinking item, got %#v", thinking)
	}
	if thinking[0].MessageID != "assistant-new-thinking" || thinking[0].Content != "new plan" {
		t.Fatalf("unexpected thinking item: %#v", thinking[0])
	}
	if thinking[0].Status != "running" {
		t.Fatalf("active thinking status = %q, want running", thinking[0].Status)
	}
}

func TestRuntimeConversationProjectionToolResultPermissionFailureAndOrdering(t *testing.T) {
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{
				{Type: "text", Text: "Checking"},
				{Type: "tool_call", ToolCallID: "tool-read-1", Name: "view"},
				{Type: "tool_call", ToolCallID: "tool-read-2", Name: "view"},
				{Type: "tool_call", ToolCallID: "tool-shell", Name: "bash"},
				{Type: "tool_call", ToolCallID: "tool-wait", Name: "bash"},
			}, CreatedAt: 20, UpdatedAt: 21, Finished: true, FinishReason: "tool_use"},
			{ID: "tool-result-msg", SessionID: "session-1", Role: "tool", Parts: []RuntimeMessagePart{{Type: "tool_result", ToolCallID: "tool-read-1", Name: "view", Content: "file", DeliveredToModel: true}}, CreatedAt: 30},
			{ID: "assistant-final", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "Done"}}, CreatedAt: 40, UpdatedAt: 41, Finished: true, FinishReason: "end_turn"},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-final", StartedAt: 1, FinishedAt: 50}},
		ToolCalls: []RuntimeToolCall{
			{ID: "tool-read-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", Display: RuntimeToolCallDisplay{Kind: "file_read"}, StartedAt: 22, FinishedAt: 23},
			{ID: "tool-read-2", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "view", Source: "builtin", Status: "completed", Display: RuntimeToolCallDisplay{Kind: "file_read"}, StartedAt: 24, FinishedAt: 25},
			{ID: "tool-shell", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "completed", ExitCode: 2, Display: RuntimeToolCallDisplay{Kind: "shell"}, StartedAt: 26, FinishedAt: 27},
			{ID: "tool-wait", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "waiting_permission", Display: RuntimeToolCallDisplay{Kind: "shell"}, StartedAt: 28, FinishedAt: 31},
		},
		Permissions: []RuntimePermissionRequest{{ID: "perm-1", SessionID: "session-1", TurnID: "turn-1", ToolCallID: "tool-wait", ToolName: "bash", Action: "run", Status: "denied", CreatedAt: 29, DecidedAt: 31}},
	}).snapshot("session-1", "1")
	assertConversationKinds(t, snapshot.Items, "user_message", "exploration_summary", "assistant_message", "tool_group", "tool_call", "tool_call", "permission_request", "assistant_message")
	if findConversationItem(t, snapshot.Items, "tool_result").ID != "" {
		t.Fatalf("tool result must not be a timeline item: %#v", snapshot.Items)
	}
	group := findConversationItem(t, snapshot.Items, "tool_group")
	if len(group.ToolCallIDs) != 2 || !group.Display.Quiet {
		t.Fatalf("tool group = %#v", group)
	}
	shell := findConversationItemByTool(t, snapshot.Items, "tool-shell")
	if shell.Status != "failed" {
		t.Fatalf("shell status = %#v", shell)
	}
	wait := findConversationItemByTool(t, snapshot.Items, "tool-wait")
	if wait.Status != "denied" || !wait.Display.DefaultExpanded {
		t.Fatalf("denied tool = %#v", wait)
	}
	final := snapshot.Items[len(snapshot.Items)-1]
	if final.Kind != "assistant_message" || final.Phase != "final" {
		t.Fatalf("final ordering = %#v in %#v", final, snapshot.Items)
	}
	if len(snapshot.ToolCalls) == 0 || snapshot.ToolCalls[0].Result.ID == "" {
		t.Fatalf("tool result not attached: %#v", snapshot.ToolCalls)
	}
}

func TestRuntimeConversationProjectionPermissionPending(t *testing.T) {
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 10},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "tool_call", ToolCallID: "tool-1", Name: "bash"}}, CreatedAt: 20, Finished: true, FinishReason: "tool_use"},
		},
		Turns:       []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		ToolCalls:   []RuntimeToolCall{{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "assistant-1", Name: "bash", Source: "shell", Status: "running", Display: RuntimeToolCallDisplay{Kind: "shell"}, StartedAt: 21}},
		Permissions: []RuntimePermissionRequest{{ID: "perm-1", SessionID: "session-1", TurnID: "turn-1", ToolCallID: "tool-1", ToolName: "bash", Action: "run", Status: "pending", CreatedAt: 22}},
	}).snapshot("session-1", "1")
	tool := findConversationItemByTool(t, snapshot.Items, "tool-1")
	if tool.Status != "waiting_permission" || !tool.Display.DefaultExpanded {
		t.Fatalf("pending permission tool = %#v", tool)
	}
	assertHasConversationKind(t, snapshot.Items, "permission_request")
}

func TestRuntimeConversationProjectionGovernanceItems(t *testing.T) {
	snapshot := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "work", CreatedAt: 10}},
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "interrupted", UserMessageID: "user-1", StartedAt: 1, FinishedAt: 100, Interrupted: &RuntimeInterruptedSummary{Reason: "runtime restarted"}}},
			ToolCalls: []RuntimeToolCall{{ID: "tool-agent", SessionID: "session-1", TurnID: "turn-1", Name: "agent", Source: "builtin", Status: "running", Display: RuntimeToolCallDisplay{Kind: "agent_task"}, StartedAt: 20}},
			Events: []RuntimeEvent{{
				ID:        "event-context",
				Sequence:  3,
				Type:      runtimeapi.EventContextSourceInjected,
				SessionID: "session-1",
				TurnID:    "turn-1",
				CreatedAt: "2026-06-28T00:00:00Z",
				Payload:   map[string]any{"source_id": "project:/work/AGENTS.md", "kind": "agents", "path": "/work/AGENTS.md", "state": "loaded", "reason": "runtime_selected", "content_summary": "project instructions"},
			}, {
				ID:        "event-todo",
				Sequence:  4,
				Type:      runtimeapi.EventTodoUpdated,
				SessionID: "session-1",
				TurnID:    "turn-1",
				CreatedAt: "2026-06-28T00:00:01Z",
			}},
		},
		Hooks: []RuntimeHookExecution{
			{ID: "hook-complete", SessionID: "session-1", TurnID: "turn-1", HookName: "noop", Status: hookStatusCompleted, StartedAt: 11, CompletedAt: 12},
			{ID: "hook-block", SessionID: "session-1", TurnID: "turn-1", HookName: "guard", Status: hookStatusBlocked, Reason: "blocked", StartedAt: 13, CompletedAt: 14},
			{ID: "hook-rewrite", SessionID: "session-1", TurnID: "turn-1", HookName: "rewrite", Status: hookStatusCompleted, InputRewritten: true, StartedAt: 15, CompletedAt: 16},
		},
		Tasks:   []RuntimeAgentTask{{ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", ParentToolCallID: "tool-agent", Title: "Investigate", Status: "running", StartedAt: 21, UpdatedAt: 22}},
		Todos:   &RuntimeTodoSummary{SessionID: "session-1", Todos: []RuntimeTodo{{Content: "a", Status: "pending"}}, Pending: 1, Total: 1, UpdatedAt: 30},
		Compact: []RuntimeCompactBoundary{{ID: "compact-1", SessionID: "session-1", TurnID: "turn-1", Kind: compactKindFull, Status: compactStatusCompleted, Trigger: "manual", CreatedAt: 40, CompletedAt: 41, MessageRefs: []string{"user-1"}}},
	}).snapshot("session-1", "1")
	assertHasConversationKind(t, snapshot.Items, "hook_run")
	assertHasConversationKind(t, snapshot.Items, "agent_task")
	assertHasConversationKind(t, snapshot.Items, "todo_summary")
	if todo := findConversationItem(t, snapshot.Items, "todo_summary"); todo.TurnID != "turn-1" || snapshot.Todos == nil || snapshot.Todos.TurnID != "turn-1" {
		t.Fatalf("todo turn ownership was not recovered: item=%#v summary=%#v", todo, snapshot.Todos)
	}
	assertHasConversationKind(t, snapshot.Items, "compact_boundary")
	assertHasConversationKind(t, snapshot.Items, "turn_terminal")
	assertHasConversationKind(t, snapshot.Items, "recovery_notice")
	compact := findConversationItem(t, snapshot.Items, "compact_boundary")
	if compact.ID != "compact-compact-1" || compact.Status != compactStatusCompleted {
		t.Fatalf("compact boundary item = %#v", compact)
	}
	for _, item := range snapshot.Items {
		if item.HookRunID == "hook-complete" {
			t.Fatalf("low-signal completed hook leaked into timeline: %#v", item)
		}
		if item.Kind == "context_source" && item.Status == "injected" {
			t.Fatalf("normal injected context source leaked into timeline: %#v", item)
		}
	}
	task := findConversationItem(t, snapshot.Items, "agent_task")
	if task.ToolCallID != "tool-agent" {
		t.Fatalf("agent task parent association = %#v", task)
	}
	tool := findConversationItemByTool(t, snapshot.Items, "tool-agent")
	if tool.Status != "interrupted" {
		t.Fatalf("unfinished tool after terminal turn = %#v", tool)
	}
}

// TestRuntimeConversationProjectionCompactItemPayload covers WP5 B10: the
// compact_boundary item's Compact payload must carry trigger/token deltas/
// summarized count/summary message id, and the full summary text looked up
// via SummaryTexts (populated separately since summary messages are
// filtered out of the regular session message list).
func TestRuntimeConversationProjectionCompactItemPayload(t *testing.T) {
	snapshot := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10}},
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", StartedAt: 1, FinishedAt: 100}},
		},
		Compact: []RuntimeCompactBoundary{{
			ID:               "compact-1",
			SessionID:        "session-1",
			TurnID:           "turn-1",
			Kind:             compactKindFull,
			Status:           compactStatusCompleted,
			Trigger:          "manual",
			SummaryMessageID: "summary-1",
			MessageRefs:      []string{"m1", "m2", "m3"},
			BudgetBefore:     &RuntimeBudgetReport{TotalEstimatedTokens: 5000},
			BudgetAfter:      &RuntimeBudgetReport{TotalEstimatedTokens: 800},
			CreatedAt:        40,
			CompletedAt:      41,
		}},
		SummaryTexts: map[string]string{"summary-1": "the compacted summary text"},
	}).snapshot("session-1", "1")
	compact := findConversationItem(t, snapshot.Items, "compact_boundary")
	if compact.Compact == nil {
		t.Fatalf("compact info missing: %#v", compact)
	}
	info := compact.Compact
	if info.Trigger != "manual" || info.Status != compactStatusCompleted || info.PreTokens != 5000 || info.PostTokens != 800 ||
		info.SummarizedCount != 3 || info.SummaryMessageID != "summary-1" || info.SummaryText != "the compacted summary text" {
		t.Fatalf("compact info = %#v", info)
	}
}

// TestRuntimeConversationProjectionCompactSummaryTextTruncated covers the
// ≤4000 rune truncation in summaryMessageTexts.
func TestRuntimeConversationProjectionCompactSummaryTextTruncated(t *testing.T) {
	long := make([]rune, runtimeCompactSummaryTextLimit+100)
	for i := range long {
		long[i] = 'a'
	}
	truncated := truncateRunes(string(long), runtimeCompactSummaryTextLimit)
	runes := []rune(truncated)
	if len(runes) != runtimeCompactSummaryTextLimit+1 || runes[len(runes)-1] != '…' {
		t.Fatalf("truncateRunes did not clip to the limit with an ellipsis: len=%d last=%q", len(runes), string(runes[len(runes)-1]))
	}
}

// TestRuntimeConversationProjectionAutoFailedCompactProducesNoItem covers B8:
// an auto-triggered compact that failed must not produce a timeline item —
// auto retries silently, the user is never bothered.
func TestRuntimeConversationProjectionAutoFailedCompactProducesNoItem(t *testing.T) {
	snapshot := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10}},
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		},
		Compact: []RuntimeCompactBoundary{{
			ID: "compact-auto-fail", SessionID: "session-1", TurnID: "turn-1", Kind: compactKindFull,
			Status: compactStatusFailed, Trigger: "auto", Error: "prompt too long", CreatedAt: 40, CompletedAt: 41,
		}},
	}).snapshot("session-1", "1")
	if item := findConversationItem(t, snapshot.Items, "compact_boundary"); item.ID != "" {
		t.Fatalf("auto-failed compact must not produce a timeline item: %#v", item)
	}
}

// TestRuntimeConversationProjectionCircuitOpenAutoFailureProducesItem covers
// the circuit-breaker exception to B8: the auto failure that opened the
// circuit is marked on the boundary error and must surface as a divider (the
// marker itself is stripped from the user-visible error).
func TestRuntimeConversationProjectionCircuitOpenAutoFailureProducesItem(t *testing.T) {
	snapshot := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10}},
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		},
		Compact: []RuntimeCompactBoundary{{
			ID: "compact-circuit-open", SessionID: "session-1", TurnID: "turn-1", Kind: compactKindFull,
			Status: compactStatusFailed, Trigger: "auto",
			Error:     compactCircuitOpenMarker + "prompt too long",
			CreatedAt: 40, CompletedAt: 41,
		}},
	}).snapshot("session-1", "1")
	item := findConversationItem(t, snapshot.Items, "compact_boundary")
	if item.ID == "" || item.Status != compactStatusFailed {
		t.Fatalf("circuit-opening auto failure must produce a timeline item: %#v", item)
	}
	if item.Error != "prompt too long" || item.Compact == nil || item.Compact.Error != "prompt too long" {
		t.Fatalf("circuit-open marker must be stripped from user-visible error: %#v", item)
	}
}

// TestRuntimeConversationProjectionManualFailedCompactProducesItem covers the
// counterpart of B8: manual (and, later, circuit_open) failures still show a
// failed divider so the user knows their /compact did not go through.
func TestRuntimeConversationProjectionManualFailedCompactProducesItem(t *testing.T) {
	snapshot := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: 10}},
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		},
		Compact: []RuntimeCompactBoundary{{
			ID: "compact-manual-fail", SessionID: "session-1", TurnID: "turn-1", Kind: compactKindFull,
			Status: compactStatusFailed, Trigger: "manual", Error: "boom", CreatedAt: 40, CompletedAt: 41,
		}},
	}).snapshot("session-1", "1")
	item := findConversationItem(t, snapshot.Items, "compact_boundary")
	if item.ID == "" || item.Status != compactStatusFailed || item.Compact == nil || item.Compact.Error != "boom" {
		t.Fatalf("manual-failed compact item = %#v", item)
	}
}

// TestCompactEventsAlwaysPairStartedWithTerminal is the integration
// assertion from WP5's test plan: every compact.started event must
// eventually be matched by a compact.completed or compact.failed event for
// the same boundary_id, across both a successful and a failed manual
// compact.
func TestCompactEventsAlwaysPairStartedWithTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "compact pairing")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID

	// Induce a failed boundary: an empty session makes runManualFullCompact
	// fail right after it has already emitted compact.started.
	if _, err := service.ManualCompact(ctx, RuntimeContextActionRequest{SessionID: session.ID, TurnID: "turn-fail"}); err == nil {
		t.Fatal("expected manual compact on an empty session to fail")
	}

	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ManualCompact(ctx, RuntimeContextActionRequest{SessionID: session.ID, TurnID: "turn-ok"}); err != nil {
		t.Fatal(err)
	}

	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := map[string]bool{}
	terminal := map[string]bool{}
	for _, event := range events.Events {
		boundaryID, _ := event.Payload["boundary_id"].(string)
		if boundaryID == "" {
			continue
		}
		switch event.Type {
		case runtimeapi.EventCompactStarted:
			started[boundaryID] = true
		case runtimeapi.EventCompactCompleted, runtimeapi.EventCompactFailed:
			terminal[boundaryID] = true
		}
	}
	if len(started) < 2 {
		t.Fatalf("expected at least 2 distinct started boundaries (one failed, one completed), got %d: %#v", len(started), events.Events)
	}
	for id := range started {
		if !terminal[id] {
			t.Fatalf("boundary %s emitted compact.started without a matching completed/failed event", id)
		}
	}
}

func TestRuntimeConversationProjectionContextEventReplay(t *testing.T) {
	event := RuntimeEvent{
		ID:        "event-context",
		Sequence:  9,
		Type:      runtimeapi.EventContextSourceFailed,
		SessionID: "session-1",
		TurnID:    "turn-1",
		CreatedAt: "2026-06-28T00:00:00Z",
		Payload:   map[string]any{"source_id": "file:/work/broken.md", "kind": "file", "path": "/work/broken.md", "error": "unreadable"},
	}
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "work", CreatedAt: 10}},
		Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", UserMessageID: "user-1", StartedAt: 1}},
		Events:    []RuntimeEvent{event},
	})
	snapshot := projection.snapshot("session-1", "9")
	item := findConversationItem(t, snapshot.Items, "context_source")
	if item.Status != "failed" || item.Error != "unreadable" {
		t.Fatalf("context source item = %#v", item)
	}
	events := projection.eventsFromRuntimeEvents([]RuntimeEvent{event})
	found := false
	for _, outputEvent := range events {
		if outputEvent.Item != nil && outputEvent.Item.Kind == "context_source" && outputEvent.Item.ContextID == "file:/work/broken.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("context item event missing: %#v", events)
	}
}

func TestRuntimeConversationProjectionNormalContextInjectedHidden(t *testing.T) {
	event := RuntimeEvent{
		ID:        "event-context-injected",
		Sequence:  10,
		Type:      runtimeapi.EventContextSourceInjected,
		SessionID: "session-1",
		TurnID:    "turn-1",
		CreatedAt: "2026-06-28T00:00:00Z",
		Payload:   map[string]any{"source_id": "project:/work/AGENTS.md", "kind": "agents", "path": "/work/AGENTS.md", "reason": "runtime_selected"},
	}
	projection := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages:  []RuntimeMessage{{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hello", CreatedAt: 10}, {ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "Hi"}}, CreatedAt: 20, Finished: true, FinishReason: "end_turn"}},
		Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed", UserMessageID: "user-1", LatestAssistantMessageID: "assistant-1", StartedAt: 1, FinishedAt: 30}},
		Events:    []RuntimeEvent{event},
	})
	for _, item := range projection.snapshot("session-1", "10").Items {
		if item.Kind == "context_source" {
			t.Fatalf("normal injected context source should be diagnostics-only: %#v", item)
		}
	}
	for _, outputEvent := range projection.eventsFromRuntimeEvents([]RuntimeEvent{event}) {
		if outputEvent.Item != nil && outputEvent.Item.Kind == "context_source" {
			t.Fatalf("normal injected context source emitted item event: %#v", outputEvent)
		}
	}
}

func assertConversationKinds(t *testing.T, items []RuntimeConversationItem, want ...string) {
	t.Helper()
	var got []string
	for _, item := range items {
		got = append(got, item.Kind)
	}
	if len(got) != len(want) {
		t.Fatalf("kinds = %#v, want %#v; items=%#v", got, want, items)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %#v, want %#v; items=%#v", got, want, items)
		}
	}
}

func assertHasConversationKind(t *testing.T, items []RuntimeConversationItem, kind string) {
	t.Helper()
	if item := findConversationItem(t, items, kind); item.ID == "" {
		t.Fatalf("missing kind %q in %#v", kind, items)
	}
}

func findConversationItem(t *testing.T, items []RuntimeConversationItem, kind string) RuntimeConversationItem {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind {
			return item
		}
	}
	return RuntimeConversationItem{}
}

func findConversationItemByTool(t *testing.T, items []RuntimeConversationItem, toolCallID string) RuntimeConversationItem {
	t.Helper()
	for _, item := range items {
		if item.ToolCallID == toolCallID {
			return item
		}
	}
	t.Fatalf("missing tool item %q in %#v", toolCallID, items)
	return RuntimeConversationItem{}
}

func TestRuntimeConversationItemSequenceStaysFloat64SafeAndOrdered(t *testing.T) {
	turnStart := int64(1_751_500_000_123) // realistic UnixMilli
	const maxSafeInteger = int64(1) << 53
	ranks := []int{0, 500, 1000, 1010, 1020, 1030, runtimeConversationRankFinal}
	previous := int64(-1)
	for _, rank := range ranks {
		sequence := runtimeConversationItemSequence(turnStart, rank, turnStart+int64(rank))
		if sequence >= maxSafeInteger {
			t.Fatalf("sequence %d for rank %d exceeds float64-safe range", sequence, rank)
		}
		if sequence <= previous {
			t.Fatalf("rank %d sequence %d not greater than previous %d", rank, sequence, previous)
		}
		previous = sequence
	}
	// A turn starting one second later must sort after every item of the
	// previous turn, including its final answer.
	lastOfEarlierTurn := runtimeConversationItemSequence(turnStart, runtimeConversationRankFinal, turnStart+60_000)
	firstOfLaterTurn := runtimeConversationItemSequence(turnStart+1000, 0, turnStart+1000)
	if firstOfLaterTurn <= lastOfEarlierTurn {
		t.Fatalf("later turn sequence %d not after earlier turn final %d", firstOfLaterTurn, lastOfEarlierTurn)
	}
	// Items created later within the same rank never sort earlier.
	early := runtimeConversationItemSequence(turnStart, 1020, turnStart+50)
	late := runtimeConversationItemSequence(turnStart, 1020, turnStart+950)
	if late < early {
		t.Fatalf("later item %d sorts before earlier item %d within the same rank", late, early)
	}
}

func TestRuntimeConversationProjectionStreamingTurnAttributionFallback(t *testing.T) {
	// Mid-stream the turn record may not yet reference the user or the
	// streaming assistant message. Attribution must fall back to timestamps
	// so no item lands in the turnStart==0 bucket (which would collapse its
	// sequence to a near-zero value and pin it to the top of the timeline).
	turnStart := int64(1_751_500_000_000)
	snapshot := buildRuntimeOutputProjection(RuntimeSessionActivityWindowResponse{
		SessionID: "session-1",
		Messages: []RuntimeMessage{
			{ID: "user-1", SessionID: "session-1", Role: "user", Content: "hi", CreatedAt: turnStart - 40, UpdatedAt: turnStart - 40},
			{ID: "assistant-1", SessionID: "session-1", Role: "assistant", Parts: []RuntimeMessagePart{{Type: "text", Text: "strea"}}, CreatedAt: turnStart + 250, UpdatedAt: turnStart + 300},
		},
		Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running", StartedAt: turnStart}},
	}).snapshot("session-1", "1")

	user := findConversationItem(t, snapshot.Items, "user_message")
	if user.TurnID != "turn-1" {
		t.Fatalf("user message not attributed to turn: %#v", user)
	}
	assistant := findConversationItem(t, snapshot.Items, "assistant_message")
	if assistant.ID == "" || assistant.TurnID != "turn-1" {
		t.Fatalf("assistant message not attributed to turn: %#v", assistant)
	}
	minSequence := (turnStart / 100) * runtimeConversationSequenceSpan
	for _, item := range snapshot.Items {
		if item.Sequence < minSequence {
			t.Fatalf("item %q sequence %d fell below the turn base %d", item.ID, item.Sequence, minSequence)
		}
	}
	if user.Sequence >= assistant.Sequence {
		t.Fatalf("user sequence %d not before assistant sequence %d", user.Sequence, assistant.Sequence)
	}
}
