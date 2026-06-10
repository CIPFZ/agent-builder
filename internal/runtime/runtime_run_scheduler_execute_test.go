package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/runtimeapi"
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
	if !resp.Accepted || resp.ExecutionStarted || resp.Reason != runtimeRunSchedulerExecuteTaskReasonAlreadyRunning {
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

func TestRuntimeRunSchedulerExecuteTaskStartsQueuedTaskOnce(t *testing.T) {
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
	if !first.Accepted || !first.ExecutionStarted || first.Reason != runtimeRunSchedulerExecuteTaskReasonForegroundExecutionStarted {
		t.Fatalf("execute responses = %#v / %#v", first, second)
	}
	if !second.Accepted || second.ExecutionStarted || second.Reason != runtimeRunSchedulerExecuteTaskReasonAlreadyRunning {
		t.Fatalf("duplicate execute response = %#v", second)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != agentTaskStatusRunning || refreshed.Progress != 10 || refreshed.StartedAt != task.StartedAt || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("task mutated by duplicate execute contract calls = %#v", refreshed)
	}
	messages, err := newRuntimeAgentTaskMessageStore(service.turns.db).ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Kind != taskMessageKindInstruction || messages[0].Status != taskMessageStatusProcessed {
		t.Fatalf("task start messages = %#v", messages)
	}
	if _, err := newRuntimeAgentTaskResultStore(service.turns.db).Get(context.Background(), task.ID); err == nil {
		t.Fatal("task execute start created a result before completion")
	}
	refs, err := service.Refs(context.Background(), RuntimeRefListRequest{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Refs) != 0 {
		t.Fatalf("task execute start created artifact refs: %#v", refs.Refs)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startEvents := 0
	for _, event := range events.Events {
		if event.Type == runtimeapi.EventTaskStarted && event.Payload["task_id"] == task.ID {
			startEvents++
		}
	}
	if startEvents != 1 {
		t.Fatalf("task started events = %d events=%#v", startEvents, events.Events)
	}
	transitions, err := service.transitions.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceTaskStarted || transitions[0].TaskID != task.ID {
		t.Fatalf("task start transitions = %#v", transitions)
	}
}

func TestRuntimeRunSchedulerExecuteTaskInvokesForegroundRunnerAndUsesCompletionEvidence(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-execute-runner-complete",
		ParentSessionID:  "session-1",
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Title:            "Review output",
		Kind:             agentTaskKindSubagent,
		Role:             "reviewer",
		Name:             "agent",
		PromptSummary:    "inspect child output",
		Provider:         "provider-1",
		Model:            "model-1",
		AllowedTools:     []string{"view", "grep"},
		CapabilityScope:  []string{"C:/work/project"},
		CWD:              "C:/work/project",
		Worktree:         "worktree-1",
		Status:           agentTaskStatusQueued,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRuntimeAgentTaskRunner{}
	service.agentTaskRunner = runner
	runner.run = func(ctx context.Context, req RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error) {
		if !req.StartAlreadyRecorded || !req.BackendOnly || !req.EventPayloadRefreshOnly {
			t.Fatalf("runner request source flags = %#v", req)
		}
		if req.RunID != run.ID || req.TaskID != task.ID || req.ParentTurnID != turn.ID || req.ChildSessionID != task.ChildSessionID || req.Worktree != task.Worktree {
			t.Fatalf("runner request ownership/scope = %#v", req)
		}
		recorder := runtimeSchedulerRecorder{service: service}
		err := recorder.AgentTaskCompleted(ctx, agent.AgentTaskRecord{
			ID:               req.TaskID,
			ParentTurnID:     req.ParentTurnID,
			ParentSessionID:  req.ParentSessionID,
			ParentToolCallID: req.ParentToolCallID,
			ChildSessionID:   req.ChildSessionID,
			Title:            req.Title,
			Kind:             req.Kind,
			Role:             req.Role,
			Name:             req.Name,
			PromptSummary:    req.PromptSummary,
			Model:            req.Model,
			Provider:         req.Provider,
			AllowedTools:     req.AllowedTools,
			CapabilityScope:  req.CapabilityScope,
			CWD:              req.CWD,
			Worktree:         req.Worktree,
			Status:           agentTaskStatusCompleted,
			Progress:         100,
			ResultSummary:    "runner completed",
			ArtifactRefs:     []string{"artifact:file:runner-result.txt"},
			StartedAt:        req.StartedAt,
			FinishedAt:       time.Now().UnixMilli(),
		})
		return RuntimeAgentTaskExecutionResult{
			TaskID:             req.TaskID,
			Status:             agentTaskStatusCompleted,
			Terminal:           true,
			RefreshTargets:     runtimeRunSchedulerRefreshTargets(),
			ArtifactRefs:       []string{"artifact:file:runner-result.txt"},
			ResultSummary:      "runner completed",
			NoStaleResume:      true,
			CompletionOnlyRefs: true,
		}, err
	}

	resp, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Accepted || !resp.ExecutionStarted || resp.Task.Status != agentTaskStatusCompleted || resp.Task.ResultSummary != "runner completed" {
		t.Fatalf("execute response = %#v", resp)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
	messages, err := newRuntimeAgentTaskMessageStore(service.turns.db).ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	startMessages := 0
	resultMessages := 0
	for _, msg := range messages {
		switch msg.Kind {
		case taskMessageKindInstruction:
			startMessages++
		case taskMessageKindResult:
			resultMessages++
		}
	}
	if startMessages != 1 || resultMessages != 1 {
		t.Fatalf("task messages start=%d result=%d messages=%#v", startMessages, resultMessages, messages)
	}
	result, err := newRuntimeAgentTaskResultStore(service.turns.db).Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agentTaskStatusCompleted || len(result.ArtifactRefs) != 1 || !strings.HasPrefix(result.ArtifactRefs[0], "runtime://refs/") {
		t.Fatalf("task result = %#v", result)
	}
	refs, err := service.Refs(context.Background(), RuntimeRefListRequest{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Refs) != 1 {
		t.Fatalf("completion refs = %#v", refs.Refs)
	}
	second, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: task.ID})
	if err == nil || !strings.Contains(err.Error(), runtimeRunSchedulerDelegateReasonTerminalTask) {
		t.Fatalf("duplicate terminal execute err=%v resp=%#v", err, second)
	}
	if runner.calls != 1 {
		t.Fatalf("duplicate terminal execute reran runner calls=%d", runner.calls)
	}
}

func TestRuntimeRunSchedulerExecuteTaskRunnerCancellationDoesNotCreateArtifactEvidence(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-execute-runner-cancelled",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRuntimeAgentTaskRunner{}
	service.agentTaskRunner = runner
	runner.run = func(ctx context.Context, req RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error) {
		recorder := runtimeSchedulerRecorder{service: service}
		err := recorder.AgentTaskFailed(ctx, agent.AgentTaskRecord{
			ID:              req.TaskID,
			ParentTurnID:    req.ParentTurnID,
			ParentSessionID: req.ParentSessionID,
			ChildSessionID:  req.ChildSessionID,
			Status:          agentTaskStatusCancelled,
			Progress:        100,
			ResultSummary:   "partial output must not become an artifact",
			ArtifactRefs:    nil,
			StartedAt:       req.StartedAt,
			FinishedAt:      time.Now().UnixMilli(),
			Error:           "cancelled by foreground context",
		})
		return RuntimeAgentTaskExecutionResult{
			TaskID:             req.TaskID,
			Status:             agentTaskStatusCancelled,
			Terminal:           true,
			ResultSummary:      "partial output must not become an artifact",
			Error:              "cancelled by foreground context",
			NoStaleResume:      true,
			CompletionOnlyRefs: true,
		}, err
	}

	resp, err := service.runtimeRunSchedulerExecuteTask(context.Background(), RuntimeRunSchedulerExecuteTaskRequest{RunID: run.ID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Task.Status != agentTaskStatusCancelled || resp.Task.Progress != 100 || len(resp.Task.ArtifactRefs) != 0 {
		t.Fatalf("cancelled runner response = %#v", resp)
	}
	result, err := newRuntimeAgentTaskResultStore(service.turns.db).Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agentTaskStatusCancelled || len(result.ArtifactRefs) != 0 {
		t.Fatalf("cancelled result created artifacts = %#v", result)
	}
	refs, err := service.Refs(context.Background(), RuntimeRefListRequest{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Refs) != 0 {
		t.Fatalf("cancelled runner created refs = %#v", refs.Refs)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
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

type recordingRuntimeAgentTaskRunner struct {
	calls int
	run   func(context.Context, RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error)
}

func (r *recordingRuntimeAgentTaskRunner) ExecuteAgentTask(ctx context.Context, req RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error) {
	r.calls++
	if r.run == nil {
		return RuntimeAgentTaskExecutionResult{
			TaskID:             req.TaskID,
			Status:             agentTaskStatusRunning,
			RefreshTargets:     runtimeRunSchedulerRefreshTargets(),
			NoStaleResume:      true,
			CompletionOnlyRefs: true,
		}, nil
	}
	return r.run(ctx, req)
}
