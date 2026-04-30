package queryengine

import (
	"context"
	"errors"
	"fmt"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"strings"
	"time"
)

type State struct {
	ActiveRunID                   string
	LastRunID                     string
	LastEvent                     string
	LastError                     string
	LastSessionID                 string
	PermissionDenials             []PermissionDenial
	LastAssistantReply            string
	LastUserInput                 string
	MessageCount                  int
	LastTurnStartedAt             time.Time
	LastTurnCompletedAt           time.Time
	LastTurnDuration              time.Duration
	StreamDeltaCount              int
	ActiveAssistantText           string
	StreamEventCount              int
	LastStreamEvent               string
	RecentStreamEvents            []string
	LastPromptTokens              int
	LastCompletionTokens          int
	LastTotalTokens               int
	TotalEstimatedTokens          int
	TokenBudget                   int
	BudgetExceeded                bool
	TurnCount                     int
	LastInputMode                 string
	LastCommandName               string
	LastImmediateMessageCount     int
	CompactBoundaryCount          int
	LastCompactBoundaryID         string
	LastModelPassCount            int
	MaxTurns                      int
	MaxTurnsExceeded              bool
	LastEstimatedContextTokens    int
	ContextWindowTokens           int
	WarningThresholdTokens        int
	ErrorThresholdTokens          int
	AutoCompactThresholdTokens    int
	BlockingThresholdTokens       int
	IsAboveWarningThreshold       bool
	IsAboveErrorThreshold         bool
	IsAboveAutoCompactThreshold   bool
	IsAtBlockingContextLimit      bool
	LastCompactionReason          string
	LastCompactionOriginalCount   int
	LastCompactionResultCount     int
	LastCompactionPhase           string
	LastCompactionReplayExecuted  bool
	LastCompactionReplayCount     int
	LastCompactionMemorySaved     bool
	LastCompactionSummaryID       string
	LastCompactionCleanupExecuted bool
	LastCompactionCleanupCount    int
}

func (q *QueryEngine) State() State {
	q.stateMu.RLock()
	defer q.stateMu.RUnlock()

	cloned := q.state
	cloned.PermissionDenials = append([]PermissionDenial(nil), q.state.PermissionDenials...)
	return cloned
}

func (q *QueryEngine) Interrupt() {
	q.cancelMu.Lock()
	cancel := q.cancel
	q.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (q *QueryEngine) emit(sink EventSink, event Event) error {
	q.recordEvent(event)
	if sink == nil {
		return nil
	}
	return sink.Emit(event)
}

func (q *QueryEngine) emitRunError(sink EventSink, event Event) {
	q.recordEvent(event)
	if sink == nil {
		return
	}
	_ = sink.Emit(event)
}

func isApprovalRequiredError(err error) bool {
	var approvalErr *ApprovalRequiredError
	return errors.As(err, &approvalErr)
}

func (q *QueryEngine) beginRun(parent context.Context, runID, sessionID string) (context.Context, func()) {
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithCancel(parent)
	q.cancelMu.Lock()
	q.cancel = cancel
	q.cancelRun = runID
	q.cancelMu.Unlock()

	q.ensureMutableMessages(sessionID)
	q.msgMu.RLock()
	messageCount := len(q.messages[sessionID])
	q.msgMu.RUnlock()
	sess, _ := q.sessions.GetByID(sessionID)

	q.stateMu.Lock()
	q.state.ActiveRunID = runID
	q.state.LastRunID = runID
	q.state.LastSessionID = sessionID
	q.state.MessageCount = messageCount
	q.state.LastTurnStartedAt = startedAt
	q.state.LastTurnCompletedAt = time.Time{}
	q.state.LastTurnDuration = 0
	q.state.StreamDeltaCount = 0
	q.state.ActiveAssistantText = ""
	q.state.StreamEventCount = 0
	q.state.LastStreamEvent = ""
	q.state.RecentStreamEvents = nil
	q.state.TokenBudget = q.tokenBudget
	q.state.MaxTurns = q.effectiveMaxTurns(sess)
	q.state.MaxTurnsExceeded = false
	q.state.LastModelPassCount = 0
	q.stateMu.Unlock()

	return ctx, func() {
		q.cancelMu.Lock()
		if q.cancelRun == runID {
			q.cancel = nil
			q.cancelRun = ""
		}
		q.cancelMu.Unlock()

		q.stateMu.Lock()
		if q.state.ActiveRunID == runID {
			q.state.ActiveRunID = ""
		}
		q.state.LastTurnCompletedAt = time.Now().UTC()
		if !q.state.LastTurnStartedAt.IsZero() {
			q.state.LastTurnDuration = q.state.LastTurnCompletedAt.Sub(q.state.LastTurnStartedAt)
			if q.state.LastTurnDuration <= 0 {
				q.state.LastTurnDuration = time.Nanosecond
			}
		}
		q.stateMu.Unlock()
	}
}

func (q *QueryEngine) recordEvent(event Event) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()

	q.state.LastRunID = event.RunID
	q.state.LastEvent = event.Type
	if event.Session.ID != "" {
		q.state.LastSessionID = event.Session.ID
	}
	if event.Error != "" {
		q.state.LastError = event.Error
	}
}

func (q *QueryEngine) recordPermissionDenial(denial PermissionDenial) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.PermissionDenials = append(q.state.PermissionDenials, denial)
}

func (q *QueryEngine) setLastAssistantReply(content string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastAssistantReply = content
}

func (q *QueryEngine) recordAssistantDelta(delta string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.StreamDeltaCount++
	q.state.ActiveAssistantText += delta
}

func (q *QueryEngine) clearActiveAssistantText() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.ActiveAssistantText = ""
}

func (q *QueryEngine) recordStreamEvent(event llm.StreamEvent) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.StreamEventCount++
	q.state.LastStreamEvent = event.Type
	entry := event.Type
	if event.ToolName != "" {
		entry += ":" + event.ToolName
	}
	q.state.RecentStreamEvents = append(q.state.RecentStreamEvents, entry)
	if len(q.state.RecentStreamEvents) > 12 {
		q.state.RecentStreamEvents = append([]string(nil), q.state.RecentStreamEvents[len(q.state.RecentStreamEvents)-12:]...)
	}
}

func (q *QueryEngine) recordUsageEstimate(sessionID, completion string) {
	promptTokens := estimateMessagesTokens(q.Messages(sessionID))
	completionTokens := estimateTextTokens(completion)
	total := promptTokens + completionTokens

	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastPromptTokens = promptTokens
	q.state.LastCompletionTokens = completionTokens
	q.state.LastTotalTokens = total
	q.state.TotalEstimatedTokens += total
	q.state.TurnCount++
	q.state.TokenBudget = q.tokenBudget
	q.state.BudgetExceeded = q.tokenBudget > 0 && q.state.TotalEstimatedTokens > q.tokenBudget
}

func (q *QueryEngine) recordModelPass() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastModelPassCount++
}

func (q *QueryEngine) recordMaxTurnsExceeded() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.MaxTurnsExceeded = true
}

func (q *QueryEngine) recordInputProcessing(result ProcessResult) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()

	q.state.LastInputMode = strings.TrimSpace(result.InputMode)
	q.state.LastCommandName = strings.TrimSpace(result.CommandName)
	q.state.LastImmediateMessageCount = len(result.Messages)
}

func (q *QueryEngine) recordCompactBoundary(boundary session.Message) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.CompactBoundaryCount++
	q.state.LastCompactBoundaryID = boundary.ID
}

func (q *QueryEngine) recordCompactionAnalysis(analysis compaction.Analysis) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastEstimatedContextTokens = analysis.EstimatedTokens
	q.state.ContextWindowTokens = analysis.ContextWindowTokens
	q.state.WarningThresholdTokens = analysis.WarningThreshold
	q.state.ErrorThresholdTokens = analysis.ErrorThreshold
	q.state.AutoCompactThresholdTokens = analysis.AutoCompactThreshold
	q.state.BlockingThresholdTokens = analysis.BlockingThreshold
	q.state.IsAboveWarningThreshold = analysis.IsAboveWarningThreshold
	q.state.IsAboveErrorThreshold = analysis.IsAboveErrorThreshold
	q.state.IsAboveAutoCompactThreshold = analysis.IsAboveAutoCompactThreshold
	q.state.IsAtBlockingContextLimit = analysis.IsAtBlockingLimit
}

func (q *QueryEngine) recordCompactionResult(result compaction.Result) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionReason = string(result.Reason)
	q.state.LastCompactionOriginalCount = result.OriginalCount
	q.state.LastCompactionResultCount = result.CompactedCount
}

func (q *QueryEngine) recordCompactionPhase(phase string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionPhase = phase
}

func (q *QueryEngine) recordCompactionReplay(count int) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionReplayExecuted = true
	q.state.LastCompactionReplayCount = count
}

func (q *QueryEngine) recordCompactionMemorySaved(summaryID string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionMemorySaved = true
	q.state.LastCompactionSummaryID = summaryID
}

func (q *QueryEngine) recordCompactionCleanup(count int) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionCleanupExecuted = true
	q.state.LastCompactionCleanupCount = count
}

func (q *QueryEngine) emitImmediateMessages(sess session.Session, items []ImmediateMessage, sink EventSink) error {
	for _, item := range items {
		reply, err := q.sessions.AppendMessage(sess.ID, fallbackRole(item.Role), item.Content)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, reply)
		if reply.Role == "assistant" {
			q.setLastAssistantReply(reply.Content)
		}
		if err := q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			Message: &reply,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (q *QueryEngine) newCompactBoundary(sessionID string) session.Message {
	id := q.nextBoundaryID.Add(1)
	return session.Message{
		ID:        fmt.Sprintf("compact-%06d", id),
		SessionID: sessionID,
		Role:      "system",
		Content:   "[compact_boundary]",
		CreatedAt: time.Now().UTC(),
	}
}

func (q *QueryEngine) seedCompactBoundaryCounter() {
	var maxID uint64
	for _, sess := range q.sessions.ListSessions() {
		messages, ok := q.sessions.Messages(sess.ID)
		if !ok {
			continue
		}
		for _, message := range messages {
			if n, ok := compactBoundaryCounter(message.ID); ok && n > maxID {
				maxID = n
			}
		}
	}
	if maxID > 0 {
		q.nextBoundaryID.Store(maxID)
	}
}

func compactBoundaryCounter(id string) (uint64, bool) {
	if !strings.HasPrefix(id, "compact-") {
		return 0, false
	}
	suffix := strings.TrimPrefix(id, "compact-")
	if suffix == "" {
		return 0, false
	}
	var n uint64
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + uint64(r-'0')
	}
	return n, true
}

type textStreamCollector struct {
	sink              EventSink
	session           session.Session
	runID             string
	builder           strings.Builder
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	onDelta           func(string)
	onMessageEnd      func()
	onStreamEvent     func(llm.StreamEvent)
	includePartial    bool
}

func (c *textStreamCollector) OnEvent(event llm.StreamEvent) error {
	if c.onStreamEvent != nil {
		c.onStreamEvent(event)
	}
	if c.includePartial && c.sink != nil {
		if err := c.sink.Emit(Event{
			Type:            "stream.event",
			Session:         c.session,
			RunID:           c.runID,
			Delta:           event.Delta,
			ToolName:        event.ToolName,
			ToolInput:       normalizedToolInput(event.ToolInput, event.ToolInputObject),
			ToolInputObject: cloneAnyMap(event.ToolInputObject),
		}); err != nil {
			return err
		}
	}
	switch event.Type {
	case "text.delta":
		c.builder.WriteString(event.Delta)
		if c.onDelta != nil {
			c.onDelta(event.Delta)
		}
		if c.sink != nil {
			return c.sink.Emit(Event{
				Type:    "assistant.delta",
				Session: c.session,
				RunID:   c.runID,
				Delta:   event.Delta,
			})
		}
	case "message.end":
		if c.onMessageEnd != nil {
			c.onMessageEnd()
		}
		if c.ToolName == "" {
			if name, input, ok := parseToolCallBlock(c.builder.String()); ok {
				c.ToolName = name
				c.ToolInput = input
				c.builder.Reset()
			}
		}
		return nil
	case "tool.call":
		c.ToolName = event.ToolName
		c.ToolInput = normalizedToolInput(event.ToolInput, event.ToolInputObject)
		c.ToolInputObject = cloneAnyMap(event.ToolInputObject)
		c.ToolUseID = event.ToolUseID
		c.ProviderMessageID = event.ProviderMessageID
		return nil
	}
	return nil
}

func (c *textStreamCollector) Content() string {
	return c.builder.String()
}

func (q *QueryEngine) latestUserMessage(sessionID string) session.Message {
	messages := q.Messages(sessionID)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i]
		}
	}
	return session.Message{}
}

func (q *QueryEngine) effectiveMaxTurns(sess session.Session) int {
	if sess.Metadata.AgentMaxTurns > 0 {
		return sess.Metadata.AgentMaxTurns
	}
	if q.maxTurns <= 0 {
		return 100
	}
	return q.maxTurns
}

func (q *QueryEngine) restoreStateFromSession(snapshot session.RecoverySnapshot) {
	if snapshot.Session.ID == "" {
		return
	}
	if skills := snapshot.RecoveredInvokedSkills(); len(skills) > 0 {
		q.toolContextMu.Lock()
		appState := q.toolAppStates[snapshot.Session.ID]
		if appState == nil {
			appState = make(map[string]any)
		}
		for _, skill := range skills {
			tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
				SkillName: skill.SkillName,
				SkillPath: skill.SkillPath,
				Content:   skill.Content,
				InvokedAt: skill.InvokedAt,
				AgentID:   skill.AgentID,
			})
		}
		q.toolAppStates[snapshot.Session.ID] = appState
		q.toolContextMu.Unlock()
	}

	lastUserInput := ""
	if message, ok := snapshot.LastUserMessage(); ok {
		lastUserInput = message.Content
	}
	lastAssistantReply := ""
	if message, ok := snapshot.LastAssistantMessage(); ok {
		lastAssistantReply = message.Content
	}

	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastSessionID = snapshot.Session.ID
	q.state.MessageCount = len(snapshot.Continuation)
	if lastUserInput != "" {
		q.state.LastUserInput = lastUserInput
	}
	if lastAssistantReply != "" {
		q.state.LastAssistantReply = lastAssistantReply
	}
	if boundary, ok := snapshot.CompactBoundary(); ok {
		q.state.LastCompactBoundaryID = boundary.ID
		q.state.CompactBoundaryCount = 1
		q.state.LastCompactionPhase = "restored"
	}
	if summary, ok := snapshot.CompactionSummary(); ok {
		q.state.LastCompactionSummaryID = summary.ID
		q.state.LastCompactionPhase = "restored"
	}
	if snapshot.Metadata.LastCompactionReason != "" {
		q.state.LastCompactionReason = snapshot.Metadata.LastCompactionReason
		q.state.LastCompactionPhase = "restored"
	}
}
