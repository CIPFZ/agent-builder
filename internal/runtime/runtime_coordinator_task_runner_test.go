package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/workbench"
)

func TestRuntimeCoordinatorTaskRunnerUsesDurableInstructionPrompt(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-coordinator-runner",
		ParentSessionID:  "session-1",
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Title:            "Review output",
		Kind:             agentTaskKindSubagent,
		Role:             config.AgentTask,
		Name:             "agent",
		PromptSummary:    "summary only",
		Status:           agentTaskStatusRunning,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.createAgentTaskMessage(context.Background(), task, RuntimeAgentTaskMessage{
		Direction:      taskMessageDirectionParentToChild,
		Kind:           taskMessageKindInstruction,
		Status:         taskMessageStatusProcessed,
		ContentSummary: "summary only",
		Payload: map[string]any{
			"prompt":        "durable full prompt",
			"prompt_source": "runtime_task_instruction",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingStartedAgentTaskExecutor{}
	runner := runtimeCoordinatorTaskRunner{service: service, executor: executor}

	result, err := runner.ExecuteAgentTask(context.Background(), runtimeAgentTaskExecutionRequest(run, task, ""))
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != task.ID || result.Status != agentTaskStatusCompleted || !result.Terminal || !result.NoStaleResume || !result.CompletionOnlyRefs {
		t.Fatalf("runner result = %#v", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d", executor.calls)
	}
	got := executor.last
	if got.TaskID != task.ID || got.ParentTurnID != turn.ID || got.ChildSessionID != task.ChildSessionID || got.Prompt != "durable full prompt" {
		t.Fatalf("executor request = %#v", got)
	}
	if !got.StartAlreadyRecorded || !got.WorkbenchOnly || !got.EventPayloadRefreshOnly {
		t.Fatalf("executor source flags = %#v", got)
	}
}

func TestRuntimeCoordinatorTaskRunnerUnsupportedRoleFailsTerminally(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-coordinator-unsupported",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child",
		Role:            "reviewer",
		Status:          agentTaskStatusRunning,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingStartedAgentTaskExecutor{}
	runner := runtimeCoordinatorTaskRunner{service: service, executor: executor}

	result, err := runner.ExecuteAgentTask(context.Background(), runtimeAgentTaskExecutionRequest(run, task, "do work"))
	if err == nil || !strings.Contains(err.Error(), runtimeCoordinatorTaskRunnerReasonUnsupportedRole) {
		t.Fatalf("unsupported role err=%v result=%#v", err, result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor called for unsupported role: %d", executor.calls)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != agentTaskStatusFailed || refreshed.Progress != 100 || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("failed task = %#v", refreshed)
	}
	refs, err := service.Objects(context.Background(), RuntimeObjectListRequest{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Objects) != 0 {
		t.Fatalf("unsupported role created refs = %#v", refs.Objects)
	}
}

func TestRuntimeCoordinatorTaskRunnerMissingPromptSourceFailsTerminally(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-coordinator-missing-prompt",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child",
		Role:            config.AgentTask,
		Status:          agentTaskStatusRunning,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingStartedAgentTaskExecutor{}
	runner := runtimeCoordinatorTaskRunner{service: service, executor: executor}

	result, err := runner.ExecuteAgentTask(context.Background(), runtimeAgentTaskExecutionRequest(run, task, ""))
	if err == nil || !strings.Contains(err.Error(), runtimeCoordinatorTaskRunnerReasonMissingPromptSource) {
		t.Fatalf("missing prompt err=%v result=%#v", err, result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor called without prompt source: %d", executor.calls)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != agentTaskStatusFailed || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("missing prompt failed task = %#v", refreshed)
	}
}

func TestRuntimeWorkbenchStartedAgentTaskExecutorRequiresServiceAndWorkspace(t *testing.T) {
	t.Parallel()

	_, err := runtimeWorkbenchStartedAgentTaskExecutor{}.ExecuteStartedAgentTask(context.Background(), agent.StartedAgentTaskExecutionRequest{TaskID: "task-1"})
	if err == nil || !strings.Contains(err.Error(), "runtime workbench is not available") {
		t.Fatalf("nil workbench err = %v", err)
	}

	_, err = runtimeWorkbenchStartedAgentTaskExecutor{workbench: workbench.New(context.Background(), nil, nil), workspaceID: " "}.ExecuteStartedAgentTask(context.Background(), agent.StartedAgentTaskExecutionRequest{TaskID: "task-1"})
	if err == nil || !strings.Contains(err.Error(), "runtime workspace id is not available") {
		t.Fatalf("empty workspace err = %v", err)
	}
}

func TestRuntimeServiceInstallsWorkbenchAgentTaskRunner(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtimeWorkbench := workbench.New(context.Background(), nil, nil)

	service.installWorkbenchAgentTaskRunner(runtimeWorkbench, "workspace-1")

	runner, ok := service.agentTaskRunner.(runtimeCoordinatorTaskRunner)
	if !ok {
		t.Fatalf("runner type = %T", service.agentTaskRunner)
	}
	if runner.service != service {
		t.Fatalf("runner service was not installed")
	}
	executor, ok := runner.executor.(runtimeWorkbenchStartedAgentTaskExecutor)
	if !ok {
		t.Fatalf("executor type = %T", runner.executor)
	}
	if executor.workbench != runtimeWorkbench || executor.workspaceID != "workspace-1" {
		t.Fatalf("executor = %#v", executor)
	}
}

func TestRuntimeCoordinatorTaskRunnerExecutorErrorFailsStartedTaskTerminally(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-coordinator-executor-error",
		ParentSessionID:  "session-1",
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Role:             config.AgentTask,
		Status:           agentTaskStatusRunning,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := runtimeCoordinatorTaskRunner{
		service: service,
		executor: &recordingStartedAgentTaskExecutor{
			err: errors.New("workspace coordinator is not available"),
		},
	}

	result, err := runner.ExecuteAgentTask(context.Background(), runtimeAgentTaskExecutionRequest(run, task, "do work"))
	if err == nil || !strings.Contains(err.Error(), "workspace coordinator is not available") {
		t.Fatalf("executor err=%v result=%#v", err, result)
	}
	if !result.Terminal || result.Status != agentTaskStatusFailed || !result.NoStaleResume || !result.CompletionOnlyRefs {
		t.Fatalf("runner result = %#v", result)
	}
	refreshed, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != agentTaskStatusFailed || refreshed.Progress != 100 || refreshed.Error != "workspace coordinator is not available" || len(refreshed.ArtifactRefs) != 0 {
		t.Fatalf("failed task = %#v", refreshed)
	}
	refs, err := service.Objects(context.Background(), RuntimeObjectListRequest{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Objects) != 0 {
		t.Fatalf("executor error created refs = %#v", refs.Objects)
	}
}

type recordingStartedAgentTaskExecutor struct {
	calls int
	last  agent.StartedAgentTaskExecutionRequest
	err   error
}

func (r *recordingStartedAgentTaskExecutor) ExecuteStartedAgentTask(_ context.Context, req agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error) {
	r.calls++
	r.last = req
	if r.err != nil {
		return agent.StartedAgentTaskExecutionResult{}, r.err
	}
	return agent.StartedAgentTaskExecutionResult{
		TaskID:             req.TaskID,
		Status:             agentTaskStatusCompleted,
		Terminal:           true,
		ResultSummary:      "completed",
		NoStaleResume:      true,
		CompletionOnlyRefs: true,
	}, nil
}
