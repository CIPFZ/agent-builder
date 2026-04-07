package orchestration

import "sort"

type Suggestion struct {
	RunID             string
	SessionID         string
	Category          string
	SuggestedAction   string
	Reason            string
	Priority          string
	Blocking          bool
	AutoExecutable    bool
	RecommendedRole   string
	RecommendedAction string
}

type SuggestionFilter struct {
	Category     string
	Priority     string
	BlockingOnly bool
}

func (c *Coordinator) EvaluateSession(sessionID string) []Suggestion {
	return c.EvaluateSessionWithFilter(sessionID, SuggestionFilter{})
}

func (c *Coordinator) EvaluateSessionWithFilter(sessionID string, filter SuggestionFilter) []Suggestion {
	runs := c.ListBySession(sessionID)
	suggestions := make([]Suggestion, 0, len(runs))
	for _, run := range runs {
		suggestion := evaluateRun(run)
		if filter.Category != "" && suggestion.Category != filter.Category {
			continue
		}
		if filter.Priority != "" && suggestion.Priority != filter.Priority {
			continue
		}
		if filter.BlockingOnly && !suggestion.Blocking {
			continue
		}
		suggestions = append(suggestions, suggestion)
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		li := priorityRank(suggestions[i].Priority)
		lj := priorityRank(suggestions[j].Priority)
		if li != lj {
			return li < lj
		}
		if suggestions[i].Blocking != suggestions[j].Blocking {
			return suggestions[i].Blocking
		}
		return suggestions[i].RunID < suggestions[j].RunID
	})
	return suggestions
}

func evaluateRun(run RunState) Suggestion {
	suggestion := Suggestion{
		RunID:             run.RunID,
		SessionID:         run.SessionID,
		SuggestedAction:   run.RecommendedAction,
		Reason:            run.DecisionReason,
		Priority:          run.DecisionPriority,
		Blocking:          false,
		AutoExecutable:    false,
		RecommendedRole:   run.RecommendedRole,
		RecommendedAction: run.RecommendedAction,
	}

	switch run.DecisionType {
	case "human_approval":
		suggestion.Category = "approval"
		suggestion.SuggestedAction = "request_human_approval"
		suggestion.Blocking = true
	case "inspect_failure":
		suggestion.Category = "failure"
		suggestion.SuggestedAction = "inspect_failure"
		suggestion.Blocking = true
	case "close_or_follow_up":
		suggestion.Category = "completion"
		suggestion.SuggestedAction = "consider_close_or_follow_up"
	case "review_response":
		suggestion.Category = "review"
		suggestion.SuggestedAction = "review_response"
	default:
		suggestion.Category = "monitor"
	}

	return suggestion
}

func priorityRank(priority string) int {
	switch priority {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}
