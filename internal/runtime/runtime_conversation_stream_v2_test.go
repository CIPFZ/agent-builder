package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestCanonicalConversationOutboxConvergesSnapshotAndPropagatesDerivedRevisions(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(context.Background(), h.service.workspace.ID, "v2 stream")
	if err != nil {
		t.Fatal(err)
	}
	base, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "run"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", UserMessageID: user.ID, StartedAt: user.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventTurnStarted, SessionID: session.ID, TurnID: "turn-1"})
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventMessageCreated, SessionID: session.ID, TurnID: "turn-1", MessageID: user.ID})
	call, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{ID: "tool-1", SessionID: session.ID, TurnID: "turn-1", Name: "shell", Source: scheduler.ToolSourceShell, Command: "echo ok"})
	if err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventToolCallStarted, SessionID: session.ID, TurnID: "turn-1", ToolCallID: call.ID})
	assistant, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.ReasoningContent{Thinking: "inspect"}, message.TextContent{Text: "done"}, message.ToolResult{ToolCallID: call.ID, Name: "shell", Content: "ok", DeliveredToModel: true}, message.Finish{Reason: "stop"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := h.service.turns.Get(h.ctx, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	turn.LatestAssistantMessageID = assistant.ID
	if _, err = h.service.turns.Upsert(h.ctx, turn); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventMessageCompleted, SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID})
	beforeTerminal, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	frozenBefore, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: base.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	frozenJSON, _ := json.Marshal(frozenBefore.Events)
	turn.Status = "completed"
	turn.FinishedAt = assistant.UpdatedAt
	if _, err = h.service.turns.Upsert(h.ctx, turn); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventTurnCompleted, SessionID: session.ID, TurnID: "turn-1"})
	fresh, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	catchup, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: base.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if catchup.SnapshotRequired {
		t.Fatalf("unexpected reset: %#v", catchup)
	}
	for _, event := range catchup.Events {
		if decimalLE(event.Sequence, event.Revision) && event.Sequence != event.Revision {
			t.Fatalf("event %s carries future revision %s at sequence %s", event.ID, event.Revision, event.Sequence)
		}
	}
	prefix := []RuntimeConversationEntityEventV2{}
	for _, event := range catchup.Events {
		seq, _ := strconv.ParseInt(event.Sequence, 10, 64)
		limit, _ := strconv.ParseInt(beforeTerminal.Cursor, 10, 64)
		if seq <= limit {
			prefix = append(prefix, event)
		}
	}
	prefixJSON, _ := json.Marshal(prefix)
	if string(prefixJSON) != string(frozenJSON) {
		t.Fatalf("later state rewrote frozen outbox payload\n%s\n%s", frozenJSON, prefixJSON)
	}
	converged := applyCanonicalEventsForTest(base, catchup.Events)
	converged.Cursor = catchup.Cursor
	assertCanonicalSnapshotsEqual(t, converged, fresh)
	restarted := h.restartedService()
	restarted.runtime = h.service.runtime
	restarted.workspace = h.service.workspace
	replayed, err := restarted.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: base.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(catchup)
	right, _ := json.Marshal(replayed)
	if string(left) != string(right) {
		t.Fatalf("restart changed outbox replay\n%s\n%s", left, right)
	}
	oldMessage := findCanonicalMessage(t, beforeTerminal, assistant.ID)
	newMessage := findCanonicalMessage(t, fresh, assistant.ID)
	if oldMessage.ReasoningContent != "inspect" || oldMessage.ReasoningContentLength != len("inspect") {
		t.Fatalf("canonical message omitted durable reasoning: %#v", oldMessage)
	}
	if oldMessage.Phase == RuntimeConversationPhaseFinal || newMessage.Phase != RuntimeConversationPhaseFinal {
		t.Fatalf("phase transition old=%s new=%s", oldMessage.Phase, newMessage.Phase)
	}
	if decimalLE(newMessage.Revision, oldMessage.Revision) {
		t.Fatalf("terminal did not advance message revision: %s -> %s", oldMessage.Revision, newMessage.Revision)
	}
	tool := fresh.ToolCalls[0]
	result := fresh.ToolResults[0]
	if decimalLE(tool.Revision, result.Revision) && tool.Revision != result.Revision {
		t.Fatalf("tool revision did not absorb result: tool=%s result=%s", tool.Revision, result.Revision)
	}
}

func TestCanonicalConversationStreamForwardsEphemeralTextDeltaWithoutPersistingIt(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "live delta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-live", SessionID: session.ID, Status: "running", StartedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventTurnStarted, SessionID: session.ID, TurnID: "turn-live"})
	base, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	stream, stop := h.service.SubscribeSessionConversationEventsV2(h.ctx, session.ID, base.Cursor)
	defer stop()
	// The subscription first emits its durable catch-up batch.
	select {
	case <-stream:
	case <-time.After(time.Second):
		t.Fatal("canonical stream did not start")
	}

	h.service.publishRuntimeEvent(newOutputTextDeltaEvent(time.Now(), session.ID, "message-live", "turn-live", "reasoning", "thinking", len("thinking")))
	select {
	case batch := <-stream:
		if len(batch.Deltas) != 1 {
			t.Fatalf("expected one live delta, got %#v", batch)
		}
		delta := batch.Deltas[0]
		if delta.MessageID != "message-live" || delta.TurnID != "turn-live" || delta.PartType != "reasoning" || delta.Delta != "thinking" || delta.ContentLength != len("thinking") {
			t.Fatalf("unexpected live delta %#v", delta)
		}
		if batch.Cursor != base.Cursor || batch.AfterCursor != base.Cursor {
			t.Fatalf("ephemeral delta advanced durable cursor: %#v", batch)
		}
	case <-time.After(time.Second):
		t.Fatal("ephemeral delta was not forwarded")
	}

	events, err := h.service.Events(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events.Events {
		if event.Type == runtimeapi.EventOutputTextDelta {
			t.Fatal("ephemeral delta leaked into durable event history")
		}
	}
}

func TestCanonicalConversationBatchesKeepRawSequenceAtomicAndAdvanceEmptyWatermark(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventContextUsageUpdated, SessionID: session.ID, Payload: map[string]any{"tokens": 1}})
	empty, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: snapshot.Cursor, LimitRawEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Cursor == snapshot.Cursor || len(empty.Events) != 0 {
		t.Fatalf("empty watermark=%#v", empty)
	}
	ws, _ := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	assistant, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.ToolResult{ToolCallID: "tool-1", Content: "ok"}, message.Finish{Reason: "stop"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", LatestAssistantMessageID: assistant.ID, StartedAt: assistant.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{ID: "tool-1", SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID, Name: "shell", Source: scheduler.ToolSourceShell}); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventMessageCompleted, SessionID: session.ID, TurnID: "turn-1", MessageID: assistant.ID})
	group, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: empty.Cursor, LimitRawEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Events) < 4 {
		t.Fatalf("raw group was split or dependencies missing: %#v", group.Events)
	}
	for _, event := range group.Events {
		if event.Sequence != group.Cursor {
			t.Fatalf("group split sequences: %#v", group)
		}
	}
}

func TestCanonicalMessageDeleteProducesRecoverableTombstones(t *testing.T) {
	raw := RuntimeEvent{ID: "raw-delete", Sequence: 42, Type: runtimeapi.EventMessageDeleted, SessionID: "session-1", TurnID: "turn-1", MessageID: "message-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{"derived_entity_ids": []any{"assistant-step:message-1", "tool-result:message-1:2"}}}
	events, err := canonicalEntityEventsForRaw(raw, RuntimeCanonicalConversationSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("tombstones=%#v", events)
	}
	for _, event := range events {
		if event.Operation != RuntimeConversationOperationDelete || event.Revision != "42" || event.TombstoneReason != "message_deleted" {
			t.Fatalf("invalid tombstone %#v", event)
		}
		if err := event.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCanonicalSessionDeleteProducesAllEntityTombstones(t *testing.T) {
	snapshot := RuntimeCanonicalConversationSnapshot{Turns: []RuntimeCanonicalTurn{{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-1", SessionID: "session-1", TurnID: "turn-1"}}}, Messages: []RuntimeCanonicalMessage{{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "message-1", SessionID: "session-1", TurnID: "turn-1"}}}}
	raw := RuntimeEvent{ID: "delete-session", Sequence: 50, Type: runtimeapi.EventSessionDeleted, SessionID: "session-1", Payload: map[string]any{"entity_refs": canonicalConversationTombstoneRefs(snapshot)}}
	events, err := canonicalEntityEventsForRaw(raw, RuntimeCanonicalConversationSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("session tombstones=%#v", events)
	}
	for _, event := range events {
		if event.Operation != "delete" || event.TombstoneReason != "session_deleted" || event.Revision != "50" {
			t.Fatalf("invalid tombstone=%#v", event)
		}
	}
}

func TestCanonicalMessageDeleteConvergesAndUpdatesToolDependencies(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "delete convergence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{}); err != nil {
		t.Fatal(err)
	}
	ws, _ := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	if _, err = h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{ID: "tool-1", SessionID: session.ID, TurnID: "turn-1", Name: "shell", Source: scheduler.ToolSourceShell}); err != nil {
		t.Fatal(err)
	}
	msg, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.ToolResult{ToolCallID: "tool-1", Content: "ok"}, message.Finish{Reason: "stop"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", LatestAssistantMessageID: msg.ID, StartedAt: msg.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventMessageCompleted, SessionID: session.ID, TurnID: "turn-1", MessageID: msg.ID})
	before, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.ToolResults) != 1 || len(before.ToolCalls[0].ResultIDs) != 1 {
		t.Fatalf("before=%#v", before)
	}
	if err = ws.Messages.Delete(h.ctx, msg.ID); err != nil {
		t.Fatal(err)
	}
	h.service.recordRuntimeEvent(pubsub.Event[any]{Payload: pubsub.Event[message.Message]{Type: pubsub.DeletedEvent, Payload: msg}})
	after, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	catchup, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: before.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	converged := applyCanonicalEventsForTest(before, catchup.Events)
	converged.Cursor = catchup.Cursor
	assertCanonicalSnapshotsEqual(t, converged, after)
	if len(after.ToolResults) != 0 || len(after.ToolCalls[0].ResultIDs) != 0 {
		t.Fatalf("delete dependencies stale: %#v", after.ToolCalls[0])
	}
	foundToolUpdate := false
	for _, event := range catchup.Events {
		if event.EntityType == RuntimeConversationEntityToolCall && event.Operation == RuntimeConversationOperationUpsert {
			foundToolUpdate = true
		}
	}
	if !foundToolUpdate {
		t.Fatal("delete batch omitted ToolCall dependency update")
	}
}

func TestCanonicalStructuredRawEventsProjectOwnedEntities(t *testing.T) {
	meta := RuntimeConversationEntityMeta{SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "1", Revision: "5", CreatedAt: 1, UpdatedAt: 5}
	snapshot := RuntimeCanonicalConversationSnapshot{
		TodoPlans:  []RuntimeCanonicalTodoPlan{{RuntimeConversationEntityMeta: withCanonicalID(meta, "plan-1"), OwnerTurnID: "turn-1", Status: "abandoned", Items: []RuntimeCanonicalTodoItem{{ID: "todo-1", Status: "pending"}}}},
		AgentTasks: []RuntimeCanonicalAgentTask{{RuntimeConversationEntityMeta: withCanonicalID(meta, "task-1"), Status: "running"}},
		Notices:    []RuntimeCanonicalNotice{{RuntimeConversationEntityMeta: withCanonicalID(meta, "notice:compact:boundary-1"), Kind: "compact", Status: "completed"}},
	}
	turnEvents, err := canonicalEntityEventsForRaw(RuntimeEvent{ID: "turn-done", Sequence: 5, Type: runtimeapi.EventTurnCompleted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z"}, snapshot)
	if err != nil || !slices.ContainsFunc(turnEvents, func(event RuntimeConversationEntityEventV2) bool {
		return event.EntityType == RuntimeConversationEntityTodoPlan && event.EntityID == "plan-1"
	}) {
		t.Fatalf("terminal TodoPlan event = %#v, err=%v", turnEvents, err)
	}
	taskEvents, err := canonicalEntityEventsForRaw(RuntimeEvent{ID: "task-message", Sequence: 5, Type: runtimeapi.EventTaskMessageCreated, SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{"task_id": "task-1"}}, snapshot)
	if err != nil || !slices.ContainsFunc(taskEvents, func(event RuntimeConversationEntityEventV2) bool {
		return event.EntityType == RuntimeConversationEntityAgentTask && event.EntityID == "task-1"
	}) {
		t.Fatalf("task message AgentTask event = %#v, err=%v", taskEvents, err)
	}
	noticeEvents, err := canonicalEntityEventsForRaw(RuntimeEvent{ID: "compact-done", Sequence: 5, Type: runtimeapi.EventCompactCompleted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{"boundary_id": "boundary-1"}}, snapshot)
	if err != nil || len(noticeEvents) != 1 || noticeEvents[0].EntityType != RuntimeConversationEntityNotice || noticeEvents[0].EntityID != "notice:compact:boundary-1" {
		t.Fatalf("compact Notice event = %#v, err=%v", noticeEvents, err)
	}
}

func withCanonicalID(meta RuntimeConversationEntityMeta, id string) RuntimeConversationEntityMeta {
	meta.ID = id
	return meta
}

func TestCanonicalConversationCursorRejectsGapsAndPreservesLargeDecimals(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	if err := store.initializeCheckpoint(h.ctx, "session-1", 9007199254740993, ""); err != nil {
		t.Fatal(err)
	}
	resp, err := h.service.SessionConversationEventsV2(h.ctx, "session-1", RuntimeCanonicalConversationEventsRequestV2{After: "9007199254740994"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.SnapshotRequired || resp.Reason != "invalid_cursor" || resp.Cursor != "9007199254740993" {
		t.Fatalf("large cursor response=%#v", resp)
	}
}

func TestCanonicalSnapshotDoesNotExposeUnprojectedRawState(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "alignment")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err = h.service.eventStore.Append(h.ctx, RuntimeEvent{ID: "unprojected", Sequence: 99, Type: runtimeapi.EventContextUsageUpdated, SessionID: session.ID, CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	stable, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if stable.Cursor != snapshot.Cursor {
		t.Fatalf("unprojected raw state advanced snapshot: %s -> %s", snapshot.Cursor, stable.Cursor)
	}
}

func TestCanonicalSnapshotReconcilesSemanticWriteLostBeforeRawEvent(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "crash recovery")
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	msg, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "persisted before crash"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", UserMessageID: msg.ID, StartedAt: msg.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	if err := store.markFailure(h.ctx, session.ID, "projector_gap"); err != nil {
		t.Fatal(err)
	}
	reset, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: before.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if !reset.SnapshotRequired || reset.Reason != "projector_gap" {
		t.Fatalf("missing recovery signal: %#v", reset)
	}
	restarted := h.restartedService()
	restarted.runtime = h.service.runtime
	restarted.workspace = h.service.workspace
	recovered, err := restarted.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Cursor == before.Cursor || len(recovered.Messages) != 1 || recovered.Messages[0].ID != msg.ID {
		t.Fatalf("semantic crash gap not reconciled: %#v", recovered)
	}
	events, err := restarted.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: before.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if events.SnapshotRequired || events.Cursor != recovered.Cursor || len(events.Events) < 2 {
		t.Fatalf("recovery outbox=%#v", events)
	}
	secondRestart := h.restartedService()
	secondRestart.runtime = h.service.runtime
	secondRestart.workspace = h.service.workspace
	stable, err := secondRestart.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(recovered)
	right, _ := json.Marshal(stable)
	if string(left) != string(right) {
		t.Fatalf("second restart changed reconciliation\n%s\n%s", left, right)
	}
}

func TestCanonicalConversationBatchChainAllowsSparseGlobalSequenceAndDetectsMissingLink(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	if err := store.initializeCheckpoint(h.ctx, "session-1", 5, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.commitBatch(h.ctx, "session-1", 10, []RuntimeConversationEntityEventV2{}); err != nil {
		t.Fatal(err)
	}
	if err := store.commitBatch(h.ctx, "session-1", 20, []RuntimeConversationEntityEventV2{}); err != nil {
		t.Fatal(err)
	}
	first, err := h.service.SessionConversationEventsV2(h.ctx, "session-1", RuntimeCanonicalConversationEventsRequestV2{After: "5", LimitRawEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotRequired || first.Cursor != "10" {
		t.Fatalf("sparse cursor rejected: %#v", first)
	}
	if _, err = h.service.eventStore.db.ExecContext(h.ctx, `DELETE FROM conversation_projector_batches_v2 WHERE session_id=? AND raw_sequence=?`, "session-1", 10); err != nil {
		t.Fatal(err)
	}
	gap, err := h.service.SessionConversationEventsV2(h.ctx, "session-1", RuntimeCanonicalConversationEventsRequestV2{After: "5"})
	if err != nil {
		t.Fatal(err)
	}
	if !gap.SnapshotRequired || gap.Reason != "projector_gap" {
		t.Fatalf("missing link accepted: %#v", gap)
	}
}

func TestCanonicalProjectorDrainsSameSessionRawEventsInSequenceWhenLaterPublisherWins(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "reverse publisher")
	if err != nil {
		t.Fatal(err)
	}
	base, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	msg, err := ws.Messages.Create(h.ctx, session.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "ordered"}}, Metadata: map[string]string{"turn_id": "turn-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: "turn-1", SessionID: session.ID, Status: "running", UserMessageID: msg.ID, StartedAt: msg.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	h.service.mu.Lock()
	earlier := h.service.appendRuntimeEventLocked(RuntimeEvent{Type: runtimeapi.EventMessageCreated, SessionID: session.ID, TurnID: "turn-1", MessageID: msg.ID})
	later := h.service.appendRuntimeEventLocked(RuntimeEvent{Type: runtimeapi.EventContextUsageUpdated, SessionID: session.ID})
	h.service.mu.Unlock()
	if err = h.service.projectCanonicalConversationEventV2(h.ctx, later); err != nil {
		t.Fatal(err)
	}
	if err = h.service.projectCanonicalConversationEventV2(h.ctx, earlier); err != nil {
		t.Fatal(err)
	}
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	for _, sequence := range []int64{earlier.Sequence, later.Sequence} {
		var rawCount, batchCount int
		if err = h.service.eventStore.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM runtime_events WHERE sequence=?`, sequence).Scan(&rawCount); err != nil {
			t.Fatal(err)
		}
		if err = h.service.eventStore.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM conversation_projector_batches_v2 WHERE session_id=? AND raw_sequence=?`, session.ID, sequence).Scan(&batchCount); err != nil {
			t.Fatal(err)
		}
		if rawCount != 1 || batchCount != 1 {
			t.Fatalf("sequence %d raw=%d batch=%d", sequence, rawCount, batchCount)
		}
	}
	checkpoint, _, _, _ := store.checkpoint(h.ctx, session.ID)
	if checkpoint != later.Sequence {
		t.Fatalf("checkpoint=%d later=%d", checkpoint, later.Sequence)
	}
	events, err := h.service.SessionConversationEventsV2(h.ctx, session.ID, RuntimeCanonicalConversationEventsRequestV2{After: base.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events.Events {
		if event.EntityType == RuntimeConversationEntityMessage && event.EntityID == msg.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("earlier semantic change lost: %#v", events)
	}
}

func TestCanonicalConversationOutboxRejectsSameIdentityDifferentPayload(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	if err := store.initializeCheckpoint(h.ctx, "session-1", 1, ""); err != nil {
		t.Fatal(err)
	}
	entity := RuntimeCanonicalTurn{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-1", SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "2", Revision: "2", CreatedAt: 1, UpdatedAt: 2}, Status: "running"}
	event := RuntimeConversationEntityEventV2{SchemaVersion: 2, ID: "event-1", SessionID: "session-1", TurnID: "turn-1", Sequence: "2", CreatedAt: 2, EntityType: RuntimeConversationEntityTurn, EntityID: "turn-1", Operation: RuntimeConversationOperationUpsert, Revision: "2", Turn: &entity}
	if err := store.commitBatch(h.ctx, "session-1", 2, []RuntimeConversationEntityEventV2{event}); err != nil {
		t.Fatal(err)
	}
	if err := store.commitBatch(h.ctx, "session-1", 2, []RuntimeConversationEntityEventV2{event}); err != nil {
		t.Fatalf("identical replay failed: %v", err)
	}
	event.Turn.Status = "completed"
	if err := store.commitBatch(h.ctx, "session-1", 2, []RuntimeConversationEntityEventV2{event}); err == nil {
		t.Fatal("conflicting same-revision payload was accepted")
	}
}

func TestCanonicalEntityEventsAreIdempotentUnderDuplicateAndOutOfOrderDelivery(t *testing.T) {
	makeEvent := func(revision, status string) RuntimeConversationEntityEventV2 {
		turn := RuntimeCanonicalTurn{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-1", SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "2", Revision: revision, CreatedAt: 1, UpdatedAt: 2}, Status: status}
		return RuntimeConversationEntityEventV2{SchemaVersion: 2, ID: "event-" + revision + "-" + status, SessionID: "session-1", TurnID: "turn-1", Sequence: revision, CreatedAt: 2, EntityType: "turn", EntityID: "turn-1", Operation: "upsert", Revision: revision, Turn: &turn}
	}
	older, newer := makeEvent("2", "running"), makeEvent("4", "completed")
	state := map[string]RuntimeConversationEntityEventV2{}
	conflict := false
	for _, event := range []RuntimeConversationEntityEventV2{newer, older, newer} {
		conflict = applyRevisionAwareEventForTest(state, event) || conflict
	}
	if conflict || state["turn:turn-1"].Turn.Status != "completed" {
		t.Fatalf("duplicate/out-of-order regressed state: %#v", state)
	}
	sameRevisionConflict := makeEvent("4", "failed")
	if !applyRevisionAwareEventForTest(state, sameRevisionConflict) {
		t.Fatal("same revision different payload did not request reset")
	}
}

func applyRevisionAwareEventForTest(state map[string]RuntimeConversationEntityEventV2, event RuntimeConversationEntityEventV2) bool {
	key := event.EntityType + ":" + event.EntityID
	current, exists := state[key]
	if !exists {
		state[key] = event
		return false
	}
	currentRevision, _ := strconv.ParseUint(current.Revision, 10, 64)
	nextRevision, _ := strconv.ParseUint(event.Revision, 10, 64)
	if nextRevision < currentRevision {
		return false
	}
	currentJSON, _ := canonicalEventEntityJSON(current)
	nextJSON, _ := canonicalEventEntityJSON(event)
	if nextRevision == currentRevision {
		if string(currentJSON) != string(nextJSON) {
			return true
		}
		return false
	}
	state[key] = event
	return false
}

func TestCanonicalRawOutboxStateAndCheckpointRollbackTogether(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	store := newRuntimeConversationEventStoreV2(h.service.eventStore.db)
	baseline := RuntimeCanonicalConversationSnapshot{SchemaVersion: 2, SessionID: "session-1", Cursor: "1", Scope: "full", Turns: []RuntimeCanonicalTurn{}, Messages: []RuntimeCanonicalMessage{}, AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}
	if err := store.seedSnapshot(h.ctx, baseline); err != nil {
		t.Fatal(err)
	}
	turn := RuntimeCanonicalTurn{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-1", SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "2", Revision: "2", CreatedAt: 2, UpdatedAt: 2}, Status: "running"}
	event := RuntimeConversationEntityEventV2{SchemaVersion: 2, ID: "duplicate-id", SessionID: "session-1", TurnID: "turn-1", Sequence: "2", CreatedAt: 2, EntityType: "turn", EntityID: "turn-1", Operation: "upsert", Revision: "2", Turn: &turn}
	raw := RuntimeEvent{ID: "raw-2", Sequence: 2, Type: runtimeapi.EventTurnStarted, SessionID: "session-1", TurnID: "turn-1", CreatedAt: "2026-01-01T00:00:00Z", Payload: map[string]any{}}
	second := event
	second.EntityID = "turn-2"
	second.Turn = &RuntimeCanonicalTurn{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: "turn-2", SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "2", Revision: "2", CreatedAt: 2, UpdatedAt: 2}, Status: "running"}
	if err := store.commitProjectedRaw(h.ctx, raw, []RuntimeConversationEntityEventV2{event, second}); err == nil {
		t.Fatal("transaction unexpectedly succeeded")
	}
	var count int
	if err := h.service.eventStore.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM runtime_events WHERE sequence=2`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("raw event survived failed outbox transaction")
	}
	checkpoint, _, _, err := store.checkpoint(h.ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint != 1 {
		t.Fatalf("checkpoint advanced after rollback: %d", checkpoint)
	}
}

func TestSubscribeCanonicalConversationEventsV2DeliversAtomicWatermark(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "subscription")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.service.SessionConversationSnapshotV2(h.ctx, session.ID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	batches, stop := h.service.SubscribeSessionConversationEventsV2(ctx, session.ID, snapshot.Cursor)
	defer stop()
	select {
	case initial := <-batches:
		if initial.Cursor != snapshot.Cursor {
			t.Fatalf("initial=%#v", initial)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initial catch-up timed out")
	}
	h.service.publishRuntimeEvent(RuntimeEvent{Type: runtimeapi.EventContextUsageUpdated, SessionID: session.ID})
	select {
	case batch := <-batches:
		if batch.Cursor == snapshot.Cursor || len(batch.Events) != 0 || batch.SnapshotRequired {
			t.Fatalf("watermark batch=%#v", batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watermark timed out")
	}
}

func TestSubscribeCanonicalConversationEventsV2OverflowIsObservable(t *testing.T) {
	out := make(chan RuntimeCanonicalConversationEventBatchV2, 1)
	out <- RuntimeCanonicalConversationEventBatchV2{Cursor: "old"}
	if keep := sendCanonicalConversationBatchV2(context.Background(), out, RuntimeCanonicalConversationEventBatchV2{Cursor: "new"}, "session-1", "10"); keep {
		t.Fatal("overflow kept stream active")
	}
	reset := <-out
	if !reset.SnapshotRequired || reset.Reason != "overflow" || reset.Cursor != "10" {
		t.Fatalf("overflow reset=%#v", reset)
	}
}

func TestSubscribeCanonicalConversationEventsV2DropsOnlyLiveDeltaOnBackpressure(t *testing.T) {
	out := make(chan RuntimeCanonicalConversationEventBatchV2, 1)
	durable := RuntimeCanonicalConversationEventBatchV2{Cursor: "11", Events: []RuntimeConversationEntityEventV2{{EntityID: "turn-1"}}}
	out <- durable
	live := RuntimeCanonicalConversationEventBatchV2{Cursor: "11", Deltas: []RuntimeConversationTextDeltaV2{{MessageID: "message-1", PartType: "text", Delta: "x", ContentLength: 1}}}
	if keep := sendCanonicalConversationBatchV2(context.Background(), out, live, "session-1", "11"); !keep {
		t.Fatal("advisory delta backpressure stopped the durable stream")
	}
	kept := <-out
	if len(kept.Events) != 1 || kept.Events[0].EntityID != "turn-1" || kept.SnapshotRequired {
		t.Fatalf("live delta displaced durable batch: %#v", kept)
	}
}

func applyCanonicalEventsForTest(s RuntimeCanonicalConversationSnapshot, events []RuntimeConversationEntityEventV2) RuntimeCanonicalConversationSnapshot {
	for _, e := range events {
		switch e.EntityType {
		case RuntimeConversationEntityTurn:
			s.Turns = upsertCanonicalForTest(s.Turns, e.Turn, e)
		case RuntimeConversationEntityMessage:
			s.Messages = upsertCanonicalForTest(s.Messages, e.Message, e)
		case RuntimeConversationEntityAssistantStep:
			s.AssistantSteps = upsertCanonicalForTest(s.AssistantSteps, e.AssistantStep, e)
		case RuntimeConversationEntityToolCall:
			s.ToolCalls = upsertCanonicalForTest(s.ToolCalls, e.ToolCall, e)
		case RuntimeConversationEntityToolResult:
			s.ToolResults = upsertCanonicalForTest(s.ToolResults, e.ToolResult, e)
		case RuntimeConversationEntityPermission:
			s.Permissions = upsertCanonicalForTest(s.Permissions, e.Permission, e)
		case RuntimeConversationEntityTodoPlan:
			s.TodoPlans = upsertCanonicalForTest(s.TodoPlans, e.TodoPlan, e)
		case RuntimeConversationEntityAgentTask:
			s.AgentTasks = upsertCanonicalForTest(s.AgentTasks, e.AgentTask, e)
		}
	}
	canonicalSortSnapshot(&s)
	return s
}
func upsertCanonicalForTest[T any](items []T, value *T, event RuntimeConversationEntityEventV2) []T {
	out := []T{}
	for _, item := range items {
		raw, _ := json.Marshal(item)
		var meta RuntimeConversationEntityMeta
		_ = json.Unmarshal(raw, &meta)
		if meta.ID != event.EntityID {
			out = append(out, item)
		}
	}
	if event.Operation == RuntimeConversationOperationUpsert && value != nil {
		out = append(out, *value)
	}
	return out
}
func assertCanonicalSnapshotsEqual(t *testing.T, a, b RuntimeCanonicalConversationSnapshot) {
	t.Helper()
	a.Cursor = b.Cursor
	a.Scope = b.Scope
	a.Window = b.Window
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("snapshots differ\n%s\n%s", left, right)
	}
}
func findCanonicalMessage(t *testing.T, s RuntimeCanonicalConversationSnapshot, id string) RuntimeCanonicalMessage {
	t.Helper()
	for _, v := range s.Messages {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("message %s missing", id)
	return RuntimeCanonicalMessage{}
}
func decimalLE(a, b string) bool {
	av, _ := strconv.ParseUint(a, 10, 64)
	bv, _ := strconv.ParseUint(b, 10, 64)
	return av <= bv
}
