package runtime

import (
	"testing"

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
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if !(events[0].Sequence < events[1].Sequence && events[1].Sequence < events[2].Sequence) || events[0].ToolCall == nil || events[1].ToolResult == nil || events[2].Item == nil {
		t.Fatalf("event order/linkage = %#v", events)
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
	assertConversationKinds(t, snapshot.Items, "user_message", "assistant_thinking", "tool_call", "turn_progress")
	if item := findConversationItem(t, snapshot.Items, "assistant_message"); item.ID != "" {
		t.Fatalf("tool-only assistant produced message item: %#v", item)
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
	assertConversationKinds(t, snapshot.Items, "user_message", "assistant_message", "tool_group", "tool_call", "tool_call", "permission_request", "assistant_message")
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
			}},
		},
		Hooks: []RuntimeHookExecution{
			{ID: "hook-complete", SessionID: "session-1", TurnID: "turn-1", HookName: "noop", Status: hookStatusCompleted, StartedAt: 11, CompletedAt: 12},
			{ID: "hook-block", SessionID: "session-1", TurnID: "turn-1", HookName: "guard", Status: hookStatusBlocked, Reason: "blocked", StartedAt: 13, CompletedAt: 14},
			{ID: "hook-rewrite", SessionID: "session-1", TurnID: "turn-1", HookName: "rewrite", Status: hookStatusCompleted, InputRewritten: true, StartedAt: 15, CompletedAt: 16},
		},
		Tasks:   []RuntimeAgentTask{{ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", ParentToolCallID: "tool-agent", Title: "Investigate", Status: "running", StartedAt: 21, UpdatedAt: 22}},
		Todos:   &RuntimeTodoSummary{SessionID: "session-1", TurnID: "turn-1", Todos: []RuntimeTodo{{Content: "a", Status: "pending"}}, Pending: 1, Total: 1, UpdatedAt: 30},
		Compact: []RuntimeCompactBoundary{{ID: "compact-1", SessionID: "session-1", TurnID: "turn-1", Kind: compactKindMicro, Status: compactStatusCompleted, Trigger: "auto", CreatedAt: 40, CompletedAt: 41, ToolCallRefs: []RuntimeCompactToolCallRef{{ToolCallID: "tool-agent", Replacement: "summary", Preserved: true, Reason: "large output"}}}},
	}).snapshot("session-1", "1")
	assertHasConversationKind(t, snapshot.Items, "hook_run")
	assertHasConversationKind(t, snapshot.Items, "agent_task")
	assertHasConversationKind(t, snapshot.Items, "todo_summary")
	assertHasConversationKind(t, snapshot.Items, "microcompact_marker")
	assertHasConversationKind(t, snapshot.Items, "tool_result_replacement")
	assertHasConversationKind(t, snapshot.Items, "turn_terminal")
	assertHasConversationKind(t, snapshot.Items, "recovery_notice")
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
