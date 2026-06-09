package runtime

import (
	"context"
	"testing"
)

func TestRuntimeRunSchedulerPlanBuildsReadOnlyExecutableTurnItem(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	resp, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:     run.ID,
		SessionID: "session-1",
		TurnID:    turn.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Source.Kind != runtimeRunSchedulerPlanSourceKind || !resp.Source.ReadOnly || resp.Source.StartsWorker || !resp.Source.SessionActivityParity {
		t.Fatalf("source = %#v", resp.Source)
	}
	if resp.Plan.RunID != run.ID || resp.Plan.StatusFromRunDetail != runtimeRunStatusActive || len(resp.Plan.Items) != 1 {
		t.Fatalf("plan = %#v", resp.Plan)
	}
	item := resp.Plan.Items[0]
	if item.Kind != runtimeRunSchedulerPlanModeUserTurn || item.TurnID != turn.ID || !item.RequiredPreflight || !item.CanSchedule || item.PreflightReason != "" {
		t.Fatalf("item = %#v", item)
	}
	if item.CancellationScope == "" || item.DiagnosticsRoute == "" || len(item.RefreshTargets) == 0 {
		t.Fatalf("item routing = %#v", item)
	}
}

func TestRuntimeRunSchedulerPlanKeepsUnlinkedOrTerminalTurnNonExecutable(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	unlinked, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-unlinked",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	unlinkedPlan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{RunID: run.ID, TurnID: unlinked.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(unlinkedPlan.Plan.Items) != 1 || unlinkedPlan.Plan.Items[0].CanSchedule || unlinkedPlan.Plan.Items[0].PreflightReason != runtimeRunSchedulerPreflightReasonMissingTurnLink {
		t.Fatalf("unlinked plan = %#v", unlinkedPlan.Plan.Items)
	}

	_, terminal := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusCompleted)
	terminalPlan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{RunID: run.ID, TurnID: terminal.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(terminalPlan.Plan.Items) != 1 || terminalPlan.Plan.Items[0].CanSchedule || terminalPlan.Plan.Items[0].PreflightReason != runtimeRunSchedulerPreflightReasonTerminalTurn {
		t.Fatalf("terminal plan = %#v", terminalPlan.Plan.Items)
	}
}

func TestRuntimeRunSchedulerPlanCheckpointItemDoesNotMutateEvidence(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.Upsert(context.Background(), RuntimeRun{
		ID:               "run-checkpoint-plan",
		WorkspaceID:      "workspace-1",
		PrimarySessionID: "session-1",
		SessionIDs:       []string{"session-1"},
		Objective:        "resume work",
		Status:           runtimeRunStatusInterrupted,
		Source:           runtimeRunSourceUserPrompt,
		Checkpoints: []RuntimeRunCheckpoint{{
			ID:           "turn:turn-source:interrupted",
			TurnID:       "turn-source",
			Status:       turnStatusInterrupted,
			Summary:      "runtime restarted",
			ArtifactRefs: []string{"artifact://report"},
			CreatedAt:    2000,
		}},
		CreatedAt: 1000,
		UpdatedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:        run.ID,
		CheckpointID: "turn:turn-source:interrupted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Plan.Items) != 1 {
		t.Fatalf("checkpoint plan = %#v", resp.Plan)
	}
	item := resp.Plan.Items[0]
	if item.Kind != runtimeRunSchedulerPlanModeCheckpointResume || item.CanSchedule || item.PreflightReason != runtimeRunSchedulerPlanReasonCheckpointRequiresTurn {
		t.Fatalf("checkpoint item = %#v", item)
	}
	refreshed, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := refreshed.Checkpoints[0]
	if checkpoint.AcknowledgedAt != 0 || checkpoint.DiscardedAt != 0 || len(checkpoint.ResumedTurnIDs) != 0 || checkpoint.ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("checkpoint evidence mutated = %#v", checkpoint)
	}
}

func TestRuntimeRunSchedulerPlanTaskItemRequiresParentRunTurnOwnership(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-parent",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-unowned",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-parent",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:  run.ID,
		TaskID: "task-unowned",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) != 1 {
		t.Fatalf("task plan = %#v", plan.Plan)
	}
	item := plan.Plan.Items[0]
	if item.CanSchedule || item.OwnershipVerified || item.PreflightReason != runtimeRunSchedulerPreflightReasonMissingTurnLink {
		t.Fatalf("unowned task item = %#v", item)
	}
}

func TestRuntimeRunSchedulerPlanTaskItemPreservesScopeAndStaysNonExecutable(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-scoped",
		ParentSessionID:  "session-1",
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Status:           agentTaskStatusQueued,
		Role:             "reviewer",
		Provider:         "provider-1",
		Model:            "model-1",
		AllowedTools:     []string{"view", "grep"},
		CapabilityScope:  []string{"C:/work/project"},
		CWD:              "C:/work/project",
		Worktree:         "worktree-1",
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:  run.ID,
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) != 1 {
		t.Fatalf("task plan = %#v", plan.Plan)
	}
	item := plan.Plan.Items[0]
	if item.CanSchedule || !item.OwnershipVerified || item.PreflightReason != runtimeRunSchedulerPlanReasonTaskSchedulerNotReady {
		t.Fatalf("owned task item = %#v", item)
	}
	scope := item.TaskScope
	if scope.Role != task.Role || scope.Provider != task.Provider || scope.Model != task.Model || scope.CWD != task.CWD || scope.Worktree != task.Worktree || scope.ParentToolCallID != task.ParentToolCallID || scope.ChildSessionID != task.ChildSessionID {
		t.Fatalf("task scope = %#v task = %#v", scope, task)
	}
	if len(scope.AllowedTools) != 2 || scope.AllowedTools[0] != "view" || scope.AllowedTools[1] != "grep" {
		t.Fatalf("allowed tools widened or reordered = %#v", scope.AllowedTools)
	}
	if len(scope.CapabilityScope) != 1 || scope.CapabilityScope[0] != "C:/work/project" {
		t.Fatalf("capability scope = %#v", scope.CapabilityScope)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != agentTaskStatusQueued || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("task mutated = %#v", refreshed)
	}
}

func runtimeRunSchedulerPlanLinkedTurnFixture(t *testing.T, service *runtimeService, status string) (RuntimeRun, RuntimeTurn) {
	t.Helper()
	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{
		ID:        "turn-" + status,
		SessionID: "session-1",
		Status:    status,
		StartedAt: 1000,
	}
	if isFinalTurnStatus(status) {
		turn.FinishedAt = 2000
	}
	turn, err = service.turns.Upsert(context.Background(), turn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	return run, turn
}
