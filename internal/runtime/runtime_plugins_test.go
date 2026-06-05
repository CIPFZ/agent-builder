package runtime

import "testing"

func TestRuntimePluginsFromCapabilitiesBuildsSkillAndMCPBundles(t *testing.T) {
	resp := runtimePluginsFromCapabilities(
		RuntimeSkillsResponse{Skills: []RuntimeSkill{
			{Name: "docs", Description: "read docs", Enabled: true, State: capabilityStateLoaded},
			{Name: "disabled", Enabled: false, State: capabilityStateDisabled},
		}},
		RuntimeMCPServersResponse{Servers: []RuntimeMCPServer{
			{
				Name:     "search",
				Type:     "stdio",
				Command:  "search-mcp",
				State:    capabilityStateLoaded,
				Counts:   RuntimeMCPCounts{Tools: 2, Resources: 1, Prompts: 3},
				Disabled: false,
			},
		}},
	)

	if len(resp.Plugins) != 2 {
		t.Fatalf("plugins len = %d, want 2: %#v", len(resp.Plugins), resp.Plugins)
	}

	var skillsPlugin, mcpPlugin RuntimePlugin
	for _, plugin := range resp.Plugins {
		switch plugin.ID {
		case "runtime:skills":
			skillsPlugin = plugin
		case "mcp:search":
			mcpPlugin = plugin
		}
	}
	if !skillsPlugin.Enabled || skillsPlugin.Kind != "skills" || len(skillsPlugin.Skills) != 2 {
		t.Fatalf("invalid skills plugin: %#v", skillsPlugin)
	}
	if !mcpPlugin.Enabled || mcpPlugin.Kind != "mcp" || mcpPlugin.ToolCount != 2 || mcpPlugin.ResourceCount != 1 || mcpPlugin.PromptCount != 3 {
		t.Fatalf("invalid mcp plugin: %#v", mcpPlugin)
	}
}
