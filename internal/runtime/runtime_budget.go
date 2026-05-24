package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const defaultRuntimeContextWindow = 200000

func (r *runtimeService) computeRuntimeBudget(ctx context.Context, sessionID, turnID, model string, promptLength int, contextSummary *RuntimeTurnContextSummary) RuntimeBudgetReport {
	report := RuntimeBudgetReport{
		SessionID:     sessionID,
		TurnID:        turnID,
		Model:         model,
		ContextWindow: defaultRuntimeContextWindow,
		UpdatedAt:     time.Now().UTC().UnixMilli(),
	}
	if promptLength > 0 {
		report.InputBudget.Count = 1
		report.InputBudget.EstimatedTokens = estimateRuntimeTokens(strings.Repeat("x", promptLength))
	}
	if r.runtime != nil && r.workspace != nil && sessionID != "" {
		if msgs, err := r.runtime.ListSessionMessages(ctx, r.workspace.ID, sessionID); err == nil {
			report.Messages.Count = len(msgs)
			for _, msg := range msgs {
				runtimeMsg := toRuntimeMessage(toProtoMessage(msg))
				report.Messages.EstimatedTokens += estimateRuntimeTokens(runtimeMsg.Content)
				for _, part := range runtimeMsg.Parts {
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Text)
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Thinking)
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Input)
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Content)
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Data)
					report.Messages.EstimatedTokens += estimateRuntimeTokens(part.Metadata)
				}
			}
		}
	}
	if contextSummary != nil {
		report.ContextSources.Count = contextSummary.AvailableCount
		report.ContextSources.EstimatedTokens = contextSummary.TokenEstimate
	}
	if calls, err := r.TurnToolCalls(ctx, turnID); err == nil {
		report.ToolOutputs.Count = len(calls.ToolCalls)
		for _, call := range calls.ToolCalls {
			report.ToolOutputs.EstimatedTokens += estimateRuntimeTokens(firstNonEmpty(call.OutputSummary, call.ModelContent, call.Structured, call.Stdout, call.Stderr, call.Error))
		}
	}
	if caps, err := r.Capabilities(ctx); err == nil {
		report.ToolSchemas.Count = len(caps.Capabilities)
		for _, cap := range caps.Capabilities {
			if cap.Kind == "skill" {
				report.Skills.Count++
				report.Skills.EstimatedTokens += estimateRuntimeTokens(cap.Name + " " + cap.Description)
				continue
			}
			if strings.HasPrefix(cap.Kind, "mcp") {
				report.MCP.Count++
				report.MCP.EstimatedTokens += estimateRuntimeTokens(cap.Name + " " + cap.Description)
				continue
			}
			report.ToolSchemas.EstimatedTokens += estimateRuntimeTokens(cap.Name + " " + cap.Description)
		}
	}
	report.TotalEstimatedTokens = report.InputBudget.EstimatedTokens +
		report.Messages.EstimatedTokens +
		report.ContextSources.EstimatedTokens +
		report.ToolSchemas.EstimatedTokens +
		report.Skills.EstimatedTokens +
		report.MCP.EstimatedTokens +
		report.ToolOutputs.EstimatedTokens +
		report.SelectedToolSchemas.EstimatedTokens
	return report
}

func (r *runtimeService) publishBudgetUpdated(sessionID, turnID string, budget RuntimeBudgetReport) {
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventBudgetUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: map[string]any{
			"total_estimated_tokens": budget.TotalEstimatedTokens,
			"context_window":         budget.ContextWindow,
			"messages":               budget.Messages,
			"context_sources":        budget.ContextSources,
			"tool_schemas":           budget.ToolSchemas,
			"skills":                 budget.Skills,
			"mcp":                    budget.MCP,
			"tool_outputs":           budget.ToolOutputs,
			"selected_tool_schemas":  budget.SelectedToolSchemas,
			"omitted_tool_schemas":   budget.OmittedToolSchemas,
			"summary":                fmt.Sprintf("%d estimated tokens", budget.TotalEstimatedTokens),
		},
	})
}

func estimateRuntimeTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	tokens := (runes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func runtimeBudgetReportFromPayload(payload map[string]any) *RuntimeBudgetReport {
	report := &RuntimeBudgetReport{
		SessionID:            stringFromMap(payload, "sessionId"),
		TurnID:               stringFromMap(payload, "turnId"),
		Model:                stringFromMap(payload, "model"),
		ContextWindow:        intFromMap(payload, "contextWindow"),
		TotalEstimatedTokens: intFromMap(payload, "totalEstimatedTokens"),
		UpdatedAt:            int64(intFromMap(payload, "updatedAt")),
	}
	report.InputBudget = runtimeBudgetBucketFromPayload(payload, "inputBudget")
	report.Messages = runtimeBudgetBucketFromPayload(payload, "messages")
	report.ContextSources = runtimeBudgetBucketFromPayload(payload, "contextSources")
	report.ToolSchemas = runtimeBudgetBucketFromPayload(payload, "toolSchemas")
	report.Skills = runtimeBudgetBucketFromPayload(payload, "skills")
	report.MCP = runtimeBudgetBucketFromPayload(payload, "mcp")
	report.ToolOutputs = runtimeBudgetBucketFromPayload(payload, "toolOutputs")
	report.SelectedToolSchemas = runtimeBudgetBucketFromPayload(payload, "selectedToolSchemas")
	report.OmittedToolSchemas = runtimeBudgetBucketFromPayload(payload, "omittedToolSchemas")
	return report
}

func runtimeBudgetBucketFromPayload(payload map[string]any, key string) RuntimeBudgetBucket {
	raw, ok := payload[key].(map[string]any)
	if !ok {
		return RuntimeBudgetBucket{}
	}
	return RuntimeBudgetBucket{
		Count:           intFromMap(raw, "count"),
		EstimatedTokens: intFromMap(raw, "estimatedTokens"),
	}
}
