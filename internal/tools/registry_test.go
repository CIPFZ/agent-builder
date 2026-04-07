package tools_test

import (
	"context"
	"strings"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/tools/system"
)

type stubTool struct {
	def         tools.Definition
	output      string
	enabled     bool
	readOnly    bool
	destructive bool
	shouldDefer bool
	alwaysLoad  bool
	promptText  string
	searchHint  string
}

func (t stubTool) Definition() tools.Definition {
	return t.def
}

func (t stubTool) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	return t.output, nil
}

func (t stubTool) IsEnabled() bool {
	return t.enabled
}

func (t stubTool) IsReadOnly(_ string) bool {
	return t.readOnly
}

func (t stubTool) IsDestructive(_ string) bool {
	return t.destructive
}

func (t stubTool) ShouldDefer() bool {
	return t.shouldDefer
}

func (t stubTool) AlwaysLoad() bool {
	return t.alwaysLoad
}

func (t stubTool) PromptDescription() string {
	return t.promptText
}

func (t stubTool) SearchHint() string {
	return t.searchHint
}

func TestRegistryInvokeResolvesAliases(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "text.upper",
				Aliases:     []string{"uppercase"},
				Description: "Uppercase text.",
			},
			output: "HELLO",
			enabled: true,
		},
	)

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.Invoke(context.Background(), sess, "uppercase", "hello")
	if err != nil {
		t.Fatalf("invoke via alias: %v", err)
	}
	if got != "HELLO" {
		t.Fatalf("output = %q, want HELLO", got)
	}
}

func TestRegistryAssembleFiltersDisabledAndBlanketDeniedTools(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "text.upper",
				Description: "Uppercase text.",
			},
			enabled: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "system.run",
				Description: "Run a shell command.",
			},
			enabled:     true,
			destructive: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "hidden.tool",
				Description: "Disabled tool.",
			},
			enabled: false,
		},
	)

	defs := registry.Assemble(tools.AssembleOptions{
		Policy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "text.upper", Action: permissions.ActionDeny},
			},
		},
	})

	if len(defs) != 1 {
		t.Fatalf("assembled len = %d, want 1", len(defs))
	}
	if defs[0].Name != "system.run" {
		t.Fatalf("assembled defs = %#v, want only system.run", defs)
	}
	if !defs[0].Destructive {
		t.Fatalf("assembled defs = %#v, want destructive metadata from tool behavior", defs)
	}
}

func TestRegistryDefinitionsRemainStableAndOrdered(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{def: tools.Definition{Name: "z.tool", Description: "z"}, enabled: true},
		stubTool{def: tools.Definition{Name: "a.tool", Description: "a"}, enabled: true},
	)

	defs := registry.Definitions()
	if len(defs) != 2 {
		t.Fatalf("definitions len = %d, want 2", len(defs))
	}
	if defs[0].Name != "z.tool" || defs[1].Name != "a.tool" {
		t.Fatalf("definitions = %#v, want insertion order preserved", defs)
	}
}

func TestRegistryDefinitionsUseBehaviorDrivenPromptMetadata(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:        tools.Definition{Name: "agent.task", Description: "static description"},
			enabled:    true,
			promptText: "Run a subagent task with delegated ownership.",
			searchHint: "delegate subtask",
		},
	)

	defs := registry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("definitions len = %d, want 1", len(defs))
	}
	if defs[0].Description != "Run a subagent task with delegated ownership." {
		t.Fatalf("definition = %#v, want prompt description from tool behavior", defs[0])
	}
	if defs[0].SearchHint != "delegate subtask" {
		t.Fatalf("definition = %#v, want search hint from tool behavior", defs[0])
	}
}

func TestRegistryExposeHidesDeferredToolsByDefault(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "tool.search", Description: "Search for tools."},
			enabled:     true,
			shouldDefer: true,
		},
		stubTool{
			def:     tools.Definition{Name: "system.run", Description: "Run command."},
			enabled: true,
		},
	)

	defs := registry.Expose(tools.ExposeOptions{})
	if len(defs) != 1 {
		t.Fatalf("exposed len = %d, want 1", len(defs))
	}
	if defs[0].Name != "system.run" {
		t.Fatalf("exposed defs = %#v, want only non-deferred tool", defs)
	}
}

func TestRegistryExposeIncludesDeferredToolsWhenRequested(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "tool.search", Description: "Search for tools."},
			enabled:     true,
			shouldDefer: true,
		},
	)

	defs := registry.Expose(tools.ExposeOptions{IncludeDeferred: true})
	if len(defs) != 1 {
		t.Fatalf("exposed len = %d, want 1", len(defs))
	}
	if !defs[0].ShouldDefer {
		t.Fatalf("exposed defs = %#v, want deferred metadata preserved", defs)
	}
}

func TestRegistryExposeIncludesAlwaysLoadDeferredTools(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			alwaysLoad:  true,
		},
	)

	defs := registry.Expose(tools.ExposeOptions{})
	if len(defs) != 1 {
		t.Fatalf("exposed len = %d, want 1", len(defs))
	}
	if !defs[0].AlwaysLoad {
		t.Fatalf("exposed defs = %#v, want always-load metadata preserved", defs)
	}
}

func TestRegistrySearchFindsDeferredToolBySearchHint(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)

	results := registry.Search("delegate", tools.SearchOptions{})
	if len(results) != 1 {
		t.Fatalf("search results len = %d, want 1", len(results))
	}
	if results[0].Name != "agent.task" {
		t.Fatalf("search results = %#v, want deferred tool by search hint", results)
	}
}

func TestRegistrySearchRespectsBlanketDenyRules(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)

	results := registry.Search("delegate", tools.SearchOptions{
		Policy: permissions.Policy{
			Rules: []permissions.Rule{
				{ToolName: "agent.task", Action: permissions.ActionDeny},
			},
		},
	})
	if len(results) != 0 {
		t.Fatalf("search results = %#v, want denied deferred tool hidden", results)
	}
}

func TestRegistryInspectUsesDynamicToolMetadata(t *testing.T) {
	registry := tools.NewRegistry(
		tools.NewTextUpperTool(),
		system.NewRunTool(nil),
	)

	textDef, ok := registry.Inspect("text.upper", "hello")
	if !ok {
		t.Fatal("expected text.upper to be inspectable")
	}
	if !textDef.ReadOnly || textDef.Destructive {
		t.Fatalf("text.upper inspect = %#v, want read-only non-destructive", textDef)
	}

	runDef, ok := registry.Inspect("system.run", "pwd")
	if !ok {
		t.Fatal("expected system.run to be inspectable")
	}
	if runDef.ReadOnly || !runDef.Destructive {
		t.Fatalf("system.run inspect = %#v, want destructive non-read-only", runDef)
	}
}

func TestRegistrySearchFindsAliasAndPromptDescription(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "system.run",
				Aliases:     []string{"bash"},
				Description: "static",
			},
			enabled:    true,
			promptText: "Run a shell command on the host system.",
		},
	)

	aliasResults := registry.Search("bash", tools.SearchOptions{})
	if len(aliasResults) != 1 || aliasResults[0].Name != "system.run" {
		t.Fatalf("alias search results = %#v, want system.run", aliasResults)
	}

	promptResults := registry.Search("shell command", tools.SearchOptions{})
	if len(promptResults) != 1 || promptResults[0].Name != "system.run" {
		t.Fatalf("prompt search results = %#v, want system.run", promptResults)
	}
}

func TestToolSearchToolReturnsDeferredMatches(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.Invoke(context.Background(), sess, "tool.search", "delegate")
	if err != nil {
		t.Fatalf("invoke tool.search: %v", err)
	}
	if got == "" || !containsAll(got, []string{"agent.task", "delegate subtask", "deferred"}) {
		t.Fatalf("tool.search output = %q, want deferred match summary", got)
	}
}

func TestRegistryExposeIncludesToolSearchWhileHidingOtherDeferredTools(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	defs := registry.Expose(tools.ExposeOptions{})
	if len(defs) != 1 {
		t.Fatalf("exposed defs = %#v, want only tool.search visible by default", defs)
	}
	if defs[0].Name != "tool.search" {
		t.Fatalf("exposed defs = %#v, want tool.search", defs)
	}
}

func TestRegistryExposeFiltersDeniedToolsToo(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			alwaysLoad:  true,
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	defs := registry.Expose(tools.ExposeOptions{
		Policy: permissions.Policy{
			Rules: []permissions.Rule{
				{ToolName: "agent.task", Action: permissions.ActionDeny},
			},
		},
	})
	if len(defs) != 1 || defs[0].Name != "tool.search" {
		t.Fatalf("exposed defs = %#v, want only non-denied tool.search", defs)
	}
}

func containsAll(input string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(input, part) {
			return false
		}
	}
	return true
}
