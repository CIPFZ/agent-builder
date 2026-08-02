package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/message"
)

const (
	sessionMemoryInitialTokens = 10_000
	sessionMemoryGrowthTokens  = 5_000
	sessionMemoryMinToolCalls  = 3
	sessionMemoryMaxTokens     = 20_000
)

var sessionMemorySections = []string{
	"Current objective", "User requirements and corrections", "Decisions and rationale",
	"Current state", "Files, symbols, commands and identifiers", "Errors and resolutions",
	"Pending work", "Exact next steps",
}

func (r *runtimeService) scheduleSessionMemoryExtraction(sessionID, turnID, messageID string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(messageID) == "" {
		return
	}
	r.sessionMemoryMu.Lock()
	if r.sessionMemoryActive == nil {
		r.sessionMemoryActive = make(map[string]bool)
	}
	if r.sessionMemoryActive[sessionID] {
		r.sessionMemoryMu.Unlock()
		return
	}
	r.sessionMemoryActive[sessionID] = true
	r.sessionMemoryMu.Unlock()
	go func() {
		defer func() {
			r.sessionMemoryMu.Lock()
			delete(r.sessionMemoryActive, sessionID)
			r.sessionMemoryMu.Unlock()
		}()
		_ = r.extractSessionMemory(context.Background(), sessionID, turnID, messageID)
	}()
}

func (r *runtimeService) extractSessionMemory(ctx context.Context, sessionID, turnID, completedAssistantID string) error {
	if err := r.ensureContextManager(ctx); err != nil {
		return err
	}
	gov := r.contextGovernanceFor(ctx, sessionID, "")
	if !gov.SessionMemoryEnabled || r.runtime == nil || r.workspace == nil {
		return nil
	}
	ws, err := r.runtime.GetWorkspace(r.workspace.ID)
	if err != nil {
		return err
	}
	messages, err := ws.Messages.List(ctx, sessionID)
	if err != nil {
		return err
	}
	completedIndex := -1
	for i := range messages {
		if messages[i].ID == completedAssistantID {
			completedIndex = i
			break
		}
	}
	if completedIndex < 0 || messages[completedIndex].Role != message.Assistant || messages[completedIndex].IsSummaryMessage || !messages[completedIndex].IsFinished() {
		return nil
	}
	finishReason := messages[completedIndex].FinishReason()
	if finishReason != message.FinishReasonEndTurn && finishReason != message.FinishReasonMaxTokens {
		return nil
	}
	latest, latestErr := r.contextStore.LatestCompletedSessionMemory(ctx, sessionID)
	if latestErr != nil && !errors.Is(latestErr, sql.ErrNoRows) {
		return latestErr
	}
	start := 0
	if latestErr == nil && latest.LastSummarizedMessageID != "" {
		found := false
		for i := range messages {
			if messages[i].ID == latest.LastSummarizedMessageID {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return fmt.Errorf("session memory anchor %s is missing", latest.LastSummarizedMessageID)
		}
	}
	source := append([]message.Message(nil), messages[start:completedIndex+1]...)
	totalTokens, growthTokens, toolCalls := 0, 0, 0
	for i := range messages[:completedIndex+1] {
		totalTokens += estimateRuntimeMessageTokens(messages[i])
	}
	for i := range source {
		growthTokens += estimateRuntimeMessageTokens(source[i])
		toolCalls += len(source[i].ToolCalls())
	}
	if totalTokens < sessionMemoryInitialTokens || growthTokens < sessionMemoryGrowthTokens {
		return nil
	}
	naturalBoundary := len(messages[completedIndex].ToolCalls()) == 0
	if toolCalls < sessionMemoryMinToolCalls && !naturalBoundary {
		return nil
	}
	revisionNumber, err := r.contextStore.NextSessionMemoryRevision(ctx, sessionID)
	if err != nil {
		return err
	}
	revision := contextmgr.SessionMemoryRevision{
		ID:        fmt.Sprintf("session_memory_%s_%06d", stableRuntimeIDPart(sessionID), revisionNumber),
		SessionID: sessionID, TurnID: turnID, Revision: revisionNumber, Status: contextmgr.SessionMemoryStatusStarted,
		BaseRevision: latest.Revision, SourceMessageCount: len(source), SourceTokenEstimate: growthTokens,
		SourceToolCallCount: toolCalls, CreatedAt: time.Now().UTC().UnixMilli(),
	}
	if status, statusErr := r.Status(ctx); statusErr == nil {
		revision.Provider = status.Provider
		revision.Model = status.Model
	}
	if _, err := r.contextStore.UpsertSessionMemoryRevision(ctx, revision); err != nil {
		return err
	}
	input := source
	if strings.TrimSpace(latest.Content) != "" {
		input = append([]message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Previous Session Memory:\n\n" + latest.Content}}}}, source...)
	}
	useSmall := gov.SummaryModel == config.ContextGovernanceSummaryModelSmall
	request := agent.CompactSummaryRequest{
		SessionID: sessionID, TurnID: turnID, Messages: input, UseSmallModel: useSmall,
		Instructions: sessionMemoryInstructions(),
	}
	var result agent.CompactSummaryResult
	var summaryErr error
	if r.sessionMemoryGenerator != nil {
		result, summaryErr = r.sessionMemoryGenerator(ctx, request)
	} else {
		result, summaryErr = r.runtime.GenerateCompactSummary(ctx, r.workspace.ID, request)
	}
	if summaryErr != nil {
		return r.failSessionMemoryRevision(ctx, revision, summaryErr)
	}
	content := strings.TrimSpace(result.Summary)
	if err := validateSessionMemory(content); err != nil {
		return r.failSessionMemoryRevision(ctx, revision, err)
	}
	hash := sha256.Sum256([]byte(content))
	revision.Status = contextmgr.SessionMemoryStatusCompleted
	revision.Content = content
	revision.ContentHash = "sha256:" + hex.EncodeToString(hash[:])
	revision.LastSummarizedMessageID = completedAssistantID
	revision.CompletedAt = time.Now().UTC().UnixMilli()
	_, err = r.contextStore.UpsertSessionMemoryRevision(ctx, revision)
	return err
}

func (r *runtimeService) failSessionMemoryRevision(ctx context.Context, revision contextmgr.SessionMemoryRevision, cause error) error {
	revision.Status = contextmgr.SessionMemoryStatusFailed
	revision.Error = cause.Error()
	revision.CompletedAt = time.Now().UTC().UnixMilli()
	_, _ = r.contextStore.UpsertSessionMemoryRevision(ctx, revision)
	return cause
}

func sessionMemoryInstructions() string {
	return "Update the Session Memory using exactly these Markdown sections:\n\n## " + strings.Join(sessionMemorySections, "\n\n## ") + "\n\nPreserve exact user corrections, file paths, symbols, commands, identifiers, errors, pending work, and next steps. Do not call tools."
}

func validateSessionMemory(content string) error {
	if content == "" {
		return errors.New("session memory output is empty")
	}
	if estimateRuntimeTokens(content) > sessionMemoryMaxTokens {
		return fmt.Errorf("session memory exceeds %d tokens", sessionMemoryMaxTokens)
	}
	for _, section := range sessionMemorySections {
		if !strings.Contains(content, "## "+section) {
			return fmt.Errorf("session memory is missing section %q", section)
		}
	}
	return nil
}

func stableRuntimeIDPart(value string) string {
	return strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_").Replace(strings.TrimSpace(value))
}
