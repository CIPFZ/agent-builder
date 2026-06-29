package runtime

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
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
		ID:             "compact-1",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		Kind:           "micro",
		Trigger:        "test",
		Status:         "completed",
		BudgetBefore:   &budget,
		BudgetAfter:    &budget,
		SummaryRef:     "runtime://turns/turn-1/compact/micro",
		ToolCallRefs:   []RuntimeCompactToolCallRef{{ToolCallID: "tool-1", Ref: "runtime://tool-calls/tool-1/output", EstimatedTokens: 300}},
		ReinjectedRefs: []RuntimeReinjectedRef{{ID: "agents:/work/AGENTS.md", Kind: "agents", Status: compactStatusCompleted, Ref: "runtime://context-sources/AGENTS.md"}},
		CreatedAt:      1000,
		CompletedAt:    1100,
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
	if len(turnBoundaries[0].ReinjectedRefs) != 1 || turnBoundaries[0].ReinjectedRefs[0].Kind != "agents" {
		t.Fatalf("reinjected refs = %#v", turnBoundaries[0].ReinjectedRefs)
	}
	sessionBoundaries, err := store.ListBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionBoundaries) != 1 || sessionBoundaries[0].ID != "compact-1" {
		t.Fatalf("session boundaries = %#v", sessionBoundaries)
	}
}

func TestRuntimeReplayExportIncludesCompactReinjectionAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	boundary := RuntimeCompactBoundary{
		ID:         "compact-full",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		Kind:       compactKindFull,
		Trigger:    "test",
		Status:     compactStatusCompleted,
		SummaryRef: "runtime://turns/turn-1/compact/full/summary",
		ReinjectedRefs: []RuntimeReinjectedRef{{
			ID:     "read_file:/work/secret.txt",
			Kind:   "read_file",
			Path:   "/work/secret.txt",
			Status: compactStatusCompleted,
			Reason: "token=sk-secret",
		}},
		CreatedAt:   time.Now().UnixMilli(),
		CompletedAt: time.Now().UnixMilli(),
	}
	replay := buildRuntimeReplaySummary(RuntimeAuditTurnSummary{Compact: []RuntimeCompactBoundary{boundary}}, []RuntimeEvent{
		{
			Type:      runtimeapi.EventContextReinjected,
			SessionID: "session-1",
			TurnID:    "turn-1",
			Payload: map[string]any{
				"compact_id": "compact-full",
				"source_id":  "agents:/work/AGENTS.md",
				"kind":       "agents",
				"path":       "/work/AGENTS.md",
				"status":     compactStatusCompleted,
				"reason":     "post_compact_context_reinjected",
			},
		},
	}, nil)
	if len(replay.CompactBoundaries) != 1 || len(replay.CompactBoundaries[0].ReinjectedRefs) != 2 {
		t.Fatalf("replay compact = %#v", replay.CompactBoundaries)
	}
	payload := redactRuntimeValue(replay)
	data := string(mustJSON(t, payload))
	if strings.Contains(data, "sk-secret") || strings.Contains(data, "token=") {
		t.Fatalf("replay leaked secret: %s", data)
	}
}

func TestRuntimeRecoveryStatusIncludesCompactBoundaries(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	service := newRuntimeService()
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
	service.recovery.interruptedTurns = []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: turnStatusInterrupted}}
	if _, err := service.compactBoundaries.Upsert(context.Background(), RuntimeCompactBoundary{
		ID:        "compact-full",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Kind:      compactKindFull,
		Trigger:   "test",
		Status:    compactStatusCompleted,
		ReinjectedRefs: []RuntimeReinjectedRef{{
			ID:     "agents:/work/AGENTS.md",
			Kind:   "agents",
			Status: compactStatusCompleted,
		}},
		CreatedAt:   time.Now().UnixMilli(),
		CompletedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	compact := service.recoveryCompactBoundaries(context.Background(), nil, service.recovery.interruptedTurns)
	if len(compact) != 1 || len(compact[0].ReinjectedRefs) != 1 {
		t.Fatalf("recovery compact = %#v", compact)
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

func TestRuntimeCompactBoundaryPayloadParsing(t *testing.T) {
	t.Parallel()

	boundary := runtimeCompactBoundaryFromPayload(map[string]any{
		"id":     "compact-1",
		"kind":   compactKindFull,
		"status": compactStatusCompleted,
		"toolCallRefs": []any{
			map[string]any{"toolCallId": "tool-1", "ref": "runtime://tool-calls/tool-1/output"},
		},
		"reinjectedRefs": []any{
			map[string]any{"id": "agents:/work/AGENTS.md", "kind": "agents", "status": compactStatusCompleted},
		},
	})
	if boundary.ID != "compact-1" || !slices.ContainsFunc(boundary.ToolCallRefs, func(ref RuntimeCompactToolCallRef) bool {
		return ref.ToolCallID == "tool-1"
	}) {
		t.Fatalf("boundary payload = %#v", boundary)
	}
}
