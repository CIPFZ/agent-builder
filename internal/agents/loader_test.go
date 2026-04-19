package agents_test

import (
	"os"
	"path/filepath"
	"testing"

	"myclaw/internal/agents"
)

func TestLoadClaudeAgentDefinitionsLoadsProjectAndUserAgentsWithProjectPrecedence(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "workspace")
	configHome := filepath.Join(dir, "home", ".claude")

	projectAgent := filepath.Join(project, ".claude", "agents", "reviewer.md")
	userAgent := filepath.Join(configHome, "agents", "reviewer.md")
	for path, body := range map[string]string{
		projectAgent: "---\nname: reviewer\ndescription: Project reviewer\ntools: Read, Grep\nmemory: project\nmodel: claude-sonnet-4-5\n---\nProject review prompt.",
		userAgent:    "---\nname: reviewer\ndescription: User reviewer\nmemory: user\n---\nUser review prompt.",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}

	loaded := agents.LoadClaudeAgentDefinitions(agents.DiscoveryOptions{
		CWD:            project,
		ConfigHome:     configHome,
		IncludeProject: true,
		IncludeUser:    true,
	})
	if len(loaded.All) != 2 {
		t.Fatalf("all agents = %#v, want both loaded records before precedence", loaded.All)
	}
	if len(loaded.Active) != 1 {
		t.Fatalf("active agents = %#v, want one reviewer after precedence merge", loaded.Active)
	}
	got := loaded.Active[0]
	if got.AgentType != "reviewer" || got.Description != "Project reviewer" {
		t.Fatalf("active agent = %#v, want project agent to win precedence", got)
	}
	if got.MemoryScope != "project" || got.Model != "claude-sonnet-4-5" {
		t.Fatalf("active agent = %#v, want parsed memory/model metadata", got)
	}
	if got.SystemPrompt != "Project review prompt." {
		t.Fatalf("active agent prompt = %q, want parsed markdown body", got.SystemPrompt)
	}
}

func TestParseAgentFileParsesClaudeFrontmatterShape(t *testing.T) {
	agent := agents.ParseAgentFile("C:/repo/.claude/agents/researcher.md", []byte(`---
name: researcher
description: Research helper
tools:
  - Read
  - Grep
disallowedTools: Edit, Write
model: inherit
effort: high
permissionMode: plan
maxTurns: 5
background: true
isolation: worktree
initialPrompt: Start with repo search
memory: local
---
Research carefully before answering.
`))

	if agent.AgentType != "researcher" {
		t.Fatalf("agent type = %q, want researcher", agent.AgentType)
	}
	if agent.Description != "Research helper" {
		t.Fatalf("description = %q, want parsed description", agent.Description)
	}
	if len(agent.Tools) != 2 || agent.Tools[0] != "Read" || agent.Tools[1] != "Grep" {
		t.Fatalf("tools = %#v, want parsed tools", agent.Tools)
	}
	if len(agent.DisallowedTools) != 2 || agent.DisallowedTools[0] != "Edit" || agent.DisallowedTools[1] != "Write" {
		t.Fatalf("disallowed = %#v, want parsed disallowed tools", agent.DisallowedTools)
	}
	if agent.Model != "" {
		t.Fatalf("model = %q, want inherit normalized to empty", agent.Model)
	}
	if agent.Effort != "high" || agent.PermissionMode != "plan" {
		t.Fatalf("agent = %#v, want parsed effort/permission mode", agent)
	}
	if agent.MaxTurns != 5 || agent.InitialPrompt != "Start with repo search" {
		t.Fatalf("agent = %#v, want parsed maxTurns/initialPrompt", agent)
	}
	if !agent.Background {
		t.Fatalf("agent = %#v, want parsed background flag", agent)
	}
	if agent.Isolation != "worktree" {
		t.Fatalf("isolation = %q, want worktree", agent.Isolation)
	}
	if agent.MemoryScope != "local" {
		t.Fatalf("memory scope = %q, want local", agent.MemoryScope)
	}
	if agent.SystemPrompt != "Research carefully before answering." {
		t.Fatalf("system prompt = %q, want body without frontmatter", agent.SystemPrompt)
	}
}
