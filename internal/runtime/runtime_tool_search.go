package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

const (
	defaultToolSearchMaxResults    = 5
	maxToolSearchResults           = 20
	maxToolSearchesPerTurn         = 8
	maxConsecutiveSameToolSearches = 3
	maxRuntimeConcurrentToolCalls  = 10
	toolDiscoveryReasonBase        = "base_tool"
	toolDiscoveryReasonDeferred    = "deferred_until_search"
)

type runtimeToolDiscoveryState struct {
	SelectedByTurn  map[string]map[string]struct{}
	SearchesByTurn  map[string]int
	LastQueryByTurn map[string]string
	RepeatByTurn    map[string]int
	RunningByTurn   map[string]int
}

func newRuntimeToolDiscoveryState() runtimeToolDiscoveryState {
	return runtimeToolDiscoveryState{
		SelectedByTurn:  make(map[string]map[string]struct{}),
		SearchesByTurn:  make(map[string]int),
		LastQueryByTurn: make(map[string]string),
		RepeatByTurn:    make(map[string]int),
		RunningByTurn:   make(map[string]int),
	}
}

func (r *runtimeService) SearchTools(ctx context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	return r.searchTools(ctx, req)
}

func (r *runtimeService) SearchToolsForAgent(ctx context.Context, req agent.SchedulerToolSearchRequest) (agent.SchedulerToolSearchResult, error) {
	resp, err := r.searchTools(ctx, RuntimeToolSearchRequest{
		Query:      req.Query,
		MaxResults: req.MaxResults,
		SessionID:  req.SessionID,
		TurnID:     req.TurnID,
		Source:     "agent_tool",
	})
	if err != nil {
		return agent.SchedulerToolSearchResult{}, err
	}
	matches := make([]string, 0, len(resp.Results))
	for _, result := range resp.Results {
		matches = append(matches, result.Name)
	}
	return agent.SchedulerToolSearchResult{
		Query:   resp.Query,
		Matches: matches,
		Total:   resp.Total,
		Message: toolSearchAgentMessage(resp),
	}, nil
}

func toolSearchAgentMessage(resp RuntimeToolSearchResponse) string {
	if resp.GuardrailError != "" {
		return resp.GuardrailError
	}
	if len(resp.Results) == 0 {
		return fmt.Sprintf("No matching runtime tools found for %q.", resp.Query)
	}
	lines := []string{"Selected runtime tools:"}
	for _, result := range resp.Results {
		lines = append(lines, fmt.Sprintf("- %s: %s", result.Name, firstNonEmpty(result.Description, result.SchemaSummary)))
	}
	return strings.Join(lines, "\n")
}

func (r *runtimeService) SelectToolsForTurn(ctx context.Context, req agent.SchedulerToolDisclosureRequest) (agent.SchedulerToolDisclosureResult, error) {
	if len(req.Tools) == 0 {
		return agent.SchedulerToolDisclosureResult{}, nil
	}
	turnID := firstNonEmpty(req.TurnID, req.SessionID, "global")
	r.mu.Lock()
	selectedSet := r.toolDiscovery.SelectedByTurn[turnID]
	if selectedSet == nil {
		selectedSet = make(map[string]struct{})
		r.toolDiscovery.SelectedByTurn[turnID] = selectedSet
	}
	r.mu.Unlock()

	var selected []string
	var omitted []string
	var selectedBucket RuntimeBudgetBucket
	var omittedBucket RuntimeBudgetBucket
	for _, tool := range req.Tools {
		if tool.Name == "" {
			continue
		}
		isSelected := tool.Base
		if _, ok := selectedSet[tool.Name]; ok {
			isSelected = true
		}
		if isSelected {
			selected = append(selected, tool.Name)
			selectedBucket.Count++
			selectedBucket.EstimatedTokens += tool.EstimatedTokens
			continue
		}
		omitted = append(omitted, tool.Name)
		omittedBucket.Count++
		omittedBucket.EstimatedTokens += tool.EstimatedTokens
	}
	slices.Sort(selected)
	slices.Sort(omitted)
	r.recordToolDisclosure(ctx, req.SessionID, req.TurnID, selected, omitted, selectedBucket, omittedBucket)
	return agent.SchedulerToolDisclosureResult{Selected: selected, Omitted: omitted}, nil
}

func (r *runtimeService) searchTools(ctx context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return RuntimeToolSearchResponse{}, errors.New("query is required")
	}
	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = defaultToolSearchMaxResults
	}
	if maxResults > maxToolSearchResults {
		maxResults = maxToolSearchResults
	}
	if ctx.Err() != nil {
		resp := RuntimeToolSearchResponse{Query: query, Guardrail: "cancelled", GuardrailError: "tool search cancelled"}
		r.recordDeadlockPrevented(req.SessionID, req.TurnID, "", "tool_search_cancelled", resp.GuardrailError)
		return resp, ctx.Err()
	}
	if blocked, reason := r.preventRepeatedToolSearch(req.TurnID, query); blocked {
		resp := RuntimeToolSearchResponse{Query: query, Guardrail: reason, GuardrailError: "tool search guardrail blocked repeated search"}
		r.recordDeadlockPrevented(req.SessionID, req.TurnID, "", reason, resp.GuardrailError)
		return resp, nil
	}
	caps, err := r.Capabilities(ctx)
	if err != nil {
		return RuntimeToolSearchResponse{}, err
	}
	results, omitted := r.filterAndScoreToolSearch(query, caps.Capabilities, maxResults)
	selectedNames := make([]string, 0, len(results))
	for _, result := range results {
		selectedNames = append(selectedNames, result.Name)
	}
	r.markDiscoveredTools(req.TurnID, selectedNames)
	resp := RuntimeToolSearchResponse{
		Query:        query,
		Results:      results,
		Omitted:      omitted,
		Total:        len(results),
		BudgetImpact: toolSearchBudgetImpact(results, omitted),
	}
	r.recordToolSearch(req, resp)
	return resp, nil
}

func (r *runtimeService) filterAndScoreToolSearch(query string, capabilities []RuntimeCapability, maxResults int) ([]RuntimeToolSearchResult, []RuntimeToolSearchOmission) {
	selectNames := parseToolSelectQuery(query)
	var scored []RuntimeToolSearchResult
	var omitted []RuntimeToolSearchOmission
	for _, cap := range capabilities {
		cap = finalizeRuntimeCapabilityMetadata(cap)
		if cap.Name == agent.ToolSearchToolName || cap.ID == "builtin:"+agent.ToolSearchToolName {
			omitted = append(omitted, toolSearchOmission(cap, "recursion_guard"))
			continue
		}
		if !cap.Enabled || cap.State == capabilityStateDisabled || cap.State == capabilityStateUnavailable || cap.State == capabilityStateFailed {
			omitted = append(omitted, toolSearchOmission(cap, firstNonEmpty(cap.Reason, cap.State, "unavailable")))
			continue
		}
		decision := r.evaluateCapabilitySearchPolicy(cap)
		if decision.Decision == permission.PolicyDeny {
			omitted = append(omitted, toolSearchOmission(cap, "policy_denied"))
			continue
		}
		score := scoreToolSearchCapability(query, selectNames, cap)
		if score <= 0 {
			omitted = append(omitted, toolSearchOmission(cap, "query_mismatch"))
			continue
		}
		scored = append(scored, RuntimeToolSearchResult{
			ID:            cap.ID,
			Kind:          cap.Kind,
			Name:          cap.Name,
			Source:        cap.Source,
			Description:   cap.Description,
			Risk:          cap.Risk,
			CapabilityID:  cap.CapabilityID,
			SchemaDigest:  cap.SchemaDigest,
			SchemaSummary: cap.SchemaSummary,
			State:         cap.State,
			Score:         score,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].ID < scored[j].ID
	})
	if len(scored) > maxResults {
		for _, result := range scored[maxResults:] {
			omitted = append(omitted, RuntimeToolSearchOmission{
				ID: result.ID, Kind: result.Kind, Name: result.Name, Source: result.Source, Reason: "max_results", Risk: result.Risk, State: result.State,
			})
		}
		scored = scored[:maxResults]
	}
	return scored, omitted
}

func (r *runtimeService) evaluateCapabilitySearchPolicy(capability RuntimeCapability) permission.PolicyResult {
	if capability.Kind == "skill" || capability.Kind == "mcp_prompt" {
		return permission.PolicyResult{Decision: permission.PolicyAllow, Risk: permission.RiskRead, Reason: "Discovery exposes metadata only.", Mode: permission.PolicyMode(r.policy.Mode)}
	}
	r.mu.Lock()
	mode := permission.PolicyMode(r.policy.Mode)
	r.mu.Unlock()
	return permission.NewPermissionPolicy(mode).Evaluate(scheduler.ToolCall{
		ID:           capability.ID,
		Name:         capability.Name,
		Source:       capabilityToolSource(capability),
		CapabilityID: capability.ID,
		Status:       scheduler.ToolCallPending,
		InputSummary: capabilityPolicySummary(capability),
	})
}

func parseToolSelectQuery(query string) map[string]struct{} {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "select:") {
		return nil
	}
	raw := strings.TrimSpace(query[len("select:"):])
	result := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func scoreToolSearchCapability(query string, selectNames map[string]struct{}, cap RuntimeCapability) int {
	if len(selectNames) > 0 {
		for _, key := range []string{cap.Name, cap.ID, cap.CapabilityID} {
			if _, ok := selectNames[strings.ToLower(key)]; ok {
				return 100
			}
		}
		return 0
	}
	queryTerms := strings.Fields(strings.ToLower(query))
	if len(queryTerms) == 0 {
		return 0
	}
	text := strings.ToLower(cap.SearchText)
	score := 0
	for _, term := range queryTerms {
		switch {
		case strings.EqualFold(term, cap.Name):
			score += 20
		case strings.Contains(strings.ToLower(cap.Name), term):
			score += 12
		case strings.Contains(strings.ToLower(cap.ID), term), strings.Contains(strings.ToLower(cap.Source), term):
			score += 8
		case strings.Contains(text, term):
			score += 3
		default:
			return 0
		}
	}
	return score
}

func toolSearchOmission(cap RuntimeCapability, reason string) RuntimeToolSearchOmission {
	return RuntimeToolSearchOmission{
		ID: cap.ID, Kind: cap.Kind, Name: cap.Name, Source: cap.Source, Reason: reason, Risk: cap.Risk, State: cap.State,
	}
}

func toolSearchBudgetImpact(results []RuntimeToolSearchResult, omitted []RuntimeToolSearchOmission) RuntimeToolSchemaBudgetImpact {
	var impact RuntimeToolSchemaBudgetImpact
	for _, result := range results {
		impact.Selected.Count++
		impact.Selected.EstimatedTokens += estimateRuntimeTokens(result.Name + " " + result.Description + " " + result.SchemaSummary)
	}
	for _, item := range omitted {
		impact.Omitted.Count++
		impact.Omitted.EstimatedTokens += estimateRuntimeTokens(item.Name + " " + item.Reason)
	}
	return impact
}

func (r *runtimeService) markDiscoveredTools(turnID string, names []string) {
	if turnID == "" || len(names) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	selected := r.toolDiscovery.SelectedByTurn[turnID]
	if selected == nil {
		selected = make(map[string]struct{})
		r.toolDiscovery.SelectedByTurn[turnID] = selected
	}
	for _, name := range names {
		selected[name] = struct{}{}
	}
}

func (r *runtimeService) preventRepeatedToolSearch(turnID, query string) (bool, string) {
	if turnID == "" {
		return false, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolDiscovery.SearchesByTurn[turnID]++
	if r.toolDiscovery.SearchesByTurn[turnID] > maxToolSearchesPerTurn {
		return true, "max_searches_per_turn"
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if r.toolDiscovery.LastQueryByTurn[turnID] == normalized {
		r.toolDiscovery.RepeatByTurn[turnID]++
	} else {
		r.toolDiscovery.LastQueryByTurn[turnID] = normalized
		r.toolDiscovery.RepeatByTurn[turnID] = 1
	}
	if r.toolDiscovery.RepeatByTurn[turnID] > maxConsecutiveSameToolSearches {
		return true, "repeat_search"
	}
	return false, ""
}

func (r *runtimeService) incrementRunningToolGuard(call agent.SchedulerToolCall) (bool, string) {
	if call.TurnID == "" {
		return false, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.toolDiscovery.RunningByTurn[call.TurnID] >= maxRuntimeConcurrentToolCalls {
		return true, "max_concurrent_tools"
	}
	r.toolDiscovery.RunningByTurn[call.TurnID]++
	return false, ""
}

func (r *runtimeService) decrementRunningToolGuard(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.toolDiscovery.RunningByTurn[turnID] > 0 {
		r.toolDiscovery.RunningByTurn[turnID]--
	}
}

func (r *runtimeService) recordToolDisclosure(ctx context.Context, sessionID, turnID string, selected, omitted []string, selectedBudget, omittedBudget RuntimeBudgetBucket) {
	payload := map[string]any{
		"selected": selected,
		"omitted":  omitted,
		"budget": map[string]any{
			"selected": selectedBudget,
			"omitted":  omittedBudget,
		},
		"summary": fmt.Sprintf("%d selected, %d omitted tool schemas", len(selected), len(omitted)),
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolDiscoverySelected, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, Payload: payload})
	if len(omitted) > 0 {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolDiscoveryOmitted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, Payload: map[string]any{"omitted": omitted, "reason": toolDiscoveryReasonDeferred, "budget": omittedBudget, "summary": fmt.Sprintf("%d omitted tool schemas", len(omitted))}})
	}
	r.writeAudit(auditEntry{RequestID: turnID, Event: "tool_discovery_selected", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, Extra: map[string]any{"selected": selected, "omitted": omitted, "budget": payload["budget"]}})
}

func (r *runtimeService) recordToolSearch(req RuntimeToolSearchRequest, resp RuntimeToolSearchResponse) {
	selected := make([]string, 0, len(resp.Results))
	for _, result := range resp.Results {
		selected = append(selected, result.Name)
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolSearchPerformed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: req.SessionID, TurnID: req.TurnID, Payload: map[string]any{"query": req.Query, "selected": selected, "omitted_count": len(resp.Omitted), "budget_impact": resp.BudgetImpact, "summary": fmt.Sprintf("%d matches", len(resp.Results))}})
	r.writeAudit(auditEntry{RequestID: req.TurnID, Event: "tool_search_performed", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: req.SessionID, Extra: map[string]any{"query": req.Query, "selected": selected, "omitted": resp.Omitted, "budget_impact": resp.BudgetImpact}})
}

func (r *runtimeService) recordDeadlockPrevented(sessionID, turnID, toolCallID, reason, detail string) {
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSchedulerDeadlockPrevented, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, ToolCallID: toolCallID, Payload: map[string]any{"reason": reason, "detail": detail, "summary": reason}})
	r.writeAudit(auditEntry{RequestID: turnID, Event: "scheduler_deadlock_prevented", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, ToolCallID: toolCallID, Extra: map[string]any{"reason": reason, "detail": detail}})
}
