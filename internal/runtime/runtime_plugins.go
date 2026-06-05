package runtime

import (
	"context"
	"slices"
	"strings"
)

func (r *runtimeService) Plugins(ctx context.Context) (RuntimePluginsResponse, error) {
	skills, err := r.Skills(ctx)
	if err != nil {
		return RuntimePluginsResponse{}, err
	}
	mcpServers, err := r.MCPServers(ctx)
	if err != nil {
		return RuntimePluginsResponse{}, err
	}
	return runtimePluginsFromCapabilities(skills, mcpServers), nil
}

func runtimePluginsFromCapabilities(skills RuntimeSkillsResponse, mcpServers RuntimeMCPServersResponse) RuntimePluginsResponse {
	plugins := make([]RuntimePlugin, 0, len(mcpServers.Servers)+1)
	if len(skills.Skills) > 0 {
		plugins = append(plugins, runtimeSkillBundlePlugin(skills.Skills))
	}
	for _, server := range mcpServers.Servers {
		plugins = append(plugins, RuntimePlugin{
			ID:            "mcp:" + server.Name,
			Name:          server.Name,
			Description:   runtimeMCPPluginDescription(server),
			Category:      "MCP",
			Source:        firstNonEmpty(server.Type, "runtime"),
			Kind:          "mcp",
			Icon:          "mcp",
			Enabled:       !server.Disabled,
			State:         server.State,
			Diagnostics:   server.Diagnostics,
			Reason:        server.Reason,
			Error:         server.Error,
			MCPServers:    []string{server.Name},
			ToolCount:     server.Counts.Tools,
			ResourceCount: server.Counts.Resources,
			PromptCount:   server.Counts.Prompts,
		})
	}
	slices.SortStableFunc(plugins, func(a, b RuntimePlugin) int {
		if a.Category != b.Category {
			return strings.Compare(a.Category, b.Category)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return RuntimePluginsResponse{Plugins: plugins}
}

func runtimeSkillBundlePlugin(skills []RuntimeSkill) RuntimePlugin {
	names := make([]string, 0, len(skills))
	enabled := false
	state := capabilityStateLoaded
	for _, skill := range skills {
		names = append(names, skill.Name)
		if skill.Enabled {
			enabled = true
		}
		if skill.State == capabilityStateFailed {
			state = capabilityStateFailed
		}
	}
	slices.Sort(names)
	if !enabled && state != capabilityStateFailed {
		state = capabilityStateDisabled
	}
	return RuntimePlugin{
		ID:          "runtime:skills",
		Name:        "Runtime Skills",
		Description: "Runtime-discovered skills and activation metadata.",
		Category:    "Skills",
		Source:      "runtime",
		Kind:        "skills",
		Icon:        "skills",
		Enabled:     enabled,
		State:       state,
		Skills:      names,
	}
}

func runtimeMCPPluginDescription(server RuntimeMCPServer) string {
	if server.Diagnostics != "" {
		return server.Diagnostics
	}
	if server.URL != "" {
		return "MCP server at " + server.URL
	}
	if server.Command != "" {
		return "MCP server command " + server.Command
	}
	return "Runtime MCP server"
}
