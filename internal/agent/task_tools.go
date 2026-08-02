package agent

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent/tools"
)

const (
	TaskListToolName    = "task_list"
	TaskGetToolName     = "task_get"
	TaskMessageToolName = "task_message"
	TaskStopToolName    = "task_stop"
	TaskOutputToolName  = "task_output"
)

type taskListParams struct {
	TurnID string `json:"turn_id,omitempty" description:"Optional runtime turn id. Defaults to the current turn."`
}

type taskGetParams struct {
	TaskID string `json:"task_id" description:"The runtime AgentTask id to inspect."`
}

type taskMessageParams struct {
	TaskID  string `json:"task_id" description:"The runtime AgentTask id to send a follow-up to."`
	Message string `json:"message" description:"Follow-up instructions for the child agent."`
	Summary string `json:"summary,omitempty" description:"Short summary of the follow-up."`
}

type taskStopParams struct {
	TaskID string `json:"task_id" description:"The runtime AgentTask id to stop."`
	Reason string `json:"reason,omitempty" description:"Reason for the stop request."`
}

type taskOutputParams struct {
	TaskID string `json:"task_id" description:"The runtime AgentTask id whose output should be read."`
}

func (c *coordinator) taskRuntimeTools() []fantasy.AgentTool {
	runtime, ok := c.agentTaskRecorder.(AgentTaskToolRuntime)
	if !ok || runtime == nil {
		return nil
	}
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(TaskListToolName, "List runtime-owned AgentTasks visible to the current turn.", func(ctx context.Context, params taskListParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := runtime.ListAgentTasksForTool(ctx, AgentTaskToolListRequest{
				SessionID: tools.GetSessionFromContext(ctx),
				TurnID:    firstNonEmpty(params.TurnID, tools.GetTurnFromContext(ctx)),
			})
			return taskToolResponse(resp, err)
		}),
		fantasy.NewAgentTool(TaskGetToolName, "Get runtime AgentTask status, messages, and result refs by id.", func(ctx context.Context, params taskGetParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.TaskID) == "" {
				return fantasy.NewTextErrorResponse("task_id is required"), nil
			}
			resp, err := runtime.GetAgentTaskForTool(ctx, AgentTaskToolGetRequest(params))
			return taskToolResponse(resp, err)
		}),
		fantasy.NewAgentTool(TaskMessageToolName, "Send a follow-up instruction to an active runtime AgentTask.", func(ctx context.Context, params taskMessageParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.TaskID) == "" {
				return fantasy.NewTextErrorResponse("task_id is required"), nil
			}
			if strings.TrimSpace(params.Message) == "" {
				return fantasy.NewTextErrorResponse("message is required"), nil
			}
			resp, err := runtime.SendAgentTaskMessageForTool(ctx, AgentTaskToolSendMessageRequest{
				TaskID:          params.TaskID,
				Message:         params.Message,
				Summary:         params.Summary,
				RelatedToolCall: call.ID,
				SessionID:       tools.GetSessionFromContext(ctx),
				TurnID:          tools.GetTurnFromContext(ctx),
			})
			return taskToolResponse(resp, err)
		}),
		fantasy.NewAgentTool(TaskStopToolName, "Stop or cancel an active runtime AgentTask by id.", func(ctx context.Context, params taskStopParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.TaskID) == "" {
				return fantasy.NewTextErrorResponse("task_id is required"), nil
			}
			resp, err := runtime.StopAgentTaskForTool(ctx, AgentTaskToolStopRequest{
				TaskID:          params.TaskID,
				Reason:          params.Reason,
				RelatedToolCall: call.ID,
			})
			return taskToolResponse(resp, err)
		}),
		fantasy.NewAgentTool(TaskOutputToolName, "Get summarized AgentTask output, artifact refs, compact refs, and output refs.", func(ctx context.Context, params taskOutputParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.TaskID) == "" {
				return fantasy.NewTextErrorResponse("task_id is required"), nil
			}
			resp, err := runtime.GetAgentTaskOutputForTool(ctx, AgentTaskToolOutputRequest(params))
			return taskToolResponse(resp, err)
		}),
	}
}

func taskToolResponse(value any, err error) (fantasy.ToolResponse, error) {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	data, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return fantasy.ToolResponse{}, marshalErr
	}
	return fantasy.NewTextResponse(string(data)), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
