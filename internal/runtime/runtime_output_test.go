package runtime

import "testing"

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
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if !(events[0].Sequence < events[1].Sequence) || events[0].ToolCall == nil || events[1].ToolResult == nil {
		t.Fatalf("event order/linkage = %#v", events)
	}
}
