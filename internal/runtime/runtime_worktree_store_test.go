package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimeWorktreeStoreUpsertListAndStatusPersistence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeWorktreeStore(conn)
	wt, err := store.Upsert(context.Background(), RuntimeWorktree{
		ID:             "wt-1",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		TaskID:         "task-1",
		BaseRepoPath:   filepath.Join(dataDir, "repo"),
		WorktreePath:   filepath.Join(dataDir, "repo", ".agent-builder", "worktrees", "wt-1"),
		Branch:         "agent-builder-wt-1",
		Ref:            "HEAD",
		Status:         worktreeStatusCreated,
		PreservePolicy: worktreePreserveOnFailure,
		CleanupPolicy:  worktreeCleanupManual,
		Owner:          "runtime",
		Metadata:       map[string]string{"root": "owned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if wt.ID != "wt-1" || wt.Status != worktreeStatusCreated || wt.Metadata["root"] != "owned" {
		t.Fatalf("worktree = %#v", wt)
	}
	wt.Status = worktreeStatusEntered
	wt.EnteredAt = 42
	wt.Error = "previous error"
	wt, err = store.Upsert(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	if wt.Status != worktreeStatusEntered || wt.EnteredAt != 42 {
		t.Fatalf("updated worktree = %#v", wt)
	}
	items, err := store.ListByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "wt-1" {
		t.Fatalf("items = %#v", items)
	}
	active, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Status != worktreeStatusEntered {
		t.Fatalf("active = %#v", active)
	}
	wt.Status = worktreeStatusCleaned
	wt.Error = ""
	wt, err = store.Upsert(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	if wt.Error != "" {
		t.Fatalf("expected cleared error, got %q", wt.Error)
	}
}

func TestRuntimeWorktreePathValidationAndSlug(t *testing.T) {
	t.Parallel()

	if _, err := runtimeWorktreeSlug("../escape", "seed"); err == nil {
		t.Fatal("path traversal slug should be rejected")
	}
	if _, err := runtimeWorktreeSlug("feature/name", "seed"); err != nil {
		t.Fatalf("nested safe slug should flatten: %v", err)
	}
	root := filepath.Join(t.TempDir(), "repo", ".agent-builder", "worktrees")
	if err := pathInsideRuntimeWorktreeRoot(root, filepath.Join(root, "wt-1")); err != nil {
		t.Fatalf("path under root rejected: %v", err)
	}
	if err := pathInsideRuntimeWorktreeRoot(root, filepath.Dir(root)); err == nil {
		t.Fatal("path outside root accepted")
	}
}

func TestRuntimeWorktreeRecoveryMissingAndCleanupPending(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	base := filepath.Join(dataDir, "repo")
	root := runtimeWorktreeRoot(base)
	path := filepath.Join(root, "wt-1")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	service := newRuntimeService()
	service.worktrees = newRuntimeWorktreeStore(conn)
	if _, err := service.worktrees.Upsert(context.Background(), RuntimeWorktree{
		ID:             "wt-1",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		TaskID:         "task-1",
		BaseRepoPath:   base,
		WorktreePath:   path,
		Branch:         "agent-builder-wt-1",
		Status:         worktreeStatusEntered,
		PreservePolicy: worktreePreserveNever,
		CleanupPolicy:  worktreeCleanupManual,
		Owner:          "runtime",
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.recoverWorktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Status != worktreeStatusCleanupPending {
		t.Fatalf("recovered = %#v", recovered)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	recovered, err = service.recoverWorktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Status != worktreeStatusMissing {
		t.Fatalf("missing recovery = %#v", recovered)
	}
}
