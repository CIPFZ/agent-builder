package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkCanonicalConversationWindow10K(b *testing.B) {
	base := benchmarkCanonicalSnapshot(10_000)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		b.StopTimer()
		snapshot := cloneBenchmarkSnapshot(base)
		b.StartTimer()
		applyCanonicalWindow(&snapshot, RuntimeCanonicalConversationSnapshotRequest{Scope: RuntimeConversationScopeWindow, Limit: 30})
		if len(snapshot.Turns) != 30 {
			b.Fatal(len(snapshot.Turns))
		}
	}
}

func TestCanonicalConversationWindowTransportStaysBounded(t *testing.T) {
	snapshot := benchmarkCanonicalSnapshot(10_000)
	applyCanonicalWindow(&snapshot, RuntimeCanonicalConversationSnapshotRequest{Scope: RuntimeConversationScopeWindow, Limit: 30})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 30 || len(snapshot.Messages) != 60 {
		t.Fatalf("window entities: turns=%d messages=%d", len(snapshot.Turns), len(snapshot.Messages))
	}
	if len(encoded) > 256*1024 {
		t.Fatalf("30-Turn canonical window grew to %d bytes", len(encoded))
	}
}

func benchmarkCanonicalSnapshot(size int) RuntimeCanonicalConversationSnapshot {
	snapshot := RuntimeCanonicalConversationSnapshot{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: "benchmark", Cursor: fmt.Sprint(size * 3), Scope: RuntimeConversationScopeFull, Turns: []RuntimeCanonicalTurn{}, Messages: []RuntimeCanonicalMessage{}, AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}
	for index := 0; index < size; index++ {
		turnID, userID, finalID := fmt.Sprintf("turn-%d", index), fmt.Sprintf("user-%d", index), fmt.Sprintf("final-%d", index)
		snapshot.Turns = append(snapshot.Turns, RuntimeCanonicalTurn{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: turnID, SessionID: "benchmark", TurnID: turnID, ActivitySequence: fmt.Sprint(index*3 + 1), Revision: fmt.Sprint(index*3 + 1)}, Status: "completed", UserMessageID: userID, FinalMessageID: finalID})
		for offset, messageID := range []string{userID, finalID} {
			snapshot.Messages = append(snapshot.Messages, RuntimeCanonicalMessage{RuntimeConversationEntityMeta: RuntimeConversationEntityMeta{ID: messageID, SessionID: "benchmark", TurnID: turnID, ActivitySequence: fmt.Sprint(index*3 + offset + 2), Revision: fmt.Sprint(index*3 + offset + 2)}, Role: []string{"user", "assistant"}[offset], Status: "completed", Content: strings.Repeat("x", 1024), ContentLength: 1024})
		}
	}
	return snapshot
}

func cloneBenchmarkSnapshot(source RuntimeCanonicalConversationSnapshot) RuntimeCanonicalConversationSnapshot {
	clone := source
	clone.Turns = append([]RuntimeCanonicalTurn(nil), source.Turns...)
	clone.Messages = append([]RuntimeCanonicalMessage(nil), source.Messages...)
	return clone
}
