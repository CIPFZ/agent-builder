package runtime

import (
	"context"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeSchedulerRecorderPersistsQueuedBeforeRunningAdmission(t *testing.T) {
	service, releaseDB := runtimeRunTransitionWriterTestService(t)
	defer releaseDB()
	recorder := runtimeSchedulerRecorder{service: service}
	call := agent.SchedulerToolCall{
		ID:           "tool-admission",
		SessionID:    "session-admission",
		TurnID:       "turn-admission",
		Name:         "bash",
		Source:       string(scheduler.ToolSourceShell),
		CapabilityID: "shell:bash",
		Command:      "go test ./...",
		InputSummary: `{"command":"go test ./..."}`,
	}

	if err := recorder.ToolCallQueued(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	queued, err := service.toolCalls.GetCall(context.Background(), call.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != scheduler.ToolCallPending {
		t.Fatalf("queued tool status = %s, want pending", queued.Status)
	}
	if len(service.events) == 0 || service.events[len(service.events)-1].Type != runtimeapi.EventToolCallQueued {
		t.Fatalf("queued canonical event = %#v", service.events)
	}

	if err := recorder.ToolCallStarted(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	running, err := service.toolCalls.GetCall(context.Background(), call.ID)
	if err != nil {
		t.Fatal(err)
	}
	if running.Status != scheduler.ToolCallRunning {
		t.Fatalf("admitted tool status = %s, want running", running.Status)
	}
	if service.events[len(service.events)-1].Type != runtimeapi.EventToolCallStarted {
		t.Fatalf("started canonical event = %#v", service.events[len(service.events)-1])
	}
}
