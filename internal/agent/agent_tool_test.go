package agent

import (
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/config"
)

func TestResolveAgentToolRoleKeepsPromptOnlyDefault(t *testing.T) {
	agents := map[string]config.Agent{
		config.AgentTask: {ID: config.AgentTask, AllowedTools: []string{"glob"}},
		"review":         {ID: "review", AllowedTools: []string{"grep"}},
	}

	role, agentCfg, err := resolveAgentToolRole(AgentParams{Prompt: "inspect"}, agents)
	if err != nil {
		t.Fatal(err)
	}
	if role != config.AgentTask || agentCfg.ID != config.AgentTask {
		t.Fatalf("role=%q agent=%#v, want default task", role, agentCfg)
	}
}

func TestResolveAgentToolRoleUsesRoleBeforeSubagentType(t *testing.T) {
	agents := map[string]config.Agent{
		config.AgentTask: {ID: config.AgentTask},
		"review":         {ID: "review"},
		"search":         {ID: "search"},
	}

	role, agentCfg, err := resolveAgentToolRole(AgentParams{Role: "review", SubagentType: "search", Prompt: "inspect"}, agents)
	if err != nil {
		t.Fatal(err)
	}
	if role != "review" || agentCfg.ID != "review" {
		t.Fatalf("role=%q agent=%#v, want review", role, agentCfg)
	}
}

func TestResolveAgentToolRoleUsesSubagentTypeAlias(t *testing.T) {
	agents := map[string]config.Agent{
		config.AgentTask: {ID: config.AgentTask},
		"search":         {ID: "search"},
	}

	role, agentCfg, err := resolveAgentToolRole(AgentParams{SubagentType: "search", Prompt: "inspect"}, agents)
	if err != nil {
		t.Fatal(err)
	}
	if role != "search" || agentCfg.ID != "search" {
		t.Fatalf("role=%q agent=%#v, want search", role, agentCfg)
	}
}

func TestResolveAgentToolRoleUnknownRoleListsAvailableRoles(t *testing.T) {
	agents := map[string]config.Agent{
		config.AgentTask: {ID: config.AgentTask},
		"review":         {ID: "review"},
	}

	_, _, err := resolveAgentToolRole(AgentParams{Role: "missing", Prompt: "inspect"}, agents)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{`unknown subagent role "missing"`, config.AgentTask, "review"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}
