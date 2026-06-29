package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func (r *runtimeSchedulerRecorder) RecordPromptAssembly(ctx context.Context, snapshot agent.PromptAssemblySnapshot) error {
	if r == nil || r.service == nil {
		return nil
	}
	return r.service.recordPromptAssembly(ctx, snapshot)
}

func (r *runtimeSchedulerRecorder) BuildModelInput(ctx context.Context, snapshot agent.ModelInputSnapshot) (agent.ModelInputProjection, error) {
	if r == nil || r.service == nil {
		return agent.ModelInputProjection{Messages: snapshot.Messages}, nil
	}
	return r.service.buildModelInputProjection(ctx, snapshot)
}

func (r *runtimeService) PromptAssembliesByTurn(ctx context.Context, turnID string) (RuntimePromptAssembliesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimePromptAssembliesResponse{}, errors.New("turn id is required")
	}
	if r.promptAssemblies.db == nil {
		return RuntimePromptAssembliesResponse{}, nil
	}
	assemblies, err := r.promptAssemblies.ListByTurn(ctx, turnID)
	if err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	if assemblies, err = r.enrichPromptAssembliesWithContext(ctx, assemblies); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	return RuntimePromptAssembliesResponse{Assemblies: assemblies}, nil
}

func (r *runtimeService) PromptAssembliesBySession(ctx context.Context, sessionID string, limit int) (RuntimePromptAssembliesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	if sessionID == "" {
		return RuntimePromptAssembliesResponse{}, errors.New("session id is required")
	}
	if r.promptAssemblies.db == nil {
		return RuntimePromptAssembliesResponse{}, nil
	}
	assemblies, err := r.promptAssemblies.ListBySession(ctx, sessionID, limit)
	if err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	if assemblies, err = r.enrichPromptAssembliesWithContext(ctx, assemblies); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	return RuntimePromptAssembliesResponse{Assemblies: assemblies}, nil
}

func (r *runtimeService) ManualCompact(ctx context.Context, req RuntimeContextActionRequest) (RuntimeManualCompactResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.SessionID == "" {
		r.mu.Lock()
		req.SessionID = r.sessionID
		r.mu.Unlock()
	}
	if req.SessionID == "" {
		return RuntimeManualCompactResponse{}, errors.New("session id is required")
	}
	if req.TurnID == "" {
		return RuntimeManualCompactResponse{}, errors.New("turn id is required")
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	result, err := r.contextManager.ManualCompact(ctx, contextmgr.ManualCompactRequest{
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		Reason:       req.Reason,
	})
	if err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      "context.compact.manual",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		Payload: map[string]any{
			"boundary_id": result.Boundary.ID,
			"reason":      result.Boundary.Trigger,
			"summary":     "manual compact requested",
		},
	})
	return RuntimeManualCompactResponse{Boundary: runtimeContextBoundaryFromContext(result.Boundary)}, nil
}

func (r *runtimeService) ManualSnip(ctx context.Context, req RuntimeContextActionRequest) (RuntimeManualSnipResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.SessionID == "" {
		r.mu.Lock()
		req.SessionID = r.sessionID
		r.mu.Unlock()
	}
	if req.SessionID == "" {
		return RuntimeManualSnipResponse{}, errors.New("session id is required")
	}
	if req.TurnID == "" {
		return RuntimeManualSnipResponse{}, errors.New("turn id is required")
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	result, err := r.contextManager.ManualSnip(ctx, contextmgr.ManualSnipRequest{
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		Reason:       req.Reason,
	})
	if err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      "context.snip.manual",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		Payload: map[string]any{
			"snip_id": result.SnipBoundary.ID,
			"reason":  result.SnipBoundary.Reason,
			"summary": "manual snip requested",
		},
	})
	return RuntimeManualSnipResponse{SnipBoundary: runtimeSnipBoundaryFromContext(result.SnipBoundary)}, nil
}

func (r *runtimeService) recordPromptAssembly(ctx context.Context, snapshot agent.PromptAssemblySnapshot) error {
	if snapshot.SessionID == "" || snapshot.TurnID == "" {
		return nil
	}
	if r.promptAssemblies.db == nil {
		db, err := r.workspaceDB(ctx)
		if err != nil {
			return err
		}
		r.promptAssemblies = newRuntimePromptAssemblyStore(db)
	}
	contextSources := []RuntimeContextSource{}
	var contextSummary RuntimeTurnContextSummary
	if contextResp, err := r.ContextSources(ctx); err == nil {
		contextSources = contextResp.Sources
		contextSources = append(contextSources, r.memoryContextSourcesForTurn(ctx, snapshot.SessionID, snapshot.TurnID)...)
		contextSummary = runtimeTurnContextSummary(contextSources)
	} else {
		contextSources = append(contextSources, RuntimeContextSource{
			ID:             "context:sources",
			Kind:           "context_source",
			Name:           "Context source inventory",
			Enabled:        true,
			State:          capabilityStateFailed,
			Reason:         "context_sources_unavailable",
			Error:          err.Error(),
			ContentSummary: "Runtime could not load context source inventory before model call.",
		})
		contextSummary = runtimeTurnContextSummary(contextSources)
	}
	var compact []RuntimeCompactBoundary
	if r.compactBoundaries.db != nil {
		compact, _ = r.compactBoundaries.ListByTurn(ctx, snapshot.TurnID)
	}
	promptLength := snapshot.Messages.TokenEstimate * 4
	budget := r.computeRuntimeBudget(ctx, snapshot.SessionID, snapshot.TurnID, snapshot.Model, promptLength, &contextSummary)
	projectionID := snapshot.ProjectionID
	if projectionID == "" {
		if projection, err := r.recordNoopContextProjection(ctx, snapshot, budget); err == nil {
			projectionID = projection.ID
		} else {
			return err
		}
	}
	disclosure := r.toolDisclosureBudget(snapshot.TurnID)
	tools := RuntimePromptToolSummary{
		Selected:       append([]string(nil), snapshot.Tools.Selected...),
		Omitted:        append([]string(nil), snapshot.Tools.Omitted...),
		SelectedCount:  firstPositiveInt(snapshot.Tools.SelectedCount, len(snapshot.Tools.Selected)),
		OmittedCount:   firstPositiveInt(snapshot.Tools.OmittedCount, len(snapshot.Tools.Omitted)),
		SelectedBudget: disclosure.Selected,
		OmittedBudget:  disclosure.Omitted,
	}
	if calls, err := r.TurnToolCalls(ctx, snapshot.TurnID); err == nil {
		tools.ResultCount = len(calls.ToolCalls)
		for _, call := range calls.ToolCalls {
			if len(call.OutputRefs) > 0 || strings.TrimSpace(call.OutputSummary) != "" && strings.Contains(call.OutputSummary, "persisted") {
				tools.PersistedResults++
			}
			if call.Compacted || call.CompactRef != "" {
				tools.CompactedResults++
			}
		}
	}
	assembly := RuntimePromptAssembly{
		ID:           runtimePromptAssemblyID(snapshot.TurnID, snapshot.Step),
		SessionID:    snapshot.SessionID,
		TurnID:       snapshot.TurnID,
		ProjectionID: projectionID,
		Step:         firstPositiveInt(snapshot.Step, 1),
		Provider:     snapshot.Provider,
		Model:        snapshot.Model,
		Sections:     runtimePromptSectionSummaries(snapshot.Sections),
		System: RuntimePromptSystemSummary{
			Source:             snapshot.System.Source,
			Hash:               snapshot.System.Hash,
			Length:             snapshot.System.Length,
			TokenEstimate:      snapshot.System.TokenEstimate,
			PromptPrefix:       snapshot.System.PromptPrefix,
			PromptPrefixHash:   snapshot.System.PromptPrefixHash,
			PromptPrefixTokens: snapshot.System.PromptPrefixTokens,
			SourceRefs:         append([]string(nil), snapshot.System.SourceRefs...),
			Redacted:           true,
		},
		Messages: RuntimePromptMessageSummary{
			Count:                snapshot.Messages.Count,
			ByRole:               cloneStringIntMap(snapshot.Messages.ByRole),
			ToolResultCount:      snapshot.Messages.ToolResultCount,
			DeliveredToolResults: snapshot.Messages.DeliveredToolResults,
			SyntheticToolResults: snapshot.Messages.SyntheticToolResults,
			AttachmentCount:      snapshot.Messages.AttachmentCount,
			ImageCount:           snapshot.Messages.ImageCount,
			TokenEstimate:        snapshot.Messages.TokenEstimate,
			RawPromptStored:      false,
		},
		Tools: tools,
		Skills: RuntimePromptSkillSummary{
			AvailableCount:   snapshot.Skills.AvailableCount,
			LoadedCount:      snapshot.Skills.LoadedCount,
			Names:            append([]string(nil), snapshot.Skills.Names...),
			LoadedNames:      append([]string(nil), snapshot.Skills.LoadedNames...),
			XMLPresent:       snapshot.Skills.XMLPresent,
			XMLHash:          snapshot.Skills.XMLHash,
			TokenEstimate:    snapshot.Skills.TokenEstimate,
			RawContentStored: false,
		},
		MCP: RuntimePromptMCPSummary{
			ServerCount:      snapshot.MCP.ServerCount,
			InstructionCount: snapshot.MCP.InstructionCount,
			Servers:          append([]string(nil), snapshot.MCP.Servers...),
			ServerListHash:   snapshot.MCP.ServerListHash,
			InstructionHash:  snapshot.MCP.InstructionHash,
			TokenEstimate:    snapshot.MCP.TokenEstimate,
			RawContentStored: false,
		},
		ContextSources: contextSources,
		Compact:        compact,
		Budget:         budget,
		CreatedAt:      firstNonZeroInt64(snapshot.CreatedAt, time.Now().UTC().UnixMilli()),
	}
	stored, err := r.promptAssemblies.Upsert(ctx, assembly)
	if err != nil {
		return err
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      "prompt.assembly.recorded",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.TurnID,
		Payload: map[string]any{
			"assembly_id":            stored.ID,
			"projection_id":          stored.ProjectionID,
			"step":                   stored.Step,
			"provider":               stored.Provider,
			"model":                  stored.Model,
			"total_estimated_tokens": stored.Budget.TotalEstimatedTokens,
			"section_count":          len(stored.Sections),
			"context_source_count":   len(stored.ContextSources),
			"selected_tool_count":    stored.Tools.SelectedCount,
			"omitted_tool_count":     stored.Tools.OmittedCount,
			"compact_boundary_count": len(stored.Compact),
			"summary":                fmt.Sprintf("step %d prompt assembly recorded", stored.Step),
		},
	})
	return nil
}

func runtimePromptSectionSummaries(sections []agent.PromptSectionSummary) []RuntimePromptSectionSummary {
	if len(sections) == 0 {
		return nil
	}
	out := make([]RuntimePromptSectionSummary, 0, len(sections))
	for _, section := range sections {
		out = append(out, RuntimePromptSectionSummary{
			ID:            section.ID,
			Name:          section.Name,
			Kind:          section.Kind,
			Role:          section.Role,
			Order:         section.Order,
			CachePolicy:   section.CachePolicy,
			Source:        section.Source,
			SourceRefs:    append([]string(nil), section.SourceRefs...),
			Scope:         section.Scope,
			Hash:          section.Hash,
			Length:        section.Length,
			TokenEstimate: section.TokenEstimate,
			Redacted:      true,
			RawStored:     false,
			Diagnostics:   section.Diagnostics,
		})
	}
	return out
}

func (r *runtimeService) buildModelInputProjection(ctx context.Context, snapshot agent.ModelInputSnapshot) (agent.ModelInputProjection, error) {
	messages, err := r.injectProjectMemory(ctx, agentModelInputSnapshotLite{
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.TurnID,
		Step:      firstPositiveInt(snapshot.Step, 1),
	}, snapshot.Messages)
	if err == nil {
		snapshot.Messages = messages
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return agent.ModelInputProjection{}, err
	}
	result, err := r.contextManager.BuildModelInput(ctx, contextmgr.BuildInputRequest{
		SessionID:     snapshot.SessionID,
		TurnID:        snapshot.TurnID,
		Step:          firstPositiveInt(snapshot.Step, 1),
		Provider:      snapshot.Provider,
		Model:         snapshot.Model,
		Source:        firstNonEmpty(snapshot.Source, contextmgr.ProjectionSourceMainTurn),
		ModelMessages: snapshot.Messages,
	})
	if err != nil {
		return agent.ModelInputProjection{}, err
	}
	projected := result.ModelMessages
	if projected == nil {
		projected = snapshot.Messages
	}
	return agent.ModelInputProjection{
		ProjectionID:          result.Projection.ID,
		Messages:              projected,
		CanonicalMessageCount: result.Projection.CanonicalMessageCount,
		ProjectedMessageCount: result.Projection.ProjectedMessageCount,
	}, nil
}

func (r *runtimeService) enrichPromptAssembliesWithContext(ctx context.Context, assemblies []RuntimePromptAssembly) ([]RuntimePromptAssembly, error) {
	if len(assemblies) == 0 {
		return assemblies, nil
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return nil, err
	}
	for i := range assemblies {
		projectionID := strings.TrimSpace(assemblies[i].ProjectionID)
		if projectionID == "" {
			continue
		}
		boundaries, err := r.contextStore.ListBoundariesByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, boundary := range boundaries {
			assemblies[i].ContextBoundaries = append(assemblies[i].ContextBoundaries, runtimeContextBoundaryFromContext(boundary))
		}
		snipBoundaries, err := r.contextStore.ListSnipBoundariesByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, snip := range snipBoundaries {
			assemblies[i].SnipBoundaries = append(assemblies[i].SnipBoundaries, runtimeSnipBoundaryFromContext(snip))
		}
		replacements, err := r.contextStore.ListContentReplacementsByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, replacement := range replacements {
			assemblies[i].Replacements = append(assemblies[i].Replacements, runtimeContentReplacementFromContext(replacement))
		}
		attempts, err := r.contextStore.ListReactiveAttemptsByTurn(ctx, assemblies[i].TurnID)
		if err != nil {
			return nil, err
		}
		for _, attempt := range attempts {
			if attempt.ProjectionID == "" || attempt.ProjectionID == projectionID {
				assemblies[i].ReactiveAttempts = append(assemblies[i].ReactiveAttempts, runtimeReactiveAttemptFromContext(attempt))
			}
		}
	}
	return assemblies, nil
}

func (r *runtimeService) ensureContextManager(ctx context.Context) error {
	if r.contextManager != nil {
		return nil
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return err
	}
	r.contextStore = contextmgr.NewSQLStore(db)
	r.contextManager = contextmgr.NewManager(contextmgr.ManagerOptions{Store: r.contextStore})
	return nil
}

func (r *runtimeService) recordNoopContextProjection(ctx context.Context, snapshot agent.PromptAssemblySnapshot, budget RuntimeBudgetReport) (contextmgr.Projection, error) {
	if err := r.ensureContextManager(ctx); err != nil {
		return contextmgr.Projection{}, err
	}
	result, err := r.contextManager.BuildModelInput(ctx, contextmgr.BuildInputRequest{
		SessionID:         snapshot.SessionID,
		TurnID:            snapshot.TurnID,
		Step:              firstPositiveInt(snapshot.Step, 1),
		Provider:          snapshot.Provider,
		Model:             snapshot.Model,
		Source:            contextmgr.ProjectionSourceMainTurn,
		CanonicalMessages: snapshot.Messages.Count,
		ProjectedMessages: snapshot.Messages.Count,
		BudgetBefore:      runtimeBudgetToContextBudget(budget),
		BudgetAfter:       runtimeBudgetToContextBudget(budget),
	})
	if err != nil {
		return contextmgr.Projection{}, err
	}
	return result.Projection, nil
}

func runtimeBudgetToContextBudget(report RuntimeBudgetReport) *contextmgr.BudgetReport {
	return &contextmgr.BudgetReport{
		TotalEstimatedTokens: report.TotalEstimatedTokens,
		ContextWindow:        report.ContextWindow,
		Model:                report.Model,
	}
}

func runtimeContextBoundaryFromContext(boundary contextmgr.Boundary) RuntimeContextBoundary {
	return RuntimeContextBoundary{
		ID:               boundary.ID,
		SessionID:        boundary.SessionID,
		TurnID:           boundary.TurnID,
		ProjectionID:     boundary.ProjectionID,
		Kind:             boundary.Kind,
		Trigger:          boundary.Trigger,
		Status:           boundary.Status,
		SummaryMessageID: boundary.SummaryMessageID,
		SummaryRef:       boundary.SummaryRef,
		MessageRefs:      append([]string(nil), boundary.MessageRefs...),
		ToolCallRefs:     append([]string(nil), boundary.ToolCallRefs...),
		ReinjectedRefs:   append([]string(nil), boundary.ReinjectedRefs...),
		BudgetBefore:     runtimeBudgetFromContextBudget(boundary.BudgetBefore),
		BudgetAfter:      runtimeBudgetFromContextBudget(boundary.BudgetAfter),
		CreatedAt:        boundary.CreatedAt,
		CompletedAt:      boundary.CompletedAt,
		Error:            boundary.Error,
	}
}

func runtimeSnipBoundaryFromContext(snip contextmgr.SnipBoundary) RuntimeSnipBoundary {
	return RuntimeSnipBoundary{
		ID:                 snip.ID,
		SessionID:          snip.SessionID,
		TurnID:             snip.TurnID,
		ProjectionID:       snip.ProjectionID,
		RemovedMessageRefs: append([]string(nil), snip.RemovedMessageRefs...),
		PreservedHeadRef:   snip.PreservedHeadRef,
		PreservedTailRef:   snip.PreservedTailRef,
		SummaryRef:         snip.SummaryRef,
		Reason:             snip.Reason,
		CreatedAt:          snip.CreatedAt,
	}
}

func runtimeContentReplacementFromContext(replacement contextmgr.ContentReplacement) RuntimeContentReplacement {
	return RuntimeContentReplacement{
		ID:                         replacement.ID,
		SessionID:                  replacement.SessionID,
		TurnID:                     replacement.TurnID,
		ProjectionID:               replacement.ProjectionID,
		ToolCallID:                 replacement.ToolCallID,
		ToolName:                   replacement.ToolName,
		Kind:                       replacement.Kind,
		OriginalRef:                replacement.OriginalRef,
		ReplacementEstimatedTokens: replacement.ReplacementEstimatedTokens,
		OriginalEstimatedTokens:    replacement.OriginalEstimatedTokens,
		OriginalSizeBytes:          replacement.OriginalSizeBytes,
		Reason:                     replacement.Reason,
		CreatedAt:                  replacement.CreatedAt,
	}
}

func runtimeReactiveAttemptFromContext(attempt contextmgr.ReactiveAttempt) RuntimeReactiveCompactAttempt {
	return RuntimeReactiveCompactAttempt{
		ID:           attempt.ID,
		SessionID:    attempt.SessionID,
		TurnID:       attempt.TurnID,
		ProjectionID: attempt.ProjectionID,
		Attempt:      attempt.Attempt,
		Action:       attempt.Action,
		Status:       attempt.Status,
		Error:        attempt.Error,
		CreatedAt:    attempt.CreatedAt,
		CompletedAt:  attempt.CompletedAt,
	}
}

func runtimeBudgetFromContextBudget(report *contextmgr.BudgetReport) *RuntimeBudgetReport {
	if report == nil {
		return nil
	}
	return &RuntimeBudgetReport{
		ContextWindow:        report.ContextWindow,
		Model:                report.Model,
		TotalEstimatedTokens: report.TotalEstimatedTokens,
	}
}

func runtimePromptAssemblyID(turnID string, step int) string {
	if step <= 0 {
		step = 1
	}
	return fmt.Sprintf("prompt_%s_step_%03d", strings.ReplaceAll(turnID, ":", "_"), step)
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
