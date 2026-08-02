package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/contextmgr"
)

func TestContextCompactionStatusRestoresSQLiteSnapshot(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)
	if err := service.ensureContextManager(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.contextStore.UpsertBoundary(ctx, contextmgr.Boundary{
		ID: "completed", SessionID: sessionID, TurnID: "turn-1", Kind: "session_memory", Trigger: "auto",
		Status: contextmgr.ProjectionStatusCompleted, MemoryRevision: 2,
		MessageRefs: []string{"m1", "m2"}, PreservedMessageRefs: []string{"m3"},
		BudgetBefore: &contextmgr.BudgetReport{TotalEstimatedTokens: 9000},
		BudgetAfter:  &contextmgr.BudgetReport{TotalEstimatedTokens: 3000}, CreatedAt: 10, CompletedAt: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.contextStore.UpsertSessionMemoryRevision(ctx, contextmgr.SessionMemoryRevision{
		ID: "memory-2", SessionID: sessionID, Revision: 2, Status: contextmgr.SessionMemoryStatusCompleted,
		LastSummarizedMessageID: "m2", SourceMessageCount: 2, SourceTokenEstimate: 6000, CreatedAt: 11, CompletedAt: 19,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < compactCircuitBreakerThreshold; i++ {
		service.incrementCompactFailure(sessionID)
	}

	status, err := service.ContextCompactionStatus(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestCompleted == nil || status.LatestCompleted.ID != "completed" || status.LatestCompleted.MemoryRevision != 2 {
		t.Fatalf("latest completed = %#v", status.LatestCompleted)
	}
	if status.LatestSessionMemory == nil || status.LatestSessionMemory.Revision != 2 {
		t.Fatalf("latest memory = %#v", status.LatestSessionMemory)
	}
	if !status.CircuitOpen || status.ConsecutiveFailures != compactCircuitBreakerThreshold {
		t.Fatalf("circuit = open:%v failures:%d", status.CircuitOpen, status.ConsecutiveFailures)
	}
}
