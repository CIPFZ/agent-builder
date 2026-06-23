package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	defaultToolSearchMaxResults    = 5
	maxToolSearchResults           = 20
	maxToolSearchesPerTurn         = 8
	maxConsecutiveSameToolSearches = 3
	maxRuntimeConcurrentToolCalls  = 10
	toolDiscoveryReasonBase        = "base_tool"
	toolDiscoveryReasonDeferred    = "deferred_until_search"
	toolDiscoveryReasonBudget      = "schema_budget"
)

type runtimeToolDiscoveryState struct {
	SelectedByTurn   map[string]map[string]struct{}
	DisclosureByTurn map[string]runtimeToolDisclosureBudget
	SearchesByTurn   map[string]int
	LastQueryByTurn  map[string]string
	RepeatByTurn     map[string]int
	RunningByTurn    map[string]int
}

func newRuntimeToolDiscoveryState() runtimeToolDiscoveryState {
	return runtimeToolDiscoveryState{
		SelectedByTurn:   make(map[string]map[string]struct{}),
		DisclosureByTurn: make(map[string]runtimeToolDisclosureBudget),
		SearchesByTurn:   make(map[string]int),
		LastQueryByTurn:  make(map[string]string),
		RepeatByTurn:     make(map[string]int),
		RunningByTurn:    make(map[string]int),
	}
}

type runtimeToolDisclosureBudget struct {
	Selected RuntimeBudgetBucket
	Omitted  RuntimeBudgetBucket
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
		ToolCallID: req.ToolCallID,
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
	var omitted []RuntimeToolSearchOmission
	var selectedBucket RuntimeBudgetBucket
	var omittedBucket RuntimeBudgetBucket
	for _, tool := range req.Tools {
		if tool.Name == "" {
			continue
		}
		if task, ok := r.agentTaskForChildSession(ctx, req.SessionID); ok {
			call := agent.SchedulerToolCall{
				SessionID:    req.SessionID,
				TurnID:       req.TurnID,
				Name:         tool.Name,
				Source:       tool.Source,
				CapabilityID: tool.CapabilityID,
				InputSummary: tool.SchemaSummary,
			}
			if reason := r.agentTaskScopeViolation(task, call); reason != "" {
				omitted = append(omitted, toolDisclosureOmission(tool, "task_scope_denied"))
				omittedBucket.Count++
				omittedBucket.EstimatedTokens += tool.EstimatedTokens
				r.recordAgentTaskScope(ctx, task, false, reason)
				continue
			}
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
		omitted = append(omitted, toolDisclosureOmission(tool, toolDiscoveryReasonDeferred))
		omittedBucket.Count++
		omittedBucket.EstimatedTokens += tool.EstimatedTokens
	}
	slices.Sort(selected)
	sort.SliceStable(omitted, func(i, j int) bool {
		return omitted[i].Name < omitted[j].Name
	})
	r.rememberToolDisclosureBudget(turnID, selectedBucket, omittedBucket)
	r.recordToolDisclosure(ctx, req.SessionID, req.TurnID, selected, omitted, selectedBucket, omittedBucket)
	omittedNames := make([]string, 0, len(omitted))
	for _, item := range omitted {
		omittedNames = append(omittedNames, item.Name)
	}
	return agent.SchedulerToolDisclosureResult{Selected: selected, Omitted: omittedNames}, nil
}

func (r *runtimeService) rememberToolDisclosureBudget(turnID string, selected, omitted RuntimeBudgetBucket) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolDiscovery.DisclosureByTurn[turnID] = runtimeToolDisclosureBudget{
		Selected: selected,
		Omitted:  omitted,
	}
}

func (r *runtimeService) toolDisclosureBudget(turnID string) runtimeToolDisclosureBudget {
	if turnID == "" {
		return runtimeToolDisclosureBudget{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolDiscovery.DisclosureByTurn[turnID]
}

func (r *runtimeService) searchTools(ctx context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return RuntimeToolSearchResponse{}, errors.New("query is required")
	}
	maxResults := req.MaxResults
	maxResultsReason := ""
	if maxResults <= 0 {
		maxResults = defaultToolSearchMaxResults
		maxResultsReason = "max_results_defaulted"
	}
	if maxResults > maxToolSearchResults {
		maxResults = maxToolSearchResults
		maxResultsReason = "max_results_clamped"
	}
	if ctx.Err() != nil {
		resp := RuntimeToolSearchResponse{Query: query, Guardrail: "cancelled", GuardrailError: "tool search cancelled"}
		r.recordToolSearch(req, resp)
		r.recordDeadlockPrevented(req.SessionID, req.TurnID, req.ToolCallID, "tool_search_cancelled", resp.GuardrailError)
		return resp, ctx.Err()
	}
	if blocked, reason := r.preventRepeatedToolSearch(req.TurnID, query); blocked {
		resp := RuntimeToolSearchResponse{Query: query, Guardrail: reason, GuardrailError: "tool search guardrail blocked repeated search"}
		r.recordToolSearch(req, resp)
		r.recordDeadlockPrevented(req.SessionID, req.TurnID, req.ToolCallID, reason, resp.GuardrailError)
		return resp, nil
	}
	caps, err := r.Capabilities(ctx)
	if err != nil {
		return RuntimeToolSearchResponse{}, err
	}
	results, omitted := r.filterAndScoreToolSearchScoped(ctx, req, query, caps.Capabilities, maxResults)
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
	if maxResultsReason != "" {
		resp.MaxResults = maxResults
		resp.MaxResultsReason = maxResultsReason
	}
	r.recordToolSearch(req, resp)
	return resp, nil
}

func (r *runtimeService) filterAndScoreToolSearchScoped(ctx context.Context, req RuntimeToolSearchRequest, query string, capabilities []RuntimeCapability, maxResults int) ([]RuntimeToolSearchResult, []RuntimeToolSearchOmission) {
	task, scoped := r.agentTaskForChildSession(ctx, req.SessionID)
	if !scoped {
		return r.filterAndScoreToolSearch(query, capabilities, maxResults)
	}
	var scopedCaps []RuntimeCapability
	var omitted []RuntimeToolSearchOmission
	for _, cap := range capabilities {
		call := agent.SchedulerToolCall{
			SessionID:    req.SessionID,
			TurnID:       req.TurnID,
			Name:         cap.Name,
			Source:       string(capabilityToolSource(cap)),
			CapabilityID: cap.ID,
			InputSummary: cap.SearchText,
		}
		if reason := r.agentTaskScopeViolation(task, call); reason != "" {
			omitted = append(omitted, toolSearchOmission(finalizeRuntimeCapabilityMetadata(cap), "task_scope_denied"))
			r.recordAgentTaskScope(ctx, task, false, reason)
			continue
		}
		scopedCaps = append(scopedCaps, cap)
	}
	results, rest := r.filterAndScoreToolSearch(query, scopedCaps, maxResults)
	omitted = append(omitted, rest...)
	return results, omitted
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
	r.mu.Lock()
	policy := r.policy
	r.mu.Unlock()
	result := evaluateRuntimeCapabilityPolicy(policy, capability)
	if result.Decision == permission.PolicyAsk && (capability.Kind == "skill" || capability.Kind == "mcp_prompt" || capability.Kind == "mcp_resource") {
		result.Decision = permission.PolicyAllow
		result.Reason = firstNonEmpty(result.Reason, "Discovery exposes metadata only.")
	}
	return result
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

func (r *runtimeService) preventNestedToolSearch(ctx context.Context, call agent.SchedulerToolCall) (bool, string) {
	if call.Name != agent.ToolSearchToolName {
		return false, ""
	}
	if task, ok := r.agentTaskForChildSession(ctx, call.SessionID); ok && len(task.AllowedTools) > 0 && !matchesRuntimeScopeValue(task.AllowedTools, agent.ToolSearchToolName, "*") {
		return true, "task_scope_denied_tool_search"
	}
	if strings.Contains(strings.ToLower(call.InputSummary), "tool_search") {
		return true, "tool_search_recursion"
	}
	return false, ""
}

func (r *runtimeService) incrementRunningToolGuard(call agent.SchedulerToolCall) (bool, string) {
	if call.TurnID == "" || call.ID == "" {
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

func toolDisclosureOmission(tool agent.SchedulerToolMetadata, reason string) RuntimeToolSearchOmission {
	return RuntimeToolSearchOmission{
		ID:     firstNonEmpty(tool.CapabilityID, tool.Source+":"+tool.Name),
		Kind:   "tool_schema",
		Name:   tool.Name,
		Source: tool.Source,
		Reason: reason,
		State:  "omitted",
	}
}

func (r *runtimeService) recordToolDisclosure(ctx context.Context, sessionID, turnID string, selected []string, omitted []RuntimeToolSearchOmission, selectedBudget, omittedBudget RuntimeBudgetBucket) {
	omittedNames := make([]string, 0, len(omitted))
	omittedReasons := make(map[string]string, len(omitted))
	for _, item := range omitted {
		omittedNames = append(omittedNames, item.Name)
		omittedReasons[item.Name] = item.Reason
	}
	payload := map[string]any{
		"selected":  selected,
		"omitted":   omittedNames,
		"omissions": omitted,
		"budget": map[string]any{
			"selected": selectedBudget,
			"omitted":  omittedBudget,
		},
		"reason":  toolDiscoveryReasonBudget,
		"summary": fmt.Sprintf("%d selected, %d omitted tool schemas", len(selected), len(omitted)),
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolDiscoverySelected, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, Payload: payload})
	if len(omitted) > 0 {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolDiscoveryOmitted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, Payload: map[string]any{"omitted": omittedNames, "omissions": omitted, "reasons": omittedReasons, "reason": toolDiscoveryReasonDeferred, "budget": omittedBudget, "summary": fmt.Sprintf("%d omitted tool schemas", len(omitted))}})
	}
	r.writeAudit(auditEntry{RequestID: turnID, Event: "tool_discovery_selected", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, Extra: map[string]any{"selected": selected, "omitted": omitted, "omitted_names": omittedNames, "budget": payload["budget"], "reason": toolDiscoveryReasonBudget}})
}

func (r *runtimeService) recordToolSearch(req RuntimeToolSearchRequest, resp RuntimeToolSearchResponse) {
	selected := make([]string, 0, len(resp.Results))
	for _, result := range resp.Results {
		selected = append(selected, result.Name)
	}
	payload := map[string]any{"query": req.Query, "selected": selected, "omitted": resp.Omitted, "omitted_count": len(resp.Omitted), "budget_impact": resp.BudgetImpact, "summary": fmt.Sprintf("%d matches", len(resp.Results))}
	if resp.Guardrail != "" {
		payload["guardrail"] = resp.Guardrail
		payload["guardrail_error"] = resp.GuardrailError
	}
	if resp.MaxResultsReason != "" {
		payload["max_results"] = resp.MaxResults
		payload["max_results_reason"] = resp.MaxResultsReason
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventToolSearchPerformed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: req.SessionID, TurnID: req.TurnID, ToolCallID: req.ToolCallID, Payload: payload})
	r.writeAudit(auditEntry{RequestID: req.TurnID, Event: "tool_search_performed", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: req.SessionID, Extra: payload})
}

func (r *runtimeService) recordDeadlockPrevented(sessionID, turnID, toolCallID, reason, detail string) {
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSchedulerDeadlockPrevented, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, TurnID: turnID, ToolCallID: toolCallID, Payload: map[string]any{"reason": reason, "detail": detail, "summary": reason}})
	r.writeAudit(auditEntry{RequestID: turnID, Event: "scheduler_deadlock_prevented", Timestamp: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, ToolCallID: toolCallID, Extra: map[string]any{"reason": reason, "detail": detail}})
}
