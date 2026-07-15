package runtime

import (
	"strings"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/message"
)

func toAPITypeMessage(msg message.Message) apitypes.Message {

	out := apitypes.Message{

		ID: msg.ID,

		SessionID: msg.SessionID,

		Role: apitypes.MessageRole(msg.Role),

		Model: msg.Model,

		Provider: msg.Provider,

		Metadata: cloneStringMap(msg.Metadata),

		CreatedAt: msg.CreatedAt,

		UpdatedAt: msg.UpdatedAt,
		Usage:     msg.Usage,
	}

	for _, part := range msg.Parts {

		switch p := part.(type) {

		case message.TextContent:

			out.Parts = append(out.Parts, apitypes.TextContent{Text: p.Text})

		case message.ReasoningContent:

			out.Parts = append(out.Parts, apitypes.ReasoningContent{

				Thinking: p.Thinking,

				Signature: p.Signature,

				StartedAt: p.StartedAt,

				FinishedAt: p.FinishedAt,
			})

		case message.ToolCall:

			out.Parts = append(out.Parts, apitypes.ToolCall{

				ID: p.ID,

				Name: p.Name,

				Input: p.Input,

				Finished: p.Finished,
			})

		case message.ToolResult:

			out.Parts = append(out.Parts, apitypes.ToolResult{

				ToolCallID: p.ToolCallID,

				Name: p.Name,

				Content: p.Content,

				Data: p.Data,

				MIMEType: p.MIMEType,

				Metadata: p.Metadata,

				IsError: p.IsError,

				DeliveredToModel: p.DeliveredToModel,

				DeliveredAtStep: p.DeliveredAtStep,

				DeliveryReason: p.DeliveryReason,

				StoredPath: p.StoredPath,

				OriginalSize: p.OriginalSize,

				TruncatedBy: p.TruncatedBy,
			})

		case message.Finish:

			out.Parts = append(out.Parts, apitypes.Finish{

				Reason: apitypes.FinishReason(p.Reason),

				Time: p.Time,

				Message: p.Message,

				Details: p.Details,
			})

		case message.ImageURLContent:

			out.Parts = append(out.Parts, apitypes.ImageURLContent{

				URL: p.URL,

				Detail: p.Detail,
			})

		case message.BinaryContent:

			out.Parts = append(out.Parts, apitypes.BinaryContent{

				Path: p.Path,

				MIMEType: p.MIMEType,

				Data: p.Data,
			})

		}

	}

	return out

}

func assistantContent(msg apitypes.Message) string {

	content := strings.TrimSpace(msg.Content().String())

	if content != "" {

		return msg.Content().String()

	}

	for _, part := range msg.Parts {

		finish, ok := part.(apitypes.Finish)

		if !ok || finish.Reason != apitypes.FinishReasonError {

			continue

		}

		switch {

		case finish.Message != "" && finish.Details != "":

			return finish.Message + ": " + finish.Details

		case finish.Message != "":

			return finish.Message

		case finish.Details != "":

			return finish.Details

		default:

			return "Provider returned an error without details. Check logs for more information."

		}

	}

	return msg.Content().String()

}

func toRuntimeMessage(msg apitypes.Message) RuntimeMessage {

	content := msg.Content().String()

	var finishError string

	var finishReason string

	finished := false

	for _, part := range msg.Parts {

		finish, ok := part.(apitypes.Finish)

		if !ok {

			continue

		}

		finished = true

		finishReason = string(finish.Reason)

		if finish.Reason != apitypes.FinishReasonError {

			continue

		}

		switch {

		case finish.Message != "" && finish.Details != "":

			finishError = finish.Message + ": " + finish.Details

		case finish.Message != "":

			finishError = finish.Message

		case finish.Details != "":

			finishError = finish.Details

		default:

			finishError = "Provider returned an error without details. Check logs for more information."

		}

	}

	if strings.TrimSpace(content) == "" && finishError != "" {

		content = finishError

	}

	return RuntimeMessage{

		ID: msg.ID,

		SessionID: msg.SessionID,

		Role: string(msg.Role),

		Content: content,

		Parts: toRuntimeMessageParts(msg),

		Metadata: cloneStringMap(msg.Metadata),

		ClientRequestID: msg.Metadata["clientRequestId"],

		InputMode: msg.Metadata["inputMode"],

		Hidden: msg.Metadata["hidden"] == "true",

		Provider: msg.Provider,

		Model: msg.Model,

		CreatedAt: msg.CreatedAt,

		UpdatedAt: msg.UpdatedAt,

		Finished: finished,

		FinishReason: finishReason,

		Error: finishError,
	}

}

func toRuntimeMessageParts(msg apitypes.Message) []RuntimeMessagePart {

	parts := make([]RuntimeMessagePart, 0, len(msg.Parts))

	for _, part := range msg.Parts {

		switch p := part.(type) {

		case apitypes.TextContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "text",

				Text: p.Text,
			})

		case apitypes.ReasoningContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "reasoning",

				Thinking: p.Thinking,

				StartedAt: p.StartedAt,

				FinishedAt: p.FinishedAt,
			})

		case apitypes.ToolCall:

			parts = append(parts, RuntimeMessagePart{

				Type: "tool_call",

				ToolCallID: p.ID,

				Name: p.Name,

				Input: preview(p.Input, runtimePartPreviewLimit),

				Finished: p.Finished,
			})

		case apitypes.ToolResult:

			parts = append(parts, RuntimeMessagePart{

				Type: "tool_result",

				ToolCallID: p.ToolCallID,

				Name: p.Name,

				Content: preview(p.Content, runtimePartPreviewLimit),

				Data: preview(p.Data, runtimePartPreviewLimit),

				MIMEType: p.MIMEType,

				Metadata: preview(p.Metadata, runtimePartPreviewLimit),

				IsError: p.IsError,

				DeliveredToModel: p.DeliveredToModel,

				DeliveredAtStep: p.DeliveredAtStep,

				DeliveryReason: p.DeliveryReason,

				StoredPath: p.StoredPath,

				OriginalSize: p.OriginalSize,

				TruncatedBy: p.TruncatedBy,
			})

		case apitypes.Finish:

			parts = append(parts, RuntimeMessagePart{

				Type: "finish",

				Reason: string(p.Reason),

				Message: p.Message,

				Details: p.Details,
			})

		case apitypes.ImageURLContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "image_url",

				Text: p.URL,
			})

		case apitypes.BinaryContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "binary",

				Text: p.Path,

				MIMEType: p.MIMEType,
			})

		}

	}

	return parts

}

func isDisplayableRuntimeMessage(msg RuntimeMessage) bool {

	if msg.Role == string(message.User) {

		return strings.TrimSpace(msg.Content) != ""

	}

	if msg.Role != string(message.Assistant) && msg.Role != string(message.Tool) {

		return false

	}

	if strings.TrimSpace(msg.Content) != "" || msg.Error != "" {

		return true

	}

	for _, part := range msg.Parts {

		switch part.Type {

		case "reasoning":

			if strings.TrimSpace(part.Thinking) != "" {

				return true

			}

		case "tool_call", "tool_result":

			return true

		}

	}

	return msg.Finished && msg.FinishReason == string(apitypes.FinishReasonError)

}
