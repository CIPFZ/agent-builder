package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

func TestExtensionInventoryProjectsRuntimeOwnedExtensionsDeterministically(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "review-local")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: Review Local
description: Review local code changes.
allowed-tools: [Read, Grep]
context: project
agent: explorer
hooks:
  pre: check
---
# Review Local
`), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	registry := tools.NewRegistry(
		extensionProbeTool{name: "ZuluTool", source: "dynamic"},
		extensionProbeTool{name: "AlphaTool", source: "dynamic"},
		extensionProbeTool{name: "DeniedTool", source: "dynamic"},
	)
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), registry, Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "DeniedTool", Action: permissions.ActionDeny, Source: string(permissions.RuleSourceSession)},
			},
		},
		Commands: []tools.Command{
			{Name: "zeta", Type: "slash", Source: "dynamic", Description: "zeta command", UserInvocable: true},
			{Name: "inspect", Type: "slash", Source: "plugin", Description: "inspect command", UserInvocable: true},
		},
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
		MCPPrompts: map[string]tools.MCPPromptsListResult{
			"beta": {Prompts: []tools.MCPPromptListItem{{Name: "triage"}}},
		},
		MCPResources: map[string][]tools.MCPResource{
			"beta": {{URI: "file://resource"}},
		},
		MCPSkills: map[string][]tools.SkillCommand{
			"beta": {{Name: "remote-skill", DisplayName: "Remote Skill", Description: "mcp skill", MCPPrompt: true, MCPServer: "beta"}},
		},
		MCPNeedsAuth: map[string]tools.MCPAuthToolResult{
			"authsrv": {Status: "needs-auth", Message: "login required"},
		},
		SkillRoots: []string{skillRoot},
	})

	inventory := runner.ExtensionInventory(sess.ID)
	toolNames := make([]string, 0, len(inventory.Tools))
	for _, tool := range inventory.Tools {
		toolNames = append(toolNames, tool.Name)
		if tool.Name == "DeniedTool" {
			t.Fatalf("denied tool was exposed in extension inventory: %#v", inventory.Tools)
		}
	}
	if !reflect.DeepEqual(toolNames, sortedCopy(toolNames)) {
		t.Fatalf("tool names are not stable sorted: %#v", toolNames)
	}
	if !containsString(toolNames, "AlphaTool") || !containsString(toolNames, "mcp__beta__remote") {
		t.Fatalf("tools = %#v, want dynamic and MCP tools", toolNames)
	}

	commandNames := make([]string, 0, len(inventory.Commands))
	for _, command := range inventory.Commands {
		commandNames = append(commandNames, command.Name)
	}
	if !reflect.DeepEqual(commandNames, []string{"inspect", "zeta"}) {
		t.Fatalf("commands = %#v, want stable sorted dynamic command projection", commandNames)
	}

	skill, ok := findExtensionSkill(inventory.Skills, "review-local")
	if !ok {
		t.Fatalf("skills = %#v, want discovered review-local skill", inventory.Skills)
	}
	if !reflect.DeepEqual(skill.AllowedTools, []string{"Grep", "Read"}) || skill.Context != "project" || skill.Agent != "explorer" || skill.Hooks == nil {
		t.Fatalf("skill projection = %#v, want frontmatter allowed tools, context, agent, hooks", skill)
	}
	if mcpSkill, ok := findExtensionSkill(inventory.Skills, "remote-skill"); !ok || mcpSkill.Source != "mcp" || mcpSkill.MCPServer != "beta" {
		t.Fatalf("MCP skill projection = %#v, found=%v", mcpSkill, ok)
	}

	serverStatuses := map[string]string{}
	for _, server := range inventory.MCPServers {
		serverStatuses[server.Name] = server.Status
	}
	if serverStatuses["beta"] != "connected" || serverStatuses["authsrv"] != "needs-auth" {
		t.Fatalf("MCP statuses = %#v, want connected and needs-auth", serverStatuses)
	}
	if len(inventory.LSPBoundaries) != 1 || inventory.LSPBoundaries[0].Status != "deferred" {
		t.Fatalf("LSP boundaries = %#v, want deferred schema boundary", inventory.LSPBoundaries)
	}
	if !containsString(inventory.DeferredCapabilities, "plugin_marketplace") {
		t.Fatalf("deferred capabilities = %#v, want plugin_marketplace deferred", inventory.DeferredCapabilities)
	}
}

func TestExtensionInventoryRebuildsDeterministicallyAfterRunnerRestart(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	options := Options{
		Commands: []tools.Command{{Name: "inspect", Type: "slash", Source: "dynamic"}},
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
	}
	first := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(extensionProbeTool{name: "AlphaTool", source: "dynamic"}), options)
	second := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(extensionProbeTool{name: "AlphaTool", source: "dynamic"}), options)

	if !reflect.DeepEqual(first.ExtensionInventory(sess.ID), second.ExtensionInventory(sess.ID)) {
		t.Fatalf("extension inventory changed across rebuild:\nfirst=%#v\nsecond=%#v", first.ExtensionInventory(sess.ID), second.ExtensionInventory(sess.ID))
	}
}

type extensionProbeTool struct {
	name   string
	source string
}

func (t extensionProbeTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        t.name,
		Description: t.name + " description",
		InputSchema: map[string]any{"type": "object"},
		Source:      t.source,
		SearchHint:  "probe",
		ReadOnly:    true,
	}
}

func (t extensionProbeTool) Invoke(context.Context, session.Session, string) (string, error) {
	return "ok", nil
}

func (t extensionProbeTool) IsEnabled() bool           { return true }
func (t extensionProbeTool) IsReadOnly(string) bool    { return true }
func (t extensionProbeTool) IsDestructive(string) bool { return false }
func (t extensionProbeTool) ShouldDefer() bool         { return false }
func (t extensionProbeTool) AlwaysLoad() bool          { return false }
func (t extensionProbeTool) PromptDescription() string { return "" }
func (t extensionProbeTool) SearchHint() string        { return "probe" }

func findExtensionSkill(skills []ExtensionSkill, name string) (ExtensionSkill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return ExtensionSkill{}, false
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if strings.ToLower(out[j]) < strings.ToLower(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
