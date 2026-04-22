package runtime

import (
	"context"
	"fmt"

	"myclaw/internal/llm"
)

type ContextSnapshot struct {
	Model               string
	UsedTokens          int
	ContextWindowTokens int
	UsagePercent        int
	CategoryLines       []string
}

func (r *Runner) AvailableModels(ctx context.Context) ([]llm.ModelDescriptor, error) {
	if r.options.ModelCatalog == nil {
		return nil, nil
	}
	return r.options.ModelCatalog.ListModels(ctx)
}

func (r *Runner) ContextSnapshot(sessionID string) (ContextSnapshot, error) {
	compactionSnapshot, err := r.CompactionSnapshot(sessionID)
	if err != nil {
		return ContextSnapshot{}, err
	}
	window := compactionSnapshot.Analysis.ContextWindowTokens
	used := compactionSnapshot.Analysis.EstimatedTokens
	if window <= 0 {
		return ContextSnapshot{
			Model:      r.ResolvedMainLoopModelForSession(sessionID),
			UsedTokens: used,
		}, nil
	}
	percent := used * 100 / window
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	freeSpace := window - used
	if freeSpace < 0 {
		freeSpace = 0
	}
	autoBuffer := window - compactionSnapshot.Analysis.AutoCompactThreshold
	lines := []string{
		fmt.Sprintf("Messages: %d tokens (%.1f%%)", used, percentOf(used, window)),
		fmt.Sprintf("Autocompact buffer: %d tokens (%.1f%%)", autoBuffer, percentOf(autoBuffer, window)),
		fmt.Sprintf("Free space: %d tokens (%.1f%%)", freeSpace, percentOf(freeSpace, window)),
	}
	return ContextSnapshot{
		Model:               r.ResolvedMainLoopModelForSession(sessionID),
		UsedTokens:          used,
		ContextWindowTokens: window,
		UsagePercent:        percent,
		CategoryLines:       lines,
	}, nil
}

func percentOf(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}
