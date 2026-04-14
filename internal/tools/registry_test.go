package tools_test

import (
	"context"
	"encoding/json"
	"strconv"
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

type structuredStubTool struct {
	stubTool
	checkInputs  *[]map[string]any
	invokeInputs *[]map[string]any
}

type observableBackfillStubTool struct {
	stubTool
}

type contextualPermissionStubTool struct {
	stubTool
	gotContext *tools.ToolUseContext
}

type contextualExecutionStubTool struct {
	stubTool
	gotContext *tools.ToolUseContext
}

type autoClassifierStubTool struct {
	stubTool
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

func (t structuredStubTool) CheckPermissionsWithInput(_ context.Context, _ session.Session, input map[string]any, _ permissions.Policy) (permissions.Decision, error) {
	if t.checkInputs != nil {
		*t.checkInputs = append(*t.checkInputs, cloneAnyMap(input))
	}
	updated := cloneAnyMap(input)
	updated["command"] = "checked"
	return permissions.Decision{Allowed: true, UpdatedInputObject: updated}, nil
}

func (t structuredStubTool) InvokeWithInput(_ context.Context, _ session.Session, input map[string]any) (string, error) {
	if t.invokeInputs != nil {
		*t.invokeInputs = append(*t.invokeInputs, cloneAnyMap(input))
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t observableBackfillStubTool) BackfillObservableInput(input map[string]any) {
	input["command"] = "overwritten"
	input["display_command"] = "run from display"
}

func (t contextualPermissionStubTool) CheckPermissionsWithContext(_ context.Context, toolCtx tools.ToolUseContext) (permissions.Decision, error) {
	if t.gotContext != nil {
		cloned := toolCtx
		cloned.InputObject = cloneAnyMap(toolCtx.InputObject)
		cloned.AvailableTools = append([]tools.Definition(nil), toolCtx.AvailableTools...)
		cloned.Messages = append([]session.Message(nil), toolCtx.Messages...)
		*t.gotContext = cloned
	}
	return permissions.Decision{Allowed: true}, nil
}

func (t contextualExecutionStubTool) InvokeWithContext(_ context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	if t.gotContext != nil {
		cloned := toolCtx
		cloned.InputObject = cloneAnyMap(toolCtx.InputObject)
		cloned.AvailableTools = append([]tools.Definition(nil), toolCtx.AvailableTools...)
		cloned.Messages = append([]session.Message(nil), toolCtx.Messages...)
		*t.gotContext = cloned
	}
	toolCtx.ReportProgress(tools.ToolProgress{ToolUseID: toolCtx.ToolUseID, Type: "tool_progress", Message: "executing"})
	return tools.ToolResult{
		Output: "contextual output",
		ContextModifier: func(next tools.ToolUseContext) tools.ToolUseContext {
			if next.AppState == nil {
				next.AppState = make(map[string]any)
			}
			next.AppState["executed"] = toolCtx.ToolName
			return next
		},
	}, nil
}

func (t autoClassifierStubTool) ToAutoClassifierInput(input string) any {
	return "classifier:" + input
}

func TestRegistryInvokeResolvesAliases(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "text.upper",
				Aliases:     []string{"uppercase"},
				Description: "Uppercase text.",
			},
			output:  "HELLO",
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

func TestRegistryCheckPermissionsWithContextPassesClaudeStyleToolUseContext(t *testing.T) {
	var got tools.ToolUseContext
	registry := tools.NewRegistry(
		contextualPermissionStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "contextual.tool", Description: "Uses context.", Enabled: true},
				enabled: true,
			},
			gotContext: &got,
		},
		stubTool{
			def:     tools.Definition{Name: "sibling.tool", Description: "Visible sibling.", Enabled: true},
			enabled: true,
		},
	)
	sess := session.NewManager(nil).GetOrCreateMain("main")
	history := []session.Message{{ID: "msg-1", Role: "user", Content: "hello"}}

	decision, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		Session:       sess,
		ToolName:      "contextual.tool",
		Input:         `{"command":"run","cwd":"/tmp"}`,
		InputObject:   map[string]any{"command": "run", "cwd": "/tmp"},
		Policy:        permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		AgentID:       "main",
		MainLoopModel: "sonnet",
		LLMProvider:   "anthropic",
		Messages:      history,
	})
	if err != nil {
		t.Fatalf("check permissions with context: %v", err)
	}
	if !checked || !decision.Allowed {
		t.Fatalf("decision = %#v checked=%v, want allowed checked decision", decision, checked)
	}
	if got.Session.ID != sess.ID || got.ToolName != "contextual.tool" || got.AgentID != "main" {
		t.Fatalf("tool context = %#v, want session/tool/agent metadata", got)
	}
	assertAnyMap(t, got.InputObject, map[string]any{"command": "run", "cwd": "/tmp"})
	if got.Policy.Mode != permissions.ModeWorkspaceWrite || got.MainLoopModel != "sonnet" || got.LLMProvider != "anthropic" {
		t.Fatalf("tool context = %#v, want policy/model/provider metadata", got)
	}
	if len(got.Messages) != 1 || got.Messages[0].ID != "msg-1" {
		t.Fatalf("tool context messages = %#v, want cloned history", got.Messages)
	}
	if len(got.AvailableTools) != 2 {
		t.Fatalf("available tools = %#v, want registry definitions in context", got.AvailableTools)
	}
}

func TestRegistryToolUseContextExposesAppStateDecisionsLimitsMCPAndCallbacks(t *testing.T) {
	var got tools.ToolUseContext
	var progress []tools.ToolProgress
	registry := tools.NewRegistry(
		contextualPermissionStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "contextual.tool", Description: "Uses context.", Enabled: true},
				enabled: true,
			},
			gotContext: &got,
		},
	)
	sess := session.NewManager(nil).GetOrCreateMain("main")
	appState := map[string]any{"mode": "ask"}

	_, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		Session:     sess,
		ToolName:    "contextual.tool",
		Input:       "input",
		Policy:      permissions.Policy{},
		AppState:    appState,
		SetAppState: func(update func(map[string]any) map[string]any) { appState = update(appState) },
		ToolDecisions: map[string]tools.ToolDecision{
			"toolu-1": {Source: "user", Decision: "accept", TimestampUnixMilli: 123},
		},
		FileReadingLimits: tools.ResourceLimits{MaxTokens: 100, MaxSizeBytes: 2048},
		GlobLimits:        tools.ResourceLimits{MaxResults: 50},
		MCPClients:        []tools.MCPConnection{{Name: "filesystem", Type: "stdio", BaseURL: "local"}},
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file:///tmp/a", Name: "a"}},
		},
		RequestPrompt: func(sourceName, summary string, request tools.PromptRequest) (tools.PromptResponse, error) {
			return tools.PromptResponse{Value: sourceName + ":" + summary + ":" + request.Message}, nil
		},
		ReportProgress: func(progressEvent tools.ToolProgress) {
			progress = append(progress, progressEvent)
		},
	})
	if err != nil {
		t.Fatalf("check permissions with context: %v", err)
	}
	if !checked {
		t.Fatal("checked = false, want contextual check to run")
	}

	got.SetAppState(func(previous map[string]any) map[string]any {
		next := make(map[string]any, len(previous)+1)
		for key, value := range previous {
			next[key] = value
		}
		next["mode"] = "updated"
		return next
	})
	if appState["mode"] != "updated" {
		t.Fatalf("appState = %#v, want SetAppState to update shared state", appState)
	}
	if got.ToolDecisions["toolu-1"].Decision != "accept" {
		t.Fatalf("tool decisions = %#v, want copied decision metadata", got.ToolDecisions)
	}
	if got.FileReadingLimits.MaxTokens != 100 || got.FileReadingLimits.MaxSizeBytes != 2048 || got.GlobLimits.MaxResults != 50 {
		t.Fatalf("limits = file %#v glob %#v, want Claude-style limits", got.FileReadingLimits, got.GlobLimits)
	}
	if len(got.MCPClients) != 1 || got.MCPClients[0].Name != "filesystem" {
		t.Fatalf("mcp clients = %#v, want MCP client metadata", got.MCPClients)
	}
	if len(got.MCPResources["filesystem"]) != 1 || got.MCPResources["filesystem"][0].URI != "file:///tmp/a" {
		t.Fatalf("mcp resources = %#v, want MCP resource metadata", got.MCPResources)
	}
	response, err := got.RequestPrompt("hook", "summary", tools.PromptRequest{Message: "question"})
	if err != nil {
		t.Fatalf("request prompt: %v", err)
	}
	if response.Value != "hook:summary:question" {
		t.Fatalf("prompt response = %#v, want callback response", response)
	}
	got.ReportProgress(tools.ToolProgress{ToolUseID: "toolu-1", Type: "hook_progress", Message: "checking"})
	if len(progress) != 1 || progress[0].Message != "checking" {
		t.Fatalf("progress = %#v, want callback progress event", progress)
	}
}

func TestRegistryInvokeWithContextPassesClaudeStyleExecutionContext(t *testing.T) {
	var got tools.ToolUseContext
	var progress []tools.ToolProgress
	registry := tools.NewRegistry(
		contextualExecutionStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "contextual.exec", Description: "Executes with context.", Enabled: true},
				enabled: true,
			},
			gotContext: &got,
		},
	)
	sess := session.NewManager(nil).GetOrCreateMain("main")

	result, err := registry.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:        sess,
		ToolName:       "contextual.exec",
		ToolUseID:      "toolu-exec",
		Input:          `{"command":"run"}`,
		InputObject:    map[string]any{"command": "run"},
		Policy:         permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		AppState:       map[string]any{"phase": "before"},
		ReportProgress: func(item tools.ToolProgress) { progress = append(progress, item) },
	})
	if err != nil {
		t.Fatalf("invoke with context: %v", err)
	}
	if result.Output != "contextual output" {
		t.Fatalf("output = %q, want contextual output", result.Output)
	}
	if result.ContextModifier == nil {
		t.Fatal("context modifier missing")
	}
	if got.ToolName != "contextual.exec" || got.ToolUseID != "toolu-exec" || got.Session.ID != sess.ID {
		t.Fatalf("tool context = %#v, want execution metadata", got)
	}
	assertAnyMap(t, got.InputObject, map[string]any{"command": "run"})
	if len(progress) != 1 || progress[0].ToolUseID != "toolu-exec" || progress[0].Message != "executing" {
		t.Fatalf("progress = %#v, want execution progress callback", progress)
	}
	modified := result.ContextModifier(got)
	if modified.AppState["executed"] != "contextual.exec" {
		t.Fatalf("modified context app state = %#v, want execution marker", modified.AppState)
	}
}

func TestRegistryAutoClassifierInputUsesToolSpecificProjection(t *testing.T) {
	registry := tools.NewRegistry(
		autoClassifierStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true},
				enabled: true,
			},
		},
	)

	got, ok := registry.AutoClassifierInput("system.run", "cat README.md")
	if !ok {
		t.Fatal("expected tool-specific classifier input")
	}
	if got != "classifier:cat README.md" {
		t.Fatalf("classifier input = %#v, want projected command", got)
	}
}

func TestRegistryBackfillObservableInputOnlyReturnsAddedFieldsOnCopy(t *testing.T) {
	registry := tools.NewRegistry(
		observableBackfillStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input."},
				enabled: true,
			},
		},
	)
	input := map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	}

	got := registry.BackfillObservableInput("structured.echo", input)

	if input["command"] != "original" {
		t.Fatalf("input = %#v, want original input not mutated", input)
	}
	if got["command"] != "original" {
		t.Fatalf("backfilled input = %#v, want overwrite ignored", got)
	}
	if got["display_command"] != "run from display" {
		t.Fatalf("backfilled input = %#v, want added display field", got)
	}
}

func TestRegistryStructuredInputAPIsReceiveParsedJSONObject(t *testing.T) {
	var checkInputs []map[string]any
	var invokeInputs []map[string]any
	registry := tools.NewRegistry(
		structuredStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input."},
				enabled: true,
			},
			checkInputs:  &checkInputs,
			invokeInputs: &invokeInputs,
		},
	)
	sess := session.NewManager(nil).GetOrCreateMain("main")

	decision, checked, err := registry.CheckPermissions(context.Background(), sess, "structured.echo", `{"command":"original","cwd":"/tmp"}`, permissions.Policy{})
	if err != nil {
		t.Fatalf("check permissions: %v", err)
	}
	if !checked {
		t.Fatal("checked = false, want structured check to run")
	}
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allow", decision)
	}
	updated, ok, err := decision.UpdatedInputValue()
	if err != nil {
		t.Fatalf("updated input value: %v", err)
	}
	if !ok {
		t.Fatal("updated input missing")
	}
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "structured.echo", updated, permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke with policy: %v", err)
	}

	assertAnyMap(t, checkInputs[0], map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	})
	if len(invokeInputs) != 1 {
		t.Fatalf("invoke inputs = %#v, want one structured invocation", invokeInputs)
	}
	assertAnyMap(t, invokeInputs[0], map[string]any{
		"command": "checked",
		"cwd":     "/tmp",
	})
	assertJSONInput(t, got, map[string]any{
		"command": "checked",
		"cwd":     "/tmp",
	})
}

func TestRegistryStructuredInputAPIsFallBackForPlainStringInput(t *testing.T) {
	var checkInputs []map[string]any
	var invokeInputs []map[string]any
	registry := tools.NewRegistry(
		structuredStubTool{
			stubTool: stubTool{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input."},
				output:  "plain-output",
				enabled: true,
			},
			checkInputs:  &checkInputs,
			invokeInputs: &invokeInputs,
		},
	)
	sess := session.NewManager(nil).GetOrCreateMain("main")

	_, checked, err := registry.CheckPermissions(context.Background(), sess, "structured.echo", "plain input", permissions.Policy{})
	if err != nil {
		t.Fatalf("check permissions: %v", err)
	}
	if checked {
		t.Fatal("checked = true, want non-JSON input to skip structured-only permission check")
	}
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "structured.echo", "plain input", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke with policy: %v", err)
	}
	if got != "plain-output" {
		t.Fatalf("output = %q, want plain-output fallback", got)
	}
	if len(checkInputs) != 0 || len(invokeInputs) != 0 {
		t.Fatalf("structured calls = check %#v invoke %#v, want none for plain input", checkInputs, invokeInputs)
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

func TestRegistryExposeRespectsServerPrefixBlanketDenyRules(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "mcp__filesystem__read_resource",
				Description: "Read MCP resource.",
			},
			enabled: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "mcp__filesystem__list_resources",
				Description: "List MCP resources.",
			},
			enabled: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "system.run",
				Description: "Run command.",
			},
			enabled: true,
		},
	)

	defs := registry.Expose(tools.ExposeOptions{
		Policy: permissions.Policy{
			Rules: []permissions.Rule{
				{ToolName: "mcp__filesystem", Action: permissions.ActionDeny},
			},
		},
	})

	if len(defs) != 1 {
		t.Fatalf("exposed defs = %#v, want only non-MCP tool to remain", defs)
	}
	if defs[0].Name != "system.run" {
		t.Fatalf("exposed defs = %#v, want only system.run", defs)
	}
}

func TestRegistryExposeRespectsServerWildcardBlanketDenyRules(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "mcp__filesystem__read_resource",
				Description: "Read MCP resource.",
			},
			enabled: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "mcp__filesystem__list_resources",
				Description: "List MCP resources.",
			},
			enabled: true,
		},
		stubTool{
			def: tools.Definition{
				Name:        "system.run",
				Description: "Run command.",
			},
			enabled: true,
		},
	)

	defs := registry.Expose(tools.ExposeOptions{
		Policy: permissions.Policy{
			Rules: []permissions.Rule{
				{ToolName: "mcp__filesystem__*", Action: permissions.ActionDeny},
			},
		},
	})

	if len(defs) != 1 {
		t.Fatalf("exposed defs = %#v, want only non-MCP tool to remain", defs)
	}
	if defs[0].Name != "system.run" {
		t.Fatalf("exposed defs = %#v, want only system.run", defs)
	}
}

func TestRegistryInvokeWithPolicyRejectsBlanketDeniedTool(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "mcp__filesystem__read_resource",
				Description: "Read MCP resource.",
			},
			enabled: true,
			output:  "secret",
		},
	)

	_, err := registry.InvokeWithPolicy(
		context.Background(),
		session.NewManager(nil).GetOrCreateMain("main"),
		"mcp__filesystem__read_resource",
		"read secret",
		permissions.Policy{
			Rules: []permissions.Rule{
				{ToolName: "mcp__filesystem", Action: permissions.ActionDeny},
			},
		},
	)
	if err == nil {
		t.Fatal("expected blanket-denied tool invocation to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
	}
}

func TestRegistryInvokeWithPolicyRejectsDisabledTool(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def: tools.Definition{
				Name:        "hidden.tool",
				Description: "Disabled tool.",
			},
			enabled: false,
			output:  "hidden",
		},
	)

	_, err := registry.InvokeWithPolicy(
		context.Background(),
		session.NewManager(nil).GetOrCreateMain("main"),
		"hidden.tool",
		"use hidden",
		permissions.Policy{},
	)
	if err == nil {
		t.Fatal("expected disabled tool invocation to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
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

func TestToolSearchToolRespectsDenyRulesDuringInvocation(t *testing.T) {
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
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "delegate", permissions.Policy{
		Rules: []permissions.Rule{
			{ToolName: "agent.task", Action: permissions.ActionDeny},
		},
	})
	if err != nil {
		t.Fatalf("invoke tool.search with policy: %v", err)
	}
	if strings.Contains(got, "agent.task") {
		t.Fatalf("tool.search output = %q, did not want denied tool in search results", got)
	}
	if got != "No matching tools found." {
		t.Fatalf("tool.search output = %q, want no matches after deny filtering", got)
	}
}

func TestToolSearchToolOnlyReturnsDeferredTools(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "delegate subtask",
		},
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "delegate", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with policy: %v", err)
	}
	if strings.Contains(got, "system.run") {
		t.Fatalf("tool.search output = %q, did not want non-deferred tool in search results", got)
	}
	if !strings.Contains(got, "agent.task") {
		t.Fatalf("tool.search output = %q, want deferred tool in search results", got)
	}
}

func TestToolSearchToolSupportsDirectSelectionByToolName(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "select:agent.task", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with direct selection: %v", err)
	}
	if got == "No matching tools found." {
		t.Fatalf("tool.search output = %q, did not want no-match result for direct selection", got)
	}
	if !strings.Contains(got, "agent.task") {
		t.Fatalf("tool.search output = %q, want selected deferred tool in search results", got)
	}
}

func TestToolSearchToolSupportsDirectMultiSelectAndLoadedToolFallback(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "shell command",
		},
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "select:agent.task,system.run", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with multi-select: %v", err)
	}
	if !strings.Contains(got, "agent.task") {
		t.Fatalf("tool.search output = %q, want deferred selected tool", got)
	}
	if !strings.Contains(got, "system.run") {
		t.Fatalf("tool.search output = %q, want loaded-tool fallback selection", got)
	}
}

func TestToolSearchToolSupportsBareExactNameSelection(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "agent.task", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with bare exact tool name: %v", err)
	}
	if got == "No matching tools found." {
		t.Fatalf("tool.search output = %q, did not want no-match result for bare exact name", got)
	}
	if !strings.Contains(got, "agent.task") {
		t.Fatalf("tool.search output = %q, want exact-name deferred tool", got)
	}
}

func TestToolSearchToolSupportsMcpPrefixSelection(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "mcp__filesystem__read_resource", Description: "Read MCP resource."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "read file",
		},
		stubTool{
			def:         tools.Definition{Name: "mcp__filesystem__list_resources", Description: "List MCP resources."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "list files",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "mcp__filesystem", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with MCP prefix: %v", err)
	}
	if !strings.Contains(got, "mcp__filesystem__read_resource") || !strings.Contains(got, "mcp__filesystem__list_resources") {
		t.Fatalf("tool.search output = %q, want MCP prefix matches", got)
	}
}

func TestToolSearchToolSupportsBareExactNameLoadedToolFallback(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "shell command",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "system.run", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with bare loaded tool name: %v", err)
	}
	if got == "No matching tools found." {
		t.Fatalf("tool.search output = %q, did not want no-match result for loaded-tool exact name", got)
	}
	if !strings.Contains(got, "system.run") {
		t.Fatalf("tool.search output = %q, want loaded-tool exact-name fallback", got)
	}
}

func TestToolSearchToolLimitsKeywordResultsToDefaultFive(t *testing.T) {
	registry := tools.NewRegistry()
	for i := 1; i <= 6; i++ {
		registry.Register(
			stubTool{
				def:         tools.Definition{Name: "agent.task." + strconv.Itoa(i), Description: "Run a subagent task."},
				enabled:     true,
				shouldDefer: true,
				searchHint:  "delegate subtask",
			},
		)
	}
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "delegate", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with default keyword search: %v", err)
	}
	if strings.Contains(got, "agent.task.6") {
		t.Fatalf("tool.search output = %q, did not want more than 5 default matches", got)
	}
	for i := 1; i <= 5; i++ {
		if !strings.Contains(got, "agent.task."+strconv.Itoa(i)) {
			t.Fatalf("tool.search output = %q, want default top-five match agent.task.%d", got, i)
		}
	}
}

func TestToolSearchToolSupportsRequiredTerms(t *testing.T) {
	registry := tools.NewRegistry(
		stubTool{
			def:         tools.Definition{Name: "mcp__slack__send_message", Description: "Send a Slack message."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "slack send message",
		},
		stubTool{
			def:         tools.Definition{Name: "mcp__email__send_message", Description: "Send an email."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "email send message",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	sess := session.NewManager(nil).GetOrCreateMain("main")
	got, err := registry.InvokeWithPolicy(context.Background(), sess, "tool.search", "+slack send", permissions.Policy{})
	if err != nil {
		t.Fatalf("invoke tool.search with required terms: %v", err)
	}
	if !strings.Contains(got, "mcp__slack__send_message") {
		t.Fatalf("tool.search output = %q, want Slack match", got)
	}
	if strings.Contains(got, "mcp__email__send_message") {
		t.Fatalf("tool.search output = %q, did not want non-required match", got)
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

func TestRegistryExposeHidesToolSearchWhenDisabledByEnv(t *testing.T) {
	t.Setenv("ENABLE_TOOL_SEARCH", "false")

	registry := tools.NewRegistry()
	registry.Register(tools.NewToolSearchTool(registry))

	defs := registry.Expose(tools.ExposeOptions{})
	if len(defs) != 0 {
		t.Fatalf("exposed defs = %#v, want tool.search hidden when env disables tool search", defs)
	}
}

func TestToolSearchToolEnableModeSemanticsFromEnv(t *testing.T) {
	cases := []struct {
		name        string
		enableValue string
		disableBeta string
		want        bool
	}{
		{name: "default enabled", want: true},
		{name: "false disables", enableValue: "false", want: false},
		{name: "auto enables", enableValue: "auto", want: true},
		{name: "auto zero enables", enableValue: "auto:0", want: true},
		{name: "auto hundred disables", enableValue: "auto:100", want: false},
		{name: "beta kill switch disables", disableBeta: "true", want: false},
		{name: "beta kill switch wins over auto", enableValue: "auto", disableBeta: "true", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENABLE_TOOL_SEARCH", tc.enableValue)
			t.Setenv("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS", tc.disableBeta)

			tool := tools.NewToolSearchTool(tools.NewRegistry())
			if got := tool.IsEnabled(); got != tc.want {
				t.Fatalf("IsEnabled() = %v, want %v", got, tc.want)
			}
		})
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

func assertJSONInput(t *testing.T, got string, want map[string]any) {
	t.Helper()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("input = %q, want JSON object: %v", got, err)
	}
	assertAnyMap(t, parsed, want)
}

func assertAnyMap(t *testing.T, got, want map[string]any) {
	t.Helper()

	if got == nil {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
	if len(got) != len(want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("input[%q] = %#v, want %#v in %#v", key, got[key], wantValue, got)
		}
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
