package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type runtimeToolCallStore = *scheduler.Scheduler

func NewRuntimeToolCallStore() scheduler.Store {
	return scheduler.NewMemoryStore()
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
		_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:           call.ID,
			SessionID:    msg.SessionID,
			TurnID:       turnID,
			MessageID:    msg.ID,
			Name:         call.Name,
			Source:       classifyToolSource(call.Name),
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
			ID:        result.ToolCallID,
			SessionID: msg.SessionID,
			TurnID:    turnID,
			MessageID: msg.ID,
			Name:      result.Name,
			Source:    classifyToolSource(result.Name),
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
	return RuntimeToolCall{
		ID:            call.ID,
		SessionID:     call.SessionID,
		TurnID:        call.TurnID,
		MessageID:     call.MessageID,
		Name:          call.Name,
		Source:        string(call.Source),
		Status:        string(call.Status),
		InputSummary:  call.InputSummary,
		OutputSummary: call.OutputSummary,
		Stdout:        call.Stdout,
		Stderr:        call.Stderr,
		IsError:       call.IsError,
		StartedAt:     call.StartedAt.UnixMilli(),
		FinishedAt:    finishedAt,
		Error:         call.Error,
	}
}

func classifyToolSource(name string) scheduler.ToolSource {
	toolName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case toolName == "bash":
		return scheduler.ToolSourceShell
	case strings.HasPrefix(toolName, "mcp_"):
		return scheduler.ToolSourceMCP
	case toolName == "":
		return scheduler.ToolSourceUnknown
	default:
		return scheduler.ToolSourceBuiltin
	}
}
