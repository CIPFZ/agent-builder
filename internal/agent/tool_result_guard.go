package agent

import (
	"log/slog"
	"slices"
	"sync"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
)

// ToolResultGuard implements Layer 1-3 of the tool result persistence pipeline.
type ToolResultGuard struct {
	config    config.ToolResultGuardConfig
	sessionID string
	persister *ResultPersister
	turnTotal int
	mu        sync.Mutex
}

// NewToolResultGuard creates a new ToolResultGuard.
func NewToolResultGuard(cfg config.ToolResultGuardConfig, sessionID string) *ToolResultGuard {
	return &ToolResultGuard{
		config:    cfg,
		sessionID: sessionID,
		persister: NewResultPersister(cfg.ResultsDir, cfg.PerSessionMaxBytes, cfg.TTLDays),
	}
}

// Process runs the guard pipeline on a tool result.
func (g *ToolResultGuard) Process(result message.ToolResult) message.ToolResult {
	if !g.config.Enabled {
		return result
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Layer 1: Exemption check
	if slices.Contains(g.config.ExemptTools, result.Name) {
		return result
	}

	// Layer 2: Single-result truncation
	threshold := g.resolveThreshold(result.Name)
	if len(result.Content) > threshold {
		slog.Info("ToolResultGuard: single result truncation",
			"tool", result.Name,
			"tool_call_id", result.ToolCallID,
			"size", len(result.Content),
			"threshold", threshold,
		)
		return g.truncateAndPersist(result, threshold, "single")
	}

	g.turnTotal += len(result.Content)

	// Layer 3: Turn budget enforcement
	if g.turnTotal > g.config.TurnBudget {
		slog.Info("ToolResultGuard: turn budget spill",
			"tool", result.Name,
			"tool_call_id", result.ToolCallID,
			"turn_total", g.turnTotal,
			"turn_budget", g.config.TurnBudget,
		)
		return g.truncateAndPersist(result, 0, "turn_budget")
	}

	return result
}

// ResetTurn resets the turn-level character counter.
func (g *ToolResultGuard) ResetTurn() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.turnTotal = 0
}

func (g *ToolResultGuard) resolveThreshold(toolName string) int {
	if t, ok := g.config.PerTool[toolName]; ok {
		return t
	}
	return g.config.MaxResultChars
}

func (g *ToolResultGuard) truncateAndPersist(result message.ToolResult, threshold int, reason string) message.ToolResult {
	originalSize := int64(len(result.Content))

	// First write full content to disk
	storedPath, err := g.persister.Persist(g.sessionID, result.ToolCallID, result.Content)
	if err != nil {
		slog.Warn("ToolResultGuard: persist failed, using inline truncation",
			"tool", result.Name,
			"tool_call_id", result.ToolCallID,
			"error", err,
		)
		result.Content = fallbackTruncate(result.Content, g.config.MaxResultChars)
		result.TruncatedBy = reason
		result.OriginalSize = originalSize
		return result
	}

	slog.Info("ToolResultGuard: persisted large result to disk",
		"tool", result.Name,
		"tool_call_id", result.ToolCallID,
		"original_size", originalSize,
		"stored_path", storedPath,
		"reason", reason,
	)

	// Then build the preview with the actual path.
	// threshold=0 means "always truncate" (used by turn_budget).
	preview, _ := Truncate(result.Content, threshold, storedPath)
	result.Content = preview
	result.StoredPath = storedPath
	result.OriginalSize = originalSize
	result.TruncatedBy = reason
	g.turnTotal += len(result.Content)
	return result
}

func fallbackTruncate(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	preview := content[:maxChars]
	return preview + "\n\n[Truncated: tool response was too large and could not be persisted to disk.]"
}

// CleanupOldFiles triggers background TTL cleanup of old result files.
func (g *ToolResultGuard) CleanupOldFiles() {
	slog.Info("ToolResultGuard: starting background cleanup",
		"results_dir", g.config.ResultsDir,
		"ttl_days", g.config.TTLDays,
	)
	go g.persister.CleanupOldFiles()
}
