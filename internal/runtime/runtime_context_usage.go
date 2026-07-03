package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/modelmeta"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	contextUsageLevelOK      = "ok"
	contextUsageLevelWarning = "warning"
	contextUsageLevelError   = "error"
)

type runtimeContextThresholds struct {
	OutputReserve     int
	EffectiveWindow   int
	AutoCompactAt     int
	WarningAt         int
	BlockingAt        int
	AutoCompactBuffer int
}

func (r *runtimeService) SessionContextUsage(ctx context.Context, sessionID string) (RuntimeContextUsage, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeContextUsage{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeContextUsage{}, errors.New("session id is required")
	}
	status, _ := r.Status(ctx)
	return r.computeContextUsage(ctx, sessionID, "", status.Model, 0, nil)
}

func (r *runtimeService) computeContextUsage(ctx context.Context, sessionID, turnID, model string, promptLength int, contextSummary *RuntimeTurnContextSummary) (RuntimeContextUsage, error) {
	r.mu.Lock()
	wsID := ""
	if r.workspace != nil {
		wsID = r.workspace.ID
	}
	r.mu.Unlock()
	if wsID == "" || r.runtime == nil {
		return RuntimeContextUsage{}, errors.New("runtime workspace is not available")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		if status, err := r.Status(ctx); err == nil {
			model = status.Model
		}
	}
	limits := r.currentRuntimeModelLimits(ctx, model)
	thresholds := contextThresholds(limits.ContextWindow, limits.MaxOutputTokens)
	budget := r.computeRuntimeBudget(ctx, sessionID, turnID, model, promptLength, contextSummary)

	messages, err := r.runtime.ListSessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeContextUsage{}, fmt.Errorf("failed to list Agent Builder session messages: %w", err)
	}
	boundaryAt, compactCount := r.latestCompletedFullBoundary(ctx, sessionID)
	anchorIndex := -1
	var anchorUsage int64
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if !messageCreatedAfterBoundary(msg.CreatedAt, boundaryAt) || msg.Role != message.Assistant || msg.IsSummaryMessage || msg.Usage.IsZero() || isSyntheticRuntimeMessage(msg) {
			continue
		}
		anchorIndex = i
		anchorUsage = msg.Usage.Total()
		break
	}

	estimated := anchorIndex < 0
	postBoundaryEstimate := 0
	for _, msg := range messages {
		if messageCreatedAfterBoundary(msg.CreatedAt, boundaryAt) {
			postBoundaryEstimate += estimateRuntimeMessageTokens(msg)
		}
	}
	anchorBased := 0
	if anchorIndex >= 0 {
		anchorBased = int(anchorUsage)
		for _, msg := range messages[anchorIndex+1:] {
			anchorBased += estimateRuntimeMessageTokens(msg)
		}
	} else {
		anchorBased = postBoundaryEstimate
	}
	localEstimate := budget.TotalEstimatedTokens - budget.Messages.EstimatedTokens + postBoundaryEstimate
	used := maxInt(anchorBased, localEstimate)
	used = clampInt(used, 0, limits.ContextWindow)

	percentUsed := percentRounded(used, limits.ContextWindow)
	percentLeft := percentRounded(maxInt(thresholds.AutoCompactAt-used, 0), thresholds.AutoCompactAt)
	level := contextUsageLevelOK
	if used >= thresholds.AutoCompactAt {
		level = contextUsageLevelError
	} else if used >= thresholds.WarningAt {
		level = contextUsageLevelWarning
	}

	return RuntimeContextUsage{
		SessionID:         sessionID,
		Model:             model,
		ContextWindow:     limits.ContextWindow,
		UsedTokens:        used,
		PercentUsed:       percentUsed,
		AutoCompactAt:     thresholds.AutoCompactAt,
		PercentLeft:       percentLeft,
		Level:             level,
		Estimated:         estimated,
		OutputReserve:     thresholds.OutputReserve,
		AutoCompactBuffer: thresholds.AutoCompactBuffer,
		Breakdown:         contextUsageBreakdown(used, limits.ContextWindow, thresholds, budget),
		CompactCount:      compactCount,
		UpdatedAt:         time.Now().UTC().UnixMilli(),
	}, nil
}

func (r *runtimeService) publishContextUsageUpdated(ctx context.Context, sessionID, turnID, model string, promptLength int, contextSummary *RuntimeTurnContextSummary) {
	usage, err := r.computeContextUsage(ctx, sessionID, turnID, model, promptLength, contextSummary)
	if err != nil {
		return
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventContextUsageUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload:   runtimeContextUsagePayload(usage),
	})
}

func contextThresholds(contextWindow, modelMaxOutputTokens int) runtimeContextThresholds {
	if contextWindow <= 0 {
		contextWindow = modelmeta.FallbackContextWindow
	}
	if modelMaxOutputTokens <= 0 {
		modelMaxOutputTokens = modelmeta.FallbackMaxOutput
	}
	outputReserve := minInt(modelMaxOutputTokens, 20000)
	effectiveWindow := maxInt(contextWindow-outputReserve, 1)
	autoBuffer := minInt(13000, contextWindow/10)
	warningBuffer := minInt(20000, contextWindow/10)
	blockingBuffer := minInt(3000, int(math.Round(float64(contextWindow)*0.02)))
	autoCompactAt := maxInt(effectiveWindow-autoBuffer, 1)
	warningAt := maxInt(autoCompactAt-warningBuffer, 0)
	blockingAt := maxInt(effectiveWindow-blockingBuffer, 1)
	return runtimeContextThresholds{
		OutputReserve:     outputReserve,
		EffectiveWindow:   effectiveWindow,
		AutoCompactAt:     autoCompactAt,
		WarningAt:         warningAt,
		BlockingAt:        blockingAt,
		AutoCompactBuffer: autoBuffer,
	}
}

func contextUsageBreakdown(used, contextWindow int, thresholds runtimeContextThresholds, budget RuntimeBudgetReport) []RuntimeContextCategory {
	tools := budget.ToolSchemas.EstimatedTokens + budget.SelectedToolSchemas.EstimatedTokens
	skills := budget.Skills.EstimatedTokens
	mcp := budget.MCP.EstimatedTokens
	memory := budget.ContextSources.EstimatedTokens
	other := tools + skills + mcp + memory
	messages := maxInt(used-other, 0)
	reserved := thresholds.OutputReserve + thresholds.AutoCompactBuffer
	free := maxInt(contextWindow-used-reserved, 0)
	return []RuntimeContextCategory{
		{Key: "tools", Label: "工具", Tokens: tools, Estimated: true},
		{Key: "skills", Label: "技能", Tokens: skills, Estimated: true},
		{Key: "mcp", Label: "MCP", Tokens: mcp, Estimated: true},
		{Key: "memory", Label: "记忆", Tokens: memory, Estimated: true},
		{Key: "messages", Label: "消息", Tokens: messages, Estimated: false},
		{Key: "reserved", Label: "预留", Tokens: reserved, Estimated: true},
		{Key: "free", Label: "剩余", Tokens: free, Estimated: true},
	}
}

func estimateRuntimeMessageTokens(msg message.Message) int {
	total := estimateRuntimeTokens(string(msg.Role))
	if msg.Model != "" {
		total += estimateRuntimeTokens(msg.Model)
	}
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case message.TextContent:
			total += estimateRuntimeTokens(p.Text)
		case message.ReasoningContent:
			total += estimateRuntimeTokens(p.Thinking)
		case message.ToolCall:
			total += estimateRuntimeTokens(p.Name) + estimateRuntimeTokens(p.Input)
		case message.ToolResult:
			total += estimateRuntimeTokens(p.Name) + estimateRuntimeTokens(p.Content) + estimateRuntimeTokens(p.Data) + estimateRuntimeTokens(p.Metadata)
		case message.ImageURLContent:
			total += 2000
			total += estimateRuntimeTokens(p.URL)
		case message.BinaryContent:
			total += 2000
			total += estimateRuntimeTokens(p.Path)
		case message.Finish:
			total += estimateRuntimeTokens(string(p.Reason)) + estimateRuntimeTokens(p.Message) + estimateRuntimeTokens(p.Details)
		}
	}
	return total
}

func isSyntheticRuntimeMessage(msg message.Message) bool {
	if msg.Metadata == nil {
		return false
	}
	for _, key := range []string{"synthetic", "isSynthetic", "runtimeSynthetic"} {
		if strings.EqualFold(msg.Metadata[key], "true") || msg.Metadata[key] == "1" {
			return true
		}
	}
	return false
}

func messageCreatedAfterBoundary(messageCreatedAt, boundaryAt int64) bool {
	if boundaryAt <= 0 {
		return true
	}
	return normalizeRuntimeTimestampMillis(messageCreatedAt) > normalizeRuntimeTimestampMillis(boundaryAt)
}

func normalizeRuntimeTimestampMillis(value int64) int64 {
	if value > 0 && value < 100_000_000_000 {
		return value * 1000
	}
	return value
}

func (r *runtimeService) latestCompletedFullBoundary(ctx context.Context, sessionID string) (int64, int) {
	conn, err := r.workspaceDB(ctx)
	if err != nil || conn == nil {
		return 0, 0
	}
	var latest sql.NullInt64
	var count int
	err = conn.QueryRowContext(ctx, `
SELECT COALESCE(MAX(COALESCE(completed_at, created_at)), 0), COUNT(*)
FROM runtime_context_boundaries
WHERE session_id = ? AND kind = 'full' AND status = 'completed'
`, sessionID).Scan(&latest, &count)
	if err != nil {
		return 0, 0
	}
	return latest.Int64, count
}

func runtimeContextUsagePayload(usage RuntimeContextUsage) map[string]any {
	data, err := json.Marshal(usage)
	if err != nil {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func percentRounded(value, total int) int {
	if total <= 0 {
		return 0
	}
	return clampInt(int(math.Round(float64(value)*100/float64(total))), 0, 100)
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
