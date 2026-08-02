package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func validSessionMemoryFixture() string {
	var b strings.Builder
	for _, section := range sessionMemorySections {
		b.WriteString("## ")
		b.WriteString(section)
		b.WriteString("\n\nKept facts.\n\n")
	}
	return b.String()
}

func TestExtractSessionMemoryPersistsCompletedRevisionAndRoutesSmallModel(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)
	if err := service.ensureContextManager(ctx); err != nil {
		t.Fatal(err)
	}
	small := "small"
	if _, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{SummaryModel: small}); err != nil {
		t.Fatal(err)
	}
	ws, err := service.runtime.GetWorkspace(service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, sessionID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("context ", 6000)}}}); err != nil {
		t.Fatal(err)
	}
	assistant, err := ws.Messages.Create(ctx, sessionID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "done"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}}})
	if err != nil {
		t.Fatal(err)
	}
	calledSmall := false
	service.sessionMemoryGenerator = func(_ context.Context, req agent.CompactSummaryRequest) (agent.CompactSummaryResult, error) {
		calledSmall = req.UseSmallModel
		return agent.CompactSummaryResult{Summary: validSessionMemoryFixture()}, nil
	}
	if err := service.extractSessionMemory(ctx, sessionID, "turn-memory", assistant.ID); err != nil {
		t.Fatal(err)
	}
	latest, err := service.contextStore.LatestCompletedSessionMemory(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 1 || latest.LastSummarizedMessageID != assistant.ID || latest.ContentHash == "" || !calledSmall {
		t.Fatalf("latest=%#v calledSmall=%v", latest, calledSmall)
	}
}

func TestExtractSessionMemoryFailureDoesNotReplaceCompletedAnchor(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)
	if err := service.ensureContextManager(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.contextStore.UpsertSessionMemoryRevision(ctx, contextmgr.SessionMemoryRevision{ID: "stable", SessionID: sessionID, Revision: 1, Status: contextmgr.SessionMemoryStatusCompleted, Content: validSessionMemoryFixture(), LastSummarizedMessageID: "anchor", CreatedAt: 1, CompletedAt: 2}); err != nil {
		t.Fatal(err)
	}
	// Store-level invariant: a later failed revision never becomes latest completed.
	if _, err := service.contextStore.UpsertSessionMemoryRevision(ctx, contextmgr.SessionMemoryRevision{ID: "failed", SessionID: sessionID, Revision: 2, Status: contextmgr.SessionMemoryStatusFailed, Error: "invalid", CreatedAt: 3, CompletedAt: 4}); err != nil {
		t.Fatal(err)
	}
	latest, err := service.contextStore.LatestCompletedSessionMemory(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "stable" || latest.LastSummarizedMessageID != "anchor" {
		t.Fatalf("latest=%#v", latest)
	}
}

func TestValidateSessionMemoryRejectsMissingSectionsAndOversize(t *testing.T) {
	if err := validateSessionMemory("## Current objective\nOnly one"); err == nil {
		t.Fatal("expected missing-section error")
	}
	oversize := validSessionMemoryFixture() + strings.Repeat("token ", sessionMemoryMaxTokens*5)
	if err := validateSessionMemory(oversize); err == nil {
		t.Fatal("expected oversize error")
	}
}
