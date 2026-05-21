package main

import (
	"strings"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/proto"
)

func toProtoMessage(msg message.Message) proto.Message {

	out := proto.Message{

		ID: msg.ID,

		SessionID: msg.SessionID,

		Role: proto.MessageRole(msg.Role),

		Model: msg.Model,

		Provider: msg.Provider,

		CreatedAt: msg.CreatedAt,

		UpdatedAt: msg.UpdatedAt,
	}

	for _, part := range msg.Parts {

		switch p := part.(type) {

		case message.TextContent:

			out.Parts = append(out.Parts, proto.TextContent{Text: p.Text})

		case message.ReasoningContent:

			out.Parts = append(out.Parts, proto.ReasoningContent{

				Thinking: p.Thinking,

				Signature: p.Signature,

				StartedAt: p.StartedAt,

				FinishedAt: p.FinishedAt,
			})

		case message.ToolCall:

			out.Parts = append(out.Parts, proto.ToolCall{

				ID: p.ID,

				Name: p.Name,

				Input: p.Input,

				Finished: p.Finished,
			})

		case message.ToolResult:

			out.Parts = append(out.Parts, proto.ToolResult{

				ToolCallID: p.ToolCallID,

				Name: p.Name,

				Content: p.Content,

				Data: p.Data,

				MIMEType: p.MIMEType,

				Metadata: p.Metadata,

				IsError: p.IsError,
			})

		case message.Finish:

			out.Parts = append(out.Parts, proto.Finish{

				Reason: proto.FinishReason(p.Reason),

				Time: p.Time,

				Message: p.Message,

				Details: p.Details,
			})

		case message.ImageURLContent:

			out.Parts = append(out.Parts, proto.ImageURLContent{

				URL: p.URL,

				Detail: p.Detail,
			})

		case message.BinaryContent:

			out.Parts = append(out.Parts, proto.BinaryContent{

				Path: p.Path,

				MIMEType: p.MIMEType,

				Data: p.Data,
			})

		}

	}

	return out

}

func assistantContent(msg proto.Message) string {

	content := strings.TrimSpace(msg.Content().String())

	if content != "" {

		return msg.Content().String()

	}

	for _, part := range msg.Parts {

		finish, ok := part.(proto.Finish)

		if !ok || finish.Reason != proto.FinishReasonError {

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

func toRuntimeMessage(msg proto.Message) RuntimeMessage {

	content := msg.Content().String()

	var finishError string

	var finishReason string

	finished := false

	for _, part := range msg.Parts {

		finish, ok := part.(proto.Finish)

		if !ok {

			continue

		}

		finished = true

		finishReason = string(finish.Reason)

		if finish.Reason != proto.FinishReasonError {

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

		Provider: msg.Provider,

		Model: msg.Model,

		CreatedAt: msg.CreatedAt,

		UpdatedAt: msg.UpdatedAt,

		Finished: finished,

		FinishReason: finishReason,

		Error: finishError,
	}

}

func toRuntimeMessageParts(msg proto.Message) []RuntimeMessagePart {

	parts := make([]RuntimeMessagePart, 0, len(msg.Parts))

	for _, part := range msg.Parts {

		switch p := part.(type) {

		case proto.TextContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "text",

				Text: p.Text,
			})

		case proto.ReasoningContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "reasoning",

				Thinking: p.Thinking,

				StartedAt: p.StartedAt,

				FinishedAt: p.FinishedAt,
			})

		case proto.ToolCall:

			parts = append(parts, RuntimeMessagePart{

				Type: "tool_call",

				ToolCallID: p.ID,

				Name: p.Name,

				Input: preview(p.Input, runtimePartPreviewLimit),

				Finished: p.Finished,
			})

		case proto.ToolResult:

			parts = append(parts, RuntimeMessagePart{

				Type: "tool_result",

				ToolCallID: p.ToolCallID,

				Name: p.Name,

				Content: preview(p.Content, runtimePartPreviewLimit),

				Data: preview(p.Data, runtimePartPreviewLimit),

				MIMEType: p.MIMEType,

				Metadata: preview(p.Metadata, runtimePartPreviewLimit),

				IsError: p.IsError,
			})

		case proto.Finish:

			parts = append(parts, RuntimeMessagePart{

				Type: "finish",

				Reason: string(p.Reason),

				Message: p.Message,

				Details: p.Details,
			})

		case proto.ImageURLContent:

			parts = append(parts, RuntimeMessagePart{

				Type: "image_url",

				Text: p.URL,
			})

		case proto.BinaryContent:

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

	return msg.Finished && msg.FinishReason == string(proto.FinishReasonError)

}
