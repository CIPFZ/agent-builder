package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRuntimeRunSchedulerDelegateAllowsLinkedUserTurn(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	plan, err := service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, turn)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Source.StartsWorker || len(plan.Plan.Items) != 1 || !plan.Plan.Items[0].CanSchedule {
		t.Fatalf("delegate plan = %#v", plan)
	}
}

func TestRuntimeRunSchedulerDelegateRejectsAndTerminalizesBeforeStartedTransition(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-rejected",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, turn)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("delegate error = %v", err)
	}
	failed, err := service.failRuntimeRunScheduledTurn(context.Background(), turn, err.Error())
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != turnStatusFailed || failed.FinishedAt == 0 || !strings.Contains(failed.Error, runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("failed turn = %#v", failed)
	}
	service.mu.Lock()
	state := service.requests[failed.ID]
	service.mu.Unlock()
	if !state.Finished || state.Status != "failed" || !strings.Contains(state.Error, runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("request state = %#v", state)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("turn_started transition recorded for rejected delegate: %#v", transitions)
	}
}

func TestRuntimeRunSchedulerDelegateAcceptsExplicitCheckpointResumeTurnOnly(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.Upsert(context.Background(), RuntimeRun{
		ID:               "run-resume-delegate",
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
	checkpointPlan, err := service.runtimeRunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:        run.ID,
		CheckpointID: "turn:turn-source:interrupted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpointPlan.Plan.Items) != 1 || checkpointPlan.Plan.Items[0].CanSchedule || checkpointPlan.Plan.Items[0].PreflightReason != runtimeRunSchedulerPlanReasonCheckpointRequiresTurn {
		t.Fatalf("checkpoint plan = %#v", checkpointPlan.Plan.Items)
	}

	resumed, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-resumed-explicit",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", resumed.ID, resumed.StartedAt); err != nil {
		t.Fatal(err)
	}
	delegatePlan, err := service.runtimeRunSchedulerDelegateUserTurn(context.Background(), run, resumed)
	if err != nil {
		t.Fatal(err)
	}
	if len(delegatePlan.Plan.Items) != 1 || !delegatePlan.Plan.Items[0].CanSchedule {
		t.Fatalf("delegate plan = %#v", delegatePlan.Plan.Items)
	}
	if _, err := service.runs.LinkCheckpointResume(context.Background(), run.ID, "turn:turn-source:interrupted", resumed.ID); err != nil {
		t.Fatal(err)
	}
	service.recordCheckpointResumeTransition(context.Background(), run, run.Checkpoints[0], resumed.ID)
	refreshed, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := refreshed.Checkpoints[0]
	if checkpoint.AcknowledgedAt != 0 || checkpoint.DiscardedAt != 0 || len(checkpoint.ResumedTurnIDs) != 1 || checkpoint.ResumedTurnIDs[0] != resumed.ID || checkpoint.ArtifactRefs[0] != "artifact://report" {
		t.Fatalf("checkpoint evidence = %#v", checkpoint)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceCheckpointResume || transitions[0].TurnID != resumed.ID {
		t.Fatalf("checkpoint resume transitions = %#v", transitions)
	}
}

func TestRuntimeRunSchedulerDelegateTaskTurnRejectsInvalidCandidatesWithoutSideEffects(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	missingPlan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, "task-missing")
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerPlanReasonMissingTask) {
		t.Fatalf("missing task delegate error = %v plan = %#v", err, missingPlan)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, "task-missing")

	unlinkedTurn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-task-unlinked",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	unowned, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-unowned-delegate",
		ParentSessionID: "session-1",
		ParentTurnID:    unlinkedTurn.ID,
		Status:          agentTaskStatusRunning,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	unownedPlan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, unowned.ID)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("unowned task delegate error = %v plan = %#v", err, unownedPlan)
	}
	refreshedUnowned, err := service.agentTasks.Get(context.Background(), unowned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedUnowned.Status != agentTaskStatusRunning || refreshedUnowned.Progress != unowned.Progress {
		t.Fatalf("unowned task mutated = %#v", refreshedUnowned)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, unowned.ID)

	_, linkedTurn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	terminal, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-terminal-delegate",
		ParentSessionID: "session-1",
		ParentTurnID:    linkedTurn.ID,
		Status:          agentTaskStatusCompleted,
		Progress:        100,
		ResultSummary:   "already done",
		ArtifactRefs:    []string{"runtime://refs/task-output"},
		StartedAt:       1200,
		FinishedAt:      1300,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalPlan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, terminal.ID)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerDelegateReasonTerminalTask) {
		t.Fatalf("terminal task delegate error = %v plan = %#v", err, terminalPlan)
	}
	refreshedTerminal, err := service.agentTasks.Get(context.Background(), terminal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedTerminal.Status != terminal.Status || refreshedTerminal.FinishedAt != terminal.FinishedAt || len(refreshedTerminal.ArtifactRefs) != 1 {
		t.Fatalf("terminal task mutated = %#v", refreshedTerminal)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, terminal.ID)

	cancelled, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-cancelled-delegate",
		ParentSessionID: "session-1",
		ParentTurnID:    linkedTurn.ID,
		Status:          agentTaskStatusCancelled,
		Progress:        100,
		Error:           "already cancelled",
		StartedAt:       1400,
		FinishedAt:      1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelledPlan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, cancelled.ID)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerDelegateReasonTerminalTask) || !strings.Contains(err.Error(), agentTaskStatusCancelled) {
		t.Fatalf("cancelled task delegate error = %v plan = %#v", err, cancelledPlan)
	}
	refreshedCancelled, err := service.agentTasks.Get(context.Background(), cancelled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedCancelled.Status != cancelled.Status || refreshedCancelled.FinishedAt != cancelled.FinishedAt || refreshedCancelled.Error != cancelled.Error {
		t.Fatalf("cancelled task mutated = %#v", refreshedCancelled)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, cancelled.ID)

	interrupted, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-interrupted-delegate",
		ParentSessionID: "session-1",
		ParentTurnID:    linkedTurn.ID,
		Status:          agentTaskStatusInterrupted,
		Progress:        100,
		Error:           "runtime interrupted",
		StartedAt:       1600,
		FinishedAt:      1700,
	})
	if err != nil {
		t.Fatal(err)
	}
	interruptedPlan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, interrupted.ID)
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerDelegateReasonTerminalTask) || !strings.Contains(err.Error(), agentTaskStatusInterrupted) {
		t.Fatalf("interrupted task delegate error = %v plan = %#v", err, interruptedPlan)
	}
	refreshedInterrupted, err := service.agentTasks.Get(context.Background(), interrupted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedInterrupted.Status != interrupted.Status || refreshedInterrupted.FinishedAt != interrupted.FinishedAt || refreshedInterrupted.Error != interrupted.Error {
		t.Fatalf("interrupted task mutated = %#v", refreshedInterrupted)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, interrupted.ID)
}

func TestRuntimeRunSchedulerDelegateTaskTurnAllowsOwnedActiveCandidateWithoutSideEffects(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-owned-delegate",
		ParentSessionID:  "session-1",
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Status:           agentTaskStatusRunning,
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
	plan, err := service.runtimeRunSchedulerDelegateTaskTurn(context.Background(), run, task.ID)
	if err != nil {
		t.Fatalf("owned task delegate error = %v plan = %#v", err, plan)
	}
	if len(plan.Plan.Items) != 1 {
		t.Fatalf("task plan = %#v", plan.Plan)
	}
	item := plan.Plan.Items[0]
	if !item.CanSchedule || !item.OwnershipVerified || item.PreflightReason != "" {
		t.Fatalf("owned task item = %#v", item)
	}
	scope := item.TaskScope
	if scope.Role != task.Role || scope.Provider != task.Provider || scope.Model != task.Model || scope.CWD != task.CWD || scope.Worktree != task.Worktree || scope.ParentToolCallID != task.ParentToolCallID || scope.ChildSessionID != task.ChildSessionID {
		t.Fatalf("scope = %#v task = %#v", scope, task)
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
	if refreshed.Status != agentTaskStatusRunning || refreshed.Role != task.Role || refreshed.CWD != task.CWD || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("owned task mutated = %#v", refreshed)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, task.ID)
}

func assertTaskDelegateNoSideEffects(t *testing.T, service *runtimeService, runID, taskID string) {
	t.Helper()
	transitions, err := service.transitions.ListByRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 0 {
		t.Fatalf("task delegate recorded transitions: %#v", transitions)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 0 {
		t.Fatalf("task delegate recorded events: %#v", events.Events)
	}
	if strings.TrimSpace(taskID) == "" {
		return
	}
	messages, err := newRuntimeAgentTaskMessageStore(service.turns.db).ListByTask(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("task delegate recorded messages: %#v", messages)
	}
	if _, err := newRuntimeAgentTaskResultStore(service.turns.db).Get(context.Background(), taskID); err == nil || !errors.Is(err, errRuntimeAgentTaskNotFound) {
		t.Fatalf("task delegate result lookup err = %v", err)
	}
}
