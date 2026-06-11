package runtime

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
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

func TestRuntimeRunStoreReadsPersistedIdentityAndSummaryWithoutProjection(t *testing.T) {
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

	run, err := store.Upsert(context.Background(), RuntimeRun{
		ID:               "run-identity-summary",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-primary",
		SessionIDs:       []string{"session-primary", "session-child"},
		Objective:        "persisted read authority",
		Status:           runtimeRunStatusActive,
		Source:           runtimeRunSourceUserPrompt,
		CreatedAt:        1000,
		UpdatedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	byID, err := store.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySession, err := store.GetBySession(context.Background(), "session-child")
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []RuntimeRun{byID, bySession, list[0]} {
		if got.ID != run.ID || got.WorkspaceID != "workspace-1" || got.PrimarySessionID != "session-primary" {
			t.Fatalf("persisted identity mismatch: %#v", got)
		}
		if got.Objective != "persisted read authority" || got.Source != runtimeRunSourceUserPrompt || got.CreatedAt != 1000 || got.UpdatedAt != 1100 {
			t.Fatalf("persisted summary mismatch: %#v", got)
		}
		if len(got.SessionIDs) != 2 || !slices.Contains(got.SessionIDs, "session-primary") || !slices.Contains(got.SessionIDs, "session-child") {
			t.Fatalf("persisted session membership mismatch: %#v", got.SessionIDs)
		}
	}
}

func TestRuntimeRunSummaryFromRunStripsLifecycleAndEvidence(t *testing.T) {
	t.Parallel()

	summary := runtimeRunSummaryFromRun(RuntimeRun{
		ID:               "run-summary",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1", "session-child"},
		Objective:        "summary only",
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:     "checkpoint-1",
			Status: runtimeRunCheckpointStatusResumable,
		}},
		CreatedAt:  1000,
		UpdatedAt:  1200,
		FinishedAt: 1300,
	})
	if summary.ID != "run-summary" || summary.WorkspaceID != "workspace-1" || summary.PrimarySessionID != "session-1" {
		t.Fatalf("summary identity = %#v", summary)
	}
	if summary.Objective != "summary only" || summary.Source != runtimeRunSourceUserPrompt || summary.CreatedAt != 1000 || summary.UpdatedAt != 1200 {
		t.Fatalf("summary metadata = %#v", summary)
	}
	if len(summary.SessionIDs) != 2 || !slices.Contains(summary.SessionIDs, "session-child") {
		t.Fatalf("summary sessions = %#v", summary.SessionIDs)
	}
}

func TestRuntimeRunSummaryJSONExcludesCheckpointMarkersAndActionability(t *testing.T) {
	t.Parallel()

	summary := runtimeRunSummaryFromRun(RuntimeRun{
		ID:               "run-summary",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Objective:        "summary only",
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:             "checkpoint-1",
			TurnID:         "turn-1",
			Status:         runtimeRunCheckpointStatusResumable,
			Summary:        "checkpoint evidence",
			ArtifactRefs:   []string{"artifact://report"},
			AcknowledgedAt: 1200,
			DiscardedAt:    1300,
			ResumedTurnIDs: []string{"turn-resume-1"},
			ResumeEligible: true,
		}},
		CreatedAt: 1000,
		UpdatedAt: 1400,
	})
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"status",
		"finishedAt",
		"checkpoints",
		"acknowledgedAt",
		"discardedAt",
		"resumedTurnIds",
		"resumeEligible",
		"artifactRefs",
	} {
		if _, ok := raw[field]; ok {
			t.Fatalf("summary leaked checkpoint/actionability field %q: %s", field, string(payload))
		}
	}
	if raw["id"] != "run-summary" || raw["primarySessionId"] != "session-1" {
		t.Fatalf("summary lost accepted identity fields: %s", string(payload))
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

func TestRuntimeRunStoreDoesNotCompleteInterruptedRunFromEmptyProjection(t *testing.T) {
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
	interrupted, err := store.Upsert(context.Background(), RuntimeRun{
		ID:               "run-interrupted",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Objective:        "recover interrupted run",
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		CreatedAt:        1000,
		UpdatedAt:        2000,
		FinishedAt:       2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusCompleted,
		CreatedAt:        interrupted.CreatedAt,
		UpdatedAt:        interrupted.UpdatedAt + 1,
		FinishedAt:       interrupted.UpdatedAt + 1,
	}, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != interrupted.ID || updated.Status != runtimeRunStatusInterrupted || updated.FinishedAt != interrupted.FinishedAt {
		t.Fatalf("empty projection completed interrupted run: updated=%#v interrupted=%#v", updated, interrupted)
	}
}

func TestRuntimeRunStoreLinksTurnAndPreservesUserPromptSourceOnReconcile(t *testing.T) {
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
	run, err := store.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := store.LinkTurn(context.Background(), run.ID, "session-1", "turn-1", run.CreatedAt+10)
	if err != nil {
		t.Fatal(err)
	}
	if linked.Status != runtimeRunStatusActive || linked.FinishedAt != 0 || linked.Source != runtimeRunSourceUserPrompt {
		t.Fatalf("linked run = %#v", linked)
	}
	var linkedTurnID string
	if err := conn.QueryRowContext(context.Background(), `SELECT turn_id FROM runtime_run_sessions WHERE run_id = ? AND session_id = ?`, run.ID, "session-1").Scan(&linkedTurnID); err != nil {
		t.Fatal(err)
	}
	if linkedTurnID != "turn-1" {
		t.Fatalf("linked turn id = %q", linkedTurnID)
	}
	reconciled, err := store.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Objective:        "write report",
		Status:           runtimeRunStatusCompleted,
		TurnIDs:          []string{"turn-1"},
		Diagnostics:      RuntimeRunDiagnostics{TurnCount: 1},
		CreatedAt:        run.CreatedAt,
		UpdatedAt:        run.CreatedAt + 100,
		FinishedAt:       run.CreatedAt + 100,
	}, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ID != run.ID || reconciled.Source != runtimeRunSourceUserPrompt || reconciled.Status != runtimeRunStatusCompleted || reconciled.FinishedAt == 0 {
		t.Fatalf("reconciled run = %#v original = %#v", reconciled, run)
	}
}

func TestRuntimeRunStoreMarksCheckpointAcknowledgedAndDiscardedIdempotently(t *testing.T) {
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
	run, err := store.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusInterrupted,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-1:interrupted",
			TurnID:       "turn-1",
			Status:       turnStatusInterrupted,
			Summary:      "runtime restarted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    1200,
		}},
		CreatedAt: 1000,
		UpdatedAt: 1300,
	}, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}

	ack, err := store.AcknowledgeCheckpoint(context.Background(), run.ID, "turn:turn-1:interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if len(ack.Checkpoints) != 1 || ack.Checkpoints[0].AcknowledgedAt == 0 || ack.Checkpoints[0].ResumeEligible {
		t.Fatalf("ack checkpoints = %#v", ack.Checkpoints)
	}
	secondAck, err := store.AcknowledgeCheckpoint(context.Background(), run.ID, "turn:turn-1:interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if secondAck.Checkpoints[0].AcknowledgedAt != ack.Checkpoints[0].AcknowledgedAt {
		t.Fatalf("ack was not idempotent: first=%#v second=%#v", ack.Checkpoints[0], secondAck.Checkpoints[0])
	}

	discarded, err := store.DiscardCheckpoint(context.Background(), run.ID, "turn:turn-1:interrupted")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := discarded.Checkpoints[0]
	if checkpoint.DiscardedAt == 0 || checkpoint.Status != turnStatusInterrupted || len(checkpoint.ArtifactRefs) != 1 {
		t.Fatalf("discard checkpoint = %#v", checkpoint)
	}
	if checkpoint.AcknowledgedAt != ack.Checkpoints[0].AcknowledgedAt {
		t.Fatalf("discard rewrote acknowledgement: %#v", checkpoint)
	}
}

func TestRuntimeRunStoreLinksCheckpointResumeTurnsWithoutMutatingEvidence(t *testing.T) {
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
	run, err := store.UpsertFromProjection(context.Background(), RuntimeRunProjection{
		ID:               runtimeRunProjectionID("session-1"),
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Status:           runtimeRunStatusInterrupted,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-1:interrupted",
			TurnID:       "turn-1",
			Status:       turnStatusInterrupted,
			Summary:      "runtime restarted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    1200,
		}},
		CreatedAt: 1000,
		UpdatedAt: 1300,
	}, runtimeRunSourceBackfill)
	if err != nil {
		t.Fatal(err)
	}

	linked, err := store.LinkCheckpointResume(context.Background(), run.ID, "turn:turn-1:interrupted", "turn-resume-1")
	if err != nil {
		t.Fatal(err)
	}
	linked, err = store.LinkCheckpointResume(context.Background(), run.ID, "turn:turn-1:interrupted", "turn-resume-1")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := linked.Checkpoints[0]
	if len(checkpoint.ResumedTurnIDs) != 1 || checkpoint.ResumedTurnIDs[0] != "turn-resume-1" {
		t.Fatalf("resumed turn ids = %#v", checkpoint.ResumedTurnIDs)
	}
	if checkpoint.Status != turnStatusInterrupted || checkpoint.Summary != "runtime restarted" || checkpoint.ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("resume link mutated evidence: %#v", checkpoint)
	}
	if !checkpoint.ResumeEligible {
		t.Fatalf("resume link should not acknowledge/discard checkpoint: %#v", checkpoint)
	}
	if linked.Status != runtimeRunStatusInterrupted || linked.FinishedAt != 0 {
		t.Fatalf("resume checkpoint link should not mark run active without resumed turn link: %#v", linked)
	}
}

func TestRuntimeRunResumePromptRedactsCheckpointSummary(t *testing.T) {
	t.Parallel()

	prompt := runtimeRunResumePrompt(RuntimeRun{
		ID:               "run-1",
		PrimarySessionID: "session-1",
	}, RuntimeRunCheckpoint{
		ID:           "checkpoint-1",
		TurnID:       "turn-1",
		Summary:      "continue with API_TOKEN=sk-secret",
		ArtifactRefs: []string{"artifact://report"},
	})
	if strings.Contains(prompt, "sk-secret") {
		t.Fatalf("resume prompt leaked secret: %s", prompt)
	}
	if !strings.Contains(prompt, "run-1") || !strings.Contains(prompt, "checkpoint-1") || !strings.Contains(prompt, "turn-1") {
		t.Fatalf("resume prompt missing structured ids: %s", prompt)
	}
}
