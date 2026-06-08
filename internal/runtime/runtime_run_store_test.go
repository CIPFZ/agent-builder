package runtime

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimeRunStoreEnsuresGeneratedRunAndKeepsIdempotentSessionLink(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	})
	store := newRuntimeRunStore(conn)

	first, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID == runtimeRunProjectionID("session-1") || first.Source != runtimeRunSourceUserPrompt {
		t.Fatalf("generated run = %#v", first)
	}
	second, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "other objective", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || len(second.SessionIDs) != 1 || second.SessionIDs[0] != "session-1" {
		t.Fatalf("idempotent run = %#v want id %q", second, first.ID)
	}
}

func TestRuntimeRunStoreBackfillsProjectionWithoutDuplicatingEvidence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	})
	store := newRuntimeRunStore(conn)
	projection := RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-parent"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-parent",
		SessionIDs:       []string{"session-parent", "session-child"},
		Objective:        "build durable runs",
		Status:           runtimeRunStatusInterrupted,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-1:interrupted",
			TurnID:       "turn-1",
			Status:       turnStatusInterrupted,
			Summary:      "runtime restarted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    1200,
		}},
		CreatedAt:  1000,
		UpdatedAt:  1300,
		FinishedAt: 1300,
	}
	first, err := store.UpsertFromProjection(context.Background(), projection, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpsertFromProjection(context.Background(), projection, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != runtimeRunProjectionID("session-parent") || second.ID != first.ID {
		t.Fatalf("backfill ids = %#v %#v", first, second)
	}
	if len(second.SessionIDs) != 2 {
		t.Fatalf("session links = %#v", second.SessionIDs)
	}
	if len(second.Checkpoints) != 1 || second.Checkpoints[0].ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("checkpoints = %#v", second.Checkpoints)
	}
	runs, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs len = %d runs = %#v", len(runs), runs)
	}
}

func TestRuntimeRunStoreDoesNotCompleteActiveRunFromEmptyProjection(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	})
	store := newRuntimeRunStore(conn)
	active, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusCompleted,
		CreatedAt:        active.CreatedAt,
		UpdatedAt:        active.UpdatedAt + 1,
		FinishedAt:       active.UpdatedAt + 1,
	}, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != active.ID || updated.Status != runtimeRunStatusActive || updated.FinishedAt != 0 {
		t.Fatalf("updated run = %#v active = %#v", updated, active)
	}
}
