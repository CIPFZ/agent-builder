package runtime

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeService) ContextSources(ctx context.Context) (RuntimeContextSourcesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeContextSourcesResponse{}, err
	}
	load := prompt.LoadContextSources(ctx, cfg, nil, nil)
	sources := runtimeContextSources(load.Sources)
	sources = append(sources, runtimeSkillContextSources(r.runtimeSkillsFromWorkspaceConfig(cfg, r.desktopSkillPaths()...).Skills)...)
	sources = append(sources, runtimeMCPContextSources()...)
	return RuntimeContextSourcesResponse{Sources: sources}, nil
}

func runtimeContextSources(sources []prompt.ContextSource) []RuntimeContextSource {
	out := make([]RuntimeContextSource, 0, len(sources))
	for _, source := range sources {
		out = append(out, runtimeContextSource(source))
	}
	return out
}

func runtimeSkillContextSources(skills []RuntimeSkill) []RuntimeContextSource {
	sources := make([]RuntimeContextSource, 0, len(skills))
	for _, skill := range skills {
		state := capabilityStateLoaded
		reason := firstNonEmpty(skill.Activation.Reason, "runtime_selected")
		if !skill.Enabled || skill.State == capabilityStateDisabled {
			state = capabilityStateDisabled
			reason = firstNonEmpty(skill.Reason, "disabled_skill")
		} else if skill.State == capabilityStateFailed {
			state = capabilityStateFailed
			reason = firstNonEmpty(skill.Reason, "skill_diagnostic")
		}
		sources = append(sources, RuntimeContextSource{
			ID:             firstNonEmpty(skill.CapabilityID, "skill:"+skill.Name),
			Kind:           "skill",
			Name:           skill.Name,
			Path:           skill.SkillFilePath,
			URI:            skill.Path,
			Scope:          "runtime",
			Enabled:        skill.Enabled,
			State:          state,
			Reason:         reason,
			Diagnostics:    skill.Diagnostics,
			Error:          skill.Error,
			ContentSummary: "Skill metadata is available to the runtime; SKILL.md body is loaded only through runtime-controlled tool access.",
		})
	}
	return sources
}

func runtimeMCPContextSources() []RuntimeContextSource {
	var sources []RuntimeContextSource
	for _, server := range mcp.GetStates() {
		state := capabilityStateUnloaded
		reason := "server_unloaded"
		if server.State == mcp.StateConnected {
			state = capabilityStateLoaded
			reason = "server_connected"
		} else if server.State == mcp.StateError {
			state = capabilityStateFailed
			reason = "server_failed"
		}
		errText := ""
		if server.Error != nil {
			errText = server.Error.Error()
		}
		sources = append(sources, RuntimeContextSource{
			ID:             "mcp:" + server.Name + ":instructions",
			Kind:           "mcp",
			Name:           server.Name,
			URI:            "mcp://" + server.Name + "/instructions",
			Scope:          "runtime",
			Enabled:        true,
			State:          state,
			Reason:         reason,
			Error:          errText,
			ContentSummary: "MCP server instruction metadata is runtime-owned and summarized without raw instruction content.",
		})
	}
	return sources
}

func runtimeContextSource(source prompt.ContextSource) RuntimeContextSource {
	loadedAt := ""
	if !source.LoadedAt.IsZero() {
		loadedAt = source.LoadedAt.UTC().Format(time.RFC3339Nano)
	}
	return RuntimeContextSource{
		ID:             source.ID,
		Kind:           string(source.Kind),
		Name:           source.Name,
		Path:           source.Path,
		URI:            source.URI,
		Scope:          source.Scope,
		Enabled:        source.Enabled,
		State:          string(source.State),
		Reason:         source.Reason,
		Diagnostics:    source.Diagnostics,
		Error:          source.Error,
		ContentSummary: source.ContentSummary,
		TokenEstimate:  source.TokenEstimate,
		LoadedAt:       loadedAt,
	}
}

func runtimeTurnContextSummary(sources []RuntimeContextSource) RuntimeTurnContextSummary {
	summary := RuntimeTurnContextSummary{AvailableCount: len(sources)}
	for _, source := range sources {
		summary.Available = append(summary.Available, source)
		switch source.State {
		case capabilityStateLoaded:
			summary.Loaded = append(summary.Loaded, source)
			summary.Injected = append(summary.Injected, source)
			summary.TokenEstimate += source.TokenEstimate
		case capabilityStateFailed:
			summary.Failed = append(summary.Failed, source)
		case capabilityStateDisabled, capabilityStateUnavailable, capabilityStateUnloaded:
			summary.Skipped = append(summary.Skipped, source)
		default:
			summary.Skipped = append(summary.Skipped, source)
		}
	}
	return summary
}

func (r *runtimeService) recordTurnContextSources(sessionID, turnID string, sources []RuntimeContextSource) RuntimeTurnContextSummary {
	summary := runtimeTurnContextSummary(sources)
	for _, source := range sources {
		r.publishContextEvent(sessionID, turnID, source, 0)
	}
	return summary
}

func (r *runtimeService) publishContextEvent(sessionID, turnID string, source RuntimeContextSource, durationMS int64) {
	eventType := runtimeapi.EventContextLoaded
	if source.State == capabilityStateLoading {
		eventType = runtimeapi.EventContextLoading
	} else if source.State == capabilityStateFailed || source.Error != "" {
		eventType = runtimeapi.EventContextFailed
	}
	payload := map[string]any{
		"source_id": source.ID,
		"kind":      source.Kind,
		"path":      source.Path,
		"uri":       source.URI,
		"state":     source.State,
		"reason":    source.Reason,
	}
	if source.Error != "" {
		payload["error"] = source.Error
	}
	if durationMS > 0 {
		payload["duration_ms"] = durationMS
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload:   payload,
	})
}
