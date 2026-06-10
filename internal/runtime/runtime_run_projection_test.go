package runtime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeRunProjectionDerivesFromSessionActivityEvidence(t *testing.T) {
	t.Parallel()

	service, sessionID, artifactPath := newRuntimeRunProjectionFixture(t)

	activity, err := service.SessionActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	run := projection.Run
	if run.ID != "run:session:"+sessionID || run.PrimarySessionID != sessionID || !slices.Contains(run.SessionIDs, sessionID) {
		t.Fatalf("run identity mismatch: %#v", run)
	}
	if run.Source.Kind != runtimeRunProjectionSourceKind || !run.Source.ReadOnly || !run.Source.SessionActivityParity {
		t.Fatalf("source boundary mismatch: %#v", run.Source)
	}
	if run.Status != runtimeRunStatusInterrupted {
		t.Fatalf("status = %q, want interrupted: %#v", run.Status, run.Diagnostics)
	}
	if !slices.Contains(run.TurnIDs, "turn-run-new") || !slices.Contains(run.ToolCallIDs, "tool-run-new") || !slices.Contains(run.PermissionRequestIDs, "perm-run-new") {
		t.Fatalf("projection ids missing turn/tool/permission evidence: %#v", run)
	}
	if !slices.Contains(run.TaskIDs, "task-run-1") || !slices.Contains(run.SessionIDs, "session-child-run") {
		t.Fatalf("projection ids missing task evidence: %#v", run)
	}
	if !slices.Contains(run.ProducedArtifacts, artifactPath) || !slices.Contains(run.VerifiedArtifacts, artifactPath) {
		t.Fatalf("artifact parity mismatch: produced=%#v verified=%#v", run.ProducedArtifacts, run.VerifiedArtifacts)
	}
	fullTurn := findRuntimeTurn(activity.Turns, "turn-run-new")
	if fullTurn.ID == "" {
		t.Fatalf("full activity turn missing: %#v", activity.Turns)
	}
	if !slices.Equal(run.VerifiedArtifacts, appendUniqueStrings(nil, fullTurn.Diagnostics.VerifiedArtifacts...)) {
		t.Fatalf("verified artifacts diverged from SessionActivity diagnostics: run=%#v full=%#v", run.VerifiedArtifacts, fullTurn.Diagnostics.VerifiedArtifacts)
	}
	if run.Diagnostics.TerminalPermissionCounts.Cancelled != 1 || run.Diagnostics.TerminalPermissionCounts.Pending != 0 {
		t.Fatalf("terminal permission counts resurrected actionability: %#v", run.Diagnostics.TerminalPermissionCounts)
	}
	if !slices.ContainsFunc(run.Checkpoints, func(checkpoint RuntimeRunCheckpoint) bool {
		return checkpoint.TurnID == "turn-run-new" && checkpoint.Status == turnStatusInterrupted && checkpoint.ResumeEligible
	}) {
		t.Fatalf("interrupted checkpoint missing: %#v", run.Checkpoints)
	}
	if len(run.UserActions.Resume) != 1 || !run.UserActions.Resume[0].Enabled || run.UserActions.Resume[0].TurnID != "turn-run-new" {
		t.Fatalf("resume action should be explicit read-only DTO evidence: %#v", run.UserActions.Resume)
	}
}

func TestRuntimeRunProjectionCursorWindowKeepsSessionActivityParity(t *testing.T) {
	t.Parallel()

	service, sessionID, artifactPath := newRuntimeRunProjectionFixture(t)

	window, err := service.SessionActivityCursorWindow(context.Background(), sessionID, "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if window.Window.FirstCursor == "" || !window.Window.HasMoreBefore {
		t.Fatalf("fixture should produce a bounded cursor window: %#v", window.Window)
	}
	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	run := projection.Run
	if run.ActivityWindow.LastCursor != window.Window.LastCursor || run.EvidenceCursor != window.Window.LastCursor {
		t.Fatalf("projection cursor mismatch run=%#v window=%#v", run.ActivityWindow, window.Window)
	}
	fullTurn := findRuntimeTurn(mustSessionActivity(t, service, sessionID).Turns, "turn-run-new")
	windowTurn := findRuntimeTurn(window.Turns, "turn-run-new")
	if fullTurn.ID == "" || windowTurn.ID == "" {
		t.Fatalf("turn-run-new missing from full/window activity: full=%#v window=%#v", fullTurn, windowTurn)
	}
	if !slices.Equal(windowTurn.Diagnostics.ProducedArtifacts, fullTurn.Diagnostics.ProducedArtifacts) ||
		!slices.Equal(windowTurn.Diagnostics.VerifiedArtifacts, fullTurn.Diagnostics.VerifiedArtifacts) ||
		windowTurn.Diagnostics.PermissionCounts != fullTurn.Diagnostics.PermissionCounts {
		t.Fatalf("cursor diagnostics diverged from full activity: full=%#v window=%#v", fullTurn.Diagnostics, windowTurn.Diagnostics)
	}
	windowTurnIDs := make([]string, 0, len(window.Turns))
	for _, turn := range window.Turns {
		windowTurnIDs = append(windowTurnIDs, turn.ID)
	}
	slices.Sort(windowTurnIDs)
	if !slices.Equal(run.TurnIDs, windowTurnIDs) {
		t.Fatalf("cursor projection turn ids diverged from window: run=%#v window=%#v", run.TurnIDs, windowTurnIDs)
	}
	if !slices.Contains(run.ProducedArtifacts, artifactPath) || run.Diagnostics.TerminalPermissionCounts.Pending != 0 {
		t.Fatalf("cursor projection parity/actionability mismatch: %#v", run)
	}
	if run.Source.Kind != runtimeRunProjectionSourceKind || !run.Source.ReadOnly {
		t.Fatalf("cursor projection became non-read-only: %#v", run.Source)
	}
}

func TestRuntimeRunDetailRefreshesPersistedStatusFromProjection(t *testing.T) {
	t.Parallel()

	service, sessionID, _ := newRuntimeRunProjectionFixture(t)

	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	runID := projection.Run.ID
	stale, err := service.runs.LinkTurn(context.Background(), runID, sessionID, "turn-stale-active", time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != runtimeRunStatusActive {
		t.Fatalf("fixture failed to create stale active run: %#v", stale)
	}

	detail, err := service.Run(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Projection.Status != runtimeRunStatusInterrupted {
		t.Fatalf("projection status = %q, want interrupted: %#v", detail.Projection.Status, detail.Projection)
	}
	if detail.Run.Status != detail.Projection.Status || detail.Run.FinishedAt != detail.Projection.FinishedAt {
		t.Fatalf("run detail did not refresh from projection: run=%#v projection=%#v", detail.Run, detail.Projection)
	}
	persisted, err := service.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != detail.Projection.Status || persisted.FinishedAt != detail.Projection.FinishedAt {
		t.Fatalf("persisted run diverged from projection: persisted=%#v projection=%#v", persisted, detail.Projection)
	}
}

func TestRuntimeRunListRefreshesPersistedStatusFromProjection(t *testing.T) {
	t.Parallel()

	service, sessionID, _ := newRuntimeRunProjectionFixture(t)

	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	runID := projection.Run.ID
	stale, err := service.runs.LinkTurn(context.Background(), runID, sessionID, "turn-stale-list-active", time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != runtimeRunStatusActive {
		t.Fatalf("fixture failed to create stale active run: %#v", stale)
	}

	list, err := service.Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	listed := findRuntimeRun(list.Runs, runID)
	if listed.ID == "" {
		t.Fatalf("run %q missing from list: %#v", runID, list.Runs)
	}
	if listed.Status != runtimeRunStatusInterrupted || listed.FinishedAt == 0 {
		t.Fatalf("run list did not refresh from projection: listed=%#v projection=%#v", listed, projection.Run)
	}
	persisted, err := service.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != listed.Status || persisted.FinishedAt != listed.FinishedAt {
		t.Fatalf("persisted run diverged from list after reconciliation: persisted=%#v listed=%#v", persisted, listed)
	}
}

func TestRuntimeRunProjectionWindowDoesNotMutatePersistedRunDetail(t *testing.T) {
	t.Parallel()

	service, sessionID, _ := newRuntimeRunProjectionFixture(t)

	full, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	runID := full.Run.ID
	before, err := service.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	windowed, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if windowed.Run.ActivityWindow.FromStart && windowed.Run.ActivityWindow.ToEnd {
		t.Fatalf("fixture did not produce a bounded projection window: %#v", windowed.Run.ActivityWindow)
	}
	if windowed.Run.ID != runID {
		t.Fatalf("bounded projection run id = %q, want durable run id %q", windowed.Run.ID, runID)
	}
	after, err := service.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != before.Status || after.FinishedAt != before.FinishedAt || after.Objective != before.Objective || len(after.SessionIDs) != len(before.SessionIDs) || len(after.Checkpoints) != len(before.Checkpoints) {
		t.Fatalf("bounded projection mutated persisted run: before=%#v after=%#v windowed=%#v", before, after, windowed.Run)
	}
}

func TestRuntimeRunDetailPreservesCheckpointMarkersThroughReconciliation(t *testing.T) {
	t.Parallel()

	service, sessionID, _ := newRuntimeRunProjectionFixture(t)

	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	runID := projection.Run.ID
	checkpointID := "turn:turn-run-new:interrupted"
	acknowledged, err := service.AcknowledgeRunCheckpoint(context.Background(), runID, checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	ackCheckpoint := findRuntimeRunCheckpoint(acknowledged.Run.Checkpoints, checkpointID)
	if ackCheckpoint.ID == "" || ackCheckpoint.AcknowledgedAt == 0 || ackCheckpoint.ResumeEligible {
		t.Fatalf("acknowledged checkpoint marker missing after run detail refresh: %#v", acknowledged.Run.Checkpoints)
	}
	if acknowledged.Action == nil || !acknowledged.Action.Accepted || acknowledged.Action.Reason != runtimeRunCheckpointActionReasonAcknowledged || acknowledged.Action.Source.Action != runtimeRunCheckpointActionAcknowledge || acknowledged.Action.Source.IdempotentBy != "run_id+checkpoint_id" {
		t.Fatalf("acknowledge action metadata = %#v", acknowledged.Action)
	}
	if acknowledged.Action.Source.Kind != runtimeRunCheckpointActionSourceKind || !acknowledged.Action.Source.BackendOnly || acknowledged.Action.Source.StartsWorker || !acknowledged.Action.Source.SessionActivityParity || len(acknowledged.Action.RefreshTargets) == 0 {
		t.Fatalf("acknowledge action source/refresh metadata = %#v", acknowledged.Action)
	}
	if !slices.Contains(acknowledged.Action.Source.Evidence, "runtime_runs") ||
		!slices.Contains(acknowledged.Action.Source.Evidence, "runtime_run_checkpoints") ||
		!slices.Contains(acknowledged.Action.Source.Evidence, "runtime_run_projection") ||
		!slices.Contains(acknowledged.Action.Source.Evidence, "session_activity") {
		t.Fatalf("acknowledge action evidence = %#v", acknowledged.Action.Source.Evidence)
	}

	discarded, err := service.DiscardRunCheckpoint(context.Background(), runID, checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	discardCheckpoint := findRuntimeRunCheckpoint(discarded.Run.Checkpoints, checkpointID)
	if discardCheckpoint.ID == "" || discardCheckpoint.AcknowledgedAt != ackCheckpoint.AcknowledgedAt || discardCheckpoint.DiscardedAt == 0 || discardCheckpoint.ResumeEligible {
		t.Fatalf("discarded checkpoint marker missing after run detail refresh: %#v", discarded.Run.Checkpoints)
	}
	if discarded.Action == nil || !discarded.Action.Accepted || discarded.Action.Reason != runtimeRunCheckpointActionReasonDiscarded || discarded.Action.Source.Action != runtimeRunCheckpointActionDiscard || discarded.Action.Source.IdempotentBy != "run_id+checkpoint_id" {
		t.Fatalf("discard action metadata = %#v", discarded.Action)
	}
	refreshed, err := service.Run(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Action != nil {
		t.Fatalf("plain run read should not carry action metadata: %#v", refreshed.Action)
	}
	refreshedCheckpoint := findRuntimeRunCheckpoint(refreshed.Run.Checkpoints, checkpointID)
	if refreshedCheckpoint.AcknowledgedAt != ackCheckpoint.AcknowledgedAt || refreshedCheckpoint.DiscardedAt != discardCheckpoint.DiscardedAt {
		t.Fatalf("checkpoint markers were not durable across reconciliation: %#v", refreshed.Run.Checkpoints)
	}
}

func TestRuntimeRunEnvelopeRestartReplayDoesNotRestoreStaleActionability(t *testing.T) {
	t.Parallel()

	runtimeBackend, workspace := backendForSkillTest(t)
	conn, err := db.Connect(context.Background(), workspace.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(workspace.DataDir)
	})
	service := newRuntimeService()
	service.runtime = runtimeBackend
	service.workspace = &workspace
	service.turns = newRuntimeTurnStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.transitions = newRuntimeRunTransitionStore(conn)

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "run-envelope-restart")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	userMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "recover run envelope"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().Add(-2 * time.Second).UnixMilli()
	run, err := service.runs.EnsureForSession(context.Background(), workspace.ID, sess.ID, "recover run envelope", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:            "turn-run-envelope",
		SessionID:     sess.ID,
		Status:        turnStatusRunning,
		UserMessageID: userMessage.ID,
		PromptPreview: "recover run envelope",
		StartedAt:     startedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, sess.ID, "turn-run-envelope", startedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-run-envelope",
		SessionID: sess.ID,
		TurnID:    "turn-run-envelope",
		Name:      "bash",
		Source:    scheduler.ToolSourceShell,
		Command:   "Start-Sleep -Seconds 60",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{
		ID:         "perm-run-envelope",
		SessionID:  sess.ID,
		TurnID:     "turn-run-envelope",
		ToolCallID: "tool-run-envelope",
		ToolName:   "bash",
		Action:     "execute",
		Status:     permissionStatusPending,
		CreatedAt:  startedAt + 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.mcpRequestStore.Upsert(context.Background(), RuntimeMCPRequest{
		ID:             "mcp-run-envelope",
		Kind:           mcpRequestKindAuth,
		Server:         "hosted-docs",
		CapabilityID:   "mcp_server:hosted-docs",
		SessionID:      sess.ID,
		TurnID:         "turn-run-envelope",
		Status:         mcpRequestStatusPending,
		PolicyDecision: "ask",
		CreatedAt:      startedAt + 20,
		UpdatedAt:      startedAt + 20,
	}); err != nil {
		t.Fatal(err)
	}

	interrupted, err := service.turns.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 || interrupted[0].Status != turnStatusInterrupted {
		t.Fatalf("interrupted turns = %#v", interrupted)
	}
	cancelledTools, err := cancelUnfinishedRuntimeToolCalls(context.Background(), service.toolCalls, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelledTools) != 1 {
		t.Fatalf("cancelled tools = %#v", cancelledTools)
	}
	expiredPermissions, err := service.expireInvalidPendingPermissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(expiredPermissions) != 1 || expiredPermissions[0].Status == permissionStatusPending {
		t.Fatalf("expired permissions = %#v", expiredPermissions)
	}
	cancelledMCP, err := service.mcpRequestStore.CancelActionableOnStartup(context.Background(), "runtime restarted; stale MCP request is no longer actionable")
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelledMCP) != 1 || cancelledMCP[0].Status != mcpRequestStatusCancelled {
		t.Fatalf("cancelled mcp = %#v", cancelledMCP)
	}
	if !runtimeRunSessionLinkedToTurn(context.Background(), service.runs, run.ID, sess.ID, "turn-run-envelope") {
		t.Fatalf("startup recovery broke run turn link: run=%s turn=%s", run.ID, "turn-run-envelope")
	}
	service.recordRunTurnTransition(context.Background(), runtimeRunTransitionSourceStartupRecovery, interrupted[0], "", runtimeRunStatusInterrupted, "runtime startup recovery interrupted unfinished turn")
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceStartupRecovery || transitions[0].TurnID != interrupted[0].ID || transitions[0].ToStatus != runtimeRunStatusInterrupted {
		t.Fatalf("startup recovery transition = %#v", transitions)
	}
	if transitions[0].CreatedAt != interrupted[0].FinishedAt {
		t.Fatalf("startup recovery transition was not recorded from terminal turn evidence: transition=%#v turn=%#v", transitions[0], interrupted[0])
	}

	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Run.ID != run.ID || projection.Run.Status != runtimeRunStatusInterrupted {
		t.Fatalf("run projection = %#v original = %#v", projection.Run, run)
	}
	if projection.Run.Diagnostics.TerminalPermissionCounts.Pending != 0 {
		t.Fatalf("run projection restored pending permission actionability: %#v", projection.Run.Diagnostics.TerminalPermissionCounts)
	}
	detail, err := service.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != runtimeRunStatusInterrupted || detail.Projection.Status != runtimeRunStatusInterrupted {
		t.Fatalf("run detail after startup recovery = run=%#v projection=%#v", detail.Run, detail.Projection)
	}
	activity, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range activity.ToolCalls {
		if call.Status == "running" || call.Status == "pending" || call.Status == "waiting_permission" {
			t.Fatalf("stale tool call restored through activity: %#v", activity.ToolCalls)
		}
	}
	recovery, err := service.RecoveryStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingPermissions) != 0 || len(recovery.PendingMCPRequests) != 0 {
		t.Fatalf("stale actionability remained after recovery: permissions=%#v mcp=%#v", recovery.PendingPermissions, recovery.PendingMCPRequests)
	}
}

func newRuntimeRunProjectionFixture(t *testing.T) (*runtimeService, string, string) {
	t.Helper()

	runtimeBackend, workspace := backendForSkillTest(t)
	conn, err := db.Connect(context.Background(), workspace.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(workspace.DataDir)
	})
	service := newRuntimeService()
	service.runtime = runtimeBackend
	service.workspace = &workspace
	service.turns = newRuntimeTurnStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "run-projection")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "old run turn"}}})
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(workspace.Path, "tmp", "runtime-dev", "run-projection.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("phase 7.1 run projection artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "write " + artifactPath}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: "turn-run-old", SessionID: sess.ID, Status: turnStatusCompleted, UserMessageID: oldMessage.ID, PromptPreview: "old run turn", StartedAt: now.Add(-2 * time.Minute).UnixMilli(), FinishedAt: now.Add(-90 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: "turn-run-new", SessionID: sess.ID, Status: turnStatusInterrupted, UserMessageID: newMessage.ID, PromptPreview: "write " + artifactPath, StartedAt: now.Add(-30 * time.Second).UnixMilli(), FinishedAt: now.Add(-5 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{ID: "tool-run-old", SessionID: sess.ID, TurnID: "turn-run-old", Name: "read", Source: scheduler.ToolSourceBuiltin}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{ToolCallID: "tool-run-old", Status: scheduler.ToolCallCompleted, OutputSummary: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{ID: "tool-run-new", SessionID: sess.ID, TurnID: "turn-run-new", Name: "write", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"` + artifactPath + `"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{ToolCallID: "tool-run-new", Status: scheduler.ToolCallCompleted, OutputSummary: "wrote", ArtifactRefs: []string{artifactPath}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{ID: "perm-run-old", SessionID: sess.ID, TurnID: "turn-run-old", ToolCallID: "tool-run-old", ToolName: "read", Action: "read", Status: permissionStatusAllowedOnce, CreatedAt: now.Add(-110 * time.Second).UnixMilli(), DecidedAt: now.Add(-100 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{ID: "perm-run-new", SessionID: sess.ID, TurnID: "turn-run-new", ToolCallID: "tool-run-new", ToolName: "write", Action: "write", Status: permissionStatusCancelled, CreatedAt: now.Add(-20 * time.Second).UnixMilli(), DecidedAt: now.Add(-4 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{ID: "task-run-1", ParentSessionID: sess.ID, ParentTurnID: "turn-run-new", ParentToolCallID: "tool-run-new", ChildSessionID: "session-child-run", Title: "subagent verify", Status: agentTaskStatusCompleted, Progress: 100, ResultSummary: "verified", ArtifactRefs: []string{artifactPath}, StartedAt: now.Add(-25 * time.Second).UnixMilli(), FinishedAt: now.Add(-2 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(RuntimeEvent{Type: "turn.started", SessionID: sess.ID, TurnID: "turn-run-old", CreatedAt: now.Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)})
	service.storeRuntimeEvent(RuntimeEvent{Type: "tool.call.completed", SessionID: sess.ID, TurnID: "turn-run-new", ToolCallID: "tool-run-new", CreatedAt: now.Add(-3 * time.Second).UTC().Format(time.RFC3339Nano)})

	return service, sess.ID, artifactPath
}

func mustSessionActivity(t *testing.T, service *runtimeService, sessionID string) RuntimeSessionActivityResponse {
	t.Helper()
	activity, err := service.SessionActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return activity
}

func findRuntimeRunCheckpoint(checkpoints []RuntimeRunCheckpoint, checkpointID string) RuntimeRunCheckpoint {
	for _, checkpoint := range checkpoints {
		if checkpoint.ID == checkpointID {
			return checkpoint
		}
	}
	return RuntimeRunCheckpoint{}
}

func findRuntimeRun(runs []RuntimeRun, runID string) RuntimeRun {
	for _, run := range runs {
		if run.ID == runID {
			return run
		}
	}
	return RuntimeRun{}
}
