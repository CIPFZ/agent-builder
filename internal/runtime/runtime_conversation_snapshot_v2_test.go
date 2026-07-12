package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestCanonicalEventRangesOnlyTrackSemanticEntityEvents(t *testing.T) {
	events := []RuntimeEvent{
		{Sequence: 10, Type: "turn.started", TurnID: "turn-1"},
		{Sequence: 11, Type: "context.usage.updated", TurnID: "turn-1"},
		{Sequence: 12, Type: "message.created", TurnID: "turn-1", MessageID: "message-1"},
		{Sequence: 13, Type: "tool.call.started", TurnID: "turn-1", ToolCallID: "tool-1"},
		{Sequence: 14, Type: "tool.call.output", TurnID: "turn-1", ToolCallID: "tool-1"},
		{Sequence: 15, Type: "turn.completed", TurnID: "turn-1"},
	}
	index := canonicalEventRanges(events)
	if got := index["turn:turn-1"]; got.first != 10 || got.last != 15 {
		t.Fatalf("turn range = %#v", got)
	}
	if got := index["message:message-1"]; got.first != 12 || got.last != 12 {
		t.Fatalf("message range = %#v", got)
	}
	if got := index["toolCall:tool-1"]; got.first != 13 || got.last != 14 {
		t.Fatalf("tool range = %#v", got)
	}
}

func TestCanonicalFinalMessageRejectsToolUseAndRequiresTerminalTurn(t *testing.T) {
	message := RuntimeMessage{ID: "assistant-1", Role: "assistant", Finished: true, FinishReason: "tool_use"}
	turn := RuntimeTurn{ID: "turn-1", Status: "completed", LatestAssistantMessageID: message.ID, FinishedAt: 10}
	if got := canonicalFinalMessageID(turn, []RuntimeMessage{message}); got != "" {
		t.Fatalf("tool-use message became final: %q", got)
	}
	message.FinishReason = "stop"
	if got := canonicalFinalMessageID(turn, []RuntimeMessage{message}); got != message.ID {
		t.Fatalf("final id = %q", got)
	}
	turn.Status = "running"
	if got := canonicalFinalMessageID(turn, []RuntimeMessage{message}); got != "" {
		t.Fatalf("running turn has final: %q", got)
	}
	message.Metadata = map[string]string{"conversation_phase": "final"}
	if got := canonicalFinalMessageID(turn, []RuntimeMessage{message}); got != "" {
		t.Fatalf("metadata bypassed terminal final gate: %q", got)
	}
}

func TestCanonicalMessagePhaseReadsSnakeCaseWithoutBypassingFinalGate(t *testing.T) {
	msg := RuntimeMessage{ID: "assistant-1", Role: "assistant", Finished: true, Metadata: map[string]string{"conversation_phase": "reasoning"}}
	turn := RuntimeTurn{ID: "turn-1", Status: "running", LatestAssistantMessageID: msg.ID}
	if got := canonicalMessagePhase(msg, turn, []RuntimeMessage{msg}); got != RuntimeConversationPhaseReasoning {
		t.Fatalf("snake phase=%q", got)
	}
	msg.Metadata["conversation_phase"] = "final"
	if got := canonicalMessagePhase(msg, turn, []RuntimeMessage{msg}); got != RuntimeConversationPhaseIntermediate {
		t.Fatalf("metadata bypassed final gate: %q", got)
	}
}

func TestCanonicalWindowFiltersAfterStableIdentityConstruction(t *testing.T) {
	s := RuntimeCanonicalConversationSnapshot{Scope: RuntimeConversationScopeFull,
		Turns:          []RuntimeCanonicalTurn{{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-1"}}, {RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-2"}}},
		Messages:       []RuntimeCanonicalMessage{{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "message-1", TurnID: "turn-1", ActivitySequence: "12", Revision: "20"}}, {RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "message-2", TurnID: "turn-2", ActivitySequence: "30", Revision: "31"}}},
		AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}
	applyCanonicalWindow(&s, RuntimeCanonicalConversationSnapshotRequest{Scope: "window", Limit: 1})
	if len(s.Turns) != 1 || s.Turns[0].ID != "turn-2" || len(s.Messages) != 1 || s.Messages[0].ID != "message-2" {
		t.Fatalf("window = %#v", s)
	}
	if s.Messages[0].ActivitySequence != "30" || s.Messages[0].Revision != "31" {
		t.Fatalf("window changed identity metadata: %#v", s.Messages[0])
	}
}

func TestCanonicalTodoPlansRequirePersistedStableItemIDs(t *testing.T) {
	events := []RuntimeEvent{{Sequence: 20, ID: "event-1", Type: "todo.updated", SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{"plan_id": "plan-1", "todos": []any{map[string]any{"id": "item-1", "content": "Do it", "status": "pending"}}}}}
	plans := canonicalTodoPlans(events, "session-1", canonicalEventRanges(events))
	if len(plans) != 1 || plans[0].ID != "plan-1" || plans[0].Items[0].ID != "item-1" || plans[0].OwnerTurnID != "turn-1" {
		t.Fatalf("plans = %#v", plans)
	}
	events[0].Payload["todos"] = []any{map[string]any{"content": "legacy"}}
	if legacy := canonicalTodoPlans(events, "session-1", canonicalEventRanges(events)); len(legacy) != 0 {
		t.Fatalf("legacy todo received fake identity: %#v", legacy)
	}
}

func TestCanonicalTodoPlanKeepsFirstCreatedAtAcrossUpdates(t *testing.T) {
	events := []RuntimeEvent{
		{Sequence: 20, Type: "todo.updated", SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{"plan_id": "plan-1", "todos": []any{map[string]any{"id": "item-1", "content": "Do it", "status": "pending"}}}},
		{Sequence: 30, Type: "todo.updated", SessionID: "session-1", TurnID: "turn-2", CreatedAt: "2026-01-01T00:01:00Z", Payload: map[string]any{"plan_id": "plan-1", "todos": []any{map[string]any{"id": "item-1", "content": "Do it", "status": "completed"}}}},
	}
	plans := canonicalTodoPlans(events, "session-1", canonicalEventRanges(events))
	if len(plans) != 1 {
		t.Fatalf("plans=%#v", plans)
	}
	if plans[0].CreatedAt != parseRuntimeEventMillis(events[0].CreatedAt) || plans[0].UpdatedAt != parseRuntimeEventMillis(events[1].CreatedAt) || plans[0].Revision != "30" || plans[0].OwnerTurnID != "turn-1" {
		t.Fatalf("unstable todo meta: %#v", plans[0])
	}
}

func TestCanonicalEntityMetaPreservesSequenceBeyondJavaScriptSafeInteger(t *testing.T) {
	const sequence int64 = 9007199254740993
	meta := canonicalMeta("message", "message-1", "session-1", "turn-1", 1, 1, map[string]canonicalEventRange{"message:message-1": {first: sequence, last: sequence}})
	if meta.ActivitySequence != "9007199254740993" || meta.Revision != "9007199254740993" {
		t.Fatalf("precision lost: %#v", meta)
	}
}

func TestSessionConversationSnapshotV2ReconstructsIdenticallyAfterRestart(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(context.Background(), h.service.workspace.ID, "canonical restart")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hello"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "done"}, message.Finish{Reason: "stop"}}, Metadata: map[string]string{"turn_id": "turn-1", "conversation_phase": "final"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", UserMessageID: user.ID, LatestAssistantMessageID: assistant.ID, StartedAt: user.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{ID: "orphan-tool", SessionID: session.ID, TurnID: "recovered-missing-turn", Name: "shell", Source: scheduler.ToolSourceShell, Command: "echo ok"}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []RuntimeEvent{{Type: runtimeapi.EventTurnStarted, SessionID: session.ID, TurnID: "turn-1"}, {Type: runtimeapi.EventMessageCreated, SessionID: session.ID, TurnID: "turn-1", MessageID: user.ID}, {Type: runtimeapi.EventMessageCreated, SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID}, {Type: runtimeapi.EventMessageCompleted, SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID}} {
		h.service.publishRuntimeEvent(event)
	}
	beforeFinal, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if beforeFinal.Turns[0].FinalMessageID != "" || beforeFinal.Messages[1].Phase == RuntimeConversationPhaseFinal {
		t.Fatalf("running snapshot marked final: %#v", beforeFinal)
	}
	if _, err := h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "completed", UserMessageID: user.ID, LatestAssistantMessageID: assistant.ID, StartedAt: user.CreatedAt, FinishedAt: assistant.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventTurnCompleted, SessionID: session.ID, TurnID: "turn-1"})
	h.service.publishRuntimeEvent(RuntimeEvent{Type: "todo.updated", SessionID: session.ID, TurnID: "turn-1", Payload: map[string]any{"plan_id": "plan-1", "todos": []any{map[string]any{"id": "todo-1", "content": "Verify", "status": "completed"}}}})
	first, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if beforeFinal.Messages[0].ID != first.Messages[0].ID || beforeFinal.Messages[1].ID != first.Messages[1].ID || beforeFinal.AssistantSteps[0].ID != first.AssistantSteps[0].ID {
		t.Fatal("finalization rebuilt process entity identities")
	}
	window, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{Scope: RuntimeConversationScopeWindow, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if window.Messages[1].ID != first.Messages[1].ID || window.Messages[1].Revision != first.Messages[1].Revision || window.AssistantSteps[0].ID != first.AssistantSteps[0].ID {
		t.Fatal("window recomputed canonical identity or revision")
	}
	h.service.events = append(h.service.events, RuntimeEvent{Sequence: 999999, Type: "message.updated", SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID})
	sameCursor, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	stableA, _ := json.Marshal(first)
	stableB, _ := json.Marshal(sameCursor)
	if string(stableA) != string(stableB) {
		t.Fatal("in-memory event changed persisted canonical snapshot")
	}
	restarted := h.restartedService()
	restarted.runtime = h.service.runtime
	restarted.workspace = h.service.workspace
	second, err := restarted.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("restart changed canonical snapshot:\n%s\n%s", a, b)
	}
	if len(first.Turns) != 1 || first.Turns[0].FinalMessageID != assistant.ID || len(first.Messages) != 2 || len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "orphan-tool" || len(first.TodoPlans) != 1 || first.TodoPlans[0].Items[0].ID != "todo-1" {
		t.Fatalf("snapshot = %#v", first)
	}
}
