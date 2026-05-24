package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	_ = isSubAgent
	if recorder == nil {
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
			Command:             shellCommandFromInput(call.Input),
			Risk:                decision.Risk,
			PolicyReason:        decision.Reason,
			PolicyMode:          decision.Mode,
			PolicyProfile:       decision.Profile,
			PolicyRuleID:        decision.RuleID,
			PolicyRuleSource:    decision.RuleSource,
			PolicyScopeKind:     decision.RuleScopeKind,
			PolicyScopeValue:    decision.RuleScopeValue,
			PolicyTargetSummary: decision.TargetSummary,
			ShellRisk:           decision.ShellRisk,
			ShellReason:         decision.ShellReason,
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
	record.Command = shellCommandFromInput(call.Input)
	record.Risk = decision.Risk
	record.PolicyReason = decision.Reason
	record.PolicyMode = decision.Mode
	record.PolicyProfile = decision.Profile
	record.PolicyRuleID = decision.RuleID
	record.PolicyRuleSource = decision.RuleSource
	record.PolicyScopeKind = decision.RuleScopeKind
	record.PolicyScopeValue = decision.RuleScopeValue
	record.PolicyTargetSummary = decision.TargetSummary
	record.ShellRisk = decision.ShellRisk
	record.ShellReason = decision.ShellReason
	if record.ID != "" {
		_ = s.recorder.ToolCallStarted(ctx, record)
	}

	if record.TurnID != "" {
		ctx = permission.WithTurnID(ctx, record.TurnID)
	}
	if decision.Decision == string(permission.PolicyAllow) && call.ID != "" {
		ctx = permission.WithHookApproval(ctx, call.ID)
	}
	resp, err := s.inner.Run(ctx, call)
	metadata := schedulerResponseMetadata(resp.Metadata)
	result := SchedulerToolCallResult{
		ToolCallID:              call.ID,
		SessionID:               record.SessionID,
		TurnID:                  record.TurnID,
		MessageID:               record.MessageID,
		Name:                    record.Name,
		Source:                  record.Source,
		JobID:                   metadata.JobID,
		Command:                 nonEmptyString(metadata.Command, shellCommandFromInput(call.Input)),
		Risk:                    decision.Risk,
		PolicyReason:            decision.Reason,
		PolicyMode:              decision.Mode,
		PolicyProfile:           decision.Profile,
		PolicyRuleID:            decision.RuleID,
		PolicyRuleSource:        decision.RuleSource,
		PolicyScopeKind:         decision.RuleScopeKind,
		PolicyScopeValue:        decision.RuleScopeValue,
		PolicyTargetSummary:     decision.TargetSummary,
		ShellRisk:               decision.ShellRisk,
		ShellReason:             decision.ShellReason,
		ExitCode:                metadata.ExitCode,
		JobStatus:               metadata.JobStatus,
		JobStartedAt:            metadata.JobStartedAt,
		JobFinishedAt:           metadata.JobFinishedAt,
		ModelVisibleContent:     resp.Content,
		StructuredOutputSummary: responseStructuredSummary(resp),
		Stdout:                  metadata.Stdout,
		Stderr:                  metadata.Stderr,
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
	if lower == ToolSearchToolName {
		return "builtin:" + ToolSearchToolName
	}
	if strings.HasPrefix(lower, "mcp_") {
		return ""
	}
	if toolName == "" {
		return ""
	}
	if lower == "bash" || lower == "job_output" || lower == "job_kill" {
		return "shell:" + lower
	}
	return "builtin:" + toolName
}

func schedulerSourceForToolName(name string) string {
	toolName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case toolName == ToolSearchToolName:
		return "builtin"
	case toolName == "bash", toolName == "job_output", toolName == "job_kill":
		return "shell"
	case strings.HasPrefix(toolName, "mcp_"):
		return "mcp"
	case toolName == "":
		return "unknown"
	default:
		return "builtin"
	}
}

func isBaseRuntimeTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolSearchToolName, "bash", "view", "edit", "write", "multiedit", "grep", "glob", "ls", "todos":
		return true
	default:
		return false
	}
}

func schedulerToolInfoEstimatedTokens(info fantasy.ToolInfo) int {
	data, _ := json.Marshal(map[string]any{
		"name":        info.Name,
		"description": info.Description,
		"parameters":  info.Parameters,
		"required":    info.Required,
	})
	chars := len(data)
	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}

func schedulerSchemaSummary(info fantasy.ToolInfo) string {
	if len(info.Required) == 0 {
		return "no required fields"
	}
	return "required: " + strings.Join(info.Required, ",")
}

func schedulerSchemaDigest(info fantasy.ToolInfo) string {
	data, _ := json.Marshal(map[string]any{
		"name":       info.Name,
		"parameters": info.Parameters,
		"required":   info.Required,
	})
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

type toolResponseMetadata struct {
	JobID         string
	Command       string
	Stdout        string
	Stderr        string
	ExitCode      int
	JobStatus     string
	JobStartedAt  int64
	JobFinishedAt int64
}

func schedulerResponseMetadata(raw string) toolResponseMetadata {
	if strings.TrimSpace(raw) == "" {
		return toolResponseMetadata{}
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return toolResponseMetadata{}
	}
	meta := toolResponseMetadata{
		JobID:   stringMetadata(values, "shell_id"),
		Command: stringMetadata(values, "command"),
		Stdout:  stringMetadata(values, "stdout"),
		Stderr:  stringMetadata(values, "stderr"),
	}
	if exitCode, ok := values["exit_code"].(float64); ok {
		meta.ExitCode = int(exitCode)
	}
	meta.JobStatus = stringMetadata(values, "status")
	if meta.JobStatus == "" {
		if bg, ok := values["background"].(bool); ok && bg {
			meta.JobStatus = "running"
		}
	}
	meta.JobStartedAt = int64Metadata(values, "start_time")
	meta.JobFinishedAt = int64Metadata(values, "end_time")
	return meta
}

func stringMetadata(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func int64Metadata(values map[string]any, key string) int64 {
	value, ok := values[key].(float64)
	if !ok {
		return 0
	}
	return int64(value)
}

func shellCommandFromInput(raw string) string {
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return ""
	}
	command, _ := values["command"].(string)
	return command
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
