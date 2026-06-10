package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestRuntimeRunSchedulerExecuteTaskAcceptsOwnedActiveCandidateWithoutStartingWorker(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-execute-owned",
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

	resp, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{
		RunID:  run.ID,
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Accepted || resp.ExecutionStarted || resp.Reason != runtimeRunSchedulerExecuteTaskReasonAcceptedPendingImplementation {
		t.Fatalf("execute response = %#v", resp)
	}
	if resp.Source.Kind != runtimeRunSchedulerExecuteTaskSourceKind || !resp.Source.BackendOnly || resp.Source.StartsWorker || !resp.Source.IdempotentByTaskID || !resp.Source.SessionActivityParity {
		t.Fatalf("execute source = %#v", resp.Source)
	}
	if resp.Task.ID != task.ID || len(resp.Plan.Plan.Items) != 1 || !resp.Plan.Plan.Items[0].CanSchedule || !resp.Plan.Plan.Items[0].OwnershipVerified {
		t.Fatalf("execute plan/task = %#v / %#v", resp.Plan, resp.Task)
	}
	if len(resp.RefreshTargets) == 0 {
		t.Fatalf("refresh targets missing = %#v", resp)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, task.ID)
}

func TestRuntimeRunSchedulerExecuteTaskIsIdempotentBeforeExecutionImplementation(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-execute-idempotent",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Accepted || !second.Accepted || first.ExecutionStarted || second.ExecutionStarted {
		t.Fatalf("execute responses = %#v / %#v", first, second)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != task.Status || refreshed.StartedAt != task.StartedAt || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("task mutated by duplicate execute contract calls = %#v", refreshed)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, task.ID)
}

func TestRuntimeRunSchedulerExecuteTaskRejectsInvalidCandidatesWithoutSideEffects(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "write report", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	unlinkedTurn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-execute-unlinked",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	unowned, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-execute-unowned",
		ParentSessionID: "session-1",
		ParentTurnID:    unlinkedTurn.ID,
		Status:          agentTaskStatusRunning,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: unowned.ID})
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerPreflightReasonMissingTurnLink) {
		t.Fatalf("unowned execute error = %v response = %#v", err, resp)
	}
	if resp.Accepted || resp.ExecutionStarted || resp.Source.StartsWorker {
		t.Fatalf("unowned execute response = %#v", resp)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, unowned.ID)

	_, linkedTurn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	terminal, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-execute-terminal",
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
	resp, err = service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: terminal.ID})
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerDelegateReasonTerminalTask) {
		t.Fatalf("terminal execute error = %v response = %#v", err, resp)
	}
	if resp.Accepted || resp.ExecutionStarted || resp.Source.StartsWorker {
		t.Fatalf("terminal execute response = %#v", resp)
	}
	refreshedTerminal, err := service.agentTasks.Get(context.Background(), terminal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedTerminal.Status != terminal.Status || refreshedTerminal.FinishedAt != terminal.FinishedAt || len(refreshedTerminal.ArtifactRefs) != 1 {
		t.Fatalf("terminal task mutated = %#v", refreshedTerminal)
	}
	assertTaskDelegateNoSideEffects(t, service, run.ID, terminal.ID)
}
