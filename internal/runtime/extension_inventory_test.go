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
	for _, want := range []string{"compact", "help", "inspect", "mcp", "memory", "model", "permissions", "resume", "status", "tasks", "zeta"} {
		if !containsString(commandNames, want) {
			t.Fatalf("commands = %#v, want runtime command %q", commandNames, want)
		}
	}
	if !reflect.DeepEqual(commandNames, sortedCopy(commandNames)) {
		t.Fatalf("commands are not stable sorted: %#v", commandNames)
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

func TestExtensionInventoryIncludesDefaultRuntimeSlashCommandsWithoutConfiguredCommands(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{})

	commandsByName := map[string]ExtensionCommand{}
	for _, command := range runner.ExtensionInventory(sess.ID).Commands {
		commandsByName[command.Name] = command
	}
	for _, want := range []string{"permissions", "status", "memory", "tasks", "mcp"} {
		command, ok := commandsByName[want]
		if !ok {
			t.Fatalf("runtime command %q missing from extension inventory: %#v", want, commandsByName)
		}
		if command.Source != "runtime" || command.Type != "slash" {
			t.Fatalf("runtime command %q = %#v, want source runtime and type slash", want, command)
		}
	}
}

func TestExtensionInventoryConfiguredCommandsOverrideRuntimeCommands(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{
		Commands: []tools.Command{
			{Name: "status", Type: "slash", Source: "plugin", Description: "plugin status", UserInvocable: true},
			{Name: "custom", Type: "slash", Source: "dynamic", Description: "custom command", UserInvocable: true},
		},
	})

	var statusCount int
	var statusCommand ExtensionCommand
	for _, command := range runner.ExtensionInventory(sess.ID).Commands {
		if command.Name == "status" {
			statusCount++
			statusCommand = command
		}
	}
	if statusCount != 1 {
		t.Fatalf("status command count = %d, want one deduped command", statusCount)
	}
	if statusCommand.Source != "plugin" || statusCommand.Description != "plugin status" {
		t.Fatalf("deduped status command = %#v, want configured command override", statusCommand)
	}
	if _, ok := findExtensionCommand(runner.ExtensionInventory(sess.ID).Commands, "custom"); !ok {
		t.Fatalf("custom configured command missing from inventory: %#v", runner.ExtensionInventory(sess.ID).Commands)
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

func TestExtensionInventoryIncludesLifecycleFieldsForAllSources(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(extensionProbeTool{name: "AlphaTool", source: "dynamic"}), Options{
		Commands: []tools.Command{{Name: "custom", Type: "slash", Source: "plugin", Version: "1.0.0"}},
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
	})

	inventory := runner.ExtensionInventory(sess.ID)
	tool, ok := findExtensionTool(inventory.Tools, "AlphaTool")
	if !ok || tool.Type != tools.ExtensionTypeTool || tool.LifecycleState != tools.ExtensionStateActive || !containsString(tool.Capabilities, "invoke") {
		t.Fatalf("tool lifecycle = %#v, found=%v", tool, ok)
	}
	command, ok := findExtensionCommand(inventory.Commands, "custom")
	if !ok || command.Type != "slash" || command.LifecycleType != tools.ExtensionTypeCommand || command.LifecycleState != tools.ExtensionStateActive || command.Version != "1.0.0" {
		t.Fatalf("command lifecycle = %#v, found=%v", command, ok)
	}
	server, ok := findMCPServer(inventory.MCPServers, "beta")
	if !ok || server.LifecycleType != tools.ExtensionTypeMCPServer || server.LifecycleState != tools.ExtensionStateActive || !containsString(server.Capabilities, "tools") {
		t.Fatalf("server lifecycle = %#v, found=%v", server, ok)
	}
	if inventory.LSPBoundaries[0].LifecycleState != tools.ExtensionStateDiscovered || inventory.LSPBoundaries[0].LifecycleType != tools.ExtensionTypeLSPBoundary {
		t.Fatalf("lsp lifecycle = %#v", inventory.LSPBoundaries[0])
	}
}

func TestExtensionLifecycleDisableEnableAndReloadBehavior(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(extensionProbeTool{name: "AlphaTool", source: "dynamic"}), Options{})
	target := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeTool, Source: "dynamic", Name: "AlphaTool"}

	if _, err := runner.DisableExtension(target); err != nil {
		t.Fatalf("disable extension: %v", err)
	}
	if tool, ok := findExtensionTool(runner.ExtensionInventory(sess.ID).Tools, "AlphaTool"); !ok || tool.LifecycleState != tools.ExtensionStateDisabled {
		t.Fatalf("disabled tool = %#v, found=%v", tool, ok)
	}
	if _, err := runner.EnableExtension(target); err != nil {
		t.Fatalf("enable extension: %v", err)
	}
	if tool, ok := findExtensionTool(runner.ExtensionInventory(sess.ID).Tools, "AlphaTool"); !ok || tool.LifecycleState != tools.ExtensionStateActive {
		t.Fatalf("enabled tool = %#v, found=%v", tool, ok)
	}
	result, err := runner.ReloadExtension(context.Background(), target)
	if err != nil {
		t.Fatalf("reload extension: %v", err)
	}
	if result.Record.State != tools.ExtensionStateReloaded {
		t.Fatalf("reload result = %#v, want reloaded state", result)
	}
}

func TestExtensionLifecycleDegradedFailedAndRestartRecovery(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	target := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeMCPServer, Source: "mcp", Name: "beta"}
	options := Options{
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
	}
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), options)

	if _, err := runner.MarkExtensionDegraded(target, "temporary auth warning"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}
	server, ok := findMCPServer(runner.ExtensionInventory(sess.ID).MCPServers, "beta")
	if !ok || server.LifecycleState != tools.ExtensionStateDegraded || server.LastError != "temporary auth warning" {
		t.Fatalf("degraded server = %#v, found=%v", server, ok)
	}
	if _, err := runner.MarkExtensionFailed(target, "connection refused"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	recovered := runner.ExtensionLifecycleRecords()
	restarted := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{
		MCPTools:           options.MCPTools,
		ExtensionLifecycle: recovered,
	})
	server, ok = findMCPServer(restarted.ExtensionInventory(sess.ID).MCPServers, "beta")
	if !ok || server.LifecycleState != tools.ExtensionStateFailed || server.LastError != "connection refused" {
		t.Fatalf("recovered failed server = %#v, found=%v", server, ok)
	}
}

func TestExtensionLifecycleUnsupportedSourceReturnsError(t *testing.T) {
	runner := NewRunnerWithOptions(session.NewManager(nil), llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{})
	_, err := runner.ReloadExtension(context.Background(), tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeLSPBoundary, Source: "lsp", Name: "language-server-protocol"})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("reload lsp error = %v, want explicit unsupported error", err)
	}
}

func TestExtensionLifecycleMetadataDoesNotBypassPermissionPolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(extensionProbeTool{name: "DeniedTool", source: "dynamic"}), Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "DeniedTool", Action: permissions.ActionDeny, Source: string(permissions.RuleSourceSession)},
			},
		},
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:         tools.ExtensionTypeTool,
			Source:       "dynamic",
			Name:         "DeniedTool",
			State:        tools.ExtensionStateActive,
			Capabilities: []string{"invoke"},
		}},
	})

	if _, ok := findExtensionTool(runner.ExtensionInventory(sess.ID).Tools, "DeniedTool"); ok {
		t.Fatalf("DeniedTool was exposed despite permission deny and advisory lifecycle metadata")
	}
}

func TestExtensionInventoryDedupeRegressionsForToolsCommandsAndSkills(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "shared")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: Shared Skill
description: Local shared skill.
---
# Shared
`), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(
		extensionProbeTool{name: "SameTool", source: "dynamic"},
		extensionProbeTool{name: "SameTool", source: "dynamic"},
	), Options{
		Commands: []tools.Command{
			{Name: "status", Type: "slash", Source: "plugin", Description: "plugin status"},
			{Name: "status", Type: "slash", Source: "dynamic", Description: "dynamic status"},
		},
		MCPSkills: map[string][]tools.SkillCommand{
			"beta": {{Name: "shared", DisplayName: "Shared Skill", Description: "remote duplicate", MCPServer: "beta"}},
		},
		SkillRoots: []string{skillRoot},
	})

	inventory := runner.ExtensionInventory(sess.ID)
	if countExtensionTools(inventory.Tools, "SameTool") != 1 {
		t.Fatalf("tools = %#v, want duplicate tool collapsed by registry projection", inventory.Tools)
	}
	if countExtensionCommands(inventory.Commands, "status") != 1 {
		t.Fatalf("commands = %#v, want duplicate command collapsed", inventory.Commands)
	}
	if countExtensionSkills(inventory.Skills, "shared") != 1 {
		t.Fatalf("skills = %#v, want duplicate local/MCP skill collapsed", inventory.Skills)
	}
}

func findExtensionCommand(commands []ExtensionCommand, name string) (ExtensionCommand, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return ExtensionCommand{}, false
}

func findExtensionTool(items []ExtensionTool, name string) (ExtensionTool, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return ExtensionTool{}, false
}

func findMCPServer(items []MCPServerSnapshot, name string) (MCPServerSnapshot, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return MCPServerSnapshot{}, false
}

func countExtensionTools(items []ExtensionTool, name string) int {
	count := 0
	for _, item := range items {
		if item.Name == name {
			count++
		}
	}
	return count
}

func countExtensionCommands(items []ExtensionCommand, name string) int {
	count := 0
	for _, item := range items {
		if item.Name == name {
			count++
		}
	}
	return count
}

func countExtensionSkills(items []ExtensionSkill, name string) int {
	count := 0
	for _, item := range items {
		if item.Name == name {
			count++
		}
	}
	return count
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
