package runtime

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/proto"
)

func TestRuntimePermissionStoreUpsertListAndMark(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimePermissionStore(conn)
	perm, err := store.Upsert(context.Background(), RuntimePermissionRequest{
		ID:           "perm-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ToolCallID:   "tool-1",
		ToolName:     "bash",
		Action:       "execute",
		Params:       map[string]any{"command": "echo hi"},
		Risk:         "execute",
		PolicyMode:   "plan",
		PolicyReason: "Plan mode blocks mutating, execute, network, destructive, or secret tool calls.",
		Decision:     "deny",
		Status:       permissionStatusPending,
		CreatedAt:    1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if perm.ID != "perm-1" || perm.Status != permissionStatusPending || perm.Params == nil {
		t.Fatalf("permission = %#v", perm)
	}
	if perm.PolicyMode != "plan" || perm.PolicyReason == "" || perm.Decision != "deny" {
		t.Fatalf("permission policy fields = %#v", perm)
	}

	pending, err := store.List(context.Background(), permissionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "perm-1" {
		t.Fatalf("pending = %#v", pending)
	}

	decided, err := store.Mark(context.Background(), "perm-1", permissionStatusExpired, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if decided.Status != permissionStatusExpired || decided.DecidedAt != 2000 {
		t.Fatalf("decided = %#v", decided)
	}

	pending, err = store.List(context.Background(), permissionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after mark = %#v", pending)
	}
}

func TestRuntimeRecoveryStatusExpiresInvalidPendingPermissions(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	service.permissionStore = newRuntimePermissionStore(conn)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-1",
		SessionID:  "session-1",
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{
		ID:         "perm-1",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
		ToolName:   "bash",
		Action:     "execute",
		Status:     permissionStatusPending,
		CreatedAt:  1000,
	}); err != nil {
		t.Fatal(err)
	}

	recovery, err := service.RecoveryStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingPermissions) != 0 {
		t.Fatalf("pending permissions = %#v", recovery.PendingPermissions)
	}
	perm, err := service.permissionStore.Get(context.Background(), "perm-1")
	if err != nil {
		t.Fatal(err)
	}
	if perm.Status != permissionStatusExpired || perm.DecidedAt == 0 {
		t.Fatalf("permission after recovery = %#v", perm)
	}
}
