package orchestration_test

import (
	"context"
	"testing"
	"time"

	"myclaw/internal/orchestration"
)

func TestCoordinatorTracksRunLifecycle(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	runID := "run-123"

	for _, event := range []orchestration.Event{
		{Type: "agent.lifecycle.start", RunID: runID, SessionID: "session-1"},
		{Type: "tool.called", RunID: runID, SessionID: "session-1", ToolName: "text.upper"},
		{Type: "message.created", RunID: runID, SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: runID, SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	state, ok := coordinator.GetRun(runID)
	if !ok {
		t.Fatal("expected run state to be present")
	}
	if state.Status != "completed" {
		t.Fatalf("status = %q, want completed", state.Status)
	}
	if state.ToolName != "text.upper" {
		t.Fatalf("tool name = %q, want text.upper", state.ToolName)
	}
}

func TestCoordinatorTracksWaitingApprovalState(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	runID := "run-456"

	if err := coordinator.Handle(context.Background(), orchestration.Event{
		Type:      "permission.required",
		RunID:     runID,
		SessionID: "session-1",
		Message:   "approval required",
	}); err != nil {
		t.Fatalf("handle event: %v", err)
	}

	state, ok := coordinator.GetRun(runID)
	if !ok {
		t.Fatal("expected run state to be present")
	}
	if state.Status != "waiting_approval" {
		t.Fatalf("status = %q, want waiting_approval", state.Status)
	}
}

func TestCoordinatorDerivesRoleSuggestions(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	runID := "run-789"

	if err := coordinator.Handle(context.Background(), orchestration.Event{
		Type:      "permission.required",
		RunID:     runID,
		SessionID: "session-1",
		Message:   "approval required",
	}); err != nil {
		t.Fatalf("handle event: %v", err)
	}

	state, ok := coordinator.GetRun(runID)
	if !ok {
		t.Fatal("expected run state")
	}
	if state.DispatcherState != "needs_review" {
		t.Fatalf("dispatcher state = %q, want needs_review", state.DispatcherState)
	}
	if state.ReviewerState != "approval_required" {
		t.Fatalf("reviewer state = %q, want approval_required", state.ReviewerState)
	}
	if state.ExecutorState != "blocked" {
		t.Fatalf("executor state = %q, want blocked", state.ExecutorState)
	}
	if state.NextAction == "" {
		t.Fatal("expected next action to be populated")
	}
	if state.RecommendedRole != "reviewer" {
		t.Fatalf("recommended role = %q, want reviewer", state.RecommendedRole)
	}
	if state.RecommendedAction != "request_approval" {
		t.Fatalf("recommended action = %q, want request_approval", state.RecommendedAction)
	}
	if state.DecisionType != "human_approval" {
		t.Fatalf("decision type = %q, want human_approval", state.DecisionType)
	}
	if state.DecisionReason == "" {
		t.Fatal("expected decision reason")
	}
	if state.AutoExecutable {
		t.Fatal("approval decision should not be auto executable")
	}
	if state.DecisionPriority != "high" {
		t.Fatalf("decision priority = %q, want high", state.DecisionPriority)
	}
}

func TestCoordinatorStoresDecisionHistory(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	runID := "run-history"

	for _, event := range []orchestration.Event{
		{Type: "agent.lifecycle.start", RunID: runID, SessionID: "session-1"},
		{Type: "tool.called", RunID: runID, SessionID: "session-1", ToolName: "text.upper"},
		{Type: "agent.lifecycle.end", RunID: runID, SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	history := coordinator.HistoryByRun(runID)
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].EventType != "agent.lifecycle.start" {
		t.Fatalf("first history event = %q, want agent.lifecycle.start", history[0].EventType)
	}
	if history[1].DecisionType != "await_tool_result" {
		t.Fatalf("tool history decision type = %q, want await_tool_result", history[1].DecisionType)
	}
	if history[2].DecisionPriority != "low" {
		t.Fatalf("final history priority = %q, want low", history[2].DecisionPriority)
	}
}

func TestCoordinatorFiltersHistoryByStatusAndPriority(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	runID := "run-filter"

	for _, event := range []orchestration.Event{
		{Type: "agent.lifecycle.start", RunID: runID, SessionID: "session-1"},
		{Type: "tool.called", RunID: runID, SessionID: "session-1", ToolName: "text.upper"},
		{Type: "agent.lifecycle.end", RunID: runID, SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	history := coordinator.FilteredHistoryByRun(runID, orchestration.HistoryFilter{
		Status:           "running_tool",
		DecisionPriority: "medium",
	})
	if len(history) != 1 {
		t.Fatalf("filtered history length = %d, want 1", len(history))
	}
	if history[0].Status != "running_tool" {
		t.Fatalf("filtered status = %q, want running_tool", history[0].Status)
	}
	if history[0].DecisionPriority != "medium" {
		t.Fatalf("filtered decision priority = %q, want medium", history[0].DecisionPriority)
	}
}

func TestCoordinatorSummarizesSessionRuns(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.start", RunID: "run-2", SessionID: "session-1"},
		{Type: "tool.called", RunID: "run-2", SessionID: "session-1", ToolName: "text.upper"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	summary := coordinator.SummaryBySession("session-1")
	if summary.RunCount != 3 {
		t.Fatalf("run count = %d, want 3", summary.RunCount)
	}
	if summary.StatusCounts["waiting_approval"] != 1 {
		t.Fatalf("waiting_approval count = %d, want 1", summary.StatusCounts["waiting_approval"])
	}
	if summary.StatusCounts["running_tool"] != 1 {
		t.Fatalf("running_tool count = %d, want 1", summary.StatusCounts["running_tool"])
	}
	if summary.PriorityCounts["high"] != 1 {
		t.Fatalf("high priority count = %d, want 1", summary.PriorityCounts["high"])
	}
	if summary.PriorityCounts["medium"] != 1 {
		t.Fatalf("medium priority count = %d, want 1", summary.PriorityCounts["medium"])
	}
	if summary.RecommendedActionCounts["request_approval"] != 1 {
		t.Fatalf("request_approval count = %d, want 1", summary.RecommendedActionCounts["request_approval"])
	}
	if summary.RecommendedActionCounts["await_tool_result"] != 1 {
		t.Fatalf("await_tool_result count = %d, want 1", summary.RecommendedActionCounts["await_tool_result"])
	}
	if summary.TopPriority != "high" {
		t.Fatalf("top priority = %q, want high", summary.TopPriority)
	}
}

func TestCoordinatorEvaluatesSessionSuggestions(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "run.error", RunID: "run-2", SessionID: "session-1", Message: "tool failed"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	suggestions := coordinator.EvaluateSession("session-1")
	if len(suggestions) != 3 {
		t.Fatalf("suggestion count = %d, want 3", len(suggestions))
	}
	if suggestions[0].RunID != "run-1" {
		t.Fatalf("top suggestion run_id = %q, want run-1", suggestions[0].RunID)
	}
	if suggestions[0].Category != "approval" {
		t.Fatalf("top suggestion category = %q, want approval", suggestions[0].Category)
	}
	if suggestions[0].SuggestedAction != "request_human_approval" {
		t.Fatalf("top suggestion action = %q, want request_human_approval", suggestions[0].SuggestedAction)
	}
	if !suggestions[0].Blocking {
		t.Fatal("approval suggestion should be blocking")
	}
	if suggestions[1].Category != "failure" {
		t.Fatalf("second suggestion category = %q, want failure", suggestions[1].Category)
	}
	if suggestions[2].Category != "completion" {
		t.Fatalf("third suggestion category = %q, want completion", suggestions[2].Category)
	}
}

func TestCoordinatorEvaluatesSessionSuggestionsWithFilter(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "run.error", RunID: "run-2", SessionID: "session-1", Message: "tool failed"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	suggestions := coordinator.EvaluateSessionWithFilter("session-1", orchestration.SuggestionFilter{
		Category:     "approval",
		Priority:     "high",
		BlockingOnly: true,
	})
	if len(suggestions) != 1 {
		t.Fatalf("filtered suggestion count = %d, want 1", len(suggestions))
	}
	if suggestions[0].RunID != "run-1" {
		t.Fatalf("filtered run_id = %q, want run-1", suggestions[0].RunID)
	}
}

func TestCoordinatorBuildsSessionPlan(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "run.error", RunID: "run-2", SessionID: "session-1", Message: "tool failed"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if plan.SessionID != "session-1" {
		t.Fatalf("plan session_id = %q, want session-1", plan.SessionID)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("plan step count = %d, want 3", len(plan.Steps))
	}
	if plan.Steps[0].RunID != "run-1" {
		t.Fatalf("first step run_id = %q, want run-1", plan.Steps[0].RunID)
	}
	if plan.Steps[0].Title == "" {
		t.Fatal("expected step title")
	}
	if plan.Steps[0].ActionKind != "approval" {
		t.Fatalf("first step action kind = %q, want approval", plan.Steps[0].ActionKind)
	}
	if plan.Steps[0].ActionID == "" {
		t.Fatal("expected stable action id")
	}
	if plan.Steps[0].Phase != "stabilize" {
		t.Fatalf("first step phase = %q, want stabilize", plan.Steps[0].Phase)
	}
	if plan.Steps[0].State != "pending" {
		t.Fatalf("first step state = %q, want pending", plan.Steps[0].State)
	}
	if len(plan.Steps) > 1 && plan.Steps[1].DependsOn != plan.Steps[0].ActionID {
		t.Fatalf("second step depends_on = %q, want %q", plan.Steps[1].DependsOn, plan.Steps[0].ActionID)
	}
	if len(plan.Steps) > 1 && plan.Steps[1].State != "blocked" {
		t.Fatalf("second step state = %q, want blocked", plan.Steps[1].State)
	}
	if plan.Summary == "" {
		t.Fatal("expected plan summary")
	}
	if plan.Groups["approval"] != 1 {
		t.Fatalf("approval group count = %d, want 1", plan.Groups["approval"])
	}
	if plan.PrioritySections["high"] != 2 {
		t.Fatalf("high priority section count = %d, want 2", plan.PrioritySections["high"])
	}
	if plan.PhaseSections["stabilize"] != 2 {
		t.Fatalf("stabilize phase count = %d, want 2", plan.PhaseSections["stabilize"])
	}
	if plan.Steps[0].UpdatedAt.IsZero() {
		t.Fatal("expected step updated_at")
	}
	if plan.Steps[0].Result != "" {
		t.Fatalf("initial step result = %q, want empty", plan.Steps[0].Result)
	}
}

func TestCoordinatorUpdatesPlanStepExecutionState(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) == 0 {
		t.Fatal("expected at least one plan step")
	}

	updated, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved manually")
	if err != nil {
		t.Fatalf("update plan step: %v", err)
	}
	if updated.State != "completed" {
		t.Fatalf("updated state = %q, want completed", updated.State)
	}
	if updated.Result != "approved manually" {
		t.Fatalf("updated result = %q, want approved manually", updated.Result)
	}

	reloaded := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if reloaded.Steps[0].State != "completed" {
		t.Fatalf("reloaded state = %q, want completed", reloaded.Steps[0].State)
	}
	if reloaded.Steps[0].Result != "approved manually" {
		t.Fatalf("reloaded result = %q, want approved manually", reloaded.Steps[0].Result)
	}
}

func TestCoordinatorRejectsInvalidPlanStepTransition(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "completed", "skipped ahead"); err == nil {
		t.Fatal("expected blocked -> completed transition to fail")
	}

	updated, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "ready", "")
	if err != nil {
		t.Fatalf("blocked -> ready transition failed: %v", err)
	}
	if updated.State != "ready" {
		t.Fatalf("updated state = %q, want ready", updated.State)
	}
}

func TestCoordinatorUnlocksDependentPlanStepAfterCompletion(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if plan.Steps[1].State != "blocked" {
		t.Fatalf("initial dependent state = %q, want blocked", plan.Steps[1].State)
	}

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved manually"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}

	reloaded := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if reloaded.Steps[1].State != "ready" {
		t.Fatalf("unlocked dependent state = %q, want ready", reloaded.Steps[1].State)
	}
}

func TestCoordinatorStoresPlanStepHistory(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) == 0 {
		t.Fatal("expected at least one plan step")
	}

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("update to in_progress: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "done"); err != nil {
		t.Fatalf("update to completed: %v", err)
	}

	history := coordinator.PlanStepHistory("session-1", plan.Steps[0].ActionID)
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].State != "in_progress" {
		t.Fatalf("first history state = %q, want in_progress", history[0].State)
	}
	if history[1].State != "completed" {
		t.Fatalf("second history state = %q, want completed", history[1].State)
	}
}

func TestCoordinatorFiltersPlanStepHistoryByState(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) == 0 {
		t.Fatal("expected at least one plan step")
	}

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("update to in_progress: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "done"); err != nil {
		t.Fatalf("update to completed: %v", err)
	}

	history := coordinator.FilteredPlanStepHistory("session-1", plan.Steps[0].ActionID, "completed")
	if len(history) != 1 {
		t.Fatalf("filtered history length = %d, want 1", len(history))
	}
	if history[0].State != "completed" {
		t.Fatalf("filtered state = %q, want completed", history[0].State)
	}
}

func TestCoordinatorSummarizesPlanExecutionOverview(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}

	overview := coordinator.PlanExecutionOverview("session-1", orchestration.SuggestionFilter{})
	if overview.TotalSteps != 2 {
		t.Fatalf("total steps = %d, want 2", overview.TotalSteps)
	}
	if overview.StateCounts["completed"] != 1 {
		t.Fatalf("completed count = %d, want 1", overview.StateCounts["completed"])
	}
	if overview.StateCounts["ready"] != 1 {
		t.Fatalf("ready count = %d, want 1", overview.StateCounts["ready"])
	}
	if overview.CompletedSteps != 1 {
		t.Fatalf("completed steps = %d, want 1", overview.CompletedSteps)
	}
	if overview.BlockingSteps != 0 {
		t.Fatalf("blocking steps = %d, want 0", overview.BlockingSteps)
	}
	if overview.ProgressPercent != 50 {
		t.Fatalf("progress percent = %d, want 50", overview.ProgressPercent)
	}
	if overview.HasBlockedSteps {
		t.Fatal("expected overview to report no blocked steps")
	}
	if overview.ActiveSteps != 1 {
		t.Fatalf("active steps = %d, want 1", overview.ActiveSteps)
	}
	if overview.ReadySteps != 1 {
		t.Fatalf("ready steps = %d, want 1", overview.ReadySteps)
	}
	if overview.PendingSteps != 0 {
		t.Fatalf("pending steps = %d, want 0", overview.PendingSteps)
	}
	if overview.InProgressSteps != 0 {
		t.Fatalf("in_progress steps = %d, want 0", overview.InProgressSteps)
	}
	if overview.TerminalSteps != 1 {
		t.Fatalf("terminal steps = %d, want 1", overview.TerminalSteps)
	}
	if overview.FailedSteps != 0 {
		t.Fatalf("failed steps = %d, want 0", overview.FailedSteps)
	}
	if overview.LatestActiveAction != plan.Steps[1].ActionID {
		t.Fatalf("latest active action = %q, want %q", overview.LatestActiveAction, plan.Steps[1].ActionID)
	}
	if overview.LatestReadyAction != plan.Steps[1].ActionID {
		t.Fatalf("latest ready action = %q, want %q", overview.LatestReadyAction, plan.Steps[1].ActionID)
	}
	if overview.LatestInProgressAction != "" {
		t.Fatalf("latest in_progress action = %q, want empty", overview.LatestInProgressAction)
	}
	if overview.LatestPendingAction != "" {
		t.Fatalf("latest pending action = %q, want empty", overview.LatestPendingAction)
	}
	if overview.LatestTerminalAction != plan.Steps[0].ActionID {
		t.Fatalf("latest terminal action = %q, want %q", overview.LatestTerminalAction, plan.Steps[0].ActionID)
	}
	if overview.LatestCompletedAction != plan.Steps[0].ActionID {
		t.Fatalf("latest completed action = %q, want %q", overview.LatestCompletedAction, plan.Steps[0].ActionID)
	}
	if overview.LatestFailedAction != "" {
		t.Fatalf("latest failed action = %q, want empty", overview.LatestFailedAction)
	}
	if overview.LatestBlockedAction != "" {
		t.Fatalf("latest blocked action = %q, want empty", overview.LatestBlockedAction)
	}
	if overview.LastUpdatedAt.IsZero() {
		t.Fatal("expected last updated at")
	}
}

func TestCoordinatorSummarizesLatestInProgressActionInPlanExecutionOverview(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	overview := coordinator.PlanExecutionOverview("session-1", orchestration.SuggestionFilter{})
	if overview.InProgressSteps != 1 {
		t.Fatalf("in_progress steps = %d, want 1", overview.InProgressSteps)
	}
	if overview.ReadySteps != 0 {
		t.Fatalf("ready steps = %d, want 0", overview.ReadySteps)
	}
	if overview.LatestInProgressAction != plan.Steps[1].ActionID {
		t.Fatalf("latest in_progress action = %q, want %q", overview.LatestInProgressAction, plan.Steps[1].ActionID)
	}
	if overview.LatestReadyAction != "" {
		t.Fatalf("latest ready action = %q, want empty", overview.LatestReadyAction)
	}
	if overview.LatestActiveAction != plan.Steps[1].ActionID {
		t.Fatalf("latest active action = %q, want %q", overview.LatestActiveAction, plan.Steps[1].ActionID)
	}
}

func TestCoordinatorSummarizesLatestBlockedActionInPlanExecutionOverview(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "run.error", RunID: "run-2", SessionID: "session-1", Message: "tool failed"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	overview := coordinator.PlanExecutionOverview("session-1", orchestration.SuggestionFilter{})
	if overview.FailedSteps != 0 {
		t.Fatalf("failed steps = %d, want 0", overview.FailedSteps)
	}
	if overview.PendingSteps != 1 {
		t.Fatalf("pending steps = %d, want 1", overview.PendingSteps)
	}
	if overview.ReadySteps != 0 {
		t.Fatalf("ready steps = %d, want 0", overview.ReadySteps)
	}
	if overview.InProgressSteps != 0 {
		t.Fatalf("in_progress steps = %d, want 0", overview.InProgressSteps)
	}
	if overview.LatestBlockedAction != "run-1:request_human_approval" {
		t.Fatalf("latest blocked action = %q, want %q", overview.LatestBlockedAction, "run-1:request_human_approval")
	}
	if overview.LatestPendingAction != "run-1:request_human_approval" {
		t.Fatalf("latest pending action = %q, want %q", overview.LatestPendingAction, "run-1:request_human_approval")
	}
	if overview.LatestFailedAction != "" {
		t.Fatalf("latest failed action = %q, want empty", overview.LatestFailedAction)
	}
	if overview.LatestCompletedAction != "" {
		t.Fatalf("latest completed action = %q, want empty", overview.LatestCompletedAction)
	}
}

func TestCoordinatorListsSessionPlanExecutionHistory(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	history := coordinator.SessionPlanExecutionHistory("session-1")
	if len(history) != 2 {
		t.Fatalf("session history length = %d, want 2", len(history))
	}
	foundFirst := false
	foundSecond := false
	for _, record := range history {
		if record.ActionID == plan.Steps[0].ActionID {
			foundFirst = true
		}
		if record.ActionID == plan.Steps[1].ActionID {
			foundSecond = true
		}
	}
	if !foundFirst || !foundSecond {
		t.Fatalf("session history action ids = %#v, want both %q and %q", history, plan.Steps[0].ActionID, plan.Steps[1].ActionID)
	}
	summary := orchestration.SummarizeSessionPlanExecutionHistory(history)
	if summary.RecordCount != 2 {
		t.Fatalf("summary record count = %d, want 2", summary.RecordCount)
	}
	if summary.ActionCounts[plan.Steps[0].ActionID] != 1 {
		t.Fatalf("first action count = %d, want 1", summary.ActionCounts[plan.Steps[0].ActionID])
	}
	if summary.StateCounts["completed"] != 1 {
		t.Fatalf("completed state count = %d, want 1", summary.StateCounts["completed"])
	}
	if summary.StateCounts["in_progress"] != 1 {
		t.Fatalf("in_progress state count = %d, want 1", summary.StateCounts["in_progress"])
	}
	if summary.LastRecordedAt.IsZero() {
		t.Fatal("expected last recorded at")
	}
}

func TestCoordinatorFiltersSessionPlanExecutionHistoryByState(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	history := coordinator.FilteredSessionPlanExecutionHistory("session-1", "in_progress", "", time.Time{}, time.Time{})
	if len(history) != 1 {
		t.Fatalf("filtered session history length = %d, want 1", len(history))
	}
	if history[0].State != "in_progress" {
		t.Fatalf("filtered session history state = %q, want in_progress", history[0].State)
	}
}

func TestCoordinatorSummarizesLatestActiveActionInSessionPlanExecutionHistory(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	summary := orchestration.SummarizeSessionPlanExecutionHistory(coordinator.SessionPlanExecutionHistory("session-1"))
	if summary.LatestActiveAction != plan.Steps[1].ActionID {
		t.Fatalf("latest active action = %q, want %q", summary.LatestActiveAction, plan.Steps[1].ActionID)
	}
	if summary.LatestActiveAt.IsZero() {
		t.Fatal("expected latest active at timestamp")
	}
	if summary.LatestActiveResult != "reviewing" {
		t.Fatalf("latest active result = %q, want reviewing", summary.LatestActiveResult)
	}
	if summary.LatestInProgressResult != "reviewing" {
		t.Fatalf("latest in_progress result = %q, want reviewing", summary.LatestInProgressResult)
	}
	if summary.LatestInProgressAt.IsZero() {
		t.Fatal("expected latest in_progress at timestamp")
	}
}

func TestCoordinatorSummarizesLatestTerminalActionsInSessionPlanExecutionHistory(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "failed", "review failed"); err != nil {
		t.Fatalf("fail second step: %v", err)
	}

	summary := orchestration.SummarizeSessionPlanExecutionHistory(coordinator.SessionPlanExecutionHistory("session-1"))
	if summary.LatestCompletedAction != plan.Steps[0].ActionID {
		t.Fatalf("latest completed action = %q, want %q", summary.LatestCompletedAction, plan.Steps[0].ActionID)
	}
	if summary.LatestFailedAction != plan.Steps[1].ActionID {
		t.Fatalf("latest failed action = %q, want %q", summary.LatestFailedAction, plan.Steps[1].ActionID)
	}
	if summary.LatestTerminalAction != plan.Steps[1].ActionID {
		t.Fatalf("latest terminal action = %q, want %q", summary.LatestTerminalAction, plan.Steps[1].ActionID)
	}
	if summary.LatestTerminalState != "failed" {
		t.Fatalf("latest terminal state = %q, want failed", summary.LatestTerminalState)
	}
	if summary.LatestTerminalResult != "review failed" {
		t.Fatalf("latest terminal result = %q, want review failed", summary.LatestTerminalResult)
	}
	if summary.LatestTerminalAt.IsZero() {
		t.Fatal("expected latest terminal at timestamp")
	}
	if summary.LatestCompletedResult != "approved" {
		t.Fatalf("latest completed result = %q, want approved", summary.LatestCompletedResult)
	}
	if summary.LatestCompletedState != "completed" {
		t.Fatalf("latest completed state = %q, want completed", summary.LatestCompletedState)
	}
	if summary.LatestFailedResult != "review failed" {
		t.Fatalf("latest failed result = %q, want review failed", summary.LatestFailedResult)
	}
	if summary.LatestFailedState != "failed" {
		t.Fatalf("latest failed state = %q, want failed", summary.LatestFailedState)
	}
	if summary.LatestCompletedAt.IsZero() {
		t.Fatal("expected latest completed at timestamp")
	}
	if summary.LatestFailedAt.IsZero() {
		t.Fatal("expected latest failed at timestamp")
	}
	if summary.LatestFailedAt.Before(summary.LatestCompletedAt) {
		t.Fatalf("latest failed at = %s, want same or after latest completed at = %s", summary.LatestFailedAt, summary.LatestCompletedAt)
	}
	if !summary.LatestTerminalAt.Equal(summary.LatestFailedAt) {
		t.Fatalf("latest terminal at = %s, want %s", summary.LatestTerminalAt, summary.LatestFailedAt)
	}
	if !summary.LatestActiveAt.IsZero() {
		t.Fatalf("latest active at = %s, want zero", summary.LatestActiveAt)
	}
	if !summary.LatestInProgressAt.IsZero() {
		t.Fatalf("latest in_progress at = %s, want zero", summary.LatestInProgressAt)
	}
	if summary.LatestInProgressResult != "" {
		t.Fatalf("latest in_progress result = %q, want empty", summary.LatestInProgressResult)
	}
	if summary.LatestInProgressState != "" {
		t.Fatalf("latest in_progress state = %q, want empty", summary.LatestInProgressState)
	}
	if !summary.LatestReadyAt.IsZero() {
		t.Fatalf("latest ready at = %s, want zero", summary.LatestReadyAt)
	}
	if summary.LatestReadyResult != "" {
		t.Fatalf("latest ready result = %q, want empty", summary.LatestReadyResult)
	}
	if summary.LatestReadyState != "" {
		t.Fatalf("latest ready state = %q, want empty", summary.LatestReadyState)
	}
	if !summary.LatestPendingAt.IsZero() {
		t.Fatalf("latest pending at = %s, want zero", summary.LatestPendingAt)
	}
	if summary.LatestPendingResult != "" {
		t.Fatalf("latest pending result = %q, want empty", summary.LatestPendingResult)
	}
	if summary.LatestPendingState != "" {
		t.Fatalf("latest pending state = %q, want empty", summary.LatestPendingState)
	}
	if summary.LatestActiveResult != "" {
		t.Fatalf("latest active result = %q, want empty", summary.LatestActiveResult)
	}
	if summary.LatestActiveState != "" {
		t.Fatalf("latest active state = %q, want empty", summary.LatestActiveState)
	}
	if summary.LatestActiveAction != "" {
		t.Fatalf("latest active action = %q, want empty", summary.LatestActiveAction)
	}
}

func TestCoordinatorSummarizesLatestExecutionStatesInSessionPlanExecutionHistory(t *testing.T) {
	records := []orchestration.PlanStepRecord{
		{SessionID: "session-1", ActionID: "run-1:request_human_approval", State: "pending", Result: "waiting", RecordedAt: time.Now().Add(-3 * time.Second).UTC()},
		{SessionID: "session-1", ActionID: "run-2:close_or_follow_up", State: "ready", Result: "queued", RecordedAt: time.Now().Add(-2 * time.Second).UTC()},
		{SessionID: "session-1", ActionID: "run-3:review_response", State: "in_progress", Result: "reviewing", RecordedAt: time.Now().Add(-1 * time.Second).UTC()},
	}

	summary := orchestration.SummarizeSessionPlanExecutionHistory(records)
	if summary.LatestPendingAction != "run-1:request_human_approval" {
		t.Fatalf("latest pending action = %q, want %q", summary.LatestPendingAction, "run-1:request_human_approval")
	}
	if summary.LatestPendingResult != "waiting" {
		t.Fatalf("latest pending result = %q, want waiting", summary.LatestPendingResult)
	}
	if summary.LatestPendingState != "pending" {
		t.Fatalf("latest pending state = %q, want pending", summary.LatestPendingState)
	}
	if summary.LatestReadyAction != "run-2:close_or_follow_up" {
		t.Fatalf("latest ready action = %q, want %q", summary.LatestReadyAction, "run-2:close_or_follow_up")
	}
	if summary.LatestReadyResult != "queued" {
		t.Fatalf("latest ready result = %q, want queued", summary.LatestReadyResult)
	}
	if summary.LatestReadyState != "ready" {
		t.Fatalf("latest ready state = %q, want ready", summary.LatestReadyState)
	}
	if summary.LatestReadyAt.IsZero() {
		t.Fatal("expected latest ready at timestamp")
	}
	if summary.LatestInProgressAction != "run-3:review_response" {
		t.Fatalf("latest in_progress action = %q, want %q", summary.LatestInProgressAction, "run-3:review_response")
	}
	if summary.LatestActiveAction != "run-3:review_response" {
		t.Fatalf("latest active action = %q, want %q", summary.LatestActiveAction, "run-3:review_response")
	}
	if summary.LatestRecordedAction != "run-3:review_response" {
		t.Fatalf("latest recorded action = %q, want %q", summary.LatestRecordedAction, "run-3:review_response")
	}
	if summary.LatestRecordedState != "in_progress" {
		t.Fatalf("latest recorded state = %q, want in_progress", summary.LatestRecordedState)
	}
	if summary.LatestRecordedResult != "reviewing" {
		t.Fatalf("latest recorded result = %q, want reviewing", summary.LatestRecordedResult)
	}
	if summary.LatestActiveAt.IsZero() {
		t.Fatal("expected latest active at timestamp")
	}
	if summary.LatestActiveResult != "reviewing" {
		t.Fatalf("latest active result = %q, want reviewing", summary.LatestActiveResult)
	}
	if summary.LatestActiveState != "in_progress" {
		t.Fatalf("latest active state = %q, want in_progress", summary.LatestActiveState)
	}
	if summary.LatestInProgressResult != "reviewing" {
		t.Fatalf("latest in_progress result = %q, want reviewing", summary.LatestInProgressResult)
	}
	if summary.LatestInProgressState != "in_progress" {
		t.Fatalf("latest in_progress state = %q, want in_progress", summary.LatestInProgressState)
	}
	if summary.LatestInProgressAt.IsZero() {
		t.Fatal("expected latest in_progress at timestamp")
	}
	if summary.LatestReadyAt.IsZero() {
		t.Fatal("expected latest ready at timestamp")
	}
	if summary.LatestPendingAt.IsZero() {
		t.Fatal("expected latest pending at timestamp")
	}
}

func TestCoordinatorFiltersSessionPlanExecutionHistoryByActionID(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "completed", "reviewed"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	history := coordinator.FilteredSessionPlanExecutionHistory("session-1", "", plan.Steps[1].ActionID, time.Time{}, time.Time{})
	if len(history) != 1 {
		t.Fatalf("filtered session history length = %d, want 1", len(history))
	}
	if history[0].ActionID != plan.Steps[1].ActionID {
		t.Fatalf("filtered session history action_id = %q, want %q", history[0].ActionID, plan.Steps[1].ActionID)
	}
}

func TestCoordinatorFiltersSessionPlanExecutionHistoryBySince(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	cutoff := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	history := coordinator.FilteredSessionPlanExecutionHistory("session-1", "", "", cutoff, time.Time{})
	if len(history) != 1 {
		t.Fatalf("filtered session history length = %d, want 1", len(history))
	}
	if history[0].ActionID != plan.Steps[1].ActionID {
		t.Fatalf("filtered session history action_id = %q, want %q", history[0].ActionID, plan.Steps[1].ActionID)
	}
}

func TestCoordinatorFiltersSessionPlanExecutionHistoryByUntil(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 2 {
		t.Fatal("expected at least two plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	firstHistory := coordinator.SessionPlanExecutionHistory("session-1")
	if len(firstHistory) != 1 {
		t.Fatalf("first history length = %d, want 1", len(firstHistory))
	}
	cutoff := firstHistory[0].RecordedAt
	time.Sleep(10 * time.Millisecond)
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}

	history := coordinator.FilteredSessionPlanExecutionHistory("session-1", "", "", time.Time{}, cutoff)
	if len(history) != 1 {
		t.Fatalf("filtered session history length = %d, want 1", len(history))
	}
	if history[0].ActionID != plan.Steps[0].ActionID {
		t.Fatalf("filtered session history action_id = %q, want %q", history[0].ActionID, plan.Steps[0].ActionID)
	}
}

func TestCoordinatorFiltersSessionPlanExecutionHistoryBySinceAndUntil(t *testing.T) {
	coordinator := orchestration.NewCoordinator()
	for _, event := range []orchestration.Event{
		{Type: "permission.required", RunID: "run-1", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-2", SessionID: "session-1"},
		{Type: "agent.lifecycle.end", RunID: "run-3", SessionID: "session-1"},
	} {
		if err := coordinator.Handle(context.Background(), event); err != nil {
			t.Fatalf("handle event %q: %v", event.Type, err)
		}
	}

	plan := coordinator.PlanSession("session-1", orchestration.SuggestionFilter{})
	if len(plan.Steps) < 3 {
		t.Fatal("expected at least three plan steps")
	}
	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[0].ActionID, "completed", "approved"); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	firstHistory := coordinator.SessionPlanExecutionHistory("session-1")
	if len(firstHistory) != 1 {
		t.Fatalf("first history length = %d, want 1", len(firstHistory))
	}
	since := firstHistory[0].RecordedAt
	time.Sleep(10 * time.Millisecond)

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[1].ActionID, "in_progress", "reviewing"); err != nil {
		t.Fatalf("advance second step: %v", err)
	}
	secondHistory := coordinator.SessionPlanExecutionHistory("session-1")
	if len(secondHistory) != 2 {
		t.Fatalf("second history length = %d, want 2", len(secondHistory))
	}
	var until time.Time
	for _, record := range secondHistory {
		if record.ActionID == plan.Steps[1].ActionID {
			until = record.RecordedAt
		}
	}
	if until.IsZero() {
		t.Fatal("expected until timestamp from second step")
	}
	time.Sleep(10 * time.Millisecond)

	if _, err := coordinator.UpdatePlanStep("session-1", plan.Steps[2].ActionID, "ready", "unblocked"); err != nil {
		t.Fatalf("advance third step: %v", err)
	}

	history := coordinator.FilteredSessionPlanExecutionHistory("session-1", "", "", since, until)
	if len(history) != 1 {
		t.Fatalf("windowed history length = %d, want 1", len(history))
	}
	if history[0].ActionID != plan.Steps[1].ActionID {
		t.Fatalf("windowed history action_id = %q, want %q", history[0].ActionID, plan.Steps[1].ActionID)
	}
}
