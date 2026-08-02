package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestSessionMemoryCompactPersistsTailBoundaryAndRestoresProjection(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)
	if err := service.ensureContextManager(ctx); err != nil {
		t.Fatal(err)
	}
	ws, err := service.runtime.GetWorkspace(service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	var anchor message.Message
	for i := 0; i < 30; i++ {
		role := message.User
		parts := []message.ContentPart{message.TextContent{Text: fmt.Sprintf("message-%02d %s", i, strings.Repeat("x", 2000))}}
		if i%2 == 1 {
			role = message.Assistant
			parts = append(parts, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()})
		}
		created, createErr := ws.Messages.Create(ctx, sessionID, message.CreateMessageParams{Role: role, Parts: parts})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if i == 21 {
			anchor = created
		}
	}
	if _, err := service.contextStore.UpsertSessionMemoryRevision(ctx, contextmgr.SessionMemoryRevision{
		ID: "memory-completed", SessionID: sessionID, Revision: 1, Status: contextmgr.SessionMemoryStatusCompleted,
		Content: validSessionMemoryFixture(), LastSummarizedMessageID: anchor.ID, CreatedAt: 1, CompletedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	result, summary, err := service.runSessionMemoryCompact(ctx, RuntimeContextActionRequest{SessionID: sessionID, TurnID: "turn-memory-compact", ProjectionID: "projection-memory", Reason: "auto"}, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if result.Boundary.Kind != "session_memory" || result.Boundary.SummaryMessageID != summary.ID || len(result.Boundary.PreservedMessageRefs) == 0 || result.Boundary.BoundaryCutoffMessageID == "" {
		t.Fatalf("boundary=%#v", result.Boundary)
	}
	canonical, err := ws.Messages.List(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := service.projectSessionHistory(ctx, sessionID, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) == 0 || projected[0].ID != summary.ID || !projected[0].IsSummaryMessage {
		t.Fatalf("projected=%#v", projected)
	}
	if len(canonical) != 31 {
		t.Fatalf("canonical messages=%d, want 31", len(canonical))
	}
}
