package runtime

import "testing"

func TestRuntimeReactCallchainTextOnlyAssistantFinal(t *testing.T) {
	resp := buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID: "session-1",
		turnID:    "turn-1",
		turns: []RuntimeTurn{{
			ID:                       "turn-1",
			SessionID:                "session-1",
			Status:                   turnStatusCompleted,
			UserMessageID:            "msg-user",
			LatestAssistantMessageID: "msg-final",
			StartedAt:                1000,
			FinishedAt:               2000,
		}},
		messages: []RuntimeMessage{
			{ID: "msg-user", SessionID: "session-1", Role: "user", Content: "hello", CreatedAt: 1000, UpdatedAt: 1000},
			{ID: "msg-final", SessionID: "session-1", Role: "assistant", Content: "hi", CreatedAt: 1100, UpdatedAt: 1200, Finished: true, FinishReason: "end_turn", Parts: []RuntimeMessagePart{{Type: "text", Text: "hi"}, {Type: "finish", Reason: "end_turn"}}},
		},
	})

	if !resp.Summary.HasFinalAssistant || resp.Summary.FinalAssistantMessageID != "msg-final" {
		t.Fatalf("final summary = %#v", resp.Summary)
	}
	if resp.Summary.StopReason != "model_stop" {
		t.Fatalf("stop reason = %q", resp.Summary.StopReason)
	}
	assertReactKinds(t, resp, []string{reactNodeUserInput, reactNodeAssistantFinal, reactNodeTurnTerminal})
	if !resp.Source.SessionActivityParity || !resp.Source.EventsAreRefreshOnly {
		t.Fatalf("source = %#v", resp.Source)
	}
}

func TestRuntimeReactCallchainToolThenFinalAssistant(t *testing.T) {
	resp := buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID: "session-1",
		turnID:    "turn-1",
		turns: []RuntimeTurn{{
			ID:                       "turn-1",
			SessionID:                "session-1",
			Status:                   turnStatusCompleted,
			UserMessageID:            "msg-user",
			LatestAssistantMessageID: "msg-final",
			StartedAt:                1000,
			FinishedAt:               3000,
		}},
		messages: []RuntimeMessage{
			{ID: "msg-user", SessionID: "session-1", Role: "user", Content: "list files", CreatedAt: 1000, UpdatedAt: 1000},
			{ID: "msg-step", SessionID: "session-1", Role: "assistant", Content: "I'll inspect.", CreatedAt: 1100, UpdatedAt: 1200, Finished: true, FinishReason: "tool_use", Parts: []RuntimeMessagePart{{Type: "text", Text: "I'll inspect."}, {Type: "tool_call", ToolCallID: "tool-1", Name: "ls", Input: "{}"}, {Type: "finish", Reason: "tool_use"}}},
			{ID: "msg-tool", SessionID: "session-1", Role: "tool", CreatedAt: 1300, UpdatedAt: 1300, Parts: []RuntimeMessagePart{{Type: "tool_result", ToolCallID: "tool-1", Name: "ls", Content: "a.txt"}}},
			{ID: "msg-final", SessionID: "session-1", Role: "assistant", Content: "Found a.txt.", CreatedAt: 1400, UpdatedAt: 1500, Finished: true, FinishReason: "end_turn", Parts: []RuntimeMessagePart{{Type: "text", Text: "Found a.txt."}, {Type: "finish", Reason: "end_turn"}}},
		},
		toolCalls: []RuntimeToolCall{{
			ID:            "tool-1",
			SessionID:     "session-1",
			TurnID:        "turn-1",
			MessageID:     "msg-step",
			Name:          "ls",
			Source:        "builtin",
			Status:        "completed",
			InputSummary:  "{}",
			OutputSummary: "a.txt",
			StartedAt:     1150,
			FinishedAt:    1300,
		}},
	})

	if resp.Summary.ToolCallCount != 1 || !resp.Summary.HasFinalAssistant {
		t.Fatalf("summary = %#v", resp.Summary)
	}
	assertReactKinds(t, resp, []string{
		reactNodeUserInput,
		reactNodeAssistantStep,
		reactNodeToolCall,
		reactNodeToolResult,
		reactNodeAssistantFinal,
		reactNodeTurnTerminal,
	})
	toolResult := findReactNode(resp, reactNodeToolResult)
	if toolResult.ParentID != "tool_call:tool-1" {
		t.Fatalf("tool result parent = %q", toolResult.ParentID)
	}
}

func TestRuntimeReactCallchainReportsMissingFinalAndOrphanResult(t *testing.T) {
	resp := buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID: "session-1",
		turnID:    "turn-1",
		turns: []RuntimeTurn{{
			ID:            "turn-1",
			SessionID:     "session-1",
			Status:        turnStatusCompleted,
			UserMessageID: "msg-user",
			StartedAt:     1000,
			FinishedAt:    2000,
		}},
		messages: []RuntimeMessage{
			{ID: "msg-user", SessionID: "session-1", Role: "user", Content: "run", CreatedAt: 1000},
			{ID: "msg-tool", SessionID: "session-1", Role: "tool", CreatedAt: 1200, Parts: []RuntimeMessagePart{{Type: "tool_result", ToolCallID: "tool-orphan", Name: "bash", Content: "ok"}}},
		},
	})

	if resp.Summary.HasFinalAssistant {
		t.Fatalf("unexpected final assistant: %#v", resp.Summary)
	}
	if resp.Summary.StopReason != "completed_without_final_assistant" {
		t.Fatalf("stop reason = %q", resp.Summary.StopReason)
	}
	assertMissingEvidence(t, resp, "tool_result_without_assistant_tool_call:tool-orphan")
	assertMissingEvidence(t, resp, "turn_completed_without_final_assistant")
}

func TestRuntimeReactCallchainGroupsPermissionAndHookUnderTool(t *testing.T) {
	resp := buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID: "session-1",
		turnID:    "turn-1",
		turns: []RuntimeTurn{{
			ID:            "turn-1",
			SessionID:     "session-1",
			Status:        turnStatusFailed,
			UserMessageID: "msg-user",
			StartedAt:     1000,
			FinishedAt:    2000,
			Error:         "blocked",
		}},
		messages: []RuntimeMessage{
			{ID: "msg-user", SessionID: "session-1", Role: "user", Content: "delete", CreatedAt: 1000},
			{ID: "msg-step", SessionID: "session-1", Role: "assistant", Content: "I'll run it.", CreatedAt: 1100, Finished: true, FinishReason: "tool_use", Parts: []RuntimeMessagePart{{Type: "tool_call", ToolCallID: "tool-1", Name: "bash", Input: "rm -rf tmp"}}},
		},
		toolCalls: []RuntimeToolCall{{
			ID:        "tool-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			MessageID: "msg-step",
			Name:      "bash",
			Status:    "denied",
			StartedAt: 1150,
		}},
		permissions: []RuntimePermissionRequest{{
			ID:         "perm-1",
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "tool-1",
			ToolName:   "bash",
			Action:     "execute",
			Status:     permissionStatusDenied,
			Decision:   "deny",
			CreatedAt:  1160,
			DecidedAt:  1170,
		}},
		hooks: []RuntimeHookExecution{{
			ID:          "hook-1",
			SessionID:   "session-1",
			TurnID:      "turn-1",
			ToolCallID:  "tool-1",
			HookID:      "pre",
			Event:       "PreToolUse",
			Status:      hookStatusBlocked,
			Reason:      "blocked by hook",
			StartedAt:   1180,
			CompletedAt: 1190,
		}},
	})

	if resp.Summary.PermissionCount != 1 || resp.Summary.HookCount != 1 {
		t.Fatalf("summary = %#v", resp.Summary)
	}
	if resp.Summary.StopReason != "permission_denied" {
		t.Fatalf("stop reason = %q", resp.Summary.StopReason)
	}
	perm := findReactNode(resp, reactNodePermissionDecision)
	hook := findReactNode(resp, reactNodeHookExecution)
	if perm.ParentID != "tool_call:tool-1" || hook.ParentID != "tool_call:tool-1" {
		t.Fatalf("parents permission=%q hook=%q", perm.ParentID, hook.ParentID)
	}
}

func TestRuntimeReactCallchainLeavesUnlinkedPermissionAndHookUnparented(t *testing.T) {
	resp := buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID: "session-1",
		turnID:    "turn-1",
		turns: []RuntimeTurn{{
			ID:        "turn-1",
			SessionID: "session-1",
			Status:    turnStatusCompleted,
			StartedAt: 1000,
		}},
		permissions: []RuntimePermissionRequest{{
			ID:        "perm-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			Status:    permissionStatusCancelled,
			CreatedAt: 1100,
		}},
		hooks: []RuntimeHookExecution{{
			ID:        "hook-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			Event:     "Stop",
			Status:    hookStatusCompleted,
			StartedAt: 1200,
		}},
	})

	perm := findReactNode(resp, reactNodePermissionDecision)
	hook := findReactNode(resp, reactNodeHookExecution)
	if perm.ParentID != "" || hook.ParentID != "" {
		t.Fatalf("parents permission=%q hook=%q", perm.ParentID, hook.ParentID)
	}
}

func TestRuntimeReactLimitTurnsKeepsMostRecentTurns(t *testing.T) {
	turns := []RuntimeTurn{
		{ID: "turn-1"},
		{ID: "turn-2"},
		{ID: "turn-3"},
	}

	got := runtimeReactLimitTurns(turns, 2)
	if len(got) != 2 || got[0].ID != "turn-2" || got[1].ID != "turn-3" {
		t.Fatalf("limited turns = %#v", got)
	}

	got[0].ID = "mutated"
	if turns[1].ID != "turn-2" {
		t.Fatalf("limit returned aliased slice: %#v", turns)
	}
}

func assertReactKinds(t *testing.T, resp RuntimeReactCallchainResponse, kinds []string) {
	t.Helper()
	if len(resp.Nodes) != len(kinds) {
		t.Fatalf("node count = %d want %d nodes=%#v", len(resp.Nodes), len(kinds), resp.Nodes)
	}
	for i, kind := range kinds {
		if resp.Nodes[i].Kind != kind {
			t.Fatalf("node %d kind = %q want %q nodes=%#v", i, resp.Nodes[i].Kind, kind, resp.Nodes)
		}
		if resp.Nodes[i].Sequence != i+1 {
			t.Fatalf("node %d sequence = %d", i, resp.Nodes[i].Sequence)
		}
	}
}

func findReactNode(resp RuntimeReactCallchainResponse, kind string) RuntimeReactCallNode {
	for _, node := range resp.Nodes {
		if node.Kind == kind {
			return node
		}
	}
	return RuntimeReactCallNode{}
}

func assertMissingEvidence(t *testing.T, resp RuntimeReactCallchainResponse, want string) {
	t.Helper()
	for _, got := range resp.Summary.MissingEvidence {
		if got == want {
			return
		}
	}
	t.Fatalf("missing evidence %q not found in %#v", want, resp.Summary.MissingEvidence)
}
