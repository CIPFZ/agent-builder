package main

import (
	"context"
	"slices"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

func (r *runtimeService) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeCapabilitiesResponse{}, err
	}
	skills := runtimeSkillsFromConfig(cfg, r.desktopSkillPaths()...)
	tools := runtimeMCPToolsFromConfig(cfg, "")
	resources := runtimeMCPResources("")
	prompts := runtimeMCPPrompts("")
	return runtimeCapabilities(cfg, skills, tools, resources, prompts), nil
}

func runtimeCapabilities(
	store *config.ConfigStore,
	skills RuntimeSkillsResponse,
	mcpTools RuntimeMCPToolsResponse,
	mcpResources RuntimeMCPResourcesResponse,
	mcpPrompts RuntimeMCPPromptsResponse,
) RuntimeCapabilitiesResponse {
	var capabilities []RuntimeCapability
	disabledTools := map[string]bool{}
	if store.Config().Options != nil {
		for _, name := range store.Config().Options.DisabledTools {
			disabledTools[name] = true
		}
	}
	for _, tool := range builtinToolCapabilities() {
		tool.Enabled = !disabledTools[tool.Name]
		capabilities = append(capabilities, tool)
	}
	for _, skill := range skills.Skills {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "skill:" + skill.Name,
			Kind:        "skill",
			Name:        skill.Name,
			Source:      skill.Path,
			Enabled:     skill.Enabled,
			Risk:        "context",
			Description: skill.Description,
		})
	}
	for _, tool := range mcpTools.Tools {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp:" + tool.Server + ":" + tool.Name,
			Kind:        "mcp_tool",
			Name:        tool.Name,
			Source:      tool.Server,
			Enabled:     tool.Enabled,
			Risk:        "external",
			Description: tool.Description,
		})
	}
	for _, resource := range mcpResources.Resources {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp_resource:" + resource.Server + ":" + resource.URI,
			Kind:        "mcp_resource",
			Name:        firstNonEmpty(resource.Name, resource.URI),
			Source:      resource.Server,
			Enabled:     true,
			Risk:        "read",
			Description: resource.Description,
		})
	}
	for _, prompt := range mcpPrompts.Prompts {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp_prompt:" + prompt.Server + ":" + prompt.Name,
			Kind:        "mcp_prompt",
			Name:        prompt.Name,
			Source:      prompt.Server,
			Enabled:     true,
			Risk:        "context",
			Description: prompt.Description,
		})
	}
	slices.SortStableFunc(capabilities, func(a, b RuntimeCapability) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return RuntimeCapabilitiesResponse{Capabilities: capabilities}
}

func builtinToolCapabilities() []RuntimeCapability {
	return []RuntimeCapability{
		{ID: "builtin:bash", Kind: "builtin_tool", Name: "bash", Enabled: true, Risk: "write", Description: "Run shell commands."},
		{ID: "builtin:crush_info", Kind: "builtin_tool", Name: "crush_info", Enabled: true, Risk: "read", Description: "Inspect runtime configuration."},
		{ID: "builtin:crush_logs", Kind: "builtin_tool", Name: "crush_logs", Enabled: true, Risk: "read", Description: "Inspect runtime logs."},
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
