package prompt

import (
	"strings"
	"testing"

	"myclaw/internal/memory"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

func TestBuildSeparatesCurrentUserMessageFromHistory(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}
	history := []session.Message{
		{ID: "msg-000001", Role: "user", Content: "old question"},
		{ID: "msg-000002", Role: "assistant", Content: "old answer"},
		{ID: "msg-000003", Role: "user", Content: "current question"},
	}
	current := history[2]

	ctx := Build(BuildInput{
		Session:     sess,
		History:     history,
		UserMessage: current,
		WorkspaceContext: workspace.Context{
			Files: []workspace.File{
				{Name: "AGENTS.md", Content: "follow the repo rules"},
			},
		},
		Tools: []tools.Definition{
			{Name: "text.upper", Description: "Convert text to uppercase"},
		},
	})

	if ctx.UserInput != "current question" {
		t.Fatalf("user input = %q, want %q", ctx.UserInput, "current question")
	}
	if len(ctx.HistoryLines) != 2 {
		t.Fatalf("history lines = %d, want 2", len(ctx.HistoryLines))
	}
	if ctx.HistoryLines[0] != "USER: old question" {
		t.Fatalf("history line 0 = %q", ctx.HistoryLines[0])
	}
	if ctx.HistoryLines[1] != "ASSISTANT: old answer" {
		t.Fatalf("history line 1 = %q", ctx.HistoryLines[1])
	}
	if len(ctx.WorkspaceLines) != 1 {
		t.Fatalf("workspace lines = %d, want 1", len(ctx.WorkspaceLines))
	}
	if ctx.WorkspaceLines[0] != "AGENTS.md:\nfollow the repo rules" {
		t.Fatalf("workspace line 0 = %q", ctx.WorkspaceLines[0])
	}
	if len(ctx.ToolLines) != 1 || ctx.ToolLines[0] != "text.upper: Convert text to uppercase" {
		t.Fatalf("tool lines = %#v", ctx.ToolLines)
	}
}

func TestBuildDefaultSystemPromptIncludesCoreAgentGuidance(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "hello"},
	})

	for _, want := range []string{
		`You are myclaw agent "main".`,
		"# Working Style",
		"Use the provided conversation context and available tools to help the user complete the current task.",
		"Keep responses concise, practical, and grounded in the current workspace state.",
		"The conversation may be compacted over time, so preserve important context when it matters for future work.",
		"# Using Tools",
		"Use tools when they help you verify facts, inspect the workspace, or complete the requested task.",
		"After a tool result arrives, continue by incorporating the result into a direct next-step response.",
		"If a tool call is denied or blocked, do not immediately repeat the exact same call. Adjust your approach or explain what you still need.",
		"# Safety",
		"Respect permission controls and approval requirements before taking risky actions.",
		"The conversation can be compacted over time, so preserve important context in your replies when needed.",
		"# Executing Actions With Care",
		"Prefer reversible local actions when possible.",
		"For actions that are risky, destructive, or affect shared systems, pause and seek confirmation unless the current mode clearly permits proceeding.",
	} {
		if !strings.Contains(ctx.SystemPrompt, want) {
			t.Fatalf("default system prompt missing %q:\n%s", want, ctx.SystemPrompt)
		}
	}
}

func TestComposeSystemContentIncludesContextLifecycleGuidance(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "continue working"},
		History: []session.Message{
			{ID: "msg-2", Role: "assistant", Content: "previous answer"},
		},
		SessionMemories: []string{
			"Summary: repo uses Go modules",
		},
	})

	content := ComposeSystemContent(ctx)
	for _, want := range []string{
		"# Context Lifecycle",
		"The conversation can continue across automatic compaction and summarization.",
		"When prior detail matters for future work, preserve the important facts in your answer or memory-worthy summary.",
		"Session Memory:",
		"Summary: repo uses Go modules",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system content missing %q:\n%s", want, content)
		}
	}
}

func TestComposeSystemContentIncludesApprovalAndToolResultGuidance(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "inspect and then edit the repo"},
		Tools: []tools.Definition{
			{Name: "system.run", Description: "Run a shell command."},
		},
		SystemContextLines: []string{
			"permission_mode=ask",
		},
	})

	content := ComposeSystemContent(ctx)
	for _, want := range []string{
		"# Tool And Approval Handling",
		"After a tool result is returned, continue with a normal assistant answer that uses the result.",
		"If a tool call is denied or requires approval, do not blindly repeat the same call.",
		"Instead, explain what happened, adapt your plan, or wait for approval when appropriate.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system content missing %q:\n%s", want, content)
		}
	}
}

func TestBuildIncludesSessionMemories(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "continue working"},
		SessionMemories: []string{
			"Summary: fixed auth bug",
			"Summary: pending docs update",
		},
	})

	if len(ctx.MemoryLines) != 2 {
		t.Fatalf("memory lines = %d, want 2", len(ctx.MemoryLines))
	}
	if ctx.MemoryLines[0] != "Summary: fixed auth bug" {
		t.Fatalf("memory line 0 = %q", ctx.MemoryLines[0])
	}
}

func TestBuildGroupsTypedSessionMemories(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "continue working"},
		SessionMemoryItems: []memory.Item{
			{Type: memory.TypeSummary, Content: "Summary: fixed auth bug"},
			{Type: memory.TypeTask, Content: "Task: finish deployment"},
			{Type: memory.TypeInstruction, Content: "Instruction: ask before rm -rf"},
		},
	})

	if len(ctx.MemoryByType[memory.TypeSummary]) != 1 {
		t.Fatalf("summary memories = %#v", ctx.MemoryByType)
	}
	if len(ctx.MemoryByType[memory.TypeTask]) != 1 {
		t.Fatalf("task memories = %#v", ctx.MemoryByType)
	}
	if len(ctx.MemoryByType[memory.TypeInstruction]) != 1 {
		t.Fatalf("instruction memories = %#v", ctx.MemoryByType)
	}
}

func TestBuildSystemPromptIncludesToolProtocolInstructions(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "show cwd"},
		Tools: []tools.Definition{
			{Name: "system.run", Description: "Run a shell command on the host system and return stdout and stderr."},
			{Name: "text.upper", Description: "Convert the given input text to uppercase."},
		},
	})

	content := ComposeSystemContent(ctx)
	for _, want := range []string{
		"Available Tools:",
		"system.run: Run a shell command on the host system and return stdout and stderr.",
		"text.upper: Convert the given input text to uppercase.",
		"# Tool And Approval Handling",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system content missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{"<tool_call>", "</tool_call>", "Tool Call Protocol:"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("system content contains legacy XML tool protocol %q:\n%s", forbidden, content)
		}
	}
}

func TestBuildToolLinesIncludeSearchHintsAndDeferredMarkers(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "find the right tool"},
		Tools: []tools.Definition{
			{
				Name:        "agent.task",
				Description: "Run a subagent task with delegated ownership.",
				SearchHint:  "delegate subtask",
				ShouldDefer: true,
				AlwaysLoad:  true,
			},
		},
	})

	if len(ctx.ToolLines) != 1 {
		t.Fatalf("tool lines = %#v, want one formatted tool line", ctx.ToolLines)
	}
	line := ctx.ToolLines[0]
	if strings.Contains(line, "[search hint: delegate subtask [deferred") {
		t.Fatalf("tool line = %q, want flat bracket formatting instead of nested markers", line)
	}
	for _, want := range []string{
		"agent.task: Run a subagent task with delegated ownership.",
		"[search hint: delegate subtask]",
		"[deferred]",
		"[always-loaded]",
		"search hint: delegate subtask",
		"deferred",
		"always-loaded",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("tool line %q missing %q", line, want)
		}
	}
}

func TestBuildSupportsCustomAppendAndOverrideSystemPrompt(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	custom := Build(BuildInput{
		Session:             sess,
		UserMessage:         session.Message{ID: "msg-1", Role: "user", Content: "hello"},
		DefaultSystemPrompt: []string{"default prompt"},
		CustomSystemPrompt:  "custom prompt",
		AppendSystemPrompt:  "append prompt",
	})
	if !strings.Contains(custom.SystemPrompt, "custom prompt") || strings.Contains(custom.SystemPrompt, "default prompt") {
		t.Fatalf("custom system prompt = %q, want custom to replace default", custom.SystemPrompt)
	}
	if !strings.Contains(custom.SystemPrompt, "append prompt") {
		t.Fatalf("custom system prompt = %q, want append prompt", custom.SystemPrompt)
	}

	override := Build(BuildInput{
		Session:              sess,
		UserMessage:          session.Message{ID: "msg-2", Role: "user", Content: "hello again"},
		DefaultSystemPrompt:  []string{"default prompt"},
		CustomSystemPrompt:   "custom prompt",
		AppendSystemPrompt:   "append prompt",
		OverrideSystemPrompt: "override prompt",
	})
	if override.SystemPrompt != "override prompt" {
		t.Fatalf("override system prompt = %q, want override only", override.SystemPrompt)
	}

	agentPrompt := Build(BuildInput{
		Session:             sess,
		UserMessage:         session.Message{ID: "msg-3", Role: "user", Content: "hello agent"},
		DefaultSystemPrompt: []string{"default prompt"},
		CustomSystemPrompt:  "custom prompt",
		AgentSystemPrompt:   "agent prompt",
		AppendSystemPrompt:  "append prompt",
	})
	if !strings.Contains(agentPrompt.SystemPrompt, "agent prompt") || strings.Contains(agentPrompt.SystemPrompt, "custom prompt") || strings.Contains(agentPrompt.SystemPrompt, "default prompt") {
		t.Fatalf("agent system prompt = %q, want agent prompt to take precedence over custom/default", agentPrompt.SystemPrompt)
	}

	coordinatorPrompt := Build(BuildInput{
		Session:                 sess,
		UserMessage:             session.Message{ID: "msg-4", Role: "user", Content: "hello coordinator"},
		DefaultSystemPrompt:     []string{"default prompt"},
		CustomSystemPrompt:      "custom prompt",
		AgentSystemPrompt:       "agent prompt",
		CoordinatorSystemPrompt: "coordinator prompt",
		AppendSystemPrompt:      "append prompt",
	})
	if !strings.Contains(coordinatorPrompt.SystemPrompt, "coordinator prompt") {
		t.Fatalf("coordinator system prompt = %q, want coordinator prompt", coordinatorPrompt.SystemPrompt)
	}
	for _, blocked := range []string{"agent prompt", "custom prompt", "default prompt"} {
		if strings.Contains(coordinatorPrompt.SystemPrompt, blocked) {
			t.Fatalf("coordinator system prompt = %q, did not want %q", coordinatorPrompt.SystemPrompt, blocked)
		}
	}
	if !strings.Contains(coordinatorPrompt.SystemPrompt, "append prompt") {
		t.Fatalf("coordinator system prompt = %q, want append prompt", coordinatorPrompt.SystemPrompt)
	}

	proactiveAgentPrompt := Build(BuildInput{
		Session:              sess,
		UserMessage:          session.Message{ID: "msg-5", Role: "user", Content: "hello proactive"},
		DefaultSystemPrompt:  []string{"default prompt"},
		AgentSystemPrompt:    "agent prompt",
		AppendSystemPrompt:   "append prompt",
		ProactiveAgentPrompt: true,
	})
	for _, want := range []string{"default prompt", "# Custom Agent Instructions", "agent prompt", "append prompt"} {
		if !strings.Contains(proactiveAgentPrompt.SystemPrompt, want) {
			t.Fatalf("proactive agent system prompt = %q, want %q", proactiveAgentPrompt.SystemPrompt, want)
		}
	}
}

func TestBuildToolLinesKeepBuiltinsAsSortedPrefixForPromptStability(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "show tools"},
		Tools: []tools.Definition{
			{Name: "z.mcp", Description: "Late MCP tool.", Source: "mcp"},
			{Name: "tool.search", Description: "Search tools."},
			{Name: "agent.task", Description: "Delegate work."},
			{Name: "a.mcp", Description: "Early MCP tool.", Source: "mcp"},
			{Name: "system.run", Description: "Run command."},
		},
	})

	want := []string{
		"agent.task: Delegate work.",
		"system.run: Run command.",
		"tool.search: Search tools.",
		"a.mcp: Early MCP tool.",
		"z.mcp: Late MCP tool.",
	}
	if len(ctx.ToolLines) != len(want) {
		t.Fatalf("tool lines = %#v, want %#v", ctx.ToolLines, want)
	}
	for i := range want {
		if ctx.ToolLines[i] != want[i] {
			t.Fatalf("tool lines = %#v, want %#v", ctx.ToolLines, want)
		}
	}
}

func TestBuildToolLinesDeduplicateByNameWithBuiltinPrecedence(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "show tools"},
		Tools: []tools.Definition{
			{Name: "system.run", Description: "MCP system.run shadow.", Source: "mcp"},
			{Name: "agent.task", Description: "Delegate work."},
			{Name: "system.run", Description: "Builtin system.run.", Source: "builtin"},
			{Name: "tool.search", Description: "Search tools."},
		},
	})

	want := []string{
		"agent.task: Delegate work.",
		"system.run: Builtin system.run.",
		"tool.search: Search tools.",
	}
	if len(ctx.ToolLines) != len(want) {
		t.Fatalf("tool lines = %#v, want %#v", ctx.ToolLines, want)
	}
	for i := range want {
		if ctx.ToolLines[i] != want[i] {
			t.Fatalf("tool lines = %#v, want %#v", ctx.ToolLines, want)
		}
	}
}

func TestComposeSystemContentIncludesWorkspaceTranscriptAndMemories(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session: sess,
		History: []session.Message{
			{ID: "msg-1", Role: "user", Content: "old question"},
			{ID: "msg-2", Role: "assistant", Content: "old answer"},
		},
		UserMessage: session.Message{ID: "msg-3", Role: "user", Content: "new question"},
		WorkspaceContext: workspace.Context{
			Files: []workspace.File{
				{Name: "AGENTS.md", Content: "follow the repo rules"},
			},
		},
		SessionMemories: []string{
			"Summary: fixed auth bug",
		},
		SessionMemoryItems: []memory.Item{
			{Type: memory.TypeInstruction, Content: "Instruction: ask before rm -rf"},
			{Type: memory.TypeTask, Content: "Task: finish deployment"},
		},
		Tools: []tools.Definition{
			{Name: "system.run", Description: "Run a shell command."},
		},
		UserContextLines: []string{
			"os=windows",
		},
		SystemContextLines: []string{
			"cwd=C:/repo",
		},
	})

	content := ComposeSystemContent(ctx)
	for _, want := range []string{
		"You are myclaw agent",
		"User Context:",
		"os=windows",
		"System Context:",
		"cwd=C:/repo",
		"Workspace Context:",
		"AGENTS.md:",
		"Recent Transcript:",
		"USER: old question",
		"ASSISTANT: old answer",
		"Session Memory:",
		"Summary: fixed auth bug",
		"Instruction Memory:",
		"Instruction: ask before rm -rf",
		"Task Memory:",
		"Task: finish deployment",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system content missing %q:\n%s", want, content)
		}
	}
}

func TestComposeSystemContentDedupesAndOrdersMemorySections(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "hello"},
		SessionMemories: []string{
			"Summary: fixed auth bug",
			"Summary: fixed auth bug",
		},
		SessionMemoryItems: []memory.Item{
			{Type: memory.TypeSummary, Content: "Summary: fixed auth bug"},
			{Type: memory.TypeInstruction, Content: "Instruction: ask before rm -rf"},
			{Type: memory.TypeTask, Content: "Task: finish deployment"},
			{Type: memory.TypeInstruction, Content: "Instruction: ask before rm -rf"},
		},
	})

	content := ComposeSystemContent(ctx)
	if strings.Count(content, "Summary: fixed auth bug") != 1 {
		t.Fatalf("system content = %q, want deduped summary memory occurrence", content)
	}

	instructionIdx := strings.Index(content, "Instruction Memory:")
	taskIdx := strings.Index(content, "Task Memory:")
	summaryIdx := strings.Index(content, "Summary Memory:")
	if !(instructionIdx >= 0 && taskIdx > instructionIdx && summaryIdx > taskIdx) {
		t.Fatalf("memory section order is wrong:\n%s", content)
	}
	if strings.Count(content, "Instruction: ask before rm -rf") != 1 {
		t.Fatalf("system content = %q, want typed instruction memory deduped", content)
	}
}

func TestBuildSkipsSummaryMessagesFromRecentTranscriptWhenTypedSummaryMemoryExists(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session: sess,
		History: []session.Message{
			{ID: "msg-1", Role: "summary", Content: "Summary: compacted context"},
			{ID: "msg-2", Role: "assistant", Content: "recent assistant answer"},
		},
		UserMessage: session.Message{ID: "msg-3", Role: "user", Content: "continue"},
		SessionMemoryItems: []memory.Item{
			{Type: memory.TypeSummary, Content: "Summary: compacted context"},
		},
	})

	content := ComposeSystemContent(ctx)
	if strings.Contains(content, "Recent Transcript:\nSUMMARY: Summary: compacted context") {
		t.Fatalf("system content = %q, did not want summary replayed in recent transcript", content)
	}
	if !strings.Contains(content, "Summary Memory:\nSummary: compacted context") {
		t.Fatalf("system content = %q, want summary memory retained", content)
	}
}

func TestComposeSystemContentIncludesExecutionBoundaries(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "inspect the repo"},
		SystemContextLines: []string{
			"permission_mode=workspace-write",
			"workspace_root=C:/repo",
			"workspace_roots=C:/repo,C:/repo/docs",
		},
	})

	content := ComposeSystemContent(ctx)
	for _, want := range []string{
		"# Execution Boundaries",
		"Current permission mode: workspace-write.",
		"Primary workspace root: C:/repo",
		"Allowed workspace roots: C:/repo, C:/repo/docs",
		"Actions outside the allowed workspace roots or risky actions may require approval.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system content missing %q:\n%s", want, content)
		}
	}
}

func TestComposeSystemContentIncludesAskModeApprovalGuidance(t *testing.T) {
	sess := session.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	ctx := Build(BuildInput{
		Session:     sess,
		UserMessage: session.Message{ID: "msg-1", Role: "user", Content: "run something risky"},
		SystemContextLines: []string{
			"permission_mode=ask",
		},
	})

	content := ComposeSystemContent(ctx)
	if !strings.Contains(content, "You are operating in ask mode, so actions that mutate the system typically require explicit approval.") {
		t.Fatalf("system content missing ask-mode approval guidance:\n%s", content)
	}
}
