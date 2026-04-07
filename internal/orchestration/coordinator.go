package orchestration

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrSessionRequired       = errors.New("session_id is required")
	ErrActionRequired        = errors.New("action_id is required")
	ErrPlanStepStateRequired = errors.New("state is required")
	ErrPlanStepInvalidState  = errors.New("invalid plan step state")
	ErrPlanStepTransition    = errors.New("invalid plan step transition")
	ErrActionNotFound        = errors.New("plan action not found")
)

type RunState struct {
	RunID             string
	SessionID         string
	SessionKey        string
	AgentID           string
	Status            string
	LastEvent         string
	ToolName          string
	Message           string
	DispatcherState   string
	ReviewerState     string
	ExecutorState     string
	NextAction        string
	RecommendedRole   string
	RecommendedAction string
	DecisionType      string
	DecisionReason    string
	DecisionPriority  string
	AutoExecutable    bool
	UpdatedAt         time.Time
}

type DecisionRecord struct {
	RunID             string
	SessionID         string
	EventType         string
	Status            string
	ToolName          string
	Message           string
	RecommendedRole   string
	RecommendedAction string
	DecisionType      string
	DecisionReason    string
	DecisionPriority  string
	AutoExecutable    bool
	RecordedAt        time.Time
}

type HistoryFilter struct {
	Status           string
	DecisionPriority string
}

type SessionSummary struct {
	SessionID               string
	RunCount                int
	StatusCounts            map[string]int
	PriorityCounts          map[string]int
	RecommendedActionCounts map[string]int
	TopPriority             string
}

type PlanStepUpdate struct {
	State     string
	Result    string
	UpdatedAt time.Time
}

type PlanStepRecord struct {
	SessionID  string
	ActionID   string
	State      string
	Result     string
	RecordedAt time.Time
}

type PlanStepHistorySummary struct {
	RecordCount int
	StateCounts map[string]int
	LatestState string
}

type SessionPlanExecutionSummary struct {
	RecordCount            int
	StateCounts            map[string]int
	ActionCounts           map[string]int
	LatestRecordedAction   string
	LatestRecordedState    string
	LatestRecordedResult   string
	LatestActiveAction     string
	LatestActiveState      string
	LatestActiveAt         time.Time
	LatestActiveResult     string
	LatestReadyAction      string
	LatestReadyAt          time.Time
	LatestReadyResult      string
	LatestReadyState       string
	LatestInProgressAction string
	LatestInProgressAt     time.Time
	LatestInProgressResult string
	LatestInProgressState  string
	LatestPendingAction    string
	LatestPendingAt        time.Time
	LatestPendingResult    string
	LatestPendingState     string
	LatestTerminalAction   string
	LatestTerminalState    string
	LatestTerminalResult   string
	LatestTerminalAt       time.Time
	LatestCompletedAction  string
	LatestCompletedState   string
	LatestCompletedResult  string
	LatestCompletedAt      time.Time
	LatestFailedAction     string
	LatestFailedState      string
	LatestFailedResult     string
	LatestFailedAt         time.Time
	LastRecordedAt         time.Time
}

type Coordinator struct {
	mu              sync.RWMutex
	runs            map[string]RunState
	history         map[string][]DecisionRecord
	planStepUpdates map[string]map[string]PlanStepUpdate
	planStepHistory map[string]map[string][]PlanStepRecord
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		runs:            make(map[string]RunState),
		history:         make(map[string][]DecisionRecord),
		planStepUpdates: make(map[string]map[string]PlanStepUpdate),
		planStepHistory: make(map[string]map[string][]PlanStepRecord),
	}
}

func (c *Coordinator) Handle(_ context.Context, event Event) error {
	if event.RunID == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.runs[event.RunID]
	state.RunID = event.RunID
	state.SessionID = event.SessionID
	state.SessionKey = event.SessionKey
	state.AgentID = event.AgentID
	state.LastEvent = event.Type
	if event.ToolName != "" {
		state.ToolName = event.ToolName
	}
	if event.Message != "" {
		state.Message = event.Message
	}
	state.Status = deriveStatus(event, state.Status)
	state.DispatcherState, state.ReviewerState, state.ExecutorState, state.NextAction, state.RecommendedRole, state.RecommendedAction = deriveGuidance(state)
	state.DecisionType, state.DecisionReason, state.DecisionPriority, state.AutoExecutable = deriveDecision(state)
	state.UpdatedAt = time.Now().UTC()
	c.runs[event.RunID] = state
	c.history[event.RunID] = append(c.history[event.RunID], DecisionRecord{
		RunID:             state.RunID,
		SessionID:         state.SessionID,
		EventType:         event.Type,
		Status:            state.Status,
		ToolName:          state.ToolName,
		Message:           state.Message,
		RecommendedRole:   state.RecommendedRole,
		RecommendedAction: state.RecommendedAction,
		DecisionType:      state.DecisionType,
		DecisionReason:    state.DecisionReason,
		DecisionPriority:  state.DecisionPriority,
		AutoExecutable:    state.AutoExecutable,
		RecordedAt:        state.UpdatedAt,
	})
	return nil
}

func (c *Coordinator) GetRun(runID string) (RunState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, ok := c.runs[runID]
	return state, ok
}

func (c *Coordinator) ListBySession(sessionID string) []RunState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := make([]RunState, 0, len(c.runs))
	for _, run := range c.runs {
		if sessionID != "" && run.SessionID != sessionID {
			continue
		}
		items = append(items, run)
	}
	return items
}

func (c *Coordinator) HistoryByRun(runID string) []DecisionRecord {
	return c.FilteredHistoryByRun(runID, HistoryFilter{})
}

func (c *Coordinator) FilteredHistoryByRun(runID string, filter HistoryFilter) []DecisionRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	records := c.history[runID]
	if len(records) == 0 {
		return nil
	}
	items := make([]DecisionRecord, 0, len(records))
	for _, record := range records {
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.DecisionPriority != "" && record.DecisionPriority != filter.DecisionPriority {
			continue
		}
		items = append(items, record)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func (c *Coordinator) SummaryBySession(sessionID string) SessionSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	summary := SessionSummary{
		SessionID:               sessionID,
		StatusCounts:            make(map[string]int),
		PriorityCounts:          make(map[string]int),
		RecommendedActionCounts: make(map[string]int),
	}
	for _, run := range c.runs {
		if sessionID != "" && run.SessionID != sessionID {
			continue
		}
		summary.RunCount++
		summary.StatusCounts[run.Status]++
		summary.PriorityCounts[run.DecisionPriority]++
		summary.RecommendedActionCounts[run.RecommendedAction]++
	}
	summary.TopPriority = highestPriority(summary.PriorityCounts)
	return summary
}

func (c *Coordinator) planStepUpdatesForSession(sessionID string) map[string]PlanStepUpdate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	updates := c.planStepUpdates[sessionID]
	if len(updates) == 0 {
		return nil
	}
	cloned := make(map[string]PlanStepUpdate, len(updates))
	for actionID, update := range updates {
		cloned[actionID] = update
	}
	return cloned
}

func (c *Coordinator) UpdatePlanStep(sessionID, actionID, state, result string) (PlanStep, error) {
	if sessionID == "" {
		return PlanStep{}, ErrSessionRequired
	}
	if actionID == "" {
		return PlanStep{}, ErrActionRequired
	}
	if state == "" {
		return PlanStep{}, ErrPlanStepStateRequired
	}
	if !isValidPlanStepState(state) {
		return PlanStep{}, ErrPlanStepInvalidState
	}

	plan := c.PlanSession(sessionID, SuggestionFilter{})
	for _, step := range plan.Steps {
		if step.ActionID != actionID {
			continue
		}
		if !isAllowedPlanStepTransition(step.State, state) {
			return PlanStep{}, ErrPlanStepTransition
		}

		update := PlanStepUpdate{
			State:     state,
			Result:    result,
			UpdatedAt: time.Now().UTC(),
		}

		c.mu.Lock()
		if c.planStepUpdates[sessionID] == nil {
			c.planStepUpdates[sessionID] = make(map[string]PlanStepUpdate)
		}
		c.planStepUpdates[sessionID][actionID] = update
		if c.planStepHistory[sessionID] == nil {
			c.planStepHistory[sessionID] = make(map[string][]PlanStepRecord)
		}
		c.planStepHistory[sessionID][actionID] = append(c.planStepHistory[sessionID][actionID], PlanStepRecord{
			SessionID:  sessionID,
			ActionID:   actionID,
			State:      update.State,
			Result:     update.Result,
			RecordedAt: update.UpdatedAt,
		})
		c.mu.Unlock()

		step.State = update.State
		step.Result = update.Result
		step.UpdatedAt = update.UpdatedAt
		return step, nil
	}

	return PlanStep{}, ErrActionNotFound
}

func (c *Coordinator) PlanStepHistory(sessionID, actionID string) []PlanStepRecord {
	return c.FilteredPlanStepHistory(sessionID, actionID, "")
}

func (c *Coordinator) FilteredPlanStepHistory(sessionID, actionID, state string) []PlanStepRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	records := c.planStepHistory[sessionID][actionID]
	if len(records) == 0 {
		return nil
	}
	items := make([]PlanStepRecord, 0, len(records))
	for _, record := range records {
		if state != "" && record.State != state {
			continue
		}
		items = append(items, record)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func SummarizePlanStepHistory(records []PlanStepRecord) PlanStepHistorySummary {
	summary := PlanStepHistorySummary{
		RecordCount: len(records),
		StateCounts: make(map[string]int),
	}
	for _, record := range records {
		summary.StateCounts[record.State]++
		summary.LatestState = record.State
	}
	return summary
}

func (c *Coordinator) SessionPlanExecutionHistory(sessionID string) []PlanStepRecord {
	return c.FilteredSessionPlanExecutionHistory(sessionID, "", "", time.Time{}, time.Time{})
}

func (c *Coordinator) FilteredSessionPlanExecutionHistory(sessionID, state, actionID string, since, until time.Time) []PlanStepRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	byAction := c.planStepHistory[sessionID]
	if len(byAction) == 0 {
		return nil
	}
	items := make([]PlanStepRecord, 0)
	for _, records := range byAction {
		for _, record := range records {
			if state != "" && record.State != state {
				continue
			}
			if actionID != "" && record.ActionID != actionID {
				continue
			}
			if !since.IsZero() && !record.RecordedAt.After(since) {
				continue
			}
			if !until.IsZero() && record.RecordedAt.After(until) {
				continue
			}
			items = append(items, record)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func SummarizeSessionPlanExecutionHistory(records []PlanStepRecord) SessionPlanExecutionSummary {
	summary := SessionPlanExecutionSummary{
		RecordCount:  len(records),
		StateCounts:  make(map[string]int),
		ActionCounts: make(map[string]int),
	}
	var latestCompletedAt time.Time
	var latestFailedAt time.Time
	var latestTerminalAt time.Time
	for _, record := range records {
		summary.StateCounts[record.State]++
		summary.ActionCounts[record.ActionID]++
		if record.State != "completed" && record.State != "failed" {
			summary.LatestActiveAction = record.ActionID
			summary.LatestActiveState = record.State
			summary.LatestActiveAt = record.RecordedAt
			summary.LatestActiveResult = record.Result
		}
		if record.State == "ready" {
			summary.LatestReadyAction = record.ActionID
			summary.LatestReadyAt = record.RecordedAt
			summary.LatestReadyResult = record.Result
			summary.LatestReadyState = record.State
		}
		if record.State == "in_progress" {
			summary.LatestInProgressAction = record.ActionID
			summary.LatestInProgressAt = record.RecordedAt
			summary.LatestInProgressResult = record.Result
			summary.LatestInProgressState = record.State
		}
		if record.State == "pending" {
			summary.LatestPendingAction = record.ActionID
			summary.LatestPendingAt = record.RecordedAt
			summary.LatestPendingResult = record.Result
			summary.LatestPendingState = record.State
		}
		if record.State == "completed" && (record.RecordedAt.After(latestCompletedAt) ||
			(record.RecordedAt.Equal(latestCompletedAt) && record.ActionID >= summary.LatestCompletedAction)) {
			latestCompletedAt = record.RecordedAt
			summary.LatestCompletedAction = record.ActionID
			summary.LatestCompletedState = record.State
			summary.LatestCompletedResult = record.Result
			summary.LatestCompletedAt = record.RecordedAt
		}
		if (record.State == "completed" || record.State == "failed") &&
			(record.RecordedAt.After(latestTerminalAt) ||
				(record.RecordedAt.Equal(latestTerminalAt) && record.ActionID >= summary.LatestTerminalAction)) {
			latestTerminalAt = record.RecordedAt
			summary.LatestTerminalAction = record.ActionID
			summary.LatestTerminalState = record.State
			summary.LatestTerminalResult = record.Result
			summary.LatestTerminalAt = record.RecordedAt
		}
		if record.State == "failed" && (record.RecordedAt.After(latestFailedAt) ||
			(record.RecordedAt.Equal(latestFailedAt) && record.ActionID >= summary.LatestFailedAction)) {
			latestFailedAt = record.RecordedAt
			summary.LatestFailedAction = record.ActionID
			summary.LatestFailedState = record.State
			summary.LatestFailedResult = record.Result
			summary.LatestFailedAt = record.RecordedAt
		}
		if record.RecordedAt.After(summary.LastRecordedAt) ||
			(record.RecordedAt.Equal(summary.LastRecordedAt) && record.ActionID >= summary.LatestRecordedAction) {
			summary.LastRecordedAt = record.RecordedAt
			summary.LatestRecordedAction = record.ActionID
			summary.LatestRecordedState = record.State
			summary.LatestRecordedResult = record.Result
		}
	}
	return summary
}

func deriveStatus(event Event, current string) string {
	switch event.Type {
	case "agent.lifecycle.start":
		return "running"
	case "permission.required":
		return "waiting_approval"
	case "tool.called":
		return "running_tool"
	case "tool.result":
		return "running"
	case "message.created":
		return "responded"
	case "agent.lifecycle.end":
		return "completed"
	case "run.error":
		return "failed"
	case "subagent.updated", "subagent.completed":
		if event.Status != "" {
			return event.Status
		}
	}
	if current != "" {
		return current
	}
	return "unknown"
}

func deriveGuidance(state RunState) (string, string, string, string, string, string) {
	switch state.Status {
	case "waiting_approval":
		return "needs_review", "approval_required", "blocked", "review pending approval request", "reviewer", "request_approval"
	case "running_tool":
		return "in_progress", "watching", "executing_tool", "wait for tool result", "executor", "await_tool_result"
	case "running":
		return "in_progress", "watching", "executing", "wait for model/tool progress", "executor", "await_progress"
	case "responded":
		return "ready_for_close", "review_optional", "responded", "inspect assistant response", "reviewer", "review_response"
	case "completed":
		return "complete", "review_optional", "complete", "archive or dispatch follow-up task", "dispatcher", "close_or_follow_up"
	case "failed":
		return "needs_retry", "review_required", "failed", "inspect error and decide retry strategy", "reviewer", "decide_retry"
	case "steered":
		return "replanned", "review_optional", "running", "observe updated subagent behavior", "dispatcher", "monitor_replanned_run"
	case "stopped":
		return "halted", "review_optional", "stopped", "decide whether to resume or replace run", "dispatcher", "decide_resume_or_replace"
	case "resumed":
		return "replanned", "review_optional", "running", "observe resumed run", "dispatcher", "monitor_resumed_run"
	default:
		return "unknown", "unknown", "unknown", "inspect run state", "dispatcher", "inspect"
	}
}

func deriveDecision(state RunState) (string, string, string, bool) {
	switch state.Status {
	case "waiting_approval":
		return "human_approval", "run is blocked until an explicit approval decision is recorded", "high", false
	case "running_tool":
		return "await_tool_result", "a tool call is in flight and should finish before the next orchestration step", "medium", false
	case "running":
		return "await_progress", "the run is still making progress and should be observed before intervention", "medium", false
	case "responded":
		return "review_response", "the assistant has responded and the output can now be reviewed for follow-up", "medium", false
	case "completed":
		return "close_or_follow_up", "the run completed and can be closed or dispatched into a follow-up task", "low", false
	case "failed":
		return "inspect_failure", "the run failed and needs human review to decide whether to retry or redirect it", "high", false
	case "steered":
		return "monitor_replanned_run", "the subagent received new control input and should be monitored for updated behavior", "medium", false
	case "stopped":
		return "decide_resume_or_replace", "the subagent is stopped and needs a coordinator decision to resume or replace it", "medium", false
	case "resumed":
		return "monitor_resumed_run", "the subagent resumed work and should be watched before issuing another action", "medium", false
	default:
		return "inspect", "the run state is not yet classified and should be inspected by the dispatcher", "low", false
	}
}

func highestPriority(counts map[string]int) string {
	for _, priority := range []string{"high", "medium", "low"} {
		if counts[priority] > 0 {
			return priority
		}
	}
	return ""
}
