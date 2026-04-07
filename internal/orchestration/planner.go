package orchestration

import (
	"fmt"
	"time"
)

type Plan struct {
	SessionID        string
	Summary          string
	Groups           map[string]int
	PrioritySections map[string]int
	PhaseSections    map[string]int
	Steps            []PlanStep
}

type PlanExecutionOverview struct {
	SessionID              string
	TotalSteps             int
	CompletedSteps         int
	FailedSteps            int
	ReadySteps             int
	PendingSteps           int
	InProgressSteps        int
	BlockingSteps          int
	ActiveSteps            int
	TerminalSteps          int
	ProgressPercent        int
	HasBlockedSteps        bool
	StateCounts            map[string]int
	LatestActiveAction     string
	LatestReadyAction      string
	LatestInProgressAction string
	LatestPendingAction    string
	LatestTerminalAction   string
	LatestCompletedAction  string
	LatestFailedAction     string
	LatestBlockedAction    string
	LastUpdatedAt          time.Time
}

type PlanStep struct {
	RunID           string
	ActionID        string
	Title           string
	Description     string
	ActionKind      string
	Phase           string
	DependsOn       string
	State           string
	Result          string
	UpdatedAt       time.Time
	SuggestedAction string
	Priority        string
	Blocking        bool
	RecommendedRole string
}

func (c *Coordinator) PlanSession(sessionID string, filter SuggestionFilter) Plan {
	suggestions := c.EvaluateSessionWithFilter(sessionID, filter)
	overrides := c.planStepUpdatesForSession(sessionID)
	steps := make([]PlanStep, 0, len(suggestions))
	groups := make(map[string]int)
	prioritySections := make(map[string]int)
	phaseSections := make(map[string]int)
	var previousActionID string
	var previousState string
	for _, suggestion := range suggestions {
		groups[suggestion.Category]++
		prioritySections[suggestion.Priority]++
		actionID := planActionID(suggestion)
		phase := planPhase(suggestion)
		phaseSections[phase]++
		dependencySatisfied := previousActionID == "" || previousState == "completed"
		step := PlanStep{
			RunID:           suggestion.RunID,
			ActionID:        actionID,
			Title:           planTitle(suggestion),
			Description:     suggestion.Reason,
			ActionKind:      suggestion.Category,
			Phase:           phase,
			DependsOn:       previousActionID,
			State:           planInitialState(suggestion, previousActionID, dependencySatisfied),
			Result:          "",
			UpdatedAt:       time.Now().UTC(),
			SuggestedAction: suggestion.SuggestedAction,
			Priority:        suggestion.Priority,
			Blocking:        suggestion.Blocking,
			RecommendedRole: suggestion.RecommendedRole,
		}
		if update, ok := overrides[actionID]; ok {
			step.State = update.State
			step.Result = update.Result
			step.UpdatedAt = update.UpdatedAt
		}
		steps = append(steps, step)
		previousActionID = actionID
		previousState = step.State
	}

	return Plan{
		SessionID:        sessionID,
		Summary:          buildPlanSummary(suggestions),
		Groups:           groups,
		PrioritySections: prioritySections,
		PhaseSections:    phaseSections,
		Steps:            steps,
	}
}

func planActionID(suggestion Suggestion) string {
	return fmt.Sprintf("%s:%s", suggestion.RunID, suggestion.SuggestedAction)
}

func planTitle(suggestion Suggestion) string {
	switch suggestion.Category {
	case "approval":
		return fmt.Sprintf("Resolve approval for %s", suggestion.RunID)
	case "failure":
		return fmt.Sprintf("Inspect failed run %s", suggestion.RunID)
	case "completion":
		return fmt.Sprintf("Close or follow up %s", suggestion.RunID)
	case "review":
		return fmt.Sprintf("Review response from %s", suggestion.RunID)
	default:
		return fmt.Sprintf("Monitor run %s", suggestion.RunID)
	}
}

func planPhase(suggestion Suggestion) string {
	switch suggestion.Category {
	case "approval", "failure":
		return "stabilize"
	case "review", "monitor":
		return "assess"
	case "completion":
		return "closeout"
	default:
		return "assess"
	}
}

func planInitialState(suggestion Suggestion, dependsOn string, dependencySatisfied bool) string {
	if dependsOn != "" && !dependencySatisfied {
		return "blocked"
	}
	if suggestion.Blocking {
		return "pending"
	}
	return "ready"
}

func buildPlanSummary(suggestions []Suggestion) string {
	if len(suggestions) == 0 {
		return "No orchestration actions are currently suggested."
	}
	return fmt.Sprintf("Prepared %d orchestration step(s), with %d blocking item(s).", len(suggestions), countBlockingSuggestionsCore(suggestions))
}

func (c *Coordinator) PlanExecutionOverview(sessionID string, filter SuggestionFilter) PlanExecutionOverview {
	plan := c.PlanSession(sessionID, filter)
	overview := PlanExecutionOverview{
		SessionID:   sessionID,
		TotalSteps:  len(plan.Steps),
		StateCounts: make(map[string]int),
	}
	for _, step := range plan.Steps {
		overview.StateCounts[step.State]++
		if step.State == "completed" {
			overview.CompletedSteps++
			overview.TerminalSteps++
			overview.LatestTerminalAction = step.ActionID
			overview.LatestCompletedAction = step.ActionID
		} else if step.State == "failed" {
			overview.FailedSteps++
			overview.TerminalSteps++
			overview.LatestTerminalAction = step.ActionID
			overview.LatestFailedAction = step.ActionID
		}
		if step.State == "pending" {
			overview.PendingSteps++
			overview.LatestPendingAction = step.ActionID
		}
		if step.State == "ready" {
			overview.ReadySteps++
			overview.LatestReadyAction = step.ActionID
		}
		if step.State == "in_progress" {
			overview.InProgressSteps++
			overview.LatestInProgressAction = step.ActionID
		}
		if step.State == "blocked" || step.State == "pending" {
			overview.BlockingSteps++
			if overview.LatestBlockedAction == "" {
				overview.LatestBlockedAction = step.ActionID
			}
		}
		if step.State == "ready" || step.State == "pending" || step.State == "in_progress" {
			overview.ActiveSteps++
			overview.LatestActiveAction = step.ActionID
		}
		if step.UpdatedAt.After(overview.LastUpdatedAt) {
			overview.LastUpdatedAt = step.UpdatedAt
		}
	}
	overview.HasBlockedSteps = overview.StateCounts["blocked"] > 0
	if overview.TotalSteps > 0 {
		overview.ProgressPercent = (overview.CompletedSteps * 100) / overview.TotalSteps
	}
	return overview
}

func countBlockingSuggestionsCore(suggestions []Suggestion) int {
	total := 0
	for _, suggestion := range suggestions {
		if suggestion.Blocking {
			total++
		}
	}
	return total
}

func isValidPlanStepState(state string) bool {
	switch state {
	case "pending", "blocked", "ready", "in_progress", "completed", "failed":
		return true
	default:
		return false
	}
}

func isAllowedPlanStepTransition(current, next string) bool {
	if current == next {
		return true
	}
	switch current {
	case "blocked":
		return next == "ready" || next == "pending"
	case "pending":
		return next == "ready" || next == "in_progress" || next == "completed" || next == "failed"
	case "ready":
		return next == "pending" || next == "in_progress" || next == "completed" || next == "failed" || next == "blocked"
	case "in_progress":
		return next == "completed" || next == "failed" || next == "ready"
	case "completed", "failed":
		return false
	default:
		return false
	}
}
