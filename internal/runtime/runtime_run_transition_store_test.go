package runtime

import (
	"context"
	"database/sql"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/pressly/goose/v3"
)

func TestRuntimeRunTransitionStoreUpsertsIdempotentlyAndListsInOrder(t *testing.T) {
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
	store := newRuntimeRunTransitionStore(conn)
	first := RuntimeRunTransition{
		RunID:      "run-1",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		FromStatus: runtimeRunStatusActive,
		ToStatus:   runtimeRunStatusCompleted,
		Reason:     "turn completed",
		Source:     "runtime_reconcile",
		EventID:    "event-1",
		CreatedAt:  1000,
		Metadata:   map[string]any{"attempt": float64(1)},
	}
	stored, err := store.Upsert(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Upsert(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != stored.ID {
		t.Fatalf("idempotent id mismatch: first=%#v second=%#v", stored, again)
	}
	second, err := store.Upsert(context.Background(), RuntimeRunTransition{
		RunID:     "run-1",
		SessionID: "session-1",
		TurnID:    "turn-2",
		ToStatus:  runtimeRunStatusInterrupted,
		Source:    "runtime_recovery",
		CreatedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	byRun, err := store.ListByRun(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byRun) != 2 || byRun[0].ID != stored.ID || byRun[1].ID != second.ID {
		t.Fatalf("by run = %#v", byRun)
	}
	bySession, err := store.ListBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(bySession) != 2 {
		t.Fatalf("by session = %#v", bySession)
	}
	byTurn, err := store.ListByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byTurn) != 1 || byTurn[0].ID != stored.ID || byTurn[0].Metadata["attempt"] != float64(1) {
		t.Fatalf("by turn = %#v", byTurn)
	}
}

func TestRuntimeRunTransitionMigrationRollsDownAndBackUp(t *testing.T) {
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
		db.ResetPool()
	})
	if !sqliteTableExists(t, conn, "runtime_run_transitions") {
		t.Fatal("runtime_run_transitions table missing after migration up")
	}
	if err := goose.DownTo(conn, "migrations", 20260609000000); err != nil {
		t.Fatal(err)
	}
	if sqliteTableExists(t, conn, "runtime_run_transitions") {
		t.Fatal("runtime_run_transitions table still exists after migration down")
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		t.Fatal(err)
	}
	if !sqliteTableExists(t, conn, "runtime_run_transitions") {
		t.Fatal("runtime_run_transitions table missing after migration back up")
	}
}

func sqliteTableExists(t *testing.T, conn *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := conn.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == nil {
		return name == table
	}
	if err == sql.ErrNoRows {
		return false
	}
	t.Fatal(err)
	return false
}
