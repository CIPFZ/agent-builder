package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func (r *runtimeService) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	return runtimeMCPServersFromConfig(cfg), nil
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
	if err := cfg.SetConfigField(config.ScopeGlobal, "mcp."+name, next); err != nil {
		return RuntimeMCPServersResponse{}, fmt.Errorf("failed to persist mcp server: %w", err)
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventMCPServerStarting,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name":     name,
			"disabled": next.Disabled,
		},
	})
	if !next.Disabled {
		runtimeCtx := ctx
		r.mu.Lock()
		if r.runtimeCtx != nil {
			runtimeCtx = r.runtimeCtx
		}
		r.mu.Unlock()
		r.runtime.RefreshMCPTools(runtimeCtx, wsID, name)
		r.runtime.MCPRefreshPrompts(runtimeCtx, wsID, name)
		r.runtime.MCPRefreshResources(runtimeCtx, wsID, name)
	}
	return runtimeMCPServersFromConfig(cfg), nil
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
	if err := cfg.SetConfigField(config.ScopeGlobal, "mcp."+name, next); err != nil {
		return RuntimeMCPServersResponse{}, fmt.Errorf("failed to persist mcp server state: %w", err)
	}
	eventType := runtimeapi.EventMCPServerDisabled
	if req.Enabled {
		eventType = runtimeapi.EventMCPServerStarting
		r.runtime.RefreshMCPTools(ctx, wsID, name)
		r.runtime.MCPRefreshPrompts(ctx, wsID, name)
		r.runtime.MCPRefreshResources(ctx, wsID, name)
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name": name,
		},
	})
	return runtimeMCPServersFromConfig(cfg), nil
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
	r.runtime.RefreshMCPTools(ctx, wsID, name)
	r.runtime.MCPRefreshPrompts(ctx, wsID, name)
	r.runtime.MCPRefreshResources(ctx, wsID, name)
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventMCPToolsUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name": name,
		},
	})
	return runtimeMCPServersFromConfig(cfg), nil
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
	if err := cfg.SetConfigField(config.ScopeGlobal, "mcp."+server, next); err != nil {
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
	return runtimeMCPToolsFromConfig(cfg, server), nil
}

func (r *runtimeService) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPToolsResponse{}, err
	}
	return runtimeMCPToolsFromConfig(cfg, strings.TrimSpace(name)), nil
}

func (r *runtimeService) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPResourcesResponse{}, err
	}
	return runtimeMCPResources(strings.TrimSpace(name)), nil
}

func (r *runtimeService) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPPromptsResponse{}, err
	}
	return runtimeMCPPrompts(strings.TrimSpace(name)), nil
}

func runtimeMCPServersFromConfig(store *config.ConfigStore) RuntimeMCPServersResponse {
	states := mcptools.GetStates()
	servers := make([]RuntimeMCPServer, 0, len(store.Config().MCP))
	for _, item := range store.Config().MCP.Sorted() {
		cfg := item.MCP
		state := "disabled"
		var counts RuntimeMCPCounts
		var errorText string
		if !cfg.Disabled {
			state = "starting"
			if info, ok := states[item.Name]; ok {
				state = info.State.String()
				counts = RuntimeMCPCounts{
					Tools:     info.Counts.Tools,
					Prompts:   info.Counts.Prompts,
					Resources: info.Counts.Resources,
				}
				if info.Error != nil {
					errorText = info.Error.Error()
				}
			}
		}
		servers = append(servers, RuntimeMCPServer{
			Name:          item.Name,
			Type:          string(cfg.Type),
			URL:           redactURL(cfg.URL),
			Command:       cfg.Command,
			Args:          slices.Clone(cfg.Args),
			Disabled:      cfg.Disabled,
			State:         state,
			Counts:        counts,
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
	normalized := strings.ToLower(key + " " + value)
	for _, marker := range []string{"authorization", "api_key", "apikey", "token", "secret", "password", "bearer "} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
