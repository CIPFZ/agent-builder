package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeSchedulerRecorder) RecordPromptAssembly(ctx context.Context, snapshot agent.PromptAssemblySnapshot) error {
	if r == nil || r.service == nil {
		return nil
	}
	return r.service.recordPromptAssembly(ctx, snapshot)
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
	return RuntimePromptAssembliesResponse{Assemblies: assemblies}, nil
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
		ID:        runtimePromptAssemblyID(snapshot.TurnID, snapshot.Step),
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.TurnID,
		Step:      firstPositiveInt(snapshot.Step, 1),
		Provider:  snapshot.Provider,
		Model:     snapshot.Model,
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
			"step":                   stored.Step,
			"provider":               stored.Provider,
			"model":                  stored.Model,
			"total_estimated_tokens": stored.Budget.TotalEstimatedTokens,
			"context_source_count":   len(stored.ContextSources),
			"selected_tool_count":    stored.Tools.SelectedCount,
			"omitted_tool_count":     stored.Tools.OmittedCount,
			"compact_boundary_count": len(stored.Compact),
			"summary":                fmt.Sprintf("step %d prompt assembly recorded", stored.Step),
		},
	})
	return nil
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
