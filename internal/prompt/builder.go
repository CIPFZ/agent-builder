package prompt

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"myclaw/internal/memory"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

type BuildInput struct {
	Session                 session.Session
	History                 []session.Message
	UserMessage             session.Message
	WorkspaceContext        workspace.Context
	Tools                   []tools.Definition
	SessionMemories         []string
	SessionMemoryItems      []memory.Item
	DefaultSystemPrompt     []string
	CustomSystemPrompt      string
	AgentSystemPrompt       string
	CoordinatorSystemPrompt string
	ProactiveAgentPrompt    bool
	AppendSystemPrompt      string
	OverrideSystemPrompt    string
	UserContextLines        []string
	SystemContextLines      []string
}

type Context struct {
	SystemPrompt       string
	HistoryLines       []string
	UserInput          string
	UserContextLines   []string
	SystemContextLines []string
	WorkspaceLines     []string
	ToolLines          []string
	MemoryLines        []string
	MemoryByType       map[memory.Type][]string
}

func Build(input BuildInput) Context {
	memoryByType := make(map[memory.Type][]string)
	memorySeen := make(map[memory.Type]map[string]struct{})
	for _, item := range input.SessionMemoryItems {
		if _, ok := memorySeen[item.Type]; !ok {
			memorySeen[item.Type] = make(map[string]struct{})
		}
		if _, ok := memorySeen[item.Type][item.Content]; ok {
			continue
		}
		memorySeen[item.Type][item.Content] = struct{}{}
		memoryByType[item.Type] = append(memoryByType[item.Type], item.Content)
	}
	memoryLines := dedupeStrings(input.SessionMemories)
	toolLines := buildToolLines(input.Tools)
	return Context{
		SystemPrompt:       buildSystemPrompt(input),
		HistoryLines:       buildHistoryLines(input.History, input.UserMessage.ID),
		UserInput:          input.UserMessage.Content,
		UserContextLines:   append([]string(nil), input.UserContextLines...),
		SystemContextLines: append([]string(nil), input.SystemContextLines...),
		WorkspaceLines:     buildWorkspaceLines(input.WorkspaceContext),
		ToolLines:          toolLines,
		MemoryLines:        memoryLines,
		MemoryByType:       memoryByType,
	}
}

func buildSystemPrompt(input BuildInput) string {
	if override := strings.TrimSpace(input.OverrideSystemPrompt); override != "" {
		return override
	}

	baseParts := input.DefaultSystemPrompt
	if len(baseParts) == 0 {
		baseParts = defaultSystemPromptParts(input.Session)
	}
	if custom := strings.TrimSpace(input.CustomSystemPrompt); custom != "" {
		baseParts = []string{custom}
	}
	if agentPrompt := strings.TrimSpace(input.AgentSystemPrompt); agentPrompt != "" {
		if input.ProactiveAgentPrompt {
			baseParts = append(append([]string(nil), baseParts...), "\n# Custom Agent Instructions\n"+agentPrompt)
		} else {
			baseParts = []string{agentPrompt}
		}
	}
	if coordinatorPrompt := strings.TrimSpace(input.CoordinatorSystemPrompt); coordinatorPrompt != "" {
		baseParts = []string{coordinatorPrompt}
	}
	if appendPrompt := strings.TrimSpace(input.AppendSystemPrompt); appendPrompt != "" {
		baseParts = append(append([]string(nil), baseParts...), appendPrompt)
	}
	return strings.Join(baseParts, "\n\n")
}

func defaultSystemPromptParts(sess session.Session) []string {
	return []string{
		fmt.Sprintf(
			`You are myclaw agent %q. Work within session %q and answer the current user request using the provided conversation context.`,
			sess.AgentID,
			sess.Key,
		),
		strings.Join([]string{
			"# Working Style",
			"- Use the provided conversation context and available tools to help the user complete the current task.",
			"- Keep responses concise, practical, and grounded in the current workspace state.",
			"- When tool results arrive, incorporate them into a direct next-step answer rather than restating the entire transcript.",
			"- The conversation may be compacted over time, so preserve important context when it matters for future work.",
		}, "\n"),
		strings.Join([]string{
			"# Using Tools",
			"- Use tools when they help you verify facts, inspect the workspace, or complete the requested task.",
			"- Prefer tool results over guesswork when the answer depends on the current repository, environment, or filesystem state.",
			"- After a tool result arrives, continue by incorporating the result into a direct next-step response.",
			"- If a tool call is denied or blocked, do not immediately repeat the exact same call. Adjust your approach or explain what you still need.",
		}, "\n"),
		strings.Join([]string{
			"# Safety",
			"- Respect permission controls and approval requirements before taking risky actions.",
			"- Prefer accurate, verifiable statements over confident guesses.",
			"- The conversation can be compacted over time, so preserve important context in your replies when needed.",
		}, "\n"),
		strings.Join([]string{
			"# Executing Actions With Care",
			"- Prefer reversible local actions when possible.",
			"- For actions that are risky, destructive, or affect shared systems, pause and seek confirmation unless the current mode clearly permits proceeding.",
			"- Match the scope of your actions to what the user actually asked for.",
		}, "\n"),
	}
}

func buildHistoryLines(history []session.Message, currentUserMessageID string) []string {
	lines := make([]string, 0, len(history))
	hasSummary := false
	for _, msg := range history {
		if msg.Role == "summary" {
			hasSummary = true
			break
		}
	}
	for _, msg := range history {
		if msg.ID == currentUserMessageID {
			continue
		}
		if msg.Role == "system" {
			continue
		}
		if hasSummary && msg.Role == "summary" {
			continue
		}

		lines = append(lines, fmt.Sprintf("%s: %s", strings.ToUpper(msg.Role), msg.Content))
	}
	return lines
}

func buildWorkspaceLines(ctx workspace.Context) []string {
	lines := make([]string, 0, len(ctx.Files))
	for _, file := range ctx.Files {
		label := strings.TrimSpace(file.Name)
		if path := strings.TrimSpace(file.Path); path != "" {
			if root := strings.TrimSpace(ctx.Root); root != "" {
				if relative, err := filepath.Rel(root, path); err == nil && relative != "." && !strings.HasPrefix(relative, "..") {
					label = filepath.ToSlash(relative)
				}
			}
			if label == "" {
				label = filepath.ToSlash(path)
			}
		}
		lines = append(lines, fmt.Sprintf("%s:\n%s", label, file.Content))
	}
	return lines
}

func buildToolLines(defs []tools.Definition) []string {
	defs = append([]tools.Definition(nil), defs...)
	slices.SortFunc(defs, func(a, b tools.Definition) int {
		aBuiltin := toolSortPartition(a) == 0
		bBuiltin := toolSortPartition(b) == 0
		if aBuiltin != bBuiltin {
			if aBuiltin {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	deduped := defs[:0]
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if _, ok := seen[def.Name]; ok {
			continue
		}
		seen[def.Name] = struct{}{}
		deduped = append(deduped, def)
	}
	lines := make([]string, 0, len(defs))
	for _, def := range deduped {
		line := fmt.Sprintf("%s: %s", def.Name, def.Description)
		if def.SearchHint != "" {
			line += fmt.Sprintf(" [search hint: %s]", def.SearchHint)
		}
		if def.ShouldDefer {
			line += " [deferred]"
		}
		if def.AlwaysLoad {
			line += " [always-loaded]"
		}
		lines = append(lines, line)
	}
	return lines
}

func toolSortPartition(def tools.Definition) int {
	source := strings.TrimSpace(def.Source)
	if source == "" || strings.EqualFold(source, "builtin") {
		return 0
	}
	return 1
}

func ComposeSystemContent(ctx Context) string {
	parts := []string{ctx.SystemPrompt}
	if len(ctx.ToolLines) > 0 {
		parts = append(parts, "Available Tools:\n"+strings.Join(ctx.ToolLines, "\n"))
		parts = append(parts, strings.Join([]string{
			"# Tool And Approval Handling",
			"Use the model's native tool calling interface when a tool is needed.",
			"After a tool result is returned, continue with a normal assistant answer that uses the result.",
			"If a tool call is denied or requires approval, do not blindly repeat the same call.",
			"Instead, explain what happened, adapt your plan, or wait for approval when appropriate.",
		}, "\n"))
		if hasSkillToolLine(ctx.ToolLines) {
			parts = append(parts, strings.Join([]string{
				"# Skill Usage",
				"Use the Skill tool when a named skill is relevant to the user's task.",
				"Skill content is injected as additional context after the tool result.",
				"Do not invoke skills just to inspect their full text; call Skill only when you intend to use it.",
			}, "\n"))
		}
	}
	if len(ctx.UserContextLines) > 0 {
		parts = append(parts, "User Context:\n"+strings.Join(ctx.UserContextLines, "\n"))
	}
	if len(ctx.SystemContextLines) > 0 {
		parts = append(parts, "System Context:\n"+strings.Join(ctx.SystemContextLines, "\n"))
	}
	if boundaryLines := buildExecutionBoundaryLines(ctx.SystemContextLines); len(boundaryLines) > 0 {
		parts = append(parts, "# Execution Boundaries\n"+strings.Join(boundaryLines, "\n"))
	}
	parts = append(parts, strings.Join([]string{
		"# Context Lifecycle",
		"The conversation can continue across automatic compaction and summarization.",
		"When prior detail matters for future work, preserve the important facts in your answer or memory-worthy summary.",
	}, "\n"))
	if len(ctx.WorkspaceLines) > 0 {
		parts = append(parts, "Workspace Context:\n"+strings.Join(ctx.WorkspaceLines, "\n\n"))
	}
	if len(ctx.HistoryLines) > 0 {
		parts = append(parts, "Recent Transcript:\n"+strings.Join(ctx.HistoryLines, "\n"))
	}
	sessionMemoryLines := ctx.MemoryLines
	if summaries := ctx.MemoryByType[memory.TypeSummary]; len(summaries) > 0 {
		sessionMemoryLines = withoutStrings(sessionMemoryLines, summaries)
	}
	if len(sessionMemoryLines) > 0 {
		parts = append(parts, "Session Memory:\n"+strings.Join(sessionMemoryLines, "\n"))
	}
	if len(ctx.MemoryByType[memory.TypeInstruction]) > 0 {
		parts = append(parts, "Instruction Memory:\n"+strings.Join(ctx.MemoryByType[memory.TypeInstruction], "\n"))
	}
	if len(ctx.MemoryByType[memory.TypeTask]) > 0 {
		parts = append(parts, "Task Memory:\n"+strings.Join(ctx.MemoryByType[memory.TypeTask], "\n"))
	}
	if len(ctx.MemoryByType[memory.TypeSummary]) > 0 {
		parts = append(parts, "Summary Memory:\n"+strings.Join(ctx.MemoryByType[memory.TypeSummary], "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func hasSkillToolLine(lines []string) bool {
	for _, line := range lines {
		name, _, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(name) == "Skill" {
			return true
		}
	}
	return false
}

func buildExecutionBoundaryLines(systemContextLines []string) []string {
	values := make(map[string]string, len(systemContextLines))
	for _, line := range systemContextLines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		values[key] = value
	}

	lines := make([]string, 0, 4)
	if mode := values["permission_mode"]; mode != "" {
		lines = append(lines, fmt.Sprintf("Current permission mode: %s.", mode))
		switch mode {
		case "ask":
			lines = append(lines, "You are operating in ask mode, so actions that mutate the system typically require explicit approval.")
		default:
			lines = append(lines, "Actions outside the allowed workspace roots or risky actions may require approval.")
		}
	}
	if root := values["workspace_root"]; root != "" {
		lines = append(lines, fmt.Sprintf("Primary workspace root: %s", root))
	}
	if roots := values["workspace_roots"]; roots != "" {
		prettyRoots := strings.Join(splitAndTrim(roots), ", ")
		if prettyRoots != "" {
			lines = append(lines, fmt.Sprintf("Allowed workspace roots: %s", prettyRoots))
		}
	}
	return lines
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func withoutStrings(values []string, blocked []string) []string {
	if len(values) == 0 || len(blocked) == 0 {
		return values
	}
	skip := make(map[string]struct{}, len(blocked))
	for _, value := range blocked {
		skip[value] = struct{}{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := skip[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}
