package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
)

var errSessionMemoryCompactNotApplicable = errors.New("session memory compact is not applicable")

const (
	sessionMemoryTailMinTokens = 10_000
	sessionMemoryTailMinText   = 5
	sessionMemoryTailMaxTokens = 40_000
)

func (r *runtimeService) runSessionMemoryCompact(ctx context.Context, req RuntimeContextActionRequest, model string) (contextmgr.CompactResult, message.Message, error) {
	if !r.beginCompactOperation(req.SessionID) {
		return contextmgr.CompactResult{}, message.Message{}, errors.New("该会话正在整理上下文，请等待当前操作完成")
	}
	defer r.endCompactOperation(req.SessionID)
	gov := r.contextGovernanceFor(ctx, req.SessionID, model)
	if !gov.SessionMemoryEnabled {
		return contextmgr.CompactResult{}, message.Message{}, errSessionMemoryCompactNotApplicable
	}
	r.waitForSessionMemoryExtraction(ctx, req.SessionID, 15*time.Second)
	memory, err := r.contextStore.LatestCompletedSessionMemory(ctx, req.SessionID)
	if err != nil || strings.TrimSpace(memory.Content) == "" || strings.TrimSpace(memory.LastSummarizedMessageID) == "" {
		return contextmgr.CompactResult{}, message.Message{}, errSessionMemoryCompactNotApplicable
	}
	if r.runtime == nil || r.workspace == nil {
		return contextmgr.CompactResult{}, message.Message{}, errSessionMemoryCompactNotApplicable
	}
	ws, err := r.runtime.GetWorkspace(r.workspace.ID)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	canonical, err := ws.Messages.List(ctx, req.SessionID)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	anchor := -1
	for i := range canonical {
		if canonical[i].ID == memory.LastSummarizedMessageID {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return contextmgr.CompactResult{}, message.Message{}, errSessionMemoryCompactNotApplicable
	}
	tailStart := sessionMemoryTailStart(canonical, anchor)
	preserved := make([]string, 0, len(canonical)-tailStart)
	postTokens := estimateRuntimeTokens(memory.Content)
	for i := tailStart; i < len(canonical); i++ {
		if canonical[i].IsSummaryMessage || canonical[i].Role == message.System {
			continue
		}
		preserved = append(preserved, canonical[i].ID)
		postTokens += estimateRuntimeMessageTokens(canonical[i])
	}
	limits := r.currentRuntimeModelLimits(ctx, req.SessionID, model)
	thresholds := contextThresholds(limits.ContextWindow, limits.MaxOutputTokens, gov.AutoCompactPercent)
	if postTokens >= thresholds.AutoCompactAt || len(preserved) == 0 {
		return contextmgr.CompactResult{}, message.Message{}, errSessionMemoryCompactNotApplicable
	}
	summarized := make([]string, 0, tailStart)
	preTokens := 0
	for i := 0; i < tailStart; i++ {
		if canonical[i].IsSummaryMessage || canonical[i].Role == message.System {
			continue
		}
		summarized = append(summarized, canonical[i].ID)
		preTokens += estimateRuntimeMessageTokens(canonical[i])
	}
	cutoff := preserved[len(preserved)-1]
	now := time.Now().UTC().UnixMilli()
	boundary := contextmgr.Boundary{
		ID:        fmt.Sprintf("ctxbound_session_memory_%s_%d", stableRuntimeIDPart(req.TurnID), now),
		SessionID: req.SessionID, TurnID: req.TurnID, ProjectionID: req.ProjectionID,
		Kind: "session_memory", Trigger: firstNonEmpty(req.Reason, "auto"), Status: contextmgr.ProjectionStatusCompleted,
		MessageRefs: summarized, PreservedMessageRefs: preserved, BoundaryCutoffMessageID: cutoff,
		SummaryMode: "session_memory", MemoryRevision: memory.Revision, BudgetBefore: &contextmgr.BudgetReport{TotalEstimatedTokens: preTokens},
		BudgetAfter: &contextmgr.BudgetReport{TotalEstimatedTokens: postTokens}, CreatedAt: now, CompletedAt: now,
	}
	r.emitCompactEvent("compact.started", req.SessionID, req.TurnID, map[string]any{
		"boundary_id": boundary.ID, "kind": boundary.Kind, "trigger": boundary.Trigger,
		"summary": "session memory compact started",
	})
	conn, err := r.workspaceDB(ctx)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	summary, err := message.NewService(db.New(tx)).Create(ctx, req.SessionID, message.CreateMessageParams{
		Role: message.User, IsSummaryMessage: true,
		Parts:    []message.ContentPart{message.TextContent{Text: "<session-memory>\n" + memory.Content + "\n</session-memory>"}},
		Metadata: map[string]string{"synthetic": "true", "summary_mode": "session_memory", "memory_revision": fmt.Sprintf("%d", memory.Revision)},
	})
	if err != nil {
		_ = tx.Rollback()
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	boundary.SummaryMessageID = summary.ID
	boundary.SummaryRef = "runtime://messages/" + summary.ID
	if _, err = r.contextStore.WithTx(tx).UpsertBoundary(ctx, boundary); err != nil {
		_ = tx.Rollback()
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET summary_message_id = ?, updated_at = ? WHERE id = ?`, summary.ID, time.Now().Unix(), req.SessionID); err != nil {
		_ = tx.Rollback()
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	if err = tx.Commit(); err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	if sess, getErr := r.runtime.GetSession(ctx, r.workspace.ID, req.SessionID); getErr == nil {
		sess.SummaryMessageID = summary.ID
		_, _ = r.runtime.SaveSession(ctx, r.workspace.ID, sess)
	}
	r.emitCompactEvent("compact.completed", req.SessionID, req.TurnID, map[string]any{
		"boundary_id": boundary.ID, "kind": boundary.Kind, "trigger": boundary.Trigger,
		"pre_tokens": preTokens, "post_tokens": postTokens, "summarized_count": len(summarized),
		"preserved_count": len(preserved), "memory_revision": memory.Revision,
	})
	// Stale tracking is auxiliary; the committed boundary remains valid.
	_ = r.runtime.MarkSessionReadFilesStale(ctx, r.workspace.ID, req.SessionID, "compact_boundary")
	r.runCompactHook(ctx, r.workspace.ID, "PostCompact", req.SessionID, req.TurnID, boundary.ID, boundary.Trigger)
	r.resetCompactFailures(req.SessionID)
	return contextmgr.CompactResult{Boundary: boundary}, summary, nil
}

func (r *runtimeService) waitForSessionMemoryExtraction(ctx context.Context, sessionID string, timeout time.Duration) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		r.sessionMemoryMu.Lock()
		active := r.sessionMemoryActive[sessionID]
		r.sessionMemoryMu.Unlock()
		if !active {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

func sessionMemoryTailStart(messages []message.Message, anchor int) int {
	start := anchor + 1
	if start >= len(messages) {
		start = anchor
	}
	tokens, textCount := 0, 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimateRuntimeMessageTokens(messages[i])
		if i < start && tokens >= sessionMemoryTailMinTokens && textCount >= sessionMemoryTailMinText && tokens+msgTokens > sessionMemoryTailMaxTokens {
			break
		}
		start = i
		tokens += msgTokens
		if strings.TrimSpace(messages[i].Content().Text) != "" {
			textCount++
		}
		if i <= anchor && tokens >= sessionMemoryTailMinTokens && textCount >= sessionMemoryTailMinText {
			break
		}
	}
	// Never start on a tool result when its assistant tool-use is immediately
	// before the selected tail.
	if start > 0 && start < len(messages) && messages[start].Role == message.Tool && messages[start-1].Role == message.Assistant {
		start--
	}
	return start
}
