package server

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/CIPFZ/agent-builder/internal/agent/notify"
	"github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/app"
	"github.com/CIPFZ/agent-builder/internal/history"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/session"
	"github.com/CIPFZ/agent-builder/internal/skills"
)

// wrapEvent converts a raw app pubsub.Event[T] payload into a pubsub.Payload
// envelope with the correct PayloadType discriminator and a API-typed inner
// payload that has proper JSON tags. Returns nil if the event type is
// unrecognized.
func wrapEvent(ev any) *pubsub.Payload {
	switch e := ev.(type) {
	case pubsub.Event[app.LSPEvent]:
		return envelope(pubsub.PayloadTypeLSPEvent, pubsub.Event[apitypes.LSPEvent]{
			Type: e.Type,
			Payload: apitypes.LSPEvent{
				Type:            apitypes.LSPEventType(e.Payload.Type),
				Name:            e.Payload.Name,
				State:           e.Payload.State,
				Error:           e.Payload.Error,
				DiagnosticCount: e.Payload.DiagnosticCount,
			},
		})
	case pubsub.Event[mcp.Event]:
		return envelope(pubsub.PayloadTypeMCPEvent, pubsub.Event[apitypes.MCPEvent]{
			Type: e.Type,
			Payload: apitypes.MCPEvent{
				Type:      mcpEventTypeToAPIType(e.Payload.Type),
				Name:      e.Payload.Name,
				State:     apitypes.MCPState(e.Payload.State),
				Error:     e.Payload.Error,
				ToolCount: e.Payload.Counts.Tools,
			},
		})
	case pubsub.Event[permission.PermissionRequest]:
		return envelope(pubsub.PayloadTypePermissionRequest, pubsub.Event[apitypes.PermissionRequest]{
			Type: e.Type,
			Payload: apitypes.PermissionRequest{
				ID:          e.Payload.ID,
				SessionID:   e.Payload.SessionID,
				ToolCallID:  e.Payload.ToolCallID,
				ToolName:    e.Payload.ToolName,
				Description: e.Payload.Description,
				Action:      e.Payload.Action,
				Path:        e.Payload.Path,
				Params:      e.Payload.Params,
			},
		})
	case pubsub.Event[permission.PermissionNotification]:
		return envelope(pubsub.PayloadTypePermissionNotification, pubsub.Event[apitypes.PermissionNotification]{
			Type: e.Type,
			Payload: apitypes.PermissionNotification{
				ToolCallID: e.Payload.ToolCallID,
				Granted:    e.Payload.Granted,
				Denied:     e.Payload.Denied,
			},
		})
	case pubsub.Event[message.Message]:
		return envelope(pubsub.PayloadTypeMessage, pubsub.Event[apitypes.Message]{
			Type:    e.Type,
			Payload: messageToAPIType(e.Payload),
		})
	case pubsub.Event[session.Session]:
		return envelope(pubsub.PayloadTypeSession, pubsub.Event[apitypes.Session]{
			Type:    e.Type,
			Payload: sessionToAPIType(e.Payload),
		})
	case pubsub.Event[history.File]:
		return envelope(pubsub.PayloadTypeFile, pubsub.Event[apitypes.File]{
			Type:    e.Type,
			Payload: fileToAPIType(e.Payload),
		})
	case pubsub.Event[notify.Notification]:
		return envelope(pubsub.PayloadTypeAgentEvent, pubsub.Event[apitypes.AgentEvent]{
			Type: e.Type,
			Payload: apitypes.AgentEvent{
				SessionID:    e.Payload.SessionID,
				SessionTitle: e.Payload.SessionTitle,
				Type:         apitypes.AgentEventType(e.Payload.Type),
			},
		})
	case pubsub.Event[skills.Event]:
		return envelope(pubsub.PayloadTypeSkillsEvent, pubsub.Event[apitypes.SkillsEvent]{
			Type:    e.Type,
			Payload: skillsEventToAPIType(e.Payload),
		})
	default:
		slog.Warn("Unrecognized event type for SSE wrapping", "type", fmt.Sprintf("%T", ev))
		return nil
	}
}

// envelope marshals the inner event and wraps it in a pubsub.Payload.
func envelope(payloadType pubsub.PayloadType, inner any) *pubsub.Payload {
	raw, err := json.Marshal(inner)
	if err != nil {
		slog.Error("Failed to marshal event payload", "error", err)
		return nil
	}
	return &pubsub.Payload{
		Type:    payloadType,
		Payload: raw,
	}
}

func mcpEventTypeToAPIType(t mcp.EventType) apitypes.MCPEventType {
	switch t {
	case mcp.EventStateChanged:
		return apitypes.MCPEventStateChanged
	case mcp.EventToolsListChanged:
		return apitypes.MCPEventToolsListChanged
	case mcp.EventPromptsListChanged:
		return apitypes.MCPEventPromptsListChanged
	case mcp.EventResourcesListChanged:
		return apitypes.MCPEventResourcesListChanged
	default:
		return apitypes.MCPEventStateChanged
	}
}

func sessionToAPIType(s session.Session) apitypes.Session {
	return apitypes.Session{
		ID:               s.ID,
		ParentSessionID:  s.ParentSessionID,
		Title:            s.Title,
		TitleSource:      s.TitleSource,
		SummaryMessageID: s.SummaryMessageID,
		MessageCount:     s.MessageCount,
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		Cost:             s.Cost,
		ProjectID:        s.ProjectID,
		Scope:            s.Scope,
		Todos:            todosToAPIType(s.Todos),
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

func todosToAPIType(todos []session.Todo) []apitypes.Todo {
	if len(todos) == 0 {
		return nil
	}
	out := make([]apitypes.Todo, len(todos))
	for i, t := range todos {
		out[i] = apitypes.Todo{
			Content:    t.Content,
			Status:     string(t.Status),
			ActiveForm: t.ActiveForm,
		}
	}
	return out
}

func fileToAPIType(f history.File) apitypes.File {
	return apitypes.File{
		ID:        f.ID,
		SessionID: f.SessionID,
		Path:      f.Path,
		Content:   f.Content,
		Version:   f.Version,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

func skillsEventToAPIType(e skills.Event) apitypes.SkillsEvent {
	if len(e.States) == 0 {
		return apitypes.SkillsEvent{}
	}
	out := apitypes.SkillsEvent{States: make([]apitypes.SkillState, len(e.States))}
	for i, s := range e.States {
		out.States[i] = apitypes.SkillState{
			Name:  s.Name,
			Path:  s.Path,
			State: apitypes.SkillDiscoveryState(s.State),
		}
		if s.Err != nil {
			out.States[i].Error = s.Err.Error()
		}
	}
	return out
}

func messageToAPIType(m message.Message) apitypes.Message {
	msg := apitypes.Message{
		ID:        m.ID,
		SessionID: m.SessionID,
		Role:      apitypes.MessageRole(m.Role),
		Model:     m.Model,
		Provider:  m.Provider,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	for _, p := range m.Parts {
		switch v := p.(type) {
		case message.TextContent:
			msg.Parts = append(msg.Parts, apitypes.TextContent{Text: v.Text})
		case message.ReasoningContent:
			msg.Parts = append(msg.Parts, apitypes.ReasoningContent{
				Thinking:   v.Thinking,
				Signature:  v.Signature,
				StartedAt:  v.StartedAt,
				FinishedAt: v.FinishedAt,
			})
		case message.ToolCall:
			msg.Parts = append(msg.Parts, apitypes.ToolCall{
				ID:       v.ID,
				Name:     v.Name,
				Input:    v.Input,
				Finished: v.Finished,
			})
		case message.ToolResult:
			msg.Parts = append(msg.Parts, apitypes.ToolResult{
				ToolCallID:       v.ToolCallID,
				Name:             v.Name,
				Content:          v.Content,
				Data:             v.Data,
				MIMEType:         v.MIMEType,
				Metadata:         v.Metadata,
				IsError:          v.IsError,
				DeliveredToModel: v.DeliveredToModel,
				DeliveredAtStep:  v.DeliveredAtStep,
				DeliveryReason:   v.DeliveryReason,
				StoredPath:       v.StoredPath,
				OriginalSize:     v.OriginalSize,
				TruncatedBy:      v.TruncatedBy,
			})
		case message.Finish:
			msg.Parts = append(msg.Parts, apitypes.Finish{
				Reason:  apitypes.FinishReason(v.Reason),
				Time:    v.Time,
				Message: v.Message,
				Details: v.Details,
			})
		case message.ImageURLContent:
			msg.Parts = append(msg.Parts, apitypes.ImageURLContent{URL: v.URL, Detail: v.Detail})
		case message.BinaryContent:
			msg.Parts = append(msg.Parts, apitypes.BinaryContent{Path: v.Path, MIMEType: v.MIMEType, Data: v.Data})
		}
	}

	return msg
}

func messagesToAPIType(msgs []message.Message) []apitypes.Message {
	out := make([]apitypes.Message, len(msgs))
	for i, m := range msgs {
		out[i] = messageToAPIType(m)
	}
	return out
}
