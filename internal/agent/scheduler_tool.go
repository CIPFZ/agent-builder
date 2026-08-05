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
	"github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/shell"
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
		InputSummary: schedulerInputSummaryWithScope(ctx, call.Input),
	}
	decision, decisionErr := s.recorder.EvaluateToolCall(ctx, record)
	if decisionErr != nil {
		return fantasy.ToolResponse{}, decisionErr
	}
	if decision.Decision == string(permission.PolicyDeny) {
		reason := nonEmptyString(decision.Reason, "Runtime policy denied tool call.")
		result := SchedulerToolCallResult{
			ToolCallID:           call.ID,
			SessionID:            record.SessionID,
			TurnID:               record.TurnID,
			MessageID:            record.MessageID,
			Name:                 record.Name,
			Source:               record.Source,
			Command:              shellCommandFromInput(call.Input),
			Risk:                 decision.Risk,
			PolicyReason:         decision.Reason,
			PolicyMode:           decision.Mode,
			PolicyProfile:        decision.Profile,
			PolicyRuleID:         decision.RuleID,
			PolicyRuleSource:     decision.RuleSource,
			PolicyScopeKind:      decision.RuleScopeKind,
			PolicyScopeValue:     decision.RuleScopeValue,
			PolicyTargetSummary:  decision.TargetSummary,
			ShellRisk:            decision.ShellRisk,
			ShellReason:          decision.ShellReason,
			SandboxDecisionID:    decision.SandboxDecisionID,
			SandboxMode:          decision.SandboxMode,
			SandboxStatus:        decision.SandboxStatus,
			SandboxExecutor:      decision.SandboxExecutor,
			SandboxReason:        decision.SandboxReason,
			SandboxError:         decision.SandboxError,
			PolicyHeadless:       decision.Headless,
			PolicyHeadlessReason: decision.HeadlessReason,
			ModelVisibleContent:  reason,
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
	record.SandboxDecisionID = decision.SandboxDecisionID
	record.SandboxMode = decision.SandboxMode
	record.SandboxStatus = decision.SandboxStatus
	record.SandboxExecutor = decision.SandboxExecutor
	record.SandboxReason = decision.SandboxReason
	record.SandboxError = decision.SandboxError
	record.PolicyHeadless = decision.Headless
	record.PolicyHeadlessReason = decision.HeadlessReason
	if record.TurnID != "" {
		ctx = permission.WithTurnID(ctx, record.TurnID)
	}
	if decision.Decision == string(permission.PolicyAllow) && call.ID != "" {
		ctx = permission.WithHookApproval(ctx, call.ID)
	}
	if decision.SandboxDecisionID != "" {
		ctx = tools.WithSandboxMetadata(ctx, tools.SandboxContextMetadata{
			DecisionID: decision.SandboxDecisionID,
			Mode:       decision.SandboxMode,
			Status:     decision.SandboxStatus,
			Executor:   decision.SandboxExecutor,
			Reason:     decision.SandboxReason,
			Error:      decision.SandboxError,
		})
	}
	resourceClass := toolResourceClass(record.Source, record.Name)
	if resourceClass != "" {
		if queueRecorder, ok := s.recorder.(SchedulerQueueRecorder); ok && record.ID != "" {
			if err := queueRecorder.ToolCallQueued(ctx, record); err != nil {
				_ = s.recorder.ToolCallFailed(ctx, SchedulerToolCallResult{ToolCallID: call.ID, SessionID: record.SessionID, TurnID: record.TurnID, MessageID: record.MessageID, Name: record.Name, Source: record.Source, Error: err.Error(), IsError: true})
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
		}
	}
	releaseResource, admissionErr := acquireToolResource(ctx, record.Source, record.Name, int64(len(call.Input)))
	if admissionErr != nil {
		_ = s.recorder.ToolCallCancelled(ctx, SchedulerToolCallResult{ToolCallID: call.ID, SessionID: record.SessionID, TurnID: record.TurnID, MessageID: record.MessageID, Name: record.Name, Source: record.Source, Error: admissionErr.Error(), IsError: true, Cancelled: true})
		return fantasy.ToolResponse{}, admissionErr
	}
	if record.ID != "" {
		if err := s.recorder.ToolCallStarted(ctx, record); err != nil {
			releaseResource()
			_ = s.recorder.ToolCallFailed(ctx, SchedulerToolCallResult{
				ToolCallID:           call.ID,
				SessionID:            record.SessionID,
				TurnID:               record.TurnID,
				MessageID:            record.MessageID,
				Name:                 record.Name,
				Source:               record.Source,
				Risk:                 decision.Risk,
				PolicyReason:         decision.Reason,
				PolicyMode:           decision.Mode,
				PolicyProfile:        decision.Profile,
				PolicyRuleID:         decision.RuleID,
				PolicyRuleSource:     decision.RuleSource,
				PolicyScopeKind:      decision.RuleScopeKind,
				PolicyScopeValue:     decision.RuleScopeValue,
				PolicyTargetSummary:  decision.TargetSummary,
				ShellRisk:            decision.ShellRisk,
				ShellReason:          decision.ShellReason,
				SandboxDecisionID:    decision.SandboxDecisionID,
				SandboxMode:          decision.SandboxMode,
				SandboxStatus:        decision.SandboxStatus,
				SandboxExecutor:      decision.SandboxExecutor,
				SandboxReason:        decision.SandboxReason,
				SandboxError:         decision.SandboxError,
				PolicyHeadless:       decision.Headless,
				PolicyHeadlessReason: decision.HeadlessReason,
				Error:                err.Error(),
				IsError:              true,
			})
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
	}
	resp, err := s.inner.Run(ctx, call)
	metadata := schedulerResponseMetadata(resp.Metadata)
	if resourceClass != ToolResourceShell || metadata.JobID == "" || metadata.JobStatus != "running" || !retainShellResourceLease(metadata.JobID, releaseResource) {
		releaseResource()
	}
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
		SandboxDecisionID:       decision.SandboxDecisionID,
		SandboxMode:             nonEmptyString(metadata.SandboxMode, decision.SandboxMode),
		SandboxStatus:           nonEmptyString(metadata.SandboxStatus, decision.SandboxStatus),
		SandboxExecutor:         nonEmptyString(metadata.SandboxExecutor, decision.SandboxExecutor),
		SandboxReason:           nonEmptyString(metadata.SandboxReason, decision.SandboxReason),
		SandboxError:            nonEmptyString(metadata.SandboxError, decision.SandboxError),
		PolicyHeadless:          decision.Headless,
		PolicyHeadlessReason:    decision.HeadlessReason,
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
		if outputErr := s.recorder.ToolCallOutput(ctx, result); outputErr != nil && err == nil {
			err = outputErr
			result.Error = outputErr.Error()
			result.IsError = true
		}
	}
	var recordErr error
	switch {
	case result.Cancelled:
		recordErr = s.recorder.ToolCallCancelled(ctx, result)
	case result.IsError:
		recordErr = s.recorder.ToolCallFailed(ctx, result)
	default:
		recordErr = s.recorder.ToolCallCompleted(ctx, result)
	}
	if err == nil && recordErr != nil {
		return resp, recordErr
	}
	return resp, err
}

func retainShellResourceLease(jobID string, release func()) bool {
	backgroundShell, ok := shell.GetBackgroundShellManager().Get(jobID)
	if !ok {
		return false
	}
	go func() {
		backgroundShell.Wait()
		release()
	}()
	return true
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
	if strings.HasPrefix(lower, "task_") {
		return "builtin:" + lower
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
	JobID           string
	Command         string
	Stdout          string
	Stderr          string
	ExitCode        int
	JobStatus       string
	JobStartedAt    int64
	JobFinishedAt   int64
	SandboxMode     string
	SandboxStatus   string
	SandboxExecutor string
	SandboxReason   string
	SandboxError    string
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
		JobID:           stringMetadata(values, "shell_id"),
		Command:         stringMetadata(values, "command"),
		Stdout:          stringMetadata(values, "stdout"),
		Stderr:          stringMetadata(values, "stderr"),
		SandboxMode:     stringMetadata(values, "sandbox_mode"),
		SandboxStatus:   stringMetadata(values, "sandbox_status"),
		SandboxExecutor: stringMetadata(values, "sandbox_executor"),
		SandboxReason:   stringMetadata(values, "sandbox_reason"),
		SandboxError:    stringMetadata(values, "sandbox_error"),
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

func schedulerInputSummaryWithScope(ctx context.Context, raw string) string {
	cwd := tools.GetEffectiveCWDFromContext(ctx)
	worktree := tools.GetWorktreePathFromContext(ctx)
	if cwd == "" && worktree == "" {
		return raw
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		values = map[string]any{"input": raw}
	}
	if cwd != "" {
		if _, exists := values["working_dir"]; !exists {
			values["working_dir"] = cwd
		}
		values["effective_cwd"] = cwd
	}
	if worktree != "" {
		values["worktree"] = worktree
	}
	data, err := json.Marshal(values)
	if err != nil {
		return raw
	}
	return string(data)
}

func responseStructuredSummary(resp fantasy.ToolResponse) string {
	switch resp.Type {
	case "image", "media":
		if resp.Metadata != "" {
			return resp.Metadata
		}
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
