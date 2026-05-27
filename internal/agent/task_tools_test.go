package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
)

type fakeAgentTaskToolRuntime struct {
	sent AgentTaskToolSendMessageRequest
}

func (f *fakeAgentTaskToolRuntime) ListAgentTasksForTool(context.Context, AgentTaskToolListRequest) (AgentTaskToolListResponse, error) {
	return AgentTaskToolListResponse{Tasks: []AgentTaskToolTask{{ID: "task-1", Status: "running"}}}, nil
}
func (f *fakeAgentTaskToolRuntime) GetAgentTaskForTool(context.Context, AgentTaskToolGetRequest) (AgentTaskToolGetResponse, error) {
	return AgentTaskToolGetResponse{Task: AgentTaskToolTask{ID: "task-1", Status: "running"}}, nil
}
func (f *fakeAgentTaskToolRuntime) SendAgentTaskMessageForTool(_ context.Context, req AgentTaskToolSendMessageRequest) (AgentTaskToolSendMessageResponse, error) {
	f.sent = req
	return AgentTaskToolSendMessageResponse{Success: true, Message: "sent", TaskID: req.TaskID, Status: "processed", Record: AgentTaskToolMessage{ID: "msg-1", Status: "processed"}}, nil
}
func (f *fakeAgentTaskToolRuntime) StopAgentTaskForTool(context.Context, AgentTaskToolStopRequest) (AgentTaskToolStopResponse, error) {
	return AgentTaskToolStopResponse{Success: true, Message: "stopped", Task: AgentTaskToolTask{ID: "task-1", Status: "cancelled"}}, nil
}
func (f *fakeAgentTaskToolRuntime) GetAgentTaskOutputForTool(context.Context, AgentTaskToolOutputRequest) (AgentTaskToolOutputResponse, error) {
	return AgentTaskToolOutputResponse{TaskID: "task-1", Status: "completed", Summary: "done", OutputRefs: []string{"runtime://refs/ref-1"}}, nil
}
func (f *fakeAgentTaskToolRuntime) AgentTaskStarted(context.Context, AgentTaskRecord) error {
	return nil
}
func (f *fakeAgentTaskToolRuntime) AgentTaskProgress(context.Context, AgentTaskRecord) error {
	return nil
}
func (f *fakeAgentTaskToolRuntime) AgentTaskCompleted(context.Context, AgentTaskRecord) error {
	return nil
}
func (f *fakeAgentTaskToolRuntime) AgentTaskFailed(context.Context, AgentTaskRecord) error {
	return nil
}

func TestTaskRuntimeToolsSendMessage(t *testing.T) {
	t.Parallel()

	runtime := &fakeAgentTaskToolRuntime{}
	c := &coordinator{agentTaskRecorder: runtime}
	built := c.taskRuntimeTools()
	if len(built) != 5 {
		t.Fatalf("tools = %d", len(built))
	}
	var sendTool fantasy.AgentTool
	for _, tool := range built {
		if tool.Info().Name == TaskMessageToolName {
			sendTool = tool
			break
		}
	}
	if sendTool == nil {
		t.Fatal("task_message tool missing")
	}
	ctx := context.WithValue(context.Background(), tools.SessionIDContextKey, "session-1")
	ctx = context.WithValue(ctx, tools.TurnIDContextKey, "turn-1")
	resp, err := sendTool.Run(ctx, fantasy.ToolCall{
		ID:    "tool-1",
		Name:  TaskMessageToolName,
		Input: `{"task_id":"task-1","message":"continue","summary":"next"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsError || !strings.Contains(resp.Content, `"success":true`) {
		t.Fatalf("response = %#v", resp)
	}
	if runtime.sent.TaskID != "task-1" || runtime.sent.Message != "continue" || runtime.sent.RelatedToolCall != "tool-1" || runtime.sent.TurnID != "turn-1" {
		t.Fatalf("sent = %#v", runtime.sent)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil || parsed["status"] != "processed" {
		t.Fatalf("json=%#v err=%v", parsed, err)
	}
}
