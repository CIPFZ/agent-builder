package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

const (
	mcpServerStateDisabled    = "disabled"
	mcpServerStateUnavailable = "unavailable"
	mcpServerStateUnloaded    = "unloaded"
	mcpServerStateLoading     = "loading"
	mcpServerStateConnected   = "connected"
	mcpServerStateFailed      = "failed"
)

func (r *runtimeService) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	return r.runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) SaveMCPServer(ctx context.Context, req RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {
	cfg, wsID, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	name, next, err := runtimeMCPConfigFromRequest(req)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	if cfg.Config().MCP == nil {
		cfg.Config().MCP = config.MCPs{}
	}
	cfg.Config().MCP[name] = next
	if err := r.saveDesktopMCPServers(func(servers config.MCPs) {
		servers[name] = next
	}); err != nil {
		return RuntimeMCPServersResponse{}, fmt.Errorf("failed to persist mcp server: %w", err)
	}
	if !next.Disabled {
		_ = r.refreshMCPServerLifecycle(ctx, cfg, wsID, name, "server_saved")
	} else {
		r.publishMCPServerEvent(runtimeapi.EventMCPServerDisabled, name, mcpServerStateDisabled, "", "disabled")
	}
	return r.runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) SetMCPServerEnabled(ctx context.Context, req RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {
	cfg, wsID, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	name, err := validateRuntimeMCPName(req.Name)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	next, ok := cfg.Config().MCP[name]
	if !ok {
		return RuntimeMCPServersResponse{}, fmt.Errorf("mcp server %s is not configured", name)
	}
	next.Disabled = !req.Enabled
	cfg.Config().MCP[name] = next
	if err := r.saveDesktopMCPServers(func(servers config.MCPs) {
		servers[name] = next
	}); err != nil {
		return RuntimeMCPServersResponse{}, fmt.Errorf("failed to persist mcp server state: %w", err)
	}
	if req.Enabled {
		_ = r.refreshMCPServerLifecycle(ctx, cfg, wsID, name, "server_enabled")
	} else {
		_ = mcptools.DisableSingle(cfg, name)
		r.publishMCPServerEvent(runtimeapi.EventMCPServerDisabled, name, mcpServerStateDisabled, "", "disabled")
		r.writeMCPAudit("server_disabled", name, "", "server", "disabled", permission.PolicyResult{
			Decision: permission.PolicyAllow,
			Risk:     permission.RiskRead,
			Reason:   "MCP server disabled by runtime request.",
			Mode:     r.currentPolicyMode(),
		}, "", 0)
	}
	return r.runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) RefreshMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {
	cfg, wsID, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return RuntimeMCPServersResponse{}, errors.New("mcp server name is required")
	}
	if _, ok := cfg.Config().MCP[name]; !ok {
		return RuntimeMCPServersResponse{}, fmt.Errorf("mcp server %s is not configured", name)
	}
	r.refreshMCPServerLifecycle(ctx, cfg, wsID, name, "server_refresh")
	return r.runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) refreshMCPServerLifecycle(ctx context.Context, cfg *config.ConfigStore, _ string, name, reason string) error {
	mcpCfg, ok := cfg.Config().MCP[name]
	if !ok {
		return fmt.Errorf("mcp server %s is not configured", name)
	}
	decision := r.evaluateMCPPolicy(name, "", "server", "refresh", permission.RiskNetwork)
	if decision.Decision == permission.PolicyDeny {
		diagnostic := decision.Reason
		r.publishMCPServerEvent(runtimeapi.EventMCPServerFailed, name, mcpServerStateFailed, diagnostic, reason)
		r.writeMCPAudit("server_refresh_denied", name, "", "server", "failed", decision, diagnostic, 0)
		r.recordMCPServerCapabilitiesFailed(name, diagnostic, 0)
		return nil
	}
	if mcpCfg.Disabled {
		r.publishMCPServerEvent(runtimeapi.EventMCPServerDisabled, name, mcpServerStateDisabled, "", "disabled")
		r.writeMCPAudit("server_refresh_disabled", name, "", "server", "disabled", decision, "", 0)
		return nil
	}
	if pending, ok := r.hasBlockingMCPRequest(ctx, name); ok {
		diagnostic := "MCP server refresh is blocked by pending " + pending.Kind + " request " + pending.ID
		r.publishMCPServerEvent(runtimeapi.EventMCPServerBlocked, name, mcpServerStateFailed, diagnostic, pending.Kind+"_pending")
		r.writeMCPAudit("server_refresh_blocked", name, "", "server", "blocked", decision, diagnostic, 0)
		r.recordMCPServerCapabilitiesBlocked(name, diagnostic, pending.Kind+"_pending")
		return nil
	}
	if decision.Decision == permission.PolicyAsk {
		req, err := r.createMCPAuthRequest(ctx, name, stableMCPCapabilityID(name, "server", ""), "Runtime policy requires user approval before starting MCP server authentication or remote connection.", decision)
		if err != nil {
			return err
		}
		diagnostic := "MCP server refresh is blocked by " + req.Kind + " request " + req.ID
		r.publishMCPServerEvent(runtimeapi.EventMCPServerBlocked, name, mcpServerStateFailed, diagnostic, req.Status)
		r.writeMCPAudit("server_refresh_blocked", name, "", "server", "blocked", decision, diagnostic, 0)
		r.recordMCPServerCapabilitiesBlocked(name, diagnostic, "auth_"+req.Status)
		return nil
	}

	start := time.Now()
	startEvent := runtimeapi.EventMCPServerStarting
	if reason == "capability_refresh" {
		startEvent = runtimeapi.EventMCPServerLazyStarted
	}
	r.publishMCPServerEvent(startEvent, name, mcpServerStateLoading, "", reason)
	r.writeMCPAudit("server_refresh_started", name, "", "server", "loading", decision, "", 0)

	if err := mcptools.InitializeSingle(ctx, name, cfg); err != nil {
		duration := time.Since(start).Milliseconds()
		failEvent := runtimeapi.EventMCPServerFailed
		if reason == "capability_refresh" {
			failEvent = runtimeapi.EventMCPServerLazyFailed
		}
		if isMCPAuthRequiredError(err) {
			req, reqErr := r.createMCPAuthRequest(ctx, name, stableMCPCapabilityID(name, "server", ""), "MCP server reported that authentication is required before refresh can continue.", decision)
			if reqErr != nil {
				return reqErr
			}
			diagnostic := "MCP server refresh is blocked by auth request " + req.ID
			r.publishMCPServerEvent(runtimeapi.EventMCPServerBlocked, name, mcpServerStateFailed, diagnostic, "auth_required")
			r.writeMCPAudit("server_refresh_auth_required", name, "", "server", "blocked", decision, diagnostic, duration)
			r.recordMCPServerCapabilitiesBlocked(name, diagnostic, "auth_required")
			return nil
		}
		if isMCPElicitationRequiredError(err) {
			req, reqErr := r.createMCPElicitationRequest(ctx, name, stableMCPCapabilityID(name, "server", ""), "MCP server requires runtime input before refresh can continue.", "MCP server reported that elicitation is required before refresh can continue.", decision)
			if reqErr != nil {
				return reqErr
			}
			diagnostic := "MCP server refresh is blocked by elicitation request " + req.ID
			r.publishMCPServerEvent(runtimeapi.EventMCPServerBlocked, name, mcpServerStateFailed, diagnostic, "elicitation_required")
			r.writeMCPAudit("server_refresh_elicitation_required", name, "", "server", "blocked", decision, diagnostic, duration)
			r.recordMCPServerCapabilitiesBlocked(name, diagnostic, "elicitation_required")
			return nil
		}
		r.publishMCPServerEvent(failEvent, name, mcpServerStateFailed, err.Error(), "connect_failed")
		r.writeMCPAudit("server_refresh_failed", name, "", "server", "failed", decision, err.Error(), duration)
		r.recordMCPServerCapabilitiesFailed(name, err.Error(), duration)
		return nil
	}

	mcptools.RefreshTools(ctx, cfg, name)
	mcptools.RefreshPrompts(ctx, name)
	mcptools.RefreshResources(ctx, name)
	duration := time.Since(start).Milliseconds()
	r.publishMCPServerEvent(runtimeapi.EventMCPServerConnected, name, mcpServerStateConnected, "", "connected")
	r.publishMCPUpdatedEvents(name)
	r.writeMCPAudit("server_refresh_completed", name, "", "server", "connected", decision, "", duration)
	r.recordMCPServerCapabilitiesLoaded(name, cfg)
	return nil
}

func (r *runtimeService) evaluateMCPPolicy(server, name, kind, action string, fallbackRisk permission.Risk) permission.PolicyResult {
	r.mu.Lock()
	policy := r.policy
	r.mu.Unlock()
	input := string(fallbackRisk) + " mcp " + kind + " " + action
	if fallbackRisk == permission.RiskRead {
		input = "read mcp " + kind + " " + action
	}
	result := runtimePermissionPolicy(policy).Evaluate(scheduler.ToolCall{
		ID:           stableMCPCapabilityID(server, kind, name),
		Name:         firstNonEmpty(name, server),
		Source:       scheduler.ToolSourceMCP,
		CapabilityID: stableMCPCapabilityID(server, kind, name),
		Status:       scheduler.ToolCallPending,
		InputSummary: input,
	})
	if result.Risk == "" {
		result.Risk = fallbackRisk
	}
	return result
}

func (r *runtimeService) currentPolicyMode() permission.PolicyMode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return permission.PolicyMode(r.policy.Mode)
}

func (r *runtimeService) publishMCPServerEvent(eventType, name, state, errText, reason string) {
	errText = redactRuntimeString("error", errText)
	payload := map[string]any{
		"name":   name,
		"state":  state,
		"reason": reason,
	}
	if errText != "" {
		payload["error"] = errText
		diagnostic := preview(errText, 240)
		payload["diagnostics"] = diagnostic
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	})
}

func (r *runtimeService) publishMCPUpdatedEvents(name string) {
	for _, eventType := range []string{
		runtimeapi.EventMCPToolsUpdated,
		runtimeapi.EventMCPResourcesUpdated,
		runtimeapi.EventMCPPromptsUpdated,
	} {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      eventType,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"name":   name,
				"server": name,
			},
		})
	}
}

func (r *runtimeService) writeMCPAudit(event, server, name, kind, status string, decision permission.PolicyResult, errText string, durationMS int64) {
	errText = redactRuntimeString("error", errText)
	r.writeAudit(auditEntry{
		Event:               "mcp_" + event,
		Timestamp:           time.Now().UTC().Format(time.RFC3339Nano),
		CapabilityID:        stableMCPCapabilityID(server, kind, name),
		CapabilityKind:      "mcp_" + kind,
		CapabilitySource:    server,
		CapabilityState:     status,
		CapabilityReason:    decision.Reason,
		CapabilityError:     errText,
		DurationMS:          durationMS,
		MCPServer:           server,
		MCPName:             name,
		MCPKind:             kind,
		MCPStatus:           status,
		MCPDecision:         string(decision.Decision),
		MCPRisk:             string(decision.Risk),
		MCPReason:           decision.Reason,
		PolicyMode:          string(decision.Mode),
		PolicyProfile:       decision.Profile,
		PolicyRuleID:        decision.RuleID,
		PolicyRuleSource:    decision.RuleSource,
		PolicyScopeKind:     decision.RuleScopeKind,
		PolicyScopeValue:    decision.RuleScopeValue,
		PolicyTargetSummary: decision.TargetSummary,
	})
}

func (r *runtimeService) recordMCPServerCapabilitiesFailed(server, errText string, durationMS int64) {
	capabilities := r.mcpCapabilitiesForServer(server)
	if len(capabilities) == 0 {
		capabilities = []RuntimeCapability{{
			ID:      "mcp_server:" + server,
			Kind:    "mcp_server",
			Name:    server,
			Source:  server,
			Enabled: true,
			Risk:    "network",
			State:   capabilityStateFailed,
		}}
	}
	for _, capability := range capabilities {
		capability.State = capabilityStateFailed
		capability.Error = errText
		capability.Diagnostics = errText
		capability.Reason = "server_refresh_failed"
		r.setCapabilityLoadRecord(capability.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateFailed,
			Diagnostics: capability.Diagnostics,
			Error:       capability.Error,
			Reason:      capability.Reason,
			UpdatedAt:   time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(capability, capabilityStateFailed, capability.Reason, errText, durationMS)
	}
}

func (r *runtimeService) recordMCPServerCapabilitiesBlocked(server, diagnostic, reason string) {
	capabilities := r.mcpCapabilitiesForServer(server)
	if len(capabilities) == 0 {
		capabilities = []RuntimeCapability{{
			ID:      "mcp_server:" + server,
			Kind:    "mcp_server",
			Name:    server,
			Source:  server,
			Enabled: true,
			Risk:    "network",
			State:   capabilityStateFailed,
		}}
	}
	for _, capability := range capabilities {
		if !capability.Enabled {
			continue
		}
		capability.State = capabilityStateFailed
		capability.Error = diagnostic
		capability.Diagnostics = diagnostic
		capability.Reason = reason
		r.setCapabilityLoadRecord(capability.ID, runtimeCapabilityLoadRecord{
			State:       capabilityStateFailed,
			Diagnostics: capability.Diagnostics,
			Error:       capability.Error,
			Reason:      capability.Reason,
			UpdatedAt:   time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(capability, capabilityStateFailed, capability.Reason, diagnostic, 0)
	}
}

func (r *runtimeService) recordMCPServerCapabilitiesLoaded(server string, cfg *config.ConfigStore) {
	capabilities := r.mcpCapabilitiesForServer(server)
	for _, tool := range runtimeMCPToolsFromConfig(cfg, server).Tools {
		id := "mcp:" + tool.Server + ":" + tool.Name
		if slices.ContainsFunc(capabilities, func(existing RuntimeCapability) bool { return existing.ID == id }) {
			continue
		}
		capabilities = append(capabilities, RuntimeCapability{
			ID:          id,
			Kind:        "mcp_tool",
			Name:        tool.Name,
			Source:      tool.Server,
			Enabled:     tool.Enabled,
			Risk:        "network",
			Description: tool.Description,
			State:       capabilityStateLoaded,
		})
	}
	for _, capability := range capabilities {
		if !capability.Enabled {
			continue
		}
		capability.State = capabilityStateLoaded
		capability.Reason = "server_refresh_completed"
		r.setCapabilityLoadRecord(capability.ID, runtimeCapabilityLoadRecord{
			State:     capabilityStateLoaded,
			Reason:    capability.Reason,
			UpdatedAt: time.Now().UnixMilli(),
		})
		r.recordCapabilityLoad(capability, capabilityStateLoaded, capability.Reason, "", 0)
	}
}

func (r *runtimeService) mcpCapabilitiesForServer(server string) []RuntimeCapability {
	cfg, _, err := r.workspaceConfig(context.Background())
	if err != nil {
		return nil
	}
	resp := runtimeCapabilities(
		cfg,
		RuntimeSkillsResponse{},
		runtimeMCPToolsFromConfig(cfg, server),
		runtimeMCPResources(server),
		runtimeMCPPrompts(server),
		r.policy,
	)
	var capabilities []RuntimeCapability
	for _, capability := range resp.Capabilities {
		if capability.Source == server && strings.HasPrefix(capability.Kind, "mcp_") {
			capabilities = append(capabilities, capability)
		}
	}
	return capabilities
}

func stableMCPCapabilityID(server, kind, name string) string {
	switch kind {
	case "tool":
		return "mcp:" + server + ":" + name
	case "resource":
		return "mcp_resource:" + server + ":" + name
	case "prompt":
		return "mcp_prompt:" + server + ":" + name
	default:
		if server == "" {
			return ""
		}
		return "mcp_server:" + server
	}
}

func (r *runtimeService) SetMCPToolEnabled(ctx context.Context, req RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPToolsResponse{}, err
	}
	server, err := validateRuntimeMCPName(req.Server)
	if err != nil {
		return RuntimeMCPToolsResponse{}, err
	}
	tool := strings.TrimSpace(req.Tool)
	if tool == "" {
		return RuntimeMCPToolsResponse{}, errors.New("mcp tool name is required")
	}
	next, ok := cfg.Config().MCP[server]
	if !ok {
		return RuntimeMCPToolsResponse{}, fmt.Errorf("mcp server %s is not configured", server)
	}
	next.EnabledTools = slices.DeleteFunc(slices.Clone(next.EnabledTools), func(existing string) bool {
		return existing == tool
	})
	next.DisabledTools = slices.DeleteFunc(slices.Clone(next.DisabledTools), func(existing string) bool {
		return existing == tool
	})
	if req.Enabled {
		if len(next.EnabledTools) > 0 {
			next.EnabledTools = append(next.EnabledTools, tool)
			slices.Sort(next.EnabledTools)
			next.EnabledTools = slices.Compact(next.EnabledTools)
		}
	} else {
		next.DisabledTools = append(next.DisabledTools, tool)
		slices.Sort(next.DisabledTools)
		next.DisabledTools = slices.Compact(next.DisabledTools)
	}
	cfg.Config().MCP[server] = next
	if err := r.saveDesktopMCPServers(func(servers config.MCPs) {
		servers[server] = next
	}); err != nil {
		return RuntimeMCPToolsResponse{}, fmt.Errorf("failed to persist mcp tool state: %w", err)
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventMCPToolsUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"server":  server,
			"tool":    tool,
			"enabled": req.Enabled,
		},
	})
	return r.filterMCPToolsByPolicy(runtimeMCPToolsFromConfig(cfg, server)), nil
}

func (r *runtimeService) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPToolsResponse{}, err
	}
	name = strings.TrimSpace(name)
	if denied := r.mcpReadPolicyDenied(name, "tools"); denied != nil {
		return RuntimeMCPToolsResponse{}, denied
	}
	return r.filterMCPToolsByPolicy(runtimeMCPToolsFromConfig(cfg, name)), nil
}

func (r *runtimeService) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPResourcesResponse{}, err
	}
	name = strings.TrimSpace(name)
	if denied := r.mcpReadPolicyDenied(name, "resources"); denied != nil {
		return RuntimeMCPResourcesResponse{}, denied
	}
	return r.filterMCPResourcesByPolicy(runtimeMCPResources(name)), nil
}

func (r *runtimeService) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPPromptsResponse{}, err
	}
	name = strings.TrimSpace(name)
	if denied := r.mcpReadPolicyDenied(name, "prompts"); denied != nil {
		return RuntimeMCPPromptsResponse{}, denied
	}
	return r.filterMCPPromptsByPolicy(runtimeMCPPrompts(name)), nil
}

func (r *runtimeService) filterMCPToolsByPolicy(resp RuntimeMCPToolsResponse) RuntimeMCPToolsResponse {
	out := make([]RuntimeMCPTool, 0, len(resp.Tools))
	for _, tool := range resp.Tools {
		decision := r.evaluateMCPPolicy(tool.Server, tool.Name, "tool", "inventory", permission.RiskNetwork)
		r.recordMCPCapabilityDecision(tool.Server, tool.Name, "tool", decision)
		if decision.Decision == permission.PolicyDeny {
			continue
		}
		out = append(out, tool)
	}
	resp.Tools = out
	return resp
}

func (r *runtimeService) filterMCPResourcesByPolicy(resp RuntimeMCPResourcesResponse) RuntimeMCPResourcesResponse {
	out := make([]RuntimeMCPResource, 0, len(resp.Resources))
	for _, resource := range resp.Resources {
		decision := r.evaluateMCPPolicy(resource.Server, resource.URI, "resource", "inventory", permission.RiskRead)
		r.recordMCPCapabilityDecision(resource.Server, resource.URI, "resource", decision)
		if decision.Decision == permission.PolicyDeny {
			continue
		}
		out = append(out, resource)
	}
	resp.Resources = out
	return resp
}

func (r *runtimeService) filterMCPPromptsByPolicy(resp RuntimeMCPPromptsResponse) RuntimeMCPPromptsResponse {
	out := make([]RuntimeMCPPrompt, 0, len(resp.Prompts))
	for _, prompt := range resp.Prompts {
		decision := r.evaluateMCPPolicy(prompt.Server, prompt.Name, "prompt", "inventory", permission.RiskRead)
		r.recordMCPCapabilityDecision(prompt.Server, prompt.Name, "prompt", decision)
		if decision.Decision == permission.PolicyDeny {
			continue
		}
		out = append(out, prompt)
	}
	resp.Prompts = out
	return resp
}

func (r *runtimeService) recordMCPCapabilityDecision(server, name, kind string, decision permission.PolicyResult) {
	eventType := runtimeapi.EventMCPCapabilityAllowed
	if decision.Decision == permission.PolicyDeny {
		eventType = runtimeapi.EventMCPCapabilityDenied
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"server":        server,
			"name":          name,
			"kind":          kind,
			"capability_id": stableMCPCapabilityID(server, kind, name),
			"decision":      decision.Decision,
			"risk":          decision.Risk,
			"reason":        decision.Reason,
			"mode":          decision.Mode,
			"summary":       firstNonEmpty(name, server),
		},
	})
}

func (r *runtimeService) mcpReadPolicyDenied(server, kind string) error {
	decision := r.evaluateMCPPolicy(server, "", kind, "list", permission.RiskRead)
	r.writeMCPAudit(kind+"_list", server, "", kind, "metadata", decision, "", 0)
	if decision.Decision == permission.PolicyDeny {
		return fmt.Errorf("runtime policy denied MCP %s list: %s", kind, decision.Reason)
	}
	return nil
}

func (r *runtimeService) saveDesktopMCPServers(update func(config.MCPs)) error {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return err
	}
	desktopCfg, err := loadDesktopMCPConfig(layout)
	if err != nil {
		return err
	}
	if desktopCfg.Servers == nil {
		desktopCfg.Servers = config.MCPs{}
	}
	update(desktopCfg.Servers)
	return saveDesktopMCPConfig(layout, desktopCfg)
}

func (r *runtimeService) runtimeMCPServersFromConfig(store *config.ConfigStore) RuntimeMCPServersResponse {
	return runtimeMCPServersFromConfigWithPolicy(store, func(server string) permission.PolicyResult {
		return r.evaluateMCPPolicy(server, "", "server", "inventory", permission.RiskRead)
	})
}

func runtimeMCPServersFromConfig(store *config.ConfigStore) RuntimeMCPServersResponse {
	return runtimeMCPServersFromConfigWithPolicy(store, nil)
}

func runtimeMCPServersFromConfigWithPolicy(store *config.ConfigStore, evaluate func(string) permission.PolicyResult) RuntimeMCPServersResponse {
	states := mcptools.GetStates()
	servers := make([]RuntimeMCPServer, 0, len(store.Config().MCP))
	for _, item := range store.Config().MCP.Sorted() {
		cfg := item.MCP
		state := mcpServerStateDisabled
		reason := "disabled"
		diagnostics := ""
		var counts RuntimeMCPCounts
		var errorText string
		if !cfg.Disabled {
			state = mcpServerStateUnloaded
			reason = "metadata_known"
			policyDenied := false
			if evaluate != nil {
				decision := evaluate(item.Name)
				if decision.Decision == permission.PolicyDeny {
					state = mcpServerStateDisabled
					reason = "policy_denied"
					diagnostics = decision.Reason
					errorText = decision.Reason
					policyDenied = true
				}
			}
			if info, ok := states[item.Name]; ok && !policyDenied {
				state, reason = normalizeMCPServerState(info.State)
				counts = RuntimeMCPCounts{
					Tools:     info.Counts.Tools,
					Prompts:   info.Counts.Prompts,
					Resources: info.Counts.Resources,
				}
				if info.Error != nil {
					errorText = redactRuntimeString("error", info.Error.Error())
					diagnostics = errorText
				}
			}
		}
		servers = append(servers, RuntimeMCPServer{
			Name:          item.Name,
			Type:          string(cfg.Type),
			URL:           redactURL(cfg.URL),
			Command:       redactRuntimeString("command", cfg.Command),
			Args:          redactStringSlice(cfg.Args),
			Disabled:      cfg.Disabled,
			State:         state,
			Counts:        counts,
			Diagnostics:   diagnostics,
			Reason:        reason,
			Error:         errorText,
			Env:           redactMap(cfg.Env),
			Headers:       redactMap(cfg.Headers),
			EnabledTools:  slices.Clone(cfg.EnabledTools),
			DisabledTools: slices.Clone(cfg.DisabledTools),
		})
	}
	return RuntimeMCPServersResponse{Servers: servers}
}

func runtimeMCPToolsFromConfig(store *config.ConfigStore, server string) RuntimeMCPToolsResponse {
	var tools []RuntimeMCPTool
	for name, serverTools := range mcptools.Tools() {
		if server != "" && name != server {
			continue
		}
		cfg := store.Config().MCP[name]
		for _, tool := range serverTools {
			tools = append(tools, RuntimeMCPTool{
				Server:      name,
				Name:        tool.Name,
				Description: tool.Description,
				Enabled:     mcpToolEnabled(cfg, tool.Name),
				InputSchema: tool.InputSchema,
			})
		}
	}
	slices.SortStableFunc(tools, func(a, b RuntimeMCPTool) int {
		if c := strings.Compare(a.Server, b.Server); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return RuntimeMCPToolsResponse{Tools: tools}
}

func normalizeMCPServerState(state mcptools.State) (string, string) {
	switch state {
	case mcptools.StateDisabled:
		return mcpServerStateDisabled, "disabled"
	case mcptools.StateStarting:
		return mcpServerStateLoading, "connecting"
	case mcptools.StateConnected:
		return mcpServerStateConnected, "connected"
	case mcptools.StateError:
		return mcpServerStateFailed, "connect_failed"
	default:
		return mcpServerStateUnavailable, "unknown_state"
	}
}

func runtimeMCPResources(server string) RuntimeMCPResourcesResponse {
	var resources []RuntimeMCPResource
	for name, serverResources := range mcptools.Resources() {
		if server != "" && name != server {
			continue
		}
		for _, resource := range serverResources {
			resources = append(resources, RuntimeMCPResource{
				Server:      name,
				URI:         resource.URI,
				Name:        resource.Name,
				Description: resource.Description,
				MIMEType:    resource.MIMEType,
			})
		}
	}
	return RuntimeMCPResourcesResponse{Resources: resources}
}

func runtimeMCPPrompts(server string) RuntimeMCPPromptsResponse {
	var prompts []RuntimeMCPPrompt
	for name, serverPrompts := range mcptools.Prompts() {
		if server != "" && name != server {
			continue
		}
		for _, prompt := range serverPrompts {
			prompts = append(prompts, RuntimeMCPPrompt{
				Server:      name,
				Name:        prompt.Name,
				Description: prompt.Description,
			})
		}
	}
	return RuntimeMCPPromptsResponse{Prompts: prompts}
}

func runtimeMCPConfigFromRequest(req RuntimeMCPServerConfigRequest) (string, config.MCPConfig, error) {
	name, err := validateRuntimeMCPName(req.Name)
	if err != nil {
		return "", config.MCPConfig{}, err
	}
	mcpType := config.MCPType(strings.TrimSpace(req.Type))
	if mcpType == "" {
		mcpType = config.MCPStdio
	}
	if mcpType != config.MCPStdio && mcpType != config.MCPHttp && mcpType != config.MCPSSE {
		return "", config.MCPConfig{}, fmt.Errorf("unsupported mcp server type: %s", req.Type)
	}
	next := config.MCPConfig{
		Type:          mcpType,
		URL:           strings.TrimSpace(req.URL),
		Command:       strings.TrimSpace(req.Command),
		Args:          trimStringSlice(req.Args),
		Disabled:      req.Disabled,
		EnabledTools:  sortedUniqueStrings(req.EnabledTools),
		DisabledTools: sortedUniqueStrings(req.DisabledTools),
		Env:           cloneStringMap(req.Env),
		Headers:       cloneStringMap(req.Headers),
	}
	switch mcpType {
	case config.MCPStdio:
		if next.Command == "" {
			return "", config.MCPConfig{}, errors.New("stdio mcp servers require command")
		}
	case config.MCPHttp, config.MCPSSE:
		if next.URL == "" {
			return "", config.MCPConfig{}, errors.New("http and sse mcp servers require url")
		}
	}
	return name, next, nil
}

func validateRuntimeMCPName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", errors.New("mcp server name is required")
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return "", fmt.Errorf("mcp server name %q must use only letters, numbers, underscore, or dash", name)
	}
	return name, nil
}

func trimStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func sortedUniqueStrings(values []string) []string {
	result := trimStringSlice(values)
	if len(result) == 0 {
		return nil
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mcpToolEnabled(cfg config.MCPConfig, name string) bool {
	if len(cfg.EnabledTools) > 0 && !slices.Contains(cfg.EnabledTools, name) {
		return false
	}
	return !slices.Contains(cfg.DisabledTools, name)
}

func redactMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if shouldRedact(key, value) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func redactStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = redactRuntimeString("", value)
	}
	return result
}

func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "@") {
		return "[REDACTED_URL]"
	}
	return raw
}

func shouldRedact(key, value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key+" "+value, "-", "_"))
	for _, marker := range []string{"authorization", "api_key", "apikey", "token", "secret", "password", "credential", "access_key", "private_key", "proxy", "bearer "} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isMCPAuthRequiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"auth required", "authentication required", "authorization required", "oauth required", "unauthorized", "401"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isMCPElicitationRequiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"elicitation required", "user input required", "ask user", "requires input", "request for input"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
