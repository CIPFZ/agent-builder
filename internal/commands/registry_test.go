package commands

import "testing"

func TestDefaultRegistryRegistersP0CommandsWithAliasesAndVisibility(t *testing.T) {
	registry := NewDefaultRegistry()
	commands := registry.List(Context{PermissionMode: "ask", HasMemory: true, HasResumableSessions: true, HasMCP: true, HasTasks: true})

	for _, name := range []string{"help", "permissions", "model", "memory", "resume", "compact", "tasks", "mcp", "status"} {
		if _, ok := findCommand(commands, name); !ok {
			t.Fatalf("default registry missing command %q in %#v", name, commands)
		}
	}
	if cmd, ok := registry.Resolve("continue"); !ok || cmd.Name != "resume" {
		t.Fatalf("continue alias resolved to %#v/%v, want resume", cmd, ok)
	}
	if cmd, ok := registry.Resolve("agents"); !ok || cmd.Name != "tasks" {
		t.Fatalf("agents alias resolved to %#v/%v, want tasks", cmd, ok)
	}

	commands = registry.List(Context{PermissionMode: "ask"})
	for _, hidden := range []string{"memory", "resume", "tasks", "mcp"} {
		if _, ok := findCommand(commands, hidden); ok {
			t.Fatalf("command %q should be hidden without runtime state: %#v", hidden, commands)
		}
	}
}

func TestDefaultRegistryExecutesImmediateAndModelContinuationResults(t *testing.T) {
	registry := NewDefaultRegistry()

	result, err := registry.Execute(Context{PermissionMode: "ask"}, "/permissions")
	if err != nil {
		t.Fatalf("execute permissions: %v", err)
	}
	if result.CommandName != "permissions" || result.ShouldQuery || result.Output == "" {
		t.Fatalf("permissions result = %#v, want immediate output", result)
	}

	result, err = registry.Execute(Context{}, "/status include runtime state")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if result.CommandName != "status" || !result.ShouldQuery || result.NormalizedInput != "include runtime state" {
		t.Fatalf("status result = %#v, want model continuation with args", result)
	}
}

func TestRegistryRejectsUnknownCommands(t *testing.T) {
	registry := NewDefaultRegistry()
	if _, err := registry.Execute(Context{}, "/does-not-exist"); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func findCommand(commands []Metadata, name string) (Metadata, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return Metadata{}, false
}
