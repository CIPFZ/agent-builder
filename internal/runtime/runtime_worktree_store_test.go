package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestRuntimeWorktreeStoreUpsertListAndStatusPersistence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

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

func TestRuntimeWorktreeRecoveryDeterministicStates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.eventStore = newRuntimeEventStore(conn)

	cleanupPath := createTestGitWorktree(t, repo, "agent-builder-clean", filepath.Join(runtimeWorktreeRoot(repo), "clean"))
	preservedPath := createTestGitWorktree(t, repo, "agent-builder-preserve", filepath.Join(runtimeWorktreeRoot(repo), "preserve"))
	missingPath := filepath.Join(runtimeWorktreeRoot(repo), "missing")
	for _, wt := range []RuntimeWorktree{
		{ID: "wt-clean", SessionID: "session-1", TurnID: "turn-1", BaseRepoPath: repo, WorktreePath: cleanupPath, Branch: "agent-builder-clean", Status: worktreeStatusCleanupPending, PreservePolicy: worktreePreserveNever, CleanupPolicy: worktreeCleanupExit, Owner: "runtime"},
		{ID: "wt-preserve", SessionID: "session-1", TurnID: "turn-1", BaseRepoPath: repo, WorktreePath: preservedPath, Branch: "agent-builder-preserve", Status: worktreeStatusEntered, PreservePolicy: worktreePreserveAlways, CleanupPolicy: worktreeCleanupExit, Owner: "runtime"},
		{ID: "wt-missing", SessionID: "session-1", TurnID: "turn-1", BaseRepoPath: repo, WorktreePath: missingPath, Branch: "agent-builder-missing", Status: worktreeStatusEntered, PreservePolicy: worktreePreserveNever, CleanupPolicy: worktreeCleanupManual, Owner: "runtime"},
	} {
		if _, err := service.worktrees.Upsert(ctx, wt); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := service.recoverWorktrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]RuntimeWorktree{}
	for _, wt := range recovered {
		byID[wt.ID] = wt
	}
	if byID["wt-clean"].Status != worktreeStatusCleaned {
		t.Fatalf("clean recovery = %#v", byID["wt-clean"])
	}
	if byID["wt-preserve"].Status != worktreeStatusPreserved {
		t.Fatalf("preserve recovery = %#v", byID["wt-preserve"])
	}
	if byID["wt-missing"].Status != worktreeStatusMissing {
		t.Fatalf("missing recovery = %#v", byID["wt-missing"])
	}
}

func TestRuntimeCreateWorktreeValidationAndPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "default", []RuntimePolicyRule{{
		ID:            "deny-worktree-create",
		Decision:      string(permission.PolicyDeny),
		BuiltinTool:   "worktree_create",
		PathPrefix:    dataDir,
		Reason:        "test denies worktree creation",
		PolicyProfile: "default",
	}}, 0)

	if _, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{SessionID: "session-1", BaseRepoPath: filepath.Join(dataDir, "not-git")}); err == nil {
		t.Fatal("expected base repo validation failure")
	}
	if _, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{SessionID: "session-1", BaseRepoPath: repo, Name: "../escape"}); err == nil {
		t.Fatal("expected invalid worktree name failure")
	}
	if _, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{SessionID: "session-1", BaseRepoPath: repo, Name: "feature"}); err == nil || !strings.Contains(err.Error(), "denies worktree creation") {
		t.Fatalf("expected policy denied create, got %v", err)
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventWorktreePolicyDenied
	}) {
		t.Fatalf("policy denied worktree event missing: %#v", events.Events)
	}
}

func TestRuntimeWorktreeEnterExitScopeAndReplay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromMode(permission.PolicyModeAutoRead, 0)
	if _, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:              "task-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ChildSessionID:  "child-1",
		Title:           "Scoped task",
		CWD:             repo,
		Status:          agentTaskStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{
		SessionID:      "session-1",
		TurnID:         "turn-1",
		TaskID:         "task-1",
		BaseRepoPath:   repo,
		Name:           "task-worktree",
		PreservePolicy: worktreePreserveNever,
		CleanupPolicy:  worktreeCleanupManual,
	})
	if err != nil {
		t.Fatal(err)
	}
	entered, err := service.EnterWorktree(ctx, created.Worktree.ID, RuntimeWorktreeActionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := service.TaskEffectiveScope(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if scope.Scope.BaseCWD != repo || scope.Scope.EffectiveCWD != entered.Worktree.WorktreePath || scope.Scope.WorktreeID != entered.Worktree.ID {
		t.Fatalf("scope = %#v", scope.Scope)
	}
	exited, err := service.ExitWorktree(ctx, created.Worktree.ID, RuntimeWorktreeActionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if exited.Worktree.Status != worktreeStatusCleanupPending {
		t.Fatalf("exit status = %#v", exited.Worktree)
	}
	scope, err = service.TaskEffectiveScope(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if scope.Scope.WorktreePath != "" || scope.Scope.EffectiveCWD != repo {
		t.Fatalf("scope after exit = %#v", scope.Scope)
	}
	replay, err := service.ReplayExport(ctx, RuntimeReplayExportRequest{SessionID: "session-1", TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(replay.Summary.Worktrees, func(wt RuntimeWorktree) bool {
		return wt.ID == created.Worktree.ID && wt.Status == worktreeStatusCleanupPending
	}) {
		t.Fatalf("replay worktrees = %#v", replay.Summary.Worktrees)
	}
}

func TestRuntimeCleanupWorktreeSafetyPreserveAndFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.policy = runtimePolicyFromMode(permission.PolicyModeAutoRead, 0)

	outside := RuntimeWorktree{ID: "wt-outside", SessionID: "session-1", BaseRepoPath: repo, WorktreePath: filepath.Join(dataDir, "outside"), Branch: "agent-builder-outside", Status: worktreeStatusCleanupPending, PreservePolicy: worktreePreserveNever, CleanupPolicy: worktreeCleanupManual, Owner: "runtime"}
	if _, err := service.worktrees.Upsert(ctx, outside); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CleanupWorktree(ctx, outside.ID, RuntimeWorktreeActionRequest{}); err == nil {
		t.Fatal("expected cleanup to refuse non-runtime-owned root path")
	}
	nonGitPath := filepath.Join(runtimeWorktreeRoot(repo), "not-git")
	if err := os.MkdirAll(nonGitPath, 0o755); err != nil {
		t.Fatal(err)
	}
	nonGit := RuntimeWorktree{ID: "wt-nongit", SessionID: "session-1", BaseRepoPath: repo, WorktreePath: nonGitPath, Branch: "agent-builder-nongit", Status: worktreeStatusCleanupPending, PreservePolicy: worktreePreserveNever, CleanupPolicy: worktreeCleanupManual, Owner: "runtime"}
	if _, err := service.worktrees.Upsert(ctx, nonGit); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CleanupWorktree(ctx, nonGit.ID, RuntimeWorktreeActionRequest{}); err == nil {
		t.Fatal("expected cleanup to refuse non-git worktree path")
	}
	stored, err := service.worktrees.Get(ctx, nonGit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != worktreeStatusCleanupFailed {
		t.Fatalf("non-git status = %#v", stored)
	}
	preservedPath := createTestGitWorktree(t, repo, "agent-builder-preserved", filepath.Join(runtimeWorktreeRoot(repo), "preserved"))
	preserved := RuntimeWorktree{ID: "wt-preserved", SessionID: "session-1", BaseRepoPath: repo, WorktreePath: preservedPath, Branch: "agent-builder-preserved", Status: worktreeStatusCleanupPending, PreservePolicy: worktreePreserveAlways, CleanupPolicy: worktreeCleanupManual, Owner: "runtime"}
	if _, err := service.worktrees.Upsert(ctx, preserved); err != nil {
		t.Fatal(err)
	}
	resp, err := service.CleanupWorktree(ctx, preserved.ID, RuntimeWorktreeActionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Worktree.Status != worktreeStatusPreserved {
		t.Fatalf("preserve cleanup = %#v", resp.Worktree)
	}
	failurePath := createTestGitWorktree(t, repo, "agent-builder-fail", filepath.Join(runtimeWorktreeRoot(repo), "fail"))
	failed := RuntimeWorktree{ID: "wt-fail", SessionID: "session-1", BaseRepoPath: repo, WorktreePath: failurePath, Branch: "agent-builder-fail", Status: worktreeStatusCleanupPending, PreservePolicy: worktreePreserveNever, CleanupPolicy: worktreeCleanupManual, Owner: "runtime"}
	if _, err := service.worktrees.Upsert(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CleanupWorktree(ctx, failed.ID, RuntimeWorktreeActionRequest{}); err == nil {
		t.Fatal("expected cleanup failure when base repo is unavailable")
	}
	stored, err = service.worktrees.Get(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != worktreeStatusCleanupFailed || stored.Error == "" {
		t.Fatalf("cleanup failed state = %#v", stored)
	}
}

func TestRuntimeWorktreePolicyHeadlessAndPathRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromParts(permission.PolicyModeAsk, "headless", nil, 0)
	if _, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{SessionID: "session-1", BaseRepoPath: repo, Name: "headless"}); err == nil {
		t.Fatal("expected headless ask to fail closed")
	}
	service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "default", []RuntimePolicyRule{{
		ID:            "deny-worktree-path",
		Decision:      string(permission.PolicyDeny),
		PathPrefix:    runtimeWorktreeRoot(repo),
		Reason:        "worktree path is blocked",
		PolicyProfile: "default",
	}}, 0)
	if _, err := service.CreateWorktree(ctx, RuntimeWorktreeCreateRequest{SessionID: "session-1", BaseRepoPath: repo, Name: "path-denied"}); err == nil || !strings.Contains(err.Error(), "worktree path is blocked") {
		t.Fatalf("expected path scoped denial, got %v", err)
	}
}

func initTestGitRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, path, "init")
	runTestGit(t, path, "config", "user.email", "test@example.com")
	runTestGit(t, path, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, path, "add", "README.md")
	runTestGit(t, path, "commit", "-m", "init")
	return path
}

func createTestGitWorktree(t *testing.T, repo, branch, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "worktree", "add", "-B", branch, path, "HEAD")
	return path
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
