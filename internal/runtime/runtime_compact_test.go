package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeCompactBoundaryStorePersistsBoundary(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	store := newRuntimeCompactBoundaryStore(conn)
	budget := RuntimeBudgetReport{
		SessionID:            "session-1",
		TurnID:               "turn-1",
		Messages:             RuntimeBudgetBucket{Count: 2, EstimatedTokens: 10},
		ToolOutputs:          RuntimeBudgetBucket{Count: 1, EstimatedTokens: 300},
		TotalEstimatedTokens: 310,
		UpdatedAt:            time.Now().UnixMilli(),
	}
	boundary, err := store.Upsert(context.Background(), RuntimeCompactBoundary{
		ID:           "compact-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Kind:         "micro",
		Trigger:      "test",
		Status:       "completed",
		BudgetBefore: &budget,
		BudgetAfter:  &budget,
		SummaryRef:   "runtime://turns/turn-1/compact/micro",
		ToolCallRefs: []RuntimeCompactToolCallRef{{ToolCallID: "tool-1", Ref: "runtime://tool-calls/tool-1/output", EstimatedTokens: 300}},
		CreatedAt:    1000,
		CompletedAt:  1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if boundary.BudgetBefore == nil || boundary.BudgetBefore.TotalEstimatedTokens != 310 {
		t.Fatalf("budget missing: %#v", boundary)
	}

	turnBoundaries, err := store.ListByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(turnBoundaries) != 1 || len(turnBoundaries[0].ToolCallRefs) != 1 {
		t.Fatalf("turn boundaries = %#v", turnBoundaries)
	}
	sessionBoundaries, err := store.ListBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionBoundaries) != 1 || sessionBoundaries[0].ID != "compact-1" {
		t.Fatalf("session boundaries = %#v", sessionBoundaries)
	}
}

func TestRuntimeBudgetAccountingIsDeterministic(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.rememberToolDisclosureBudget("turn-1", RuntimeBudgetBucket{Count: 2, EstimatedTokens: 12}, RuntimeBudgetBucket{Count: 3, EstimatedTokens: 30})
	report := service.computeRuntimeBudget(context.Background(), "session-1", "turn-1", "test-model", 16, &RuntimeTurnContextSummary{
		AvailableCount: 2,
		TokenEstimate:  7,
	})
	if report.InputBudget.Count != 1 || report.InputBudget.EstimatedTokens != 4 {
		t.Fatalf("input budget = %#v", report.InputBudget)
	}
	if report.ContextSources.Count != 2 || report.ContextSources.EstimatedTokens != 7 {
		t.Fatalf("context budget = %#v", report.ContextSources)
	}
	if report.SelectedToolSchemas.Count != 2 || report.SelectedToolSchemas.EstimatedTokens != 12 {
		t.Fatalf("selected tool schema budget = %#v", report.SelectedToolSchemas)
	}
	if report.OmittedToolSchemas.Count != 3 || report.OmittedToolSchemas.EstimatedTokens != 30 {
		t.Fatalf("omitted tool schema budget = %#v", report.OmittedToolSchemas)
	}
	if report.TotalEstimatedTokens < 23 {
		t.Fatalf("total budget = %#v", report)
	}
}

func TestRuntimeMicroCompactPreservesToolCallRefs(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
	for i := 0; i < 4; i++ {
		id := "tool-" + string(rune('1'+i))
		if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
			ID:           id,
			SessionID:    "session-1",
			TurnID:       "turn-1",
			Name:         "bash",
			Source:       scheduler.ToolSourceShell,
			InputSummary: "cmd",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
			ToolCallID:    id,
			Status:        scheduler.ToolCallCompleted,
			OutputSummary: strings.Repeat("output ", 800),
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := RuntimeBudgetReport{
		SessionID:            "session-1",
		TurnID:               "turn-1",
		ToolOutputs:          RuntimeBudgetBucket{Count: 4, EstimatedTokens: 4000},
		TotalEstimatedTokens: 4000,
	}
	after, boundary := service.maybeMicroCompactToolOutputs(context.Background(), "session-1", "turn-1", before)
	if boundary == nil {
		t.Fatal("expected micro compact boundary")
	}
	if len(boundary.ToolCallRefs) != 2 {
		t.Fatalf("tool refs = %#v", boundary.ToolCallRefs)
	}
	if after.ToolOutputs.EstimatedTokens >= before.ToolOutputs.EstimatedTokens {
		t.Fatalf("budget did not shrink: before=%#v after=%#v", before, after)
	}
	first, err := service.toolCalls.GetCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Compacted || first.CompactRef == "" || first.CompactBoundaryID != boundary.ID {
		t.Fatalf("compacted tool = %#v boundary=%#v", first, boundary)
	}
	recent, err := service.toolCalls.GetCall(context.Background(), "tool-4")
	if err != nil {
		t.Fatal(err)
	}
	if recent.Compacted {
		t.Fatalf("recent tool should be preserved: %#v", recent)
	}
}

func TestCompactEventsAndAuditRedactSecrets(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	boundary := RuntimeCompactBoundary{
		ID:        "compact-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Kind:      "micro",
		Trigger:   "test",
		Status:    "completed",
		ToolCallRefs: []RuntimeCompactToolCallRef{{
			ToolCallID:  "tool-1",
			Ref:         "runtime://tool-calls/tool-1/output?token=secret",
			Replacement: "Authorization: Bearer secret",
		}},
		CreatedAt:   1000,
		CompletedAt: 1100,
	}
	payload, err := auditPayload(auditEntry{Event: "compact_micro_completed", RequestID: "turn-1", CompactBoundary: &boundary})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "Bearer") {
		t.Fatalf("compact audit leaked secret: %s", data)
	}

	service.storeRuntimeEvent(runtimeapi.Event{
		Type:      runtimeapi.EventCompactMicroCompleted,
		CreatedAt: "2026-05-24T00:00:00Z",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Payload: map[string]any{
			"replacement": "Authorization: Bearer secret",
		},
	})
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 1 || events.Events[0].Payload["replacement"] != "[REDACTED]" {
		t.Fatalf("event redaction failed: %#v", events.Events)
	}
}
