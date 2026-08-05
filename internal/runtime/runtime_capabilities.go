package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

const (
	runtimeCapabilityLoadRecordMaxEntries = 256
	runtimeCapabilityLoadRecordTextBytes  = 2 * 1024
)

const (
	capabilityStateUnavailable = "unavailable"
	capabilityStateDisabled    = "disabled"
	capabilityStateUnloaded    = "unloaded"
	capabilityStateLoading     = "loading"
	capabilityStateLoaded      = "loaded"
	capabilityStateFailed      = "failed"
)

func (r *runtimeService) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeCapabilitiesResponse{}, err
	}
	skills := r.runtimeSkillsFromWorkspaceConfig(cfg, r.desktopSkillPaths()...)
	tools := runtimeMCPToolsFromConfig(cfg, "")
	resources := runtimeMCPResources("")
	prompts := runtimeMCPPrompts("")

	r.mu.Lock()
	loads := cloneCapabilityLoadRecords(r.capabilityLoads)
	r.mu.Unlock()
	r.mu.Lock()
	policy := r.policy
	r.mu.Unlock()
	return runtimeCapabilities(cfg, skills, tools, resources, prompts, loads, policy), nil
}

func (r *runtimeService) RefreshCapability(ctx context.Context, capabilityID string) (RuntimeCapabilityResponse, error) {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return RuntimeCapabilityResponse{}, errors.New("capability id is required")
	}
	_, wsID, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeCapabilityResponse{}, err
	}
	before, ok := r.findCapability(ctx, capabilityID)
	if !ok {
		failed := RuntimeCapability{
			ID:          capabilityID,
			Kind:        "unknown",
			Name:        capabilityID,
			Enabled:     false,
			Risk:        "unknown",
			State:       capabilityStateUnavailable,
			Diagnostics: "Capability is not present in the current runtime inventory.",
			Error:       "capability not found",
			Reason:      "not_found",
		}
		r.recordCapabilityLoad(failed, capabilityStateFailed, "not_found", failed.Error, 0)
		return RuntimeCapabilityResponse{Capability: failed}, nil
	}
	if !before.Enabled || before.State == capabilityStateDisabled {
		disabled := before
		disabled.State = capabilityStateDisabled
		disabled.Reason = firstNonEmpty(disabled.Reason, "disabled")
		disabled.Diagnostics = firstNonEmpty(disabled.Diagnostics, "Capability is disabled by runtime configuration.")
		r.setCapabilityLoadRecord(disabled.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateDisabled,
			Diagnostics: disabled.Diagnostics,
			Reason:      disabled.Reason,
			UpdatedAt:   time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(disabled, capabilityStateDisabled, disabled.Reason, "", 0)
		return RuntimeCapabilityResponse{Capability: disabled}, nil
	}
	if decision := r.evaluateCapabilityLoadPolicy(before); decision.Decision == permission.PolicyDeny {
		denied := before
		denied.State = capabilityStateFailed
		denied.Reason = string(decision.Decision)
		denied.Diagnostics = decision.Reason
		denied.Error = decision.Reason
		r.setCapabilityLoadRecord(denied.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateFailed,
			Diagnostics: denied.Diagnostics,
			Error:       denied.Error,
			Reason:      denied.Reason,
			UpdatedAt:   time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(denied, capabilityStateFailed, denied.Reason, denied.Error, 0)
		return RuntimeCapabilityResponse{Capability: denied}, nil
	}

	start := time.Now()
	loading := before
	loading.State = capabilityStateLoading
	loading.Reason = "refresh_requested"
	r.setCapabilityLoadRecord(loading.ID, runtimeCapabilityLoadRecord{
		State:     capabilityStateLoading,
		Reason:    loading.Reason,
		UpdatedAt: start.UnixMilli(),
	})
	r.publishCapabilityEvent(runtimeapi.EventCapabilityLoading, loading, loading.Reason, "", 0)

	var loadErr error
	switch before.Kind {
	case "builtin_tool":
		// Builtin tools are registered with the runtime at startup; refresh only
		// records the runtime-owned loaded state for client recovery.
	case "skill":
		_, loadErr = r.refreshSkills(ctx, true)
	case "mcp_tool", "mcp_resource", "mcp_prompt":
		server := before.Source
		if server == "" {
			loadErr = errors.New("mcp capability is missing server source")
			break
		}
		cfg, _, cfgErr := r.workspaceConfig(ctx)
		if cfgErr != nil {
			loadErr = cfgErr
			break
		}
		loadErr = r.refreshMCPServerLifecycle(ctx, cfg, wsID, server, "capability_refresh")
	default:
		loadErr = fmt.Errorf("capability kind %s is not refreshable", before.Kind)
	}
	duration := time.Since(start).Milliseconds()
	if loadErr != nil {
		failed := before
		failed.State = capabilityStateFailed
		failed.Reason = "refresh_failed"
		failed.Error = loadErr.Error()
		failed.Diagnostics = loadErr.Error()
		r.setCapabilityLoadRecord(failed.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateFailed,
			Diagnostics: failed.Diagnostics,
			Error:       failed.Error,
			Reason:      failed.Reason,
			UpdatedAt:   time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(failed, capabilityStateFailed, failed.Reason, failed.Error, duration)
		return RuntimeCapabilityResponse{Capability: failed}, nil
	}

	after, ok := r.findCapability(ctx, capabilityID)
	if !ok {
		after = before
	}
	after.State = capabilityStateLoaded
	after.Reason = "refresh_completed"
	after.Diagnostics = ""
	after.Error = ""
	r.setCapabilityLoadRecord(after.ID, runtimeCapabilityLoadRecord{
		State:     capabilityStateLoaded,
		Reason:    after.Reason,
		UpdatedAt: time.Now().UnixMilli(),
	})
	r.recordCapabilityLoad(after, capabilityStateLoaded, after.Reason, "", duration)
	return RuntimeCapabilityResponse{Capability: after}, nil
}

func (r *runtimeService) findCapability(ctx context.Context, capabilityID string) (RuntimeCapability, bool) {
	resp, err := r.Capabilities(ctx)
	if err != nil {
		return RuntimeCapability{}, false
	}
	for _, capability := range resp.Capabilities {
		if capability.ID == capabilityID {
			return capability, true
		}
	}
	return RuntimeCapability{}, false
}

func (r *runtimeService) evaluateCapabilityLoadPolicy(capability RuntimeCapability) permission.PolicyResult {
	r.mu.Lock()
	policy := r.policy
	r.mu.Unlock()
	return evaluateRuntimeCapabilityPolicy(policy, capability)
}

func evaluateRuntimeCapabilityPolicy(policy RuntimePolicy, capability RuntimeCapability) permission.PolicyResult {
	capability = finalizeRuntimeCapabilityMetadata(capability)
	return runtimePermissionPolicy(policy).Evaluate(runtimeCapabilityPolicyCall(capability))
}

func runtimeCapabilityPolicyCall(capability RuntimeCapability) scheduler.ToolCall {
	capability = finalizeRuntimeCapabilityMetadata(capability)
	return scheduler.ToolCall{
		ID:           capability.ID,
		Name:         capability.Name,
		Source:       capabilityToolSource(capability),
		CapabilityID: capability.ID,
		Status:       scheduler.ToolCallPending,
		InputSummary: capabilityPolicySummary(capability),
	}
}

func capabilityToolSource(capability RuntimeCapability) scheduler.ToolSource {
	switch capability.Kind {
	case "mcp_tool", "mcp_resource", "mcp_prompt":
		return scheduler.ToolSourceMCP
	case "builtin_tool":
		return classifyToolSource(capability.Name)
	default:
		return scheduler.ToolSourceUnknown
	}
}

func capabilityPolicySummary(capability RuntimeCapability) string {
	switch capability.Kind {
	case "skill":
		return "read skill capability " + capability.Name
	case "mcp_resource":
		return "read mcp resource " + capability.Name
	case "mcp_prompt":
		return "read mcp prompt " + capability.Name
	}
	switch capability.Risk {
	case "write":
		return "write capability refresh"
	case "execute":
		return "execute capability refresh"
	case "network", "external":
		return "network capability refresh"
	case "secret":
		return "secret capability refresh"
	case "destructive":
		return "destructive capability refresh"
	default:
		return "read capability refresh"
	}
}

func (r *runtimeService) setCapabilityLoadRecord(id string, record runtimeCapabilityLoadRecord) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	record.Diagnostics = preview(record.Diagnostics, runtimeCapabilityLoadRecordTextBytes)
	record.Error = preview(record.Error, runtimeCapabilityLoadRecordTextBytes)
	record.Reason = preview(record.Reason, runtimeCapabilityLoadRecordTextBytes)
	if record.UpdatedAt <= 0 {
		record.UpdatedAt = time.Now().UnixMilli()
	}
	r.mu.Lock()
	if r.capabilityLoads == nil {
		r.capabilityLoads = make(map[string]runtimeCapabilityLoadRecord)
	}
	r.capabilityLoads[id] = record
	for len(r.capabilityLoads) > runtimeCapabilityLoadRecordMaxEntries {
		oldestID := ""
		var oldestUpdatedAt int64
		for candidateID, candidate := range r.capabilityLoads {
			if candidateID == id {
				continue
			}
			if oldestID == "" || candidate.UpdatedAt < oldestUpdatedAt || (candidate.UpdatedAt == oldestUpdatedAt && candidateID < oldestID) {
				oldestID = candidateID
				oldestUpdatedAt = candidate.UpdatedAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(r.capabilityLoads, oldestID)
	}
	r.mu.Unlock()
}

func (r *runtimeService) recordCapabilityLoad(capability RuntimeCapability, state, reason, errText string, durationMS int64) {
	eventType := runtimeapi.EventCapabilityLoaded
	if state == capabilityStateLoading {
		eventType = runtimeapi.EventCapabilityLoading
	} else if state == capabilityStateFailed || errText != "" {
		eventType = runtimeapi.EventCapabilityFailed
	}
	r.publishCapabilityEvent(eventType, capability, reason, errText, durationMS)
	r.writeAudit(auditEntry{
		Event:            "capability_" + state,
		Timestamp:        time.Now().Format(time.RFC3339Nano),
		CapabilityID:     capability.ID,
		CapabilityKind:   capability.Kind,
		CapabilitySource: capability.Source,
		CapabilityState:  state,
		CapabilityReason: reason,
		CapabilityError:  errText,
		DurationMS:       durationMS,
	})
}

func (r *runtimeService) publishCapabilityEvent(eventType string, capability RuntimeCapability, reason, errText string, durationMS int64) {
	payload := map[string]any{
		"capability_id": capability.ID,
		"kind":          capability.Kind,
		"source":        capability.Source,
		"state":         capability.State,
		"reason":        reason,
	}
	if errText != "" {
		payload["error"] = errText
	}
	if durationMS > 0 {
		payload["duration_ms"] = durationMS
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	})
}

func runtimeCapabilities(
	store *config.ConfigStore,
	skills RuntimeSkillsResponse,
	mcpTools RuntimeMCPToolsResponse,
	mcpResources RuntimeMCPResourcesResponse,
	mcpPrompts RuntimeMCPPromptsResponse,
	args ...any,
) RuntimeCapabilitiesResponse {
	var capabilities []RuntimeCapability
	loadRecords := map[string]runtimeCapabilityLoadRecord{}
	policy := defaultRuntimePolicy()
	for _, arg := range args {
		switch value := arg.(type) {
		case map[string]runtimeCapabilityLoadRecord:
			loadRecords = value
		case RuntimePolicy:
			policy = value
		}
	}
	disabledTools := map[string]bool{}
	if store.Config().Options != nil {
		for _, name := range store.Config().Options.DisabledTools {
			disabledTools[name] = true
		}
	}
	for _, tool := range builtinToolCapabilities() {
		tool.Enabled = !disabledTools[tool.Name]
		tool.State = capabilityStateLoaded
		if !tool.Enabled {
			tool.State = capabilityStateDisabled
			tool.Reason = "disabled_tool"
			tool.Diagnostics = "Builtin tool is disabled by runtime configuration."
		}
		capabilities = append(capabilities, applyCapabilityLoadRecord(tool, loadRecords))
	}
	for _, skill := range skills.Skills {
		capability := RuntimeCapability{
			ID:          "skill:" + skill.Name,
			Kind:        "skill",
			Name:        skill.Name,
			Source:      skill.Path,
			Enabled:     skill.Enabled,
			Risk:        "context",
			Description: skill.Description,
			State:       capabilityStateUnloaded,
		}
		capability.SchemaSummary = schemaSummaryForSkill(skill)
		if !skill.Enabled {
			capability.State = capabilityStateDisabled
			capability.Reason = firstNonEmpty(skill.Reason, "disabled_skill")
			capability.Diagnostics = firstNonEmpty(skill.Diagnostics, "Skill is disabled by runtime configuration.")
		}
		if len(skill.AllowedTools) > 0 && capability.Diagnostics == "" {
			capability.Diagnostics = "Skill declares allowed_tools metadata; runtime preserves it as policy hints only."
		}
		if skill.State == capabilityStateFailed {
			capability.Enabled = false
			capability.State = capabilityStateFailed
			capability.Error = skill.Error
			capability.Diagnostics = firstNonEmpty(skill.Diagnostics, skill.Error)
			capability.Reason = firstNonEmpty(skill.Reason, "skill_diagnostic")
		}
		decision := evaluateRuntimeCapabilityPolicy(policy, capability)
		if capability.Enabled && decision.Decision == permission.PolicyDeny {
			capability.Enabled = false
			capability.State = capabilityStateDisabled
			capability.Reason = "policy_denied"
			capability.Diagnostics = decision.Reason
		}
		capabilities = append(capabilities, applyCapabilityLoadRecord(capability, loadRecords))
	}
	for _, tool := range mcpTools.Tools {
		capability := RuntimeCapability{
			ID:          "mcp:" + tool.Server + ":" + tool.Name,
			Kind:        "mcp_tool",
			Name:        tool.Name,
			Source:      tool.Server,
			Enabled:     tool.Enabled,
			Risk:        "network",
			Description: tool.Description,
			State:       capabilityStateUnloaded,
		}
		capability.SchemaSummary = schemaSummaryForValue(tool.InputSchema)
		capability.SchemaDigest = schemaDigestForValue(tool.InputSchema)
		if !tool.Enabled {
			capability.State = capabilityStateDisabled
			capability.Reason = "disabled_mcp_tool"
			capability.Diagnostics = "MCP tool is disabled by runtime configuration."
		}
		decision := evaluateRuntimeCapabilityPolicy(policy, capability)
		if capability.Enabled && decision.Decision == permission.PolicyDeny {
			capability.Enabled = false
			capability.State = capabilityStateDisabled
			capability.Reason = "policy_denied"
			capability.Diagnostics = decision.Reason
		}
		capabilities = append(capabilities, applyCapabilityLoadRecord(capability, loadRecords))
	}
	for _, resource := range mcpResources.Resources {
		capability := RuntimeCapability{
			ID:          "mcp_resource:" + resource.Server + ":" + resource.URI,
			Kind:        "mcp_resource",
			Name:        firstNonEmpty(resource.Name, resource.URI),
			Source:      resource.Server,
			Enabled:     true,
			Risk:        "read",
			Description: resource.Description,
			State:       capabilityStateUnloaded,
		}
		capability.SchemaSummary = "uri"
		decision := evaluateRuntimeCapabilityPolicy(policy, capability)
		if decision.Decision == permission.PolicyDeny {
			capability.Enabled = false
			capability.State = capabilityStateDisabled
			capability.Reason = "policy_denied"
			capability.Diagnostics = decision.Reason
		}
		capabilities = append(capabilities, applyCapabilityLoadRecord(capability, loadRecords))
	}
	for _, prompt := range mcpPrompts.Prompts {
		capability := RuntimeCapability{
			ID:          "mcp_prompt:" + prompt.Server + ":" + prompt.Name,
			Kind:        "mcp_prompt",
			Name:        prompt.Name,
			Source:      prompt.Server,
			Enabled:     true,
			Risk:        "context",
			Description: prompt.Description,
			State:       capabilityStateUnloaded,
		}
		capability.SchemaSummary = "prompt"
		decision := evaluateRuntimeCapabilityPolicy(policy, capability)
		if decision.Decision == permission.PolicyDeny {
			capability.Enabled = false
			capability.State = capabilityStateDisabled
			capability.Reason = "policy_denied"
			capability.Diagnostics = decision.Reason
		}
		capabilities = append(capabilities, applyCapabilityLoadRecord(capability, loadRecords))
	}
	slices.SortStableFunc(capabilities, func(a, b RuntimeCapability) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return RuntimeCapabilitiesResponse{Capabilities: capabilities}
}

func applyCapabilityLoadRecord(capability RuntimeCapability, records map[string]runtimeCapabilityLoadRecord) RuntimeCapability {
	capability = finalizeRuntimeCapabilityMetadata(capability)
	if capability.State == "" {
		capability.State = capabilityStateUnloaded
	}
	if !capability.Enabled {
		capability.State = capabilityStateDisabled
	}
	record, ok := records[capability.ID]
	if !ok || capability.State == capabilityStateDisabled {
		return capability
	}
	if record.State != "" {
		capability.State = normalizeCapabilityState(record.State, capability.Enabled)
	}
	capability.Diagnostics = firstNonEmpty(record.Diagnostics, capability.Diagnostics)
	capability.Error = firstNonEmpty(record.Error, capability.Error)
	capability.Reason = firstNonEmpty(record.Reason, capability.Reason)
	return capability
}

func finalizeRuntimeCapabilityMetadata(capability RuntimeCapability) RuntimeCapability {
	if capability.CapabilityID == "" {
		capability.CapabilityID = capability.ID
	}
	if capability.SchemaSummary == "" {
		capability.SchemaSummary = schemaSummaryForCapability(capability)
	}
	if capability.SchemaDigest == "" {
		capability.SchemaDigest = schemaDigestForValue(map[string]any{
			"id":      capability.ID,
			"kind":    capability.Kind,
			"name":    capability.Name,
			"source":  capability.Source,
			"summary": capability.SchemaSummary,
		})
	}
	if capability.SearchText == "" {
		capability.SearchText = strings.TrimSpace(strings.Join([]string{
			capability.ID,
			capability.Kind,
			capability.Name,
			capability.Source,
			capability.Description,
			capability.Risk,
			capability.SchemaSummary,
		}, " "))
	}
	return capability
}

func schemaSummaryForCapability(capability RuntimeCapability) string {
	switch capability.Kind {
	case "builtin_tool":
		return builtinToolSchemaSummary(capability.Name)
	case "skill":
		return "skill instructions"
	case "mcp_tool":
		return "mcp input schema"
	case "mcp_resource":
		return "uri"
	case "mcp_prompt":
		return "prompt"
	default:
		return "metadata"
	}
}

func builtinToolSchemaSummary(name string) string {
	switch strings.TrimSpace(name) {
	case "bash":
		return "command, description, timeout, background"
	case "download":
		return "url, file_path"
	case "edit":
		return "file_path, old_string, new_string, replace_all"
	case "multiedit":
		return "file_path, edits"
	case "fetch", "web_fetch":
		return "url, prompt"
	case "glob":
		return "pattern, path"
	case "grep":
		return "pattern, path, include"
	case "ls":
		return "path, ignore"
	case "view":
		return "file_path, offset, limit"
	case "write":
		return "file_path, content"
	case "todos":
		return "todos"
	case "job_output":
		return "job_id"
	case "job_kill":
		return "job_id"
	case "list_mcp_resources":
		return "server"
	case "read_mcp_resource":
		return "server, uri"
	default:
		return "tool input schema"
	}
}

func schemaSummaryForSkill(skill RuntimeSkill) string {
	if len(skill.AllowedTools) == 0 {
		return "skill instructions"
	}
	return "skill instructions; allowed_tools=" + strings.Join(skill.AllowedTools, ",")
}

func schemaSummaryForValue(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(redactRuntimeValue(value))
	if err != nil {
		return "schema"
	}
	return preview(string(data), 240)
}

func schemaDigestForValue(value any) string {
	data, err := json.Marshal(redactRuntimeValue(value))
	if err != nil {
		data = []byte(fmt.Sprint(value))
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func normalizeCapabilityState(state string, enabled bool) string {
	if !enabled {
		return capabilityStateDisabled
	}
	switch strings.TrimSpace(state) {
	case capabilityStateUnavailable, capabilityStateUnloaded, capabilityStateLoading, capabilityStateLoaded, capabilityStateFailed:
		return state
	default:
		return capabilityStateUnloaded
	}
}

func cloneCapabilityLoadRecords(records map[string]runtimeCapabilityLoadRecord) map[string]runtimeCapabilityLoadRecord {
	if len(records) == 0 {
		return nil
	}
	result := make(map[string]runtimeCapabilityLoadRecord, len(records))
	for key, value := range records {
		result[key] = value
	}
	return result
}

func builtinToolCapabilities() []RuntimeCapability {
	return []RuntimeCapability{
		{ID: "builtin:bash", Kind: "builtin_tool", Name: "bash", Enabled: true, Risk: "write", Description: "Run shell commands."},
		{ID: "builtin:tool_search", Kind: "builtin_tool", Name: "tool_search", Enabled: true, Risk: "read", Description: "Search and select deferred runtime tools."},
		{ID: "builtin:agent_builder_info", Kind: "builtin_tool", Name: "agent_builder_info", Enabled: true, Risk: "read", Description: "Inspect runtime configuration."},
		{ID: "builtin:agent_builder_logs", Kind: "builtin_tool", Name: "agent_builder_logs", Enabled: true, Risk: "read", Description: "Inspect runtime logs."},
		{ID: "builtin:diagnostics", Kind: "builtin_tool", Name: "diagnostics", Enabled: true, Risk: "read", Description: "Read LSP diagnostics."},
		{ID: "builtin:download", Kind: "builtin_tool", Name: "download", Enabled: true, Risk: "write", Description: "Download a URL to a file."},
		{ID: "builtin:edit", Kind: "builtin_tool", Name: "edit", Enabled: true, Risk: "write", Description: "Edit a file."},
		{ID: "builtin:fetch", Kind: "builtin_tool", Name: "fetch", Enabled: true, Risk: "read", Description: "Fetch URL content."},
		{ID: "builtin:glob", Kind: "builtin_tool", Name: "glob", Enabled: true, Risk: "read", Description: "Find files by glob."},
		{ID: "builtin:grep", Kind: "builtin_tool", Name: "grep", Enabled: true, Risk: "read", Description: "Search file contents."},
		{ID: "builtin:job_kill", Kind: "builtin_tool", Name: "job_kill", Enabled: true, Risk: "write", Description: "Stop a background job."},
		{ID: "builtin:job_output", Kind: "builtin_tool", Name: "job_output", Enabled: true, Risk: "read", Description: "Read background job output."},
		{ID: "builtin:list_mcp_resources", Kind: "builtin_tool", Name: "list_mcp_resources", Enabled: true, Risk: "read", Description: "List MCP resources."},
		{ID: "builtin:ls", Kind: "builtin_tool", Name: "ls", Enabled: true, Risk: "read", Description: "List directory contents."},
		{ID: "builtin:lsp_restart", Kind: "builtin_tool", Name: "lsp_restart", Enabled: true, Risk: "write", Description: "Restart an LSP server."},
		{ID: "builtin:multiedit", Kind: "builtin_tool", Name: "multiedit", Enabled: true, Risk: "write", Description: "Apply multiple file edits."},
		{ID: "builtin:read_mcp_resource", Kind: "builtin_tool", Name: "read_mcp_resource", Enabled: true, Risk: "read", Description: "Read an MCP resource."},
		{ID: "builtin:references", Kind: "builtin_tool", Name: "references", Enabled: true, Risk: "read", Description: "Find symbol references."},
		{ID: "builtin:sourcegraph", Kind: "builtin_tool", Name: "sourcegraph", Enabled: true, Risk: "read", Description: "Search Sourcegraph."},
		{ID: "builtin:todos", Kind: "builtin_tool", Name: "todos", Enabled: true, Risk: "write", Description: "Track todo items."},
		{ID: "builtin:view", Kind: "builtin_tool", Name: "view", Enabled: true, Risk: "read", Description: "Read files."},
		{ID: "builtin:web_fetch", Kind: "builtin_tool", Name: "web_fetch", Enabled: true, Risk: "read", Description: "Fetch web content."},
		{ID: "builtin:web_search", Kind: "builtin_tool", Name: "web_search", Enabled: true, Risk: "read", Description: "Search the web."},
		{ID: "builtin:write", Kind: "builtin_tool", Name: "write", Enabled: true, Risk: "write", Description: "Write a file."},
	}
}
