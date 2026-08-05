package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent/prompt"
	"github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func (r *runtimeService) ContextSources(ctx context.Context) (RuntimeContextSourcesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeContextSourcesResponse{}, err
	}
	load := prompt.LoadContextSources(ctx, cfg, nil, nil)
	sources := runtimeContextSources(load.Sources)
	sources = append(sources, runtimeSkillContextSources(r.runtimeSkillsFromWorkspaceConfig(cfg, r.desktopSkillPaths()...).Skills)...)
	sources = append(sources, r.runtimeMCPContextSources()...)
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

func (r *runtimeService) runtimeMCPContextSources() []RuntimeContextSource {
	var sources []RuntimeContextSource
	for _, server := range mcp.GetStates() {
		state := capabilityStateUnloaded
		reason := "server_unloaded"
		enabled := true
		switch server.State {
		case mcp.StateUnloaded:
			state = capabilityStateUnloaded
			reason = "server_unloaded"
		case mcp.StateConnected:
			state = capabilityStateLoaded
			reason = "server_connected"
		case mcp.StateError:
			state = capabilityStateFailed
			reason = "server_failed"
		case mcp.StateDisabled:
			state = capabilityStateDisabled
			reason = "server_disabled"
			enabled = false
		}
		errText := ""
		if server.Error != nil {
			errText = server.Error.Error()
		}
		decision := r.evaluateMCPPolicy(server.Name, "", "server", "context", permission.RiskRead)
		if decision.Decision == permission.PolicyDeny {
			state = capabilityStateDisabled
			reason = "policy_denied"
			enabled = false
			errText = decision.Reason
		}
		sources = append(sources, RuntimeContextSource{
			ID:             "mcp:" + server.Name + ":instructions",
			Kind:           "mcp",
			Name:           server.Name,
			URI:            "mcp://" + server.Name + "/instructions",
			Scope:          "runtime",
			Enabled:        enabled,
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
		Provenance:     source.Provenance,
		ParentID:       source.ParentID,
		RuleGlobs:      source.RuleGlobs,
		SizeBytes:      source.SizeBytes,
		MTimeUnix:      source.MTimeUnix,
		ContentHash:    source.ContentHash,
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
	eventType := runtimeapi.EventContextSourceLoaded
	if source.State == capabilityStateLoading {
		eventType = runtimeapi.EventContextLoading
	} else if source.State == capabilityStateFailed || source.Error != "" {
		eventType = runtimeapi.EventContextFailed
	}
	payload := map[string]any{
		"source_id":      source.ID,
		"kind":           source.Kind,
		"path":           source.Path,
		"uri":            source.URI,
		"state":          source.State,
		"reason":         source.Reason,
		"provenance":     source.Provenance,
		"parent_id":      source.ParentID,
		"token_estimate": source.TokenEstimate,
		"content_hash":   source.ContentHash,
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
	if source.State == capabilityStateLoaded {
		injected := payload
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventContextSourceInjected,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: sessionID,
			TurnID:    turnID,
			Payload:   injected,
		})
	}
}

func (r *runtimeService) ReadFiles(ctx context.Context, sessionID string) (RuntimeReadFilesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeReadFilesResponse{}, err
	}
	sessionID = firstNonEmpty(sessionID, r.sessionID)
	conn := r.turns.db
	if conn == nil {
		var err error
		conn, err = r.workspaceDB(ctx)
		if err != nil {
			return RuntimeReadFilesResponse{}, err
		}
	}
	files, err := db.New(conn).ListSessionReadFiles(ctx, sessionID)
	if err != nil {
		return RuntimeReadFilesResponse{}, err
	}
	out := make([]RuntimeReadFileState, 0, len(files))
	for _, file := range files {
		state := runtimeReadFileStateFromDB(r, file)
		out = append(out, state)
	}
	return RuntimeReadFilesResponse{Files: out}, nil
}

func runtimeReadFileStateFromDB(r *runtimeService, file db.ReadFile) RuntimeReadFileState {
	path := r.resolveReadFilePath(file.Path)
	state := firstNonEmpty(file.State, "recorded")
	reason := file.Reason
	diagnostics := ""
	if info, err := os.Stat(path); err != nil {
		state = "missing"
		reason = "read_file_missing"
		diagnostics = err.Error()
	} else if file.MtimeUnix > 0 && info.ModTime().Unix() != file.MtimeUnix {
		state = "stale"
		reason = "mtime_changed"
		diagnostics = "file modification time changed since last read"
	} else if file.ContentHash != "" {
		if data, err := os.ReadFile(path); err == nil {
			sum := sha256.Sum256(data)
			if "sha256:"+hex.EncodeToString(sum[:]) != file.ContentHash {
				state = "stale"
				reason = "hash_changed"
				diagnostics = "file content hash changed since last read"
			}
		}
	}
	return RuntimeReadFileState{
		SessionID:     file.SessionID,
		TurnID:        file.TurnID,
		ToolCallID:    file.ToolCallID,
		Path:          filepath.ToSlash(path),
		ReadAt:        file.ReadAt,
		SizeBytes:     file.SizeBytes,
		ContentHash:   file.ContentHash,
		MTimeUnix:     file.MtimeUnix,
		Offset:        file.Offset,
		Limit:         file.ReadLimit,
		Partial:       file.Partial != 0,
		TokenEstimate: int(file.TokenEstimate),
		State:         state,
		Reason:        reason,
		Diagnostics:   diagnostics,
	}
}
