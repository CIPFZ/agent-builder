package tui

import (
	"myclaw/internal/approval"
	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func clientEventFromRuntimeEvent(event runtime.RuntimeEvent) clientEvent {
	out := clientEvent{
		Type:    event.Type,
		Session: clientSessionFromSession(event.Session),
		Error:   event.Error,
	}
	if event.Message != nil {
		message := clientMessage{
			ID:      event.Message.ID,
			Role:    event.Message.Role,
			Content: event.Message.Content,
			Blocks:  clientBlocksFromModel(event.Message.Blocks),
		}
		if event.Message.IsCompactSummary {
			message.Role = "summary"
		}
		if event.Message.Subtype == "compact_boundary" {
			message.Content = "[compact_boundary]"
		}
		out.Message = &message
	}
	if event.Type == "assistant.delta" && event.Delta != "" {
		out.Message = &clientMessage{Role: "assistant", Content: event.Delta}
		out.Tool = &clientToolEvent{ProgressMessage: event.Delta}
	}
	if needsToolProjection(event) {
		out.Tool = &clientToolEvent{
			RunID:           event.RunID,
			ToolName:        event.ToolName,
			ToolUseID:       event.ToolUseID,
			ToolInput:       event.ToolInput,
			ToolInputObject: cloneAnyMap(event.ToolInputObject),
		}
	}
	if event.Progress != nil && (event.Progress.ToolUseID != "" || event.Progress.Message != "" || len(event.Progress.Data) > 0) {
		if out.Tool == nil {
			out.Tool = &clientToolEvent{}
		}
		out.Tool.ToolUseID = valueOrDefault(event.Progress.ToolUseID, out.Tool.ToolUseID)
		out.Tool.ProgressType = event.Progress.Type
		out.Tool.ProgressMessage = event.Progress.Message
		out.Tool.ProgressData = cloneAnyMap(event.Progress.Data)
	}
	if event.Approval != nil {
		if out.Tool == nil {
			out.Tool = &clientToolEvent{}
		}
		out.Tool.Approval = clientApprovalFromRuntimeApproval(event.Approval)
	}
	return out
}

func runtimeEventFromClientEvent(event clientEvent) runtime.RuntimeEvent {
	out := runtime.RuntimeEvent{
		Type:      event.Type,
		Session:   session.Session{ID: event.Session.ID, Key: event.Session.Key, AgentID: event.Session.AgentID, IsMain: event.Session.IsMain},
		Error:     event.Error,
		RunID:     "",
		ToolName:  "",
		ToolUseID: "",
	}
	if event.Message != nil {
		out.Message = &session.Message{
			ID:      event.Message.ID,
			Role:    event.Message.Role,
			Content: event.Message.Content,
			Blocks:  cloneClientBlocksToModel(event.Message.Blocks),
		}
	}
	if event.Tool != nil {
		out.RunID = event.Tool.RunID
		out.ToolName = event.Tool.ToolName
		out.ToolUseID = event.Tool.ToolUseID
		out.ToolInput = event.Tool.ToolInput
		out.ToolInputObject = cloneAnyMap(event.Tool.ToolInputObject)
		if event.Tool.ProgressType != "" || event.Tool.ProgressMessage != "" || len(event.Tool.ProgressData) > 0 {
			out.Progress = &tools.ToolProgress{
				Type:      event.Tool.ProgressType,
				ToolUseID: event.Tool.ToolUseID,
				Message:   event.Tool.ProgressMessage,
				Data:      cloneAnyMap(event.Tool.ProgressData),
			}
		}
		if event.Tool.Approval != nil {
			out.Approval = &approval.Request{
				ID:              event.Tool.Approval.ID,
				SessionID:       event.Tool.Approval.SessionID,
				RunID:           event.Tool.Approval.RunID,
				ToolName:        event.Tool.Approval.ToolName,
				ToolInput:       event.Tool.Approval.ToolInput,
				ToolInputObject: cloneAnyMap(event.Tool.Approval.ToolInputObject),
				Category:        event.Tool.Approval.Category,
				RuleSource:      event.Tool.Approval.RuleSource,
				Reason:          event.Tool.Approval.Reason,
				DecisionReason:  event.Tool.Approval.DecisionReason,
				AcceptFeedback:  event.Tool.Approval.AcceptFeedback,
				Status:          approval.Status(event.Tool.Approval.Status),
			}
		}
	}
	if event.Type == "assistant.delta" && event.Message != nil {
		out.Delta = event.Message.Content
	}
	return out
}

func clientSessionFromSession(sess session.Session) clientSessionRef {
	return clientSessionRef{
		ID:      sess.ID,
		Key:     sess.Key,
		AgentID: sess.AgentID,
		IsMain:  sess.IsMain,
	}
}

func clientApprovalFromRuntimeApproval(req *approval.Request) *clientApproval {
	if req == nil {
		return nil
	}
	return &clientApproval{
		ID:              req.ID,
		SessionID:       req.SessionID,
		RunID:           req.RunID,
		ToolName:        req.ToolName,
		ToolInput:       req.ToolInput,
		ToolInputObject: cloneAnyMap(req.ToolInputObject),
		Category:        req.Category,
		RuleSource:      req.RuleSource,
		Reason:          req.Reason,
		DecisionReason:  req.DecisionReason,
		AcceptFeedback:  req.AcceptFeedback,
		Status:          string(req.Status),
	}
}

func cloneToolProgress(progress tools.ToolProgress) tools.ToolProgress {
	return tools.ToolProgress{
		Type:      progress.Type,
		ToolUseID: progress.ToolUseID,
		Message:   progress.Message,
		Data:      cloneAnyMap(progress.Data),
	}
}

func needsToolProjection(event runtime.RuntimeEvent) bool {
	return event.ToolName != "" || event.ToolUseID != "" || event.ToolInput != "" || event.ToolInputObject != nil
}

func transcriptEntryFromClientMessage(message clientMessage) (transcriptEntry, bool) {
	switch message.Role {
	case "tool":
		return transcriptEntry{
			Role:    "tool",
			Content: message.Content,
			Blocks:  cloneClientMessageBlocks(message.Blocks),
		}, true
	default:
		return transcriptEntry{}, false
	}
}

func clientBlocksFromModel(blocks []model.MessageBlock) []clientMessageBlock {
	if len(blocks) == 0 {
		return nil
	}
	items := make([]clientMessageBlock, 0, len(blocks))
	for _, block := range blocks {
		items = append(items, clientMessageBlock{
			Type:        string(block.Type),
			ID:          block.ID,
			ToolUseID:   block.ToolUseID,
			Text:        block.Text,
			Name:        block.Name,
			Input:       block.Input,
			InputObject: cloneAnyMap(block.InputObject),
			Content:     block.Content,
			IsError:     block.IsError,
		})
	}
	return items
}

func cloneClientBlocksToModel(blocks []clientMessageBlock) []model.MessageBlock {
	if len(blocks) == 0 {
		return nil
	}
	items := make([]model.MessageBlock, 0, len(blocks))
	for _, block := range blocks {
		items = append(items, model.MessageBlock{
			Type:        model.MessageBlockType(block.Type),
			ID:          block.ID,
			ToolUseID:   block.ToolUseID,
			Text:        block.Text,
			Name:        block.Name,
			Input:       block.Input,
			InputObject: cloneAnyMap(block.InputObject),
			Content:     block.Content,
			IsError:     block.IsError,
		})
	}
	return items
}
