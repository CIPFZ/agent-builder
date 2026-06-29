package contextmgr

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestSQLStoreProjectionCRUD(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	store := NewSQLStore(conn)

	projection := Projection{
		ID:                    "projection-1",
		SessionID:             "session-1",
		TurnID:                "turn-1",
		Step:                  1,
		Provider:              "openai",
		Model:                 "gpt-test",
		Source:                ProjectionSourceMainTurn,
		Status:                ProjectionStatusCompleted,
		CanonicalMessageCount: 2,
		ProjectedMessageCount: 2,
		BudgetBefore:          &BudgetReport{TotalEstimatedTokens: 20, ContextWindow: 100, Model: "gpt-test"},
		BudgetAfter:           &BudgetReport{TotalEstimatedTokens: 20, ContextWindow: 100, Model: "gpt-test"},
		CreatedAt:             1000,
		CompletedAt:           1001,
	}
	stored, err := store.UpsertProjection(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != projection.ID || stored.BudgetAfter.TotalEstimatedTokens != 20 {
		t.Fatalf("stored projection = %#v", stored)
	}

	_, err = store.UpsertProjectionMessage(context.Background(), ProjectionMessage{
		ID:                 "projection-1-msg-1",
		ProjectionID:       projection.ID,
		SessionID:          projection.SessionID,
		TurnID:             projection.TurnID,
		Sequence:           1,
		Role:               "user",
		CanonicalMessageID: "message-1",
		Status:             "selected",
		CreatedAt:          1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	messages, err := store.ListProjectionMessages(context.Background(), projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].CanonicalMessageID != "message-1" {
		t.Fatalf("projection messages = %#v", messages)
	}

	byTurn, err := store.ListProjectionsByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byTurn) != 1 || byTurn[0].ID != projection.ID {
		t.Fatalf("turn projections = %#v", byTurn)
	}
}

func TestProjectionDTOJSON(t *testing.T) {
	t.Parallel()

	projection := Projection{
		ID:                    "projection-1",
		SessionID:             "session-1",
		TurnID:                "turn-1",
		Step:                  1,
		Source:                ProjectionSourceMainTurn,
		Status:                ProjectionStatusCompleted,
		CanonicalMessageCount: 3,
		ProjectedMessageCount: 2,
		BudgetAfter:           &BudgetReport{TotalEstimatedTokens: 40},
		CreatedAt:             1234,
	}
	data, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Projection
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != projection.ID || decoded.BudgetAfter.TotalEstimatedTokens != 40 {
		t.Fatalf("decoded projection = %#v", decoded)
	}
}

func TestManagerBuildModelInputCreatesNoopProjection(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	store := NewSQLStore(conn)
	manager := NewManager(ManagerOptions{
		Store: store,
		Now:   func() time.Time { return time.UnixMilli(5000).UTC() },
	})

	result, err := manager.BuildModelInput(context.Background(), BuildInputRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Step:      2,
		Provider:  "openai",
		Model:     "gpt-test",
		Messages: []message.Message{{
			ID:   "message-1",
			Role: message.User,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Projection.ID != "ctxproj_turn-1_step_002" || result.Projection.ProjectedMessageCount != 1 {
		t.Fatalf("projection = %#v", result.Projection)
	}
	storedMessages, err := store.ListProjectionMessages(context.Background(), result.Projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedMessages) != 1 || storedMessages[0].CanonicalMessageID != "message-1" {
		t.Fatalf("stored projection messages = %#v", storedMessages)
	}
}

func TestManagerManualActionsRecordGovernanceState(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	store := NewSQLStore(conn)
	manager := NewManager(ManagerOptions{
		Store: store,
		Now:   func() time.Time { return time.UnixMilli(6000).UTC() },
	})

	compact, err := manager.ManualCompact(context.Background(), ManualCompactRequest{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: "projection-1",
		Reason:       "user_requested",
	})
	if err != nil {
		t.Fatal(err)
	}
	if compact.Boundary.Kind != "manual" || compact.Boundary.Trigger != "user_requested" {
		t.Fatalf("manual compact = %#v", compact.Boundary)
	}

	snip, err := manager.ManualSnip(context.Background(), ManualSnipRequest{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: "projection-1",
		Reason:       "trim_scrollback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snip.SnipBoundary.Reason != "trim_scrollback" {
		t.Fatalf("manual snip = %#v", snip.SnipBoundary)
	}

	boundaries, err := store.ListBoundariesByProjection(context.Background(), "projection-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(boundaries) != 1 || boundaries[0].ID != compact.Boundary.ID {
		t.Fatalf("manual compact projection list = %#v", boundaries)
	}
	snips, err := store.ListSnipBoundariesByProjection(context.Background(), "projection-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snips) != 1 || snips[0].ID != snip.SnipBoundary.ID {
		t.Fatalf("manual snip projection list = %#v", snips)
	}
}
