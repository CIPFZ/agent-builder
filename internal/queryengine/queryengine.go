package queryengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/permissions"
	"myclaw/internal/prompt"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
)

type EventSink interface {
	Emit(Event) error
}

type Event struct {
	Type      string
	Session   session.Session
	RunID     string
	Message   *session.Message
	Delta     string
	ToolName  string
	ToolInput string
	Error     string
	Approval  *approval.Request
}

type Config struct {
	Sessions         *session.Manager
	Client           llm.Client
	WorkspaceLoader  *workspace.Loader
	ToolRegistry     *tools.Registry
	AgentManager     *agent.Manager
	InputProcessor   InputProcessor
	IncludePartialStreamEvents bool
	EstimatedTokenBudget int
	MaxTurns         int
	SnipReplay       func(session.Message, []session.Message) *SnipReplayResult
	PostCompactCleanup func(session.Message, []session.Message) *PostCompactCleanupResult
	PermissionPolicy permissions.Policy
	Compactor        *compaction.Service
	MemoryService    *memory.Service
	ApprovalManager  *approval.Manager
}

type ProcessResult struct {
	NormalizedInput string
	ShouldQuery     bool
	ResultText      string
	InputMode       string
	CommandName     string
	Messages        []ImmediateMessage
}

type ImmediateMessage struct {
	Role    string
	Content string
}

type InputProcessor interface {
	Process(context.Context, session.Session, string) (ProcessResult, error)
}

type noopInputProcessor struct{}

type InputProcessorFunc func(context.Context, session.Session, string) (ProcessResult, error)

func (f InputProcessorFunc) Process(ctx context.Context, sess session.Session, input string) (ProcessResult, error) {
	return f(ctx, sess, input)
}

func (noopInputProcessor) Process(_ context.Context, _ session.Session, input string) (ProcessResult, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return ProcessResult{}, nil
	}
	return ProcessResult{
		NormalizedInput: normalized,
		ShouldQuery:     true,
		InputMode:       "prompt",
	}, nil
}

type PermissionDenial struct {
	RunID     string
	SessionID string
	ToolName  string
	ToolInput string
	Reason    string
}

type ApprovalRequiredError struct {
	ToolName  string
	ToolInput string
	Reason    string
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("tool %s requires approval: %s", e.ToolName, e.Reason)
}

type SnipReplayResult struct {
	Messages []session.Message
	Executed bool
}

type PostCompactCleanupResult struct {
	Messages []session.Message
	Executed bool
}

type State struct {
	ActiveRunID        string
	LastRunID          string
	LastEvent          string
	LastError          string
	LastSessionID      string
	PermissionDenials  []PermissionDenial
	LastAssistantReply string
	LastUserInput      string
	MessageCount       int
	LastTurnStartedAt  time.Time
	LastTurnCompletedAt time.Time
	LastTurnDuration   time.Duration
	StreamDeltaCount   int
	ActiveAssistantText string
	StreamEventCount    int
	LastStreamEvent     string
	RecentStreamEvents  []string
	LastPromptTokens    int
	LastCompletionTokens int
	LastTotalTokens     int
	TotalEstimatedTokens int
	TokenBudget          int
	BudgetExceeded       bool
	TurnCount            int
	LastInputMode         string
	LastCommandName       string
	LastImmediateMessageCount int
	CompactBoundaryCount   int
	LastCompactBoundaryID  string
	LastModelPassCount     int
	MaxTurns               int
	MaxTurnsExceeded       bool
	LastEstimatedContextTokens int
	ContextWindowTokens        int
	WarningThresholdTokens     int
	ErrorThresholdTokens       int
	AutoCompactThresholdTokens int
	BlockingThresholdTokens    int
	IsAboveWarningThreshold    bool
	IsAboveErrorThreshold      bool
	IsAboveAutoCompactThreshold bool
	IsAtBlockingContextLimit   bool
	LastCompactionReason       string
	LastCompactionOriginalCount int
	LastCompactionResultCount   int
	LastCompactionPhase         string
	LastCompactionReplayExecuted bool
	LastCompactionReplayCount    int
	LastCompactionMemorySaved    bool
	LastCompactionSummaryID      string
	LastCompactionCleanupExecuted bool
	LastCompactionCleanupCount    int
}

type QueryEngine struct {
	nextRunID atomic.Uint64
	nextBoundaryID atomic.Uint64
	sessions  *session.Manager
	client    llm.Client
	workspace *workspace.Loader
	tools     *tools.Registry
	compactor *compaction.Service
	memory    *memory.Service
	approvals *approval.Manager
	tokenBudget int
	maxTurns  int
	policy    permissions.Policy
	policyMu  sync.RWMutex
	policies  map[string]permissions.Policy
	stateMu   sync.RWMutex
	state     State
	cancelMu  sync.Mutex
	cancel    context.CancelFunc
	cancelRun string
	msgMu     sync.RWMutex
	messages  map[string][]session.Message
	inputs    InputProcessor
	includePartialStreamEvents bool
	snipReplay func(session.Message, []session.Message) *SnipReplayResult
	postCompactCleanup func(session.Message, []session.Message) *PostCompactCleanupResult
}

func New(cfg Config) *QueryEngine {
	sessionsMgr := cfg.Sessions
	if sessionsMgr == nil {
		sessionsMgr = session.NewManager(nil)
	}
	client := cfg.Client
	if client == nil {
		client = llm.NewMockClient()
	}
	workspaceLoader := cfg.WorkspaceLoader
	if workspaceLoader == nil {
		workspaceLoader = workspace.NewLoader("")
	}
	toolRegistry := cfg.ToolRegistry
	if toolRegistry == nil {
		router := sandbox.NewRouter(nil, nil)
		agentManager := cfg.AgentManager
		if agentManager == nil {
			agentManager = agent.NewManager()
		}
		toolRegistry = tools.NewRegistry(
			tools.NewTextUpperTool(),
			systemtools.NewRunTool(router),
		)
		toolRegistry.Register(tools.NewAgentTaskTool(agentManager, nil))
		toolRegistry.Register(tools.NewToolSearchTool(toolRegistry))
	}
	memSvc := cfg.MemoryService
	if memSvc == nil {
		memSvc = memory.NewService()
	}
	approvalMgr := cfg.ApprovalManager
	if approvalMgr == nil {
		approvalMgr = approval.NewManager()
	}
	policy := cfg.PermissionPolicy
	if policy.Mode == "" {
		policy.Mode = permissions.ModeDangerFullAccess
	}
	inputs := cfg.InputProcessor
	if inputs == nil {
		inputs = noopInputProcessor{}
	}

	return &QueryEngine{
		sessions:  sessionsMgr,
		client:    client,
		workspace: workspaceLoader,
		tools:     toolRegistry,
		compactor: cfg.Compactor,
		memory:    memSvc,
		approvals: approvalMgr,
		tokenBudget: cfg.EstimatedTokenBudget,
		maxTurns:  cfg.MaxTurns,
		policy:    policy,
		policies:  make(map[string]permissions.Policy),
		messages:  make(map[string][]session.Message),
		inputs:    inputs,
		includePartialStreamEvents: cfg.IncludePartialStreamEvents,
		snipReplay: cfg.SnipReplay,
		postCompactCleanup: cfg.PostCompactCleanup,
	}
}

func (q *QueryEngine) SubmitPrompt(ctx context.Context, sess session.Session, promptText string, sink EventSink) error {
	result, err := q.inputs.Process(ctx, sess, promptText)
	if err != nil {
		return err
	}
	q.recordInputProcessing(result)
	if len(result.Messages) > 0 {
		if err := q.emitImmediateMessages(sess, result.Messages, sink); err != nil {
			return err
		}
	}
	if !result.ShouldQuery {
		if len(result.Messages) > 0 {
			return nil
		}
		if strings.TrimSpace(result.ResultText) == "" {
			return nil
		}
		reply, err := q.sessions.AppendMessage(sess.ID, "assistant", result.ResultText)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, reply)
		q.setLastAssistantReply(reply.Content)
		return q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			Message: &reply,
		})
	}
	normalized := result.NormalizedInput
	if strings.TrimSpace(normalized) == "" {
		return nil
	}
	msg, err := q.sessions.AppendMessage(sess.ID, "user", normalized)
	if err != nil {
		return err
	}
	q.appendMutableMessage(sess.ID, msg)
	return q.SubmitMessage(ctx, sess, msg, sink)
}

func (q *QueryEngine) SubmitMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink) error {
	runID := fmt.Sprintf("run-%06d", q.nextRunID.Add(1))
	ctx, release := q.beginRun(ctx, runID, sess.ID)
	defer release()
	q.ensureMutableMessages(sess.ID)
	q.ensureUserMessageTracked(sess.ID, userMessage)
	if err := q.emit(sink, Event{
		Type:    "agent.lifecycle.start",
		Session: sess,
		RunID:   runID,
		Message: &userMessage,
	}); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
	if err != nil {
		q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: runID, Error: err.Error()})
		return err
	}
	q.recordModelPass()
	reply, err := q.executeTurnLoop(ctx, sess, userMessage, runID, sink, &toolCall{
		name:  stream.ToolName,
		input: stream.ToolInput,
	}, stream)
	if err != nil {
		if !isApprovalRequiredError(err) {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: runID, Error: err.Error()})
		}
		return err
	}
	return q.emit(sink, Event{
		Type:    "agent.lifecycle.end",
		Session: sess,
		RunID:   runID,
		Message: &reply,
	})
}

func (q *QueryEngine) ApproveAndContinue(ctx context.Context, approvalID string, sink EventSink) error {
	request, ok := q.approvals.Get(approvalID)
	if !ok {
		return fmt.Errorf("approval %q not found", approvalID)
	}
	ctx, release := q.beginRun(ctx, request.RunID, request.SessionID)
	defer release()
	if request.Status == approval.StatusRejected {
		return fmt.Errorf("approval %q was already rejected", approvalID)
	}
	if request.Status == approval.StatusPending {
		updated, err := q.approvals.UpdateStatus(approvalID, approval.StatusApproved)
		if err != nil {
			return err
		}
		request = updated
	}

	sess, ok := q.sessions.GetByID(request.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", request.SessionID)
	}
	userMessage, ok := q.findMessageByID(request.SessionID, request.UserMessageID)
	if !ok {
		return fmt.Errorf("user message %q not found", request.UserMessageID)
	}

	reply, err := q.executeTurnLoop(ctx, sess, userMessage, request.RunID, sink, &toolCall{
		name:           request.ToolName,
		input:          request.ToolInput,
		skipPermission: true,
	}, nil)
	if err != nil {
		if !isApprovalRequiredError(err) {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: request.RunID, Error: err.Error()})
		}
		return err
	}
	return q.emit(sink, Event{
		Type:    "agent.lifecycle.end",
		Session: sess,
		RunID:   request.RunID,
		Message: &reply,
	})
}

func (q *QueryEngine) Sessions() *session.Manager {
	return q.sessions
}

func (q *QueryEngine) MemoryService() *memory.Service {
	return q.memory
}

func (q *QueryEngine) ApprovalManager() *approval.Manager {
	return q.approvals
}

func (q *QueryEngine) Messages(sessionID string) []session.Message {
	q.ensureMutableMessages(sessionID)
	q.msgMu.RLock()
	defer q.msgMu.RUnlock()
	items := q.messages[sessionID]
	return append([]session.Message(nil), items...)
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

func (q *QueryEngine) PermissionPolicyForSession(sessionID string) permissions.Policy {
	q.policyMu.RLock()
	policy, ok := q.policies[sessionID]
	q.policyMu.RUnlock()
	if ok {
		return policy
	}
	return q.policy
}

func (q *QueryEngine) SetSessionPermissionPolicy(sessionID string, policy permissions.Policy) {
	q.policyMu.Lock()
	defer q.policyMu.Unlock()
	q.policies[sessionID] = policy
}

func (q *QueryEngine) runModelPass(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink) (*textStreamCollector, error) {
	q.ensureMutableMessages(sess.ID)
	history := q.Messages(sess.ID)
	if q.compactor != nil {
		analysis := q.compactor.Analyze(history)
		q.recordCompactionAnalysis(analysis)
		if analysis.IsAboveWarningThreshold {
			q.recordCompactionPhase("warning")
			if err := q.emit(sink, Event{
				Type:    "compact.warning",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		if analysis.IsAboveErrorThreshold {
			q.recordCompactionPhase("error")
			if err := q.emit(sink, Event{
				Type:    "compact.error",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		result := q.compactor.Compact(history)
		q.recordCompactionResult(result)
		if analysis.IsAboveAutoCompactThreshold && result.Changed {
			q.recordCompactionPhase("auto")
			if err := q.emit(sink, Event{
				Type:    "compact.auto",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		if analysis.IsAtBlockingLimit && !result.Changed {
			q.recordCompactionPhase("blocked")
			if err := q.emit(sink, Event{
				Type:    "compact.blocked",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("context window blocking limit reached")
		}
		if result.Changed {
			compacted := result.Messages
			if err := q.sessions.ReplaceMessages(sess.ID, compacted); err != nil {
				return nil, err
			}
			q.replaceMutableMessages(sess.ID, compacted)
			history = compacted
			boundary := q.newCompactBoundary(sess.ID)
			q.recordCompactBoundary(boundary)
			if err := q.emit(sink, Event{
				Type:    "compact.boundary",
				Session: sess,
				RunID:   runID,
				Message: &boundary,
			}); err != nil {
				return nil, err
			}
			if q.snipReplay != nil {
				if replay := q.snipReplay(boundary, q.Messages(sess.ID)); replay != nil && replay.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, replay.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, replay.Messages)
					q.recordCompactionReplay(len(replay.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.replayed",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
			if result.SummaryMessage != nil && q.memory != nil {
				summary := *result.SummaryMessage
				if _, saved := q.memory.SaveCompactionSummary(sess, summary); saved {
					q.recordCompactionMemorySaved(summary.ID)
					_ = q.emit(sink, Event{Type: "compact.memory_saved", Session: sess, RunID: runID, Message: &summary})
					_ = q.emit(sink, Event{Type: "memory.saved", Session: sess, RunID: runID, Message: &summary})
				}
			}
			if q.postCompactCleanup != nil {
				if cleanup := q.postCompactCleanup(boundary, q.Messages(sess.ID)); cleanup != nil && cleanup.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, cleanup.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, cleanup.Messages)
					q.recordCompactionCleanup(len(cleanup.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.cleaned",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
		}
	}

	workspaceContext, err := q.workspace.Load()
	if err != nil {
		return nil, err
	}
	contextInput := prompt.Build(prompt.BuildInput{
		Session:            sess,
		History:            history,
		UserMessage:        userMessage,
		UserContextLines:   q.userContextLines(sess),
		SystemContextLines: q.systemContextLines(sess, workspaceContext),
		WorkspaceContext:   workspaceContext,
		Tools: q.tools.Expose(tools.ExposeOptions{
			Policy: q.PermissionPolicyForSession(sess.ID),
		}),
		SessionMemories:    q.memoryLines(sess.ID),
		SessionMemoryItems: q.memoryItems(sess.ID),
	})
	stream := &textStreamCollector{
		sink:           sink,
		session:        sess,
		runID:          runID,
		onDelta:        q.recordAssistantDelta,
		onMessageEnd:   q.clearActiveAssistantText,
		onStreamEvent:  q.recordStreamEvent,
		includePartial: q.includePartialStreamEvents,
	}
	if err := q.client.Stream(ctx, llm.GenerateRequest{
		Session:     sess,
		UserMessage: userMessage,
		History:     history,
		Context:     contextInput,
	}, stream); err != nil {
		return nil, err
	}
	return stream, nil
}

func (q *QueryEngine) memoryLines(sessionID string) []string {
	if q.memory == nil {
		return nil
	}
	items := q.memory.List(sessionID)
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, item.Content)
	}
	return lines
}

func (q *QueryEngine) memoryItems(sessionID string) []memory.Item {
	if q.memory == nil {
		return nil
	}
	return q.memory.List(sessionID)
}

func (q *QueryEngine) userContextLines(sess session.Session) []string {
	return []string{
		"session_id=" + sess.ID,
		"session_key=" + sess.Key,
		"agent_id=" + sess.AgentID,
	}
}

func (q *QueryEngine) systemContextLines(sess session.Session, workspaceContext workspace.Context) []string {
	policy := q.PermissionPolicyForSession(sess.ID)
	lines := []string{
		"permission_mode=" + string(policy.Mode),
	}
	if policy.PlanMode {
		lines = append(lines, "plan_mode=true")
	}
	if policy.AutoMode {
		lines = append(lines, "auto_mode=true")
	}
	if workspaceContext.Root != "" {
		lines = append(lines, "workspace_root="+workspaceContext.Root)
	}
	if len(policy.WorkspaceRoots) > 0 {
		lines = append(lines, "workspace_roots="+strings.Join(policy.WorkspaceRoots, ","))
	}
	return lines
}

func (q *QueryEngine) findMessageByID(sessionID, messageID string) (session.Message, bool) {
	messages, ok := q.sessions.Messages(sessionID)
	if !ok {
		return session.Message{}, false
	}
	for _, message := range messages {
		if message.ID == messageID {
			return message, true
		}
	}
	return session.Message{}, false
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
	q.state.MaxTurns = q.effectiveMaxTurns()
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

func (q *QueryEngine) ensureMutableMessages(sessionID string) {
	q.msgMu.RLock()
	_, ok := q.messages[sessionID]
	q.msgMu.RUnlock()
	if ok {
		return
	}
	items, _ := q.sessions.Messages(sessionID)
	q.replaceMutableMessages(sessionID, items)
}

func (q *QueryEngine) ensureUserMessageTracked(sessionID string, msg session.Message) {
	q.msgMu.RLock()
	items := q.messages[sessionID]
	for _, existing := range items {
		if existing.ID == msg.ID {
			q.msgMu.RUnlock()
			return
		}
	}
	q.msgMu.RUnlock()
	q.appendMutableMessage(sessionID, msg)
}

func (q *QueryEngine) appendMutableMessage(sessionID string, msg session.Message) {
	q.msgMu.Lock()
	q.messages[sessionID] = append(q.messages[sessionID], msg)
	count := len(q.messages[sessionID])
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	if msg.Role == "user" {
		q.state.LastUserInput = msg.Content
	}
	q.stateMu.Unlock()
}

func (q *QueryEngine) replaceMutableMessages(sessionID string, items []session.Message) {
	cloned := append([]session.Message(nil), items...)
	q.msgMu.Lock()
	q.messages[sessionID] = cloned
	count := len(cloned)
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	q.stateMu.Unlock()
}

type textStreamCollector struct {
	sink          EventSink
	session       session.Session
	runID         string
	builder       strings.Builder
	ToolName      string
	ToolInput     string
	onDelta       func(string)
	onMessageEnd  func()
	onStreamEvent func(llm.StreamEvent)
	includePartial bool
}

func (c *textStreamCollector) OnEvent(event llm.StreamEvent) error {
	if c.onStreamEvent != nil {
		c.onStreamEvent(event)
	}
	if c.includePartial && c.sink != nil {
		if err := c.sink.Emit(Event{
			Type:      "stream.event",
			Session:   c.session,
			RunID:     c.runID,
			Delta:     event.Delta,
			ToolName:  event.ToolName,
			ToolInput: event.ToolInput,
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
		c.ToolInput = event.ToolInput
		return nil
	}
	return nil
}

func (c *textStreamCollector) Content() string {
	return c.builder.String()
}

func parseToolCallBlock(content string) (string, string, bool) {
	raw := strings.TrimSpace(content)
	start := strings.Index(raw, "<tool_call>")
	end := strings.Index(raw, "</tool_call>")
	if start < 0 || end < 0 || end < start {
		return "", "", false
	}
	body := strings.TrimSpace(raw[start+len("<tool_call>") : end])
	var name string
	var input string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "input:"):
			input = strings.TrimSpace(strings.TrimPrefix(line, "input:"))
		}
	}
	if name == "" || input == "" {
		return "", "", false
	}
	return name, input, true
}

func estimateMessagesTokens(messages []session.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTextTokens(msg.Content)
	}
	return total
}

func estimateTextTokens(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}
	return (len(content) + 3) / 4
}

func resolveWorkDir(sess session.Session, loader *workspace.Loader) string {
	if loader == nil {
		return sess.Key
	}
	ctx, err := loader.Load()
	if err != nil || ctx.Root == "" {
		return sess.Key
	}
	return ctx.Root
}

type toolCall struct {
	name           string
	input          string
	skipPermission bool
}

func (q *QueryEngine) executeTurnLoop(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink, pending *toolCall, current *textStreamCollector) (session.Message, error) {
	var lastExecutedToolName string
	var lastExecutedToolInput string
	var lastToolMessage *session.Message
	var deferredToolExecuted bool
	var approvedToolExecuted bool
	for {
		if pending != nil && pending.name != "" {
			if pending.name == lastExecutedToolName && pending.input == lastExecutedToolInput {
				if lastToolMessage != nil {
					return q.completeWithToolResult(sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated identical tool call detected: %s %s", pending.name, pending.input)
			}
			toolDef, _ := q.tools.Inspect(pending.name, pending.input)
			if deferredToolExecuted && toolDef.ShouldDefer {
				if lastToolMessage != nil {
					return q.completeWithToolResult(sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated deferred tool call detected: %s", pending.name)
			}
			if !pending.skipPermission {
				decision := q.PermissionPolicyForSession(sess.ID).Evaluate(permissions.Request{
					ToolName:    pending.name,
					Command:     pending.input,
					WorkDir:     resolveWorkDir(sess, q.workspace),
					ReadOnly:    toolDef.ReadOnly,
					Destructive: toolDef.Destructive,
				})
				if !decision.Allowed {
					q.recordPermissionDenial(PermissionDenial{
						RunID:     runID,
						SessionID: sess.ID,
						ToolName:  pending.name,
						ToolInput: pending.input,
						Reason:    decision.Reason,
					})
					req := q.approvals.Create(sess.ID, runID, userMessage.ID, pending.name, pending.input, decision.Reason, string(decision.Category), decision.RuleSource)
					if err := q.emit(sink, Event{
						Type:      "permission.required",
						Session:   sess,
						RunID:     runID,
						ToolName:  pending.name,
						ToolInput: pending.input,
						Approval:  &req,
					}); err != nil {
						return session.Message{}, err
					}
					return session.Message{}, &ApprovalRequiredError{
						ToolName:  pending.name,
						ToolInput: pending.input,
						Reason:    decision.Reason,
					}
				}
			}

			if err := q.emit(sink, Event{
				Type:      "tool.called",
				Session:   sess,
				RunID:     runID,
				ToolName:  pending.name,
				ToolInput: pending.input,
			}); err != nil {
				return session.Message{}, err
			}
			toolOutput, err := q.tools.Invoke(ctx, sess, pending.name, pending.input)
			if err != nil {
				return session.Message{}, err
			}
			toolMsg, err := q.sessions.AppendMessage(sess.ID, "tool", fmt.Sprintf("%s: %s", pending.name, toolOutput))
			if err != nil {
				return session.Message{}, err
			}
			q.appendMutableMessage(sess.ID, toolMsg)
			if err := q.emit(sink, Event{
				Type:      "tool.result",
				Session:   sess,
				RunID:     runID,
				Message:   &toolMsg,
				ToolName:  pending.name,
				ToolInput: pending.input,
			}); err != nil {
				return session.Message{}, err
			}
			lastToolMessage = &toolMsg
			lastExecutedToolName = pending.name
			lastExecutedToolInput = pending.input
			if pending.skipPermission {
				approvedToolExecuted = true
			}
			if toolDef.ShouldDefer {
				deferredToolExecuted = true
			}
			pending = nil
			current = nil
		}

		if current == nil {
			if limit := q.effectiveMaxTurns(); limit > 0 && q.State().LastModelPassCount >= limit {
				q.recordMaxTurnsExceeded()
				return session.Message{}, fmt.Errorf("reached maximum number of turns (%d)", limit)
			}
			stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
			if err != nil {
				return session.Message{}, err
			}
			q.recordModelPass()
			current = stream
		}

		if current.ToolName != "" {
			if approvedToolExecuted && lastToolMessage != nil {
				return q.completeWithToolResult(sess, runID, sink, *lastToolMessage)
			}
			pending = &toolCall{name: current.ToolName, input: current.ToolInput}
			current = nil
			continue
		}

		reply, err := q.sessions.AppendMessage(sess.ID, "assistant", current.Content())
		if err != nil {
			return session.Message{}, err
		}
		q.appendMutableMessage(sess.ID, reply)
		q.recordUsageEstimate(sess.ID, reply.Content)
		q.setLastAssistantReply(reply.Content)
		if err := q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			RunID:   runID,
			Message: &reply,
		}); err != nil {
			return session.Message{}, err
		}
		return reply, nil
	}
}

func (q *QueryEngine) completeWithToolResult(sess session.Session, runID string, sink EventSink, toolMsg session.Message) (session.Message, error) {
	reply, err := q.sessions.AppendMessage(sess.ID, "assistant", "Using tool result: "+toolMsg.Content)
	if err != nil {
		return session.Message{}, err
	}
	q.appendMutableMessage(sess.ID, reply)
	q.recordUsageEstimate(sess.ID, reply.Content)
	q.setLastAssistantReply(reply.Content)
	if err := q.emit(sink, Event{
		Type:    "message.created",
		Session: sess,
		RunID:   runID,
		Message: &reply,
	}); err != nil {
		return session.Message{}, err
	}
	return reply, nil
}

func (q *QueryEngine) effectiveMaxTurns() int {
	if q.maxTurns <= 0 {
		return 8
	}
	return q.maxTurns
}

func fallbackRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "user", "assistant", "tool", "system", "summary":
		return role
	default:
		return "assistant"
	}
}
