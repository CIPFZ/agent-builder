package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/crush/internal/agent"
)

func (r *runtimeService) ListAgentTasksForTool(ctx context.Context, req agent.AgentTaskToolListRequest) (agent.AgentTaskToolListResponse, error) {
	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		turnID = r.activeTurnForSession(req.SessionID)
	}
	if turnID == "" {
		return agent.AgentTaskToolListResponse{}, errors.New("turn id is required")
	}
	tasks, err := r.agentTasks.ListByTurn(ctx, turnID)
	if err != nil {
		return agent.AgentTaskToolListResponse{}, err
	}
	out := make([]agent.AgentTaskToolTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, agentTaskToolTask(task))
	}
	return agent.AgentTaskToolListResponse{Tasks: out}, nil
}

func (r *runtimeService) GetAgentTaskForTool(ctx context.Context, req agent.AgentTaskToolGetRequest) (agent.AgentTaskToolGetResponse, error) {
	resp, err := r.AgentTask(ctx, req.TaskID)
	if err != nil {
		return agent.AgentTaskToolGetResponse{}, err
	}
	messages := make([]agent.AgentTaskToolMessage, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		messages = append(messages, agentTaskToolMessage(msg))
	}
	var result *agent.AgentTaskToolResult
	if resp.Result != nil {
		converted := agentTaskToolResult(*resp.Result)
		result = &converted
	}
	return agent.AgentTaskToolGetResponse{
		Task:     agentTaskToolTask(resp.Task),
		Messages: messages,
		Result:   result,
	}, nil
}

func (r *runtimeService) SendAgentTaskMessageForTool(ctx context.Context, req agent.AgentTaskToolSendMessageRequest) (agent.AgentTaskToolSendMessageResponse, error) {
	resp, err := r.SendAgentTaskFollowUp(ctx, req.TaskID, RuntimeAgentTaskMessageCreateRequest{
		Direction:         taskMessageDirectionParentToChild,
		Kind:              taskMessageKindInstruction,
		ContentSummary:    firstNonEmpty(req.Message, req.Summary),
		RelatedToolCallID: req.RelatedToolCall,
		Payload: map[string]any{
			"summary":    req.Summary,
			"turn_id":    req.TurnID,
			"session_id": req.SessionID,
		},
	})
	msg := agentTaskToolMessage(resp.Message)
	success := err == nil && resp.Message.Status != taskMessageStatusRejected
	status := resp.Message.Status
	text := "message delivered to agent task"
	if !success {
		text = firstNonEmpty(resp.Message.Error, "message was rejected")
	}
	return agent.AgentTaskToolSendMessageResponse{
		Success: success,
		Message: text,
		TaskID:  req.TaskID,
		Status:  status,
		Record:  msg,
	}, err
}

func (r *runtimeService) StopAgentTaskForTool(ctx context.Context, req agent.AgentTaskToolStopRequest) (agent.AgentTaskToolStopResponse, error) {
	if strings.TrimSpace(req.Reason) != "" {
		_, _ = r.CreateAgentTaskMessage(ctx, req.TaskID, RuntimeAgentTaskMessageCreateRequest{
			Direction:         taskMessageDirectionParentToChild,
			Kind:              taskMessageKindControl,
			ContentSummary:    "stop requested: " + strings.TrimSpace(req.Reason),
			RelatedToolCallID: req.RelatedToolCall,
			Payload: map[string]any{
				"action": "stop",
				"reason": strings.TrimSpace(req.Reason),
			},
		})
	}
	resp, err := r.CancelAgentTask(ctx, req.TaskID)
	if err != nil {
		return agent.AgentTaskToolStopResponse{}, err
	}
	return agent.AgentTaskToolStopResponse{
		Success: resp.Task.Status == agentTaskStatusCancelled,
		Message: "agent task stop requested",
		Task:    agentTaskToolTask(resp.Task),
	}, nil
}

func (r *runtimeService) GetAgentTaskOutputForTool(ctx context.Context, req agent.AgentTaskToolOutputRequest) (agent.AgentTaskToolOutputResponse, error) {
	resp, err := r.AgentTask(ctx, req.TaskID)
	if err != nil {
		return agent.AgentTaskToolOutputResponse{}, err
	}
	out := agent.AgentTaskToolOutputResponse{
		TaskID:    resp.Task.ID,
		Status:    resp.Task.Status,
		Summary:   preview(resp.Task.ResultSummary, runtimePartPreviewLimit),
		Error:     preview(resp.Task.Error, runtimePartPreviewLimit),
		Artifacts: append([]string(nil), resp.Task.ArtifactRefs...),
	}
	if resp.Result != nil {
		out.Status = firstNonEmpty(resp.Result.Status, out.Status)
		out.Summary = preview(firstNonEmpty(resp.Result.Summary, out.Summary), runtimePartPreviewLimit)
		out.Error = preview(firstNonEmpty(resp.Result.ErrorDetail, out.Error), runtimePartPreviewLimit)
		out.Artifacts = appendUniqueStrings(out.Artifacts, resp.Result.ArtifactRefs...)
		out.CompactRefs = append([]string(nil), resp.Result.CompactBoundaryRefs...)
		out.ToolCallRefs = append([]string(nil), resp.Result.RelatedToolCallRefs...)
	}
	for _, msg := range resp.Messages {
		if msg.Kind == taskMessageKindResult || msg.Kind == taskMessageKindArtifact {
			out.Messages = append(out.Messages, agentTaskToolMessage(msg))
			out.Artifacts = appendUniqueStrings(out.Artifacts, msg.ArtifactRefs...)
		}
	}
	refs, err := r.Refs(ctx, RuntimeRefListRequest{TaskID: req.TaskID})
	if err == nil {
		for _, ref := range refs.Refs {
			out.OutputRefs = appendUniqueStrings(out.OutputRefs, ref.URI)
		}
	}
	return out, nil
}

func agentTaskToolTask(task RuntimeAgentTask) agent.AgentTaskToolTask {
	return agent.AgentTaskToolTask{
		ID:             task.ID,
		Title:          task.Title,
		Kind:           task.Kind,
		Role:           task.Role,
		Name:           task.Name,
		Status:         task.Status,
		Progress:       task.Progress,
		ParentTurnID:   task.ParentTurnID,
		ChildSessionID: task.ChildSessionID,
		Summary:        firstNonEmpty(task.ResultSummary, task.PromptSummary),
		Error:          task.Error,
		ArtifactRefs:   append([]string(nil), task.ArtifactRefs...),
	}
}

func agentTaskToolMessage(msg RuntimeAgentTaskMessage) agent.AgentTaskToolMessage {
	return agent.AgentTaskToolMessage{
		ID:          msg.ID,
		TaskID:      msg.TaskID,
		Direction:   msg.Direction,
		Kind:        msg.Kind,
		Status:      msg.Status,
		Sequence:    msg.Sequence,
		Summary:     msg.ContentSummary,
		Error:       msg.Error,
		Artifacts:   append([]string(nil), msg.ArtifactRefs...),
		CreatedAt:   msg.CreatedAt,
		DeliveredAt: msg.DeliveredAt,
		ProcessedAt: msg.ProcessedAt,
	}
}

func agentTaskToolResult(result RuntimeAgentTaskResult) agent.AgentTaskToolResult {
	return agent.AgentTaskToolResult{
		TaskID:       result.TaskID,
		Status:       result.Status,
		Summary:      result.Summary,
		Error:        result.ErrorDetail,
		Artifacts:    append([]string(nil), result.ArtifactRefs...),
		MessageRefs:  append([]string(nil), result.RelatedMessageRefs...),
		ToolCallRefs: append([]string(nil), result.RelatedToolCallRefs...),
		CompactRefs:  append([]string(nil), result.CompactBoundaryRefs...),
	}
}
