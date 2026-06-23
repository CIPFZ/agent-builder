package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestRuntimeRunStatusWriterRejectsUnknownAndSourcelessInputs(t *testing.T) {
	t.Parallel()

	store, release := runtimeRunStatusWriterTestStore(t)
	defer release()
	run, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}

	for _, req := range []runtimeRunStatusWriteRequest{
		{RunID: run.ID, Status: runtimeRunStatusActive, Source: "runtime_event_payload"},
		{RunID: run.ID, Status: runtimeRunStatusActive},
		{RunID: run.ID, Status: "unknown", Source: runtimeRunTransitionSourceTurnStarted, TurnID: "turn-1"},
	} {
		if _, err := store.writeRuntimeRunStatus(context.Background(), req); err == nil {
			t.Fatalf("status write unexpectedly accepted: %#v", req)
		}
	}

	persisted, err := store.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != runtimeRunStatusActive || persisted.FinishedAt != 0 {
		t.Fatalf("rejected status write mutated run: %#v", persisted)
	}
}

func TestRuntimeRunStatusWriterRequiresProjectionParityForTerminalWrites(t *testing.T) {
	t.Parallel()

	store, release := runtimeRunStatusWriterTestStore(t)
	defer release()
	run, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	bounded := runtimeRunStatusWriterProjection(run, runtimeRunStatusCompleted)
	bounded.ActivityWindow = RuntimeActivityWindow{Limit: 1, FromStart: false, ToEnd: true, HasMoreBefore: true}

	for _, req := range []runtimeRunStatusWriteRequest{
		{
			RunID:        run.ID,
			SessionID:    "session-1",
			Status:       runtimeRunStatusCompleted,
			Source:       runtimeRunStatusWriteSourceProjectionReconcile,
			EvidenceKind: runtimeRunStatusWriteEvidenceProjection,
			Timestamp:    2000,
		},
		{
			RunID:                    run.ID,
			SessionID:                "session-1",
			Status:                   runtimeRunStatusCompleted,
			Source:                   runtimeRunStatusWriteSourceProjectionReconcile,
			EvidenceKind:             runtimeRunStatusWriteEvidenceProjection,
			Timestamp:                2000,
			RequiresProjectionParity: true,
			Projection:               &bounded,
		},
	} {
		if _, err := store.writeRuntimeRunStatus(context.Background(), req); err == nil {
			t.Fatalf("terminal status write unexpectedly accepted: %#v", req)
		}
	}

	persisted, err := store.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != runtimeRunStatusActive || persisted.FinishedAt != 0 {
		t.Fatalf("rejected terminal write mutated run: %#v", persisted)
	}
}

func TestRuntimeRunStatusWriterUpdatesOnlyStatusColumns(t *testing.T) {
	t.Parallel()

	store, release := runtimeRunStatusWriterTestStore(t)
	defer release()
	run, err := store.Upsert(context.Background(), RuntimeRun{
		ID:               "run-status-writer",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1", "session-child"},
		Objective:        "write report",
		Status:           runtimeRunStatusActive,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-1:interrupted",
			TurnID:       "turn-1",
			Status:       turnStatusInterrupted,
			Summary:      "interrupted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    1500,
		}},
		CreatedAt: 1000,
		UpdatedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := runtimeRunStatusWriterProjection(run, runtimeRunStatusCompleted)
	projection.UpdatedAt = 3000
	projection.FinishedAt = 3000

	updated, err := store.writeRuntimeRunStatus(context.Background(), runtimeRunStatusWriteRequest{
		RunID:                    run.ID,
		SessionID:                "session-1",
		Status:                   runtimeRunStatusCompleted,
		Source:                   runtimeRunStatusWriteSourceProjectionReconcile,
		Reason:                   "test terminal parity",
		EvidenceKind:             runtimeRunStatusWriteEvidenceProjection,
		Timestamp:                3000,
		RequiresProjectionParity: true,
		Projection:               &projection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != runtimeRunStatusCompleted || updated.UpdatedAt != 3000 || updated.FinishedAt != 3000 {
		t.Fatalf("status columns not updated: %#v", updated)
	}
	if updated.Objective != run.Objective || len(updated.SessionIDs) != 2 || len(updated.Checkpoints) != 1 {
		t.Fatalf("status writer mutated non-status run evidence: before=%#v after=%#v", run, updated)
	}
	if updated.Checkpoints[0].Summary != "interrupted" || updated.Checkpoints[0].ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("status writer mutated checkpoint evidence: %#v", updated.Checkpoints)
	}
}

func runtimeRunStatusWriterTestStore(t *testing.T) (runtimeRunStore, func()) {
	t.Helper()
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return newRuntimeRunStore(conn), func() {
		if err := db.Release(dataDir); err != nil {
			t.Fatalf("release db: %v", err)
		}
	}
}

func runtimeRunStatusWriterProjection(run RuntimeRun, status string) RuntimeRunProjection {
	return RuntimeRunProjection{
		ID:               run.ID,
		WorkspaceID:      run.WorkspaceID,
		PrimarySessionID: run.PrimarySessionID,
		SessionIDs:       append([]string(nil), run.SessionIDs...),
		Objective:        run.Objective,
		Status:           status,
		Diagnostics:      RuntimeRunDiagnostics{TurnCount: 1},
		ActivityWindow:   RuntimeActivityWindow{FromStart: true, ToEnd: true},
		Source: RuntimeRunProjectionSource{
			Kind:                  runtimeRunProjectionSourceKind,
			ReadOnly:              true,
			SessionActivityParity: true,
		},
		CreatedAt:  run.CreatedAt,
		UpdatedAt:  2000,
		FinishedAt: 2000,
	}
}
