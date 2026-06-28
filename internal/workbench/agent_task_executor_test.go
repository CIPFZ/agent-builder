package workbench

import (
	"context"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/app"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func TestWorkbenchExecuteStartedAgentTaskRoutesToWorkspaceCoordinator(t *testing.T) {
	t.Parallel()

	workbenchService := New(context.Background(), nil, nil)
	coord := &recordingStartedTaskCoordinator{}
	workbenchService.workspaces.Set("workspace-1", &Workspace{App: &app.App{AgentCoordinator: coord}})

	result, err := workbenchService.ExecuteStartedAgentTask(context.Background(), "workspace-1", agent.StartedAgentTaskExecutionRequest{
		TaskID:               "task-1",
		ParentSessionID:      "session-parent",
		ParentTurnID:         "turn-1",
		ChildSessionID:       "session-child",
		Role:                 config.AgentTask,
		Prompt:               "prompt",
		StartAlreadyRecorded: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if coord.calls != 1 || coord.last.TaskID != "task-1" || coord.last.Prompt != "prompt" {
		t.Fatalf("coordinator call count=%d last=%#v", coord.calls, coord.last)
	}
	if result.TaskID != "task-1" || result.Status != "completed" || !result.Terminal {
		t.Fatalf("result = %#v", result)
	}
}

func TestWorkbenchExecuteStartedAgentTaskRequiresWorkspaceCoordinator(t *testing.T) {
	t.Parallel()

	workbenchService := New(context.Background(), nil, nil)
	workbenchService.workspaces.Set("workspace-1", &Workspace{App: &app.App{}})

	_, err := workbenchService.ExecuteStartedAgentTask(context.Background(), "workspace-1", agent.StartedAgentTaskExecutionRequest{TaskID: "task-1"})
	if !errors.Is(err, ErrAgentNotInitialized) {
		t.Fatalf("err = %v", err)
	}
	_, err = workbenchService.ExecuteStartedAgentTask(context.Background(), "missing-workspace", agent.StartedAgentTaskExecutionRequest{TaskID: "task-1"})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("missing workspace err = %v", err)
	}
}

type recordingStartedTaskCoordinator struct {
	calls int
	last  agent.StartedAgentTaskExecutionRequest
}

func (c *recordingStartedTaskCoordinator) Run(context.Context, string, string, string, ...message.Attachment) (*fantasy.AgentResult, error) {
	return nil, nil
}
func (c *recordingStartedTaskCoordinator) RunWithMetadata(ctx context.Context, sessionID, turnID, prompt string, _ map[string]string, attachments ...message.Attachment) (*fantasy.AgentResult, error) {
	return c.Run(ctx, sessionID, turnID, prompt, attachments...)
}
func (c *recordingStartedTaskCoordinator) Cancel(string) {}
func (c *recordingStartedTaskCoordinator) SendToSession(context.Context, string, string, string) error {
	return nil
}
func (c *recordingStartedTaskCoordinator) CancelAll()                              {}
func (c *recordingStartedTaskCoordinator) IsSessionBusy(string) bool               { return false }
func (c *recordingStartedTaskCoordinator) IsBusy() bool                            { return false }
func (c *recordingStartedTaskCoordinator) QueuedPrompts(string) int                { return 0 }
func (c *recordingStartedTaskCoordinator) QueuedPromptsList(string) []string       { return nil }
func (c *recordingStartedTaskCoordinator) ClearQueue(string)                       {}
func (c *recordingStartedTaskCoordinator) Summarize(context.Context, string) error { return nil }
func (c *recordingStartedTaskCoordinator) Model() agent.Model                      { return agent.Model{} }
func (c *recordingStartedTaskCoordinator) UpdateModels(context.Context) error      { return nil }
func (c *recordingStartedTaskCoordinator) RefreshSkills(context.Context) error     { return nil }
func (c *recordingStartedTaskCoordinator) ExecuteConfiguredStartedAgentTask(_ context.Context, req agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error) {
	c.calls++
	c.last = req
	return agent.StartedAgentTaskExecutionResult{
		TaskID:             req.TaskID,
		Status:             "completed",
		Terminal:           true,
		ResultSummary:      "done",
		NoStaleResume:      true,
		CompletionOnlyRefs: true,
	}, nil
}
