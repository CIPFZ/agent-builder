package queryengine

import (
	"context"
	"strings"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/session"
	storememory "myclaw/internal/store/memory"
	"myclaw/internal/tools"
)

func TestDisabledRuntimeCommandCannotExecuteThroughDefaultInputProcessor(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:   tools.ExtensionTypeCommand,
			Source: "runtime",
			Name:   "status",
			State:  tools.ExtensionStateDisabled,
		}},
	})

	command, ok := findQueryEngineLifecycleCommand(engine.ExtensionInventory(sess.ID).Commands, "status")
	if !ok || command.LifecycleState != tools.ExtensionStateDisabled {
		t.Fatalf("status inventory command = %#v, found=%v, want disabled", command, ok)
	}

	err := engine.SubmitPrompt(context.Background(), sess, "/status", &lifecycleCaptureSink{})
	if err == nil || !strings.Contains(err.Error(), "extension lifecycle state disabled") || !strings.Contains(err.Error(), "/status") {
		t.Fatalf("disabled /status error = %v, want explicit lifecycle disabled error", err)
	}
}

func TestDisabledConfiguredCommandOverrideCannotExecuteThroughDefaultInputProcessor(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		Commands: []tools.Command{{
			Name:        "status",
			Type:        "slash",
			Source:      "plugin",
			Description: "plugin status override",
		}},
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:   tools.ExtensionTypeCommand,
			Source: "plugin",
			Name:   "status",
			State:  tools.ExtensionStateDisabled,
		}},
	})

	var statusCount int
	var statusCommand ExtensionCommand
	for _, command := range engine.ExtensionInventory(sess.ID).Commands {
		if command.Name == "status" {
			statusCount++
			statusCommand = command
		}
	}
	if statusCount != 1 || statusCommand.Source != "plugin" || statusCommand.LifecycleState != tools.ExtensionStateDisabled {
		t.Fatalf("status command count/state = %d/%#v, want one disabled plugin override", statusCount, statusCommand)
	}

	err := engine.SubmitPrompt(context.Background(), sess, "/status", &lifecycleCaptureSink{})
	if err == nil || !strings.Contains(err.Error(), "extension lifecycle state disabled") || !strings.Contains(err.Error(), "/status") {
		t.Fatalf("disabled configured /status error = %v, want explicit lifecycle disabled error", err)
	}
}

func TestLifecycleOverlayPersistsThroughSessionStoreRecoveryAndEnableClears(t *testing.T) {
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
	})
	target := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeCommand, Source: "runtime", Name: "status"}
	if _, err := engine.DisableExtension(target); err != nil {
		t.Fatalf("disable status: %v", err)
	}

	recoveredManager := session.NewManager(store)
	recoveredSession, ok := recoveredManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("recovered session missing")
	}
	recovered := New(Config{
		Sessions: recoveredManager,
		Client:   llm.NewMockClient(),
	})
	command, ok := findQueryEngineLifecycleCommand(recovered.ExtensionInventory(recoveredSession.ID).Commands, "status")
	if !ok || command.LifecycleState != tools.ExtensionStateDisabled {
		t.Fatalf("recovered status command = %#v, found=%v, want disabled", command, ok)
	}
	err := recovered.SubmitPrompt(context.Background(), recoveredSession, "/status", &lifecycleCaptureSink{})
	if err == nil || !strings.Contains(err.Error(), "extension lifecycle state disabled") {
		t.Fatalf("recovered disabled status err = %v, want disabled lifecycle error", err)
	}

	if _, err := recovered.EnableExtension(target); err != nil {
		t.Fatalf("enable status: %v", err)
	}
	enabledManager := session.NewManager(store)
	enabledSession, ok := enabledManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("enabled session missing")
	}
	enabled := New(Config{
		Sessions: enabledManager,
		Client:   llm.NewMockClient(),
	})
	command, ok = findQueryEngineLifecycleCommand(enabled.ExtensionInventory(enabledSession.ID).Commands, "status")
	if !ok || command.LifecycleState == tools.ExtensionStateDisabled {
		t.Fatalf("enabled recovered status command = %#v, found=%v, want not disabled", command, ok)
	}
	err = enabled.SubmitPrompt(context.Background(), enabledSession, "/status", &lifecycleCaptureSink{})
	if err != nil {
		t.Fatalf("enabled /status returned error: %v", err)
	}
}

func TestFailedAndDegradedLifecycleOverlayPersistsThroughSessionStoreRecovery(t *testing.T) {
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	engine := New(Config{
		Sessions: manager,
		Client:   llm.NewMockClient(),
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
	})
	degraded := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeMCPServer, Source: "mcp", Name: "beta"}
	if _, err := engine.MarkExtensionDegraded(degraded, "auth warning"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}
	failed := tools.ExtensionLifecycleRecord{Type: tools.ExtensionTypeMCPServer, Source: "mcp", Name: "gamma"}
	if _, err := engine.MarkExtensionFailed(failed, "connection refused"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	recoveredManager := session.NewManager(store)
	recoveredSession, ok := recoveredManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("recovered session missing")
	}
	recovered := New(Config{
		Sessions: recoveredManager,
		Client:   llm.NewMockClient(),
		MCPTools: map[string]tools.MCPToolsListResult{
			"beta": {Tools: []tools.MCPToolListItem{{Name: "remote", Description: "remote tool"}}},
		},
	})
	servers := recovered.ExtensionInventory(recoveredSession.ID).MCPServers
	beta, ok := findQueryEngineLifecycleMCPServer(servers, "beta")
	if !ok || beta.LifecycleState != tools.ExtensionStateDegraded || beta.LastError != "auth warning" {
		t.Fatalf("recovered beta = %#v, found=%v, want degraded auth warning", beta, ok)
	}
	gamma, ok := findQueryEngineLifecycleMCPServer(servers, "gamma")
	if !ok || gamma.LifecycleState != tools.ExtensionStateFailed || gamma.LastError != "connection refused" {
		t.Fatalf("recovered gamma = %#v, found=%v, want failed connection refused", gamma, ok)
	}
}

func TestToolLifecycleDisabledBlocksRuntimeExecutionBoundary(t *testing.T) {
	engine := New(Config{
		Client: llm.NewMockClient(),
		ToolRegistry: tools.NewRegistry(queryengineLifecycleProbeTool{
			name:   "DeniedLifecycleTool",
			source: "dynamic",
		}),
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:   tools.ExtensionTypeTool,
			Source: "dynamic",
			Name:   "DeniedLifecycleTool",
			State:  tools.ExtensionStateDisabled,
		}},
	})

	def, ok := engine.tools.InspectWithPolicy("DeniedLifecycleTool", "", engine.PermissionPolicyForSession(""))
	if !ok {
		t.Fatalf("tool missing from registry")
	}
	if !engine.toolLifecycleDisabled(def) {
		t.Fatalf("tool lifecycle disabled boundary returned false")
	}
}

type lifecycleCaptureSink struct{}

func (lifecycleCaptureSink) Emit(Event) error { return nil }

func findQueryEngineLifecycleCommand(commands []ExtensionCommand, name string) (ExtensionCommand, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return ExtensionCommand{}, false
}

func findQueryEngineLifecycleMCPServer(servers []MCPServerSnapshot, name string) (MCPServerSnapshot, bool) {
	for _, server := range servers {
		if server.Name == name {
			return server, true
		}
	}
	return MCPServerSnapshot{}, false
}

type queryengineLifecycleProbeTool struct {
	name   string
	source string
}

func (t queryengineLifecycleProbeTool) Definition() tools.Definition {
	return tools.Definition{Name: t.name, Source: t.source}
}

func (t queryengineLifecycleProbeTool) Invoke(context.Context, session.Session, string) (string, error) {
	return "ok", nil
}

func (t queryengineLifecycleProbeTool) IsEnabled() bool           { return true }
func (t queryengineLifecycleProbeTool) IsReadOnly(string) bool    { return true }
func (t queryengineLifecycleProbeTool) IsDestructive(string) bool { return false }
func (t queryengineLifecycleProbeTool) ShouldDefer() bool         { return false }
func (t queryengineLifecycleProbeTool) AlwaysLoad() bool          { return false }
func (t queryengineLifecycleProbeTool) PromptDescription() string { return "" }
func (t queryengineLifecycleProbeTool) SearchHint() string        { return "" }
