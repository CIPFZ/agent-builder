package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type runtimeToolCallStore = *scheduler.Scheduler

func NewRuntimeToolCallStore() scheduler.Store {
	return scheduler.NewMemoryStore()
}

func NewRuntimeToolCallStoreForDB(db *sql.DB) scheduler.Store {
	return newRuntimeSQLiteToolCallStore(db)
}

func (r *runtimeService) ToolCall(ctx context.Context, toolCallID string) (RuntimeToolCallResponse, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return RuntimeToolCallResponse{}, errors.New("tool call id is required")
	}
	call, err := r.toolCalls.GetCall(ctx, toolCallID)
	if err != nil {
		return RuntimeToolCallResponse{}, fmt.Errorf("tool call %s was not found: %w", toolCallID, err)
	}
	return RuntimeToolCallResponse{ToolCall: toRuntimeToolCall(call)}, nil
}

func (r *runtimeService) TurnToolCalls(ctx context.Context, turnID string) (RuntimeToolCallsResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeToolCallsResponse{}, errors.New("turn id is required")
	}
	calls, err := r.toolCalls.ListCalls(ctx, turnID)
	if err != nil {
		return RuntimeToolCallsResponse{}, err
	}
	out := make([]RuntimeToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, toRuntimeToolCall(call))
	}
	return RuntimeToolCallsResponse{ToolCalls: out}, nil
}

func (r *runtimeService) recordToolCallsFromMessage(ctx context.Context, msg proto.Message, turnID string, createdAt time.Time) {
	if r.toolCalls == nil {
		return
	}
	for _, call := range msg.ToolCalls() {
		if call.ID == "" {
			continue
		}
		if existing, err := r.toolCalls.GetCall(ctx, call.ID); err == nil && isFinalToolCallStatus(string(existing.Status)) {
			continue
		}
		_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:           call.ID,
			SessionID:    msg.SessionID,
			TurnID:       turnID,
			MessageID:    msg.ID,
			Name:         call.Name,
			Source:       classifyToolSource(call.Name),
			CapabilityID: capabilityIDForToolName(call.Name),
			InputSummary: preview(call.Input, runtimePartPreviewLimit),
		})
		if call.Finished {
			_, _ = r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
				ToolCallID: call.ID,
				Status:     scheduler.ToolCallCompleted,
			})
		}
	}
	for _, result := range msg.ToolResults() {
		if result.ToolCallID == "" {
			continue
		}
		status := scheduler.ToolCallCompleted
		errText := ""
		if result.IsError {
			status = scheduler.ToolCallFailed
			errText = preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit)
		}
		if existing, err := r.toolCalls.GetCall(ctx, result.ToolCallID); err == nil && isFinalToolCallStatus(string(existing.Status)) {
			continue
		}
		_, err := r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
			ToolCallID:    result.ToolCallID,
			Status:        status,
			OutputSummary: preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit),
			IsError:       result.IsError,
			Error:         errText,
		})
		if err == nil {
			continue
		}
		_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:           result.ToolCallID,
			SessionID:    msg.SessionID,
			TurnID:       turnID,
			MessageID:    msg.ID,
			Name:         result.Name,
			Source:       classifyToolSource(result.Name),
			CapabilityID: capabilityIDForToolName(result.Name),
		})
		_, _ = r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
			ToolCallID:    result.ToolCallID,
			Status:        status,
			OutputSummary: preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit),
			IsError:       result.IsError,
			Error:         errText,
		})
	}
	_ = createdAt
}

func toRuntimeToolCall(call scheduler.ToolCall) RuntimeToolCall {
	var finishedAt int64
	if !call.FinishedAt.IsZero() {
		finishedAt = call.FinishedAt.UnixMilli()
	}
	var jobStartedAt int64
	if !call.JobStartedAt.IsZero() {
		jobStartedAt = call.JobStartedAt.UnixMilli()
	}
	var jobFinishedAt int64
	if !call.JobFinishedAt.IsZero() {
		jobFinishedAt = call.JobFinishedAt.UnixMilli()
	}
	var compactedAt int64
	if !call.CompactedAt.IsZero() {
		compactedAt = call.CompactedAt.UnixMilli()
	}
	redacted := redactRuntimePayload(map[string]any{
		"input":                  call.InputSummary,
		"output":                 call.OutputSummary,
		"model_content":          call.ModelContent,
		"structured":             call.Structured,
		"stdout":                 call.Stdout,
		"stderr":                 call.Stderr,
		"error":                  call.Error,
		"command":                call.Command,
		"policy_reason":          call.PolicyReason,
		"policy_target_summary":  call.PolicyTargetSummary,
		"policy_headless_reason": call.PolicyHeadlessReason,
		"shell_reason":           call.ShellReason,
	})
	return RuntimeToolCall{
		ID:                             call.ID,
		SessionID:                      call.SessionID,
		TurnID:                         call.TurnID,
		MessageID:                      call.MessageID,
		Name:                           call.Name,
		Source:                         string(call.Source),
		CapabilityID:                   call.CapabilityID,
		JobID:                          call.JobID,
		Command:                        stringFromMap(redacted, "command"),
		Risk:                           call.Risk,
		PolicyReason:                   stringFromMap(redacted, "policy_reason"),
		PolicyMode:                     call.PolicyMode,
		PolicyProfile:                  call.PolicyProfile,
		PolicyHeadless:                 call.PolicyHeadless,
		PolicyHeadlessReason:           stringFromMap(redacted, "policy_headless_reason"),
		PolicyRuleID:                   call.PolicyRuleID,
		PolicyRuleSource:               call.PolicyRuleSource,
		PolicyScopeKind:                call.PolicyScopeKind,
		PolicyScopeValue:               call.PolicyScopeValue,
		PolicyTargetSummary:            stringFromMap(redacted, "policy_target_summary"),
		ShellRisk:                      call.ShellRisk,
		ShellReason:                    stringFromMap(redacted, "shell_reason"),
		ExitCode:                       call.ExitCode,
		JobStatus:                      call.JobStatus,
		JobStartedAt:                   jobStartedAt,
		JobFinishedAt:                  jobFinishedAt,
		Status:                         string(call.Status),
		InputSummary:                   stringFromMap(redacted, "input"),
		OutputSummary:                  stringFromMap(redacted, "output"),
		ModelContent:                   stringFromMap(redacted, "model_content"),
		Structured:                     stringFromMap(redacted, "structured"),
		Stdout:                         stringFromMap(redacted, "stdout"),
		Stderr:                         stringFromMap(redacted, "stderr"),
		OutputRefs:                     append([]string(nil), call.OutputRefs...),
		ArtifactRefs:                   append([]string(nil), call.ArtifactRefs...),
		DiffRefs:                       append([]string(nil), call.DiffRefs...),
		IsError:                        call.IsError,
		Compacted:                      call.Compacted,
		CompactRef:                     call.CompactRef,
		CompactBoundaryID:              call.CompactBoundaryID,
		CompactOriginalEstimatedTokens: call.CompactOriginalEstimatedTokens,
		CompactedAt:                    compactedAt,
		StartedAt:                      call.StartedAt.UnixMilli(),
		FinishedAt:                     finishedAt,
		Error:                          stringFromMap(redacted, "error"),
	}
}

func classifyToolSource(name string) scheduler.ToolSource {
	toolName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case toolName == "bash", toolName == "job_output", toolName == "job_kill":
		return scheduler.ToolSourceShell
	case strings.HasPrefix(toolName, "mcp_"):
		return scheduler.ToolSourceMCP
	case toolName == "":
		return scheduler.ToolSourceUnknown
	default:
		return scheduler.ToolSourceBuiltin
	}
}

func capabilityIDForToolName(name string) string {
	toolName := strings.TrimSpace(name)
	lower := strings.ToLower(toolName)
	if strings.HasPrefix(lower, "mcp_") {
		rest := toolName[len("mcp_"):]
		var bestServer string
		for server := range mcptools.Tools() {
			prefix := server + "_"
			if strings.HasPrefix(rest, prefix) && len(server) > len(bestServer) {
				bestServer = server
			}
		}
		if bestServer != "" {
			return "mcp:" + bestServer + ":" + strings.TrimPrefix(rest, bestServer+"_")
		}
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
