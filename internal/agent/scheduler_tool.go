package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/permission"
)

type schedulerTool struct {
	inner    fantasy.AgentTool
	recorder SchedulerRecorder
}

func newSchedulerTool(inner fantasy.AgentTool, recorder SchedulerRecorder) *schedulerTool {
	return &schedulerTool{inner: inner, recorder: recorder}
}

func wrapToolsWithSchedulerRecorder(agentTools []fantasy.AgentTool, recorder SchedulerRecorder, isSubAgent bool) []fantasy.AgentTool {
	if recorder == nil || isSubAgent {
		return agentTools
	}
	out := make([]fantasy.AgentTool, len(agentTools))
	for i, tool := range agentTools {
		out[i] = newSchedulerTool(tool, recorder)
	}
	return out
}

func (s *schedulerTool) Info() fantasy.ToolInfo {
	return s.inner.Info()
}

func (s *schedulerTool) ProviderOptions() fantasy.ProviderOptions {
	return s.inner.ProviderOptions()
}

func (s *schedulerTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	s.inner.SetProviderOptions(opts)
}

func (s *schedulerTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	toolInfo := s.inner.Info()
	record := SchedulerToolCall{
		ID:           call.ID,
		SessionID:    tools.GetSessionFromContext(ctx),
		TurnID:       tools.GetTurnFromContext(ctx),
		MessageID:    tools.GetMessageFromContext(ctx),
		Name:         nonEmptyString(call.Name, toolInfo.Name),
		Source:       schedulerSourceForToolName(nonEmptyString(call.Name, toolInfo.Name)),
		CapabilityID: schedulerCapabilityIDForAgentTool(s.inner, nonEmptyString(call.Name, toolInfo.Name)),
		InputSummary: call.Input,
	}
	decision, decisionErr := s.recorder.EvaluateToolCall(ctx, record)
	if decisionErr != nil {
		return fantasy.ToolResponse{}, decisionErr
	}
	if decision.Decision == string(permission.PolicyDeny) {
		reason := nonEmptyString(decision.Reason, "Runtime policy denied tool call.")
		result := SchedulerToolCallResult{
			ToolCallID:          call.ID,
			SessionID:           record.SessionID,
			TurnID:              record.TurnID,
			MessageID:           record.MessageID,
			Name:                record.Name,
			Source:              record.Source,
			ModelVisibleContent: reason,
			StructuredOutputSummary: fmt.Sprintf("policy=%s risk=%s mode=%s",
				decision.Decision,
				decision.Risk,
				decision.Mode,
			),
			Error:   reason,
			IsError: true,
			Status:  "denied",
		}
		_ = s.recorder.ToolCallFailed(ctx, result)
		resp := fantasy.NewTextErrorResponse(reason)
		resp.StopTurn = true
		return resp, nil
	}
	if record.ID != "" {
		_ = s.recorder.ToolCallStarted(ctx, record)
	}

	if record.TurnID != "" {
		ctx = permission.WithTurnID(ctx, record.TurnID)
	}
	resp, err := s.inner.Run(ctx, call)
	result := SchedulerToolCallResult{
		ToolCallID:              call.ID,
		SessionID:               record.SessionID,
		TurnID:                  record.TurnID,
		MessageID:               record.MessageID,
		Name:                    record.Name,
		Source:                  record.Source,
		ModelVisibleContent:     resp.Content,
		StructuredOutputSummary: responseStructuredSummary(resp),
		Error:                   responseError(resp, err),
		IsError:                 resp.IsError || err != nil,
		Cancelled:               errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled),
	}
	if result.ToolCallID == "" {
		return resp, err
	}

	if resp.Content != "" || result.StructuredOutputSummary != "" || result.Error != "" {
		_ = s.recorder.ToolCallOutput(ctx, result)
	}
	switch {
	case result.Cancelled:
		_ = s.recorder.ToolCallCancelled(ctx, result)
	case result.IsError:
		_ = s.recorder.ToolCallFailed(ctx, result)
	default:
		_ = s.recorder.ToolCallCompleted(ctx, result)
	}
	return resp, err
}

type mcpToolIdentity interface {
	MCP() string
	MCPToolName() string
}

func schedulerCapabilityIDForAgentTool(tool fantasy.AgentTool, fallbackName string) string {
	if mcpTool, ok := tool.(mcpToolIdentity); ok {
		server := strings.TrimSpace(mcpTool.MCP())
		name := strings.TrimSpace(mcpTool.MCPToolName())
		if server != "" && name != "" {
			return "mcp:" + server + ":" + name
		}
	}
	return schedulerCapabilityIDForToolName(fallbackName)
}

func schedulerCapabilityIDForToolName(name string) string {
	toolName := strings.TrimSpace(name)
	lower := strings.ToLower(toolName)
	if strings.HasPrefix(lower, "mcp_") {
		return ""
	}
	if toolName == "" {
		return ""
	}
	return "builtin:" + toolName
}

func schedulerSourceForToolName(name string) string {
	toolName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case toolName == "bash":
		return "shell"
	case strings.HasPrefix(toolName, "mcp_"):
		return "mcp"
	case toolName == "":
		return "unknown"
	default:
		return "builtin"
	}
}

func responseStructuredSummary(resp fantasy.ToolResponse) string {
	switch resp.Type {
	case "image", "media":
		if resp.MediaType != "" {
			return resp.Type + ":" + resp.MediaType
		}
		return resp.Type
	default:
		return resp.Metadata
	}
}

func responseError(resp fantasy.ToolResponse, err error) string {
	if err != nil {
		return err.Error()
	}
	if resp.IsError {
		return resp.Content
	}
	return ""
}

func nonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
