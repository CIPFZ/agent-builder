package queryengine

import (
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
)

func (q *QueryEngine) compactorForSession(sessionID string) *compaction.Service {
	if q.compactor == nil {
		return nil
	}
	cfg := llm.ClaudeCompactionConfig(q.compactor.Config(), q.ResolvedMainLoopModelForSession(sessionID), q.modelCatalog)
	return compaction.NewService(cfg)
}
