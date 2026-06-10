package runtime

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRuntimeSchedulerUICandidateSeedExposesDurableHTTPPlanAndExecute(t *testing.T) {
	t.Parallel()

	service, release := runtimeRunTransitionWriterTestService(t)
	defer release()

	run, turn := runtimeRunSchedulerPlanLinkedTurnFixture(t, service, turnStatusQueued)
	queued, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-ui-queued",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child-queued",
		PromptSummary:   "queued task prompt",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-ui-terminal",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child-terminal",
		PromptSummary:   "terminal task prompt",
		Status:          agentTaskStatusCompleted,
		Progress:        100,
		StartedAt:       1200,
		FinishedAt:      1300,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := newRuntimeHTTPServer(service)
	client := runtimeSmokeClient{server: server, token: server.Token()}

	storedRun, err := service.runs.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.ID != run.ID || storedRun.PrimarySessionID != "session-1" {
		t.Fatalf("stored run = %#v", storedRun)
	}

	queuedPlan := schedulerUITestRequest[RuntimeRunSchedulerPlanResponse](t, client, http.MethodGet, "/v1/run-scheduler-plan?run_id="+run.ID+"&task_id="+queued.ID, "")
	if len(queuedPlan.Plan.Items) != 1 || !queuedPlan.Plan.Items[0].CanSchedule || !queuedPlan.Plan.Items[0].OwnershipVerified {
		t.Fatalf("queued plan = %#v", queuedPlan.Plan.Items)
	}
	terminalPlan := schedulerUITestRequest[RuntimeRunSchedulerPlanResponse](t, client, http.MethodGet, "/v1/run-scheduler-plan?run_id="+run.ID+"&task_id="+terminal.ID, "")
	if len(terminalPlan.Plan.Items) != 1 || terminalPlan.Plan.Items[0].CanSchedule || terminalPlan.Plan.Items[0].PreflightReason != runtimeRunSchedulerPlanReasonTerminalTask {
		t.Fatalf("terminal plan = %#v", terminalPlan.Plan.Items)
	}

	execute := schedulerUITestRequest[RuntimeRunSchedulerExecuteTaskResponse](t, client, http.MethodPost, "/v1/runs/"+run.ID+"/tasks/"+queued.ID+"/execute", "")
	if !execute.Accepted || !execute.ExecutionStarted || execute.Source.StartsWorker || !execute.Source.IdempotentByTaskID {
		t.Fatalf("execute response = %#v", execute)
	}
	refreshed := schedulerUITestRequest[RuntimeRunSchedulerPlanResponse](t, client, http.MethodGet, "/v1/run-scheduler-plan?run_id="+run.ID+"&task_id="+queued.ID, "")
	if len(refreshed.Plan.Items) != 1 || !refreshed.Plan.Items[0].CanSchedule || !refreshed.Plan.Items[0].OwnershipVerified {
		t.Fatalf("refreshed queued plan = %#v", refreshed.Plan.Items)
	}
	task, err := service.agentTasks.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != agentTaskStatusRunning || task.Progress != 10 || len(task.ArtifactRefs) != 0 {
		t.Fatalf("queued task after execute = %#v", task)
	}
}

func schedulerUITestRequest[T any](t *testing.T, client runtimeSmokeClient, method, path, body string) T {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var target T
	client.doJSON(t, req, &target)
	return target
}
