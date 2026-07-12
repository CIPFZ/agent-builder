package runtime

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCanonicalConversationV2Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/conversation_contract_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot RuntimeCanonicalConversationSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if snapshot.Cursor != "9007199254740993" {
		t.Fatalf("cursor lost precision: %q", snapshot.Cursor)
	}
	if len(snapshot.ToolCalls) != 1 || snapshot.ToolCalls[0].ActivitySequence != "4" || snapshot.ToolCalls[0].ResultIDs[0] != "result-1" {
		t.Fatalf("tool contract mismatch: %#v", snapshot.ToolCalls)
	}
	if len(snapshot.TodoPlans) != 1 || snapshot.TodoPlans[0].OwnerTurnID != "turn-1" || snapshot.TodoPlans[0].Items[0].ID == "" {
		t.Fatalf("todo contract mismatch: %#v", snapshot.TodoPlans)
	}
	if len(snapshot.AgentTasks) != 1 || snapshot.AgentTasks[0].TeamID != "team-1" {
		t.Fatalf("agent task contract mismatch: %#v", snapshot.AgentTasks)
	}
}

func TestCanonicalConversationV2EventValidation(t *testing.T) {
	meta := RuntimeConversationEntityMeta{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", ActivitySequence: "1", Revision: "1"}
	upsert := RuntimeConversationEntityEventV2{SchemaVersion: 2, ID: "event-1", SessionID: "session-1", TurnID: "turn-1", Sequence: "9007199254740993", CreatedAt: 1, EntityType: RuntimeConversationEntityToolCall, EntityID: "tool-1", Operation: RuntimeConversationOperationUpsert, Revision: "1", ToolCall: &RuntimeCanonicalToolCall{RuntimeConversationEntityMeta: meta, Name: "bash", Source: "shell", Status: "running"}}
	if err := upsert.Validate(); err != nil {
		t.Fatalf("valid upsert rejected: %v", err)
	}
	bad := upsert
	bad.Message = &RuntimeCanonicalMessage{}
	if err := bad.Validate(); err == nil {
		t.Fatal("upsert with multiple payloads must fail")
	}
	deleteEvent := upsert
	deleteEvent.Operation = RuntimeConversationOperationDelete
	deleteEvent.ToolCall = nil
	if err := deleteEvent.Validate(); err != nil {
		t.Fatalf("valid delete rejected: %v", err)
	}
	deleteEvent.ToolCall = upsert.ToolCall
	if err := deleteEvent.Validate(); err == nil {
		t.Fatal("delete with payload must fail")
	}
	mismatch := upsert
	mismatch.EntityID = "tool-2"
	if err := mismatch.Validate(); err == nil {
		t.Fatal("event/payload identity mismatch must fail")
	}
}

func TestCanonicalConversationV2RejectsNullCollections(t *testing.T) {
	snapshot := RuntimeCanonicalConversationSnapshot{SchemaVersion: 2, SessionID: "session-1", Cursor: "0", Scope: RuntimeConversationScopeFull}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("nil canonical collections must fail")
	}
}
