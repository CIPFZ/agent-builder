package tools

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

type SkillPromptBuilder func(args string, toolCtx ToolUseContext) (string, error)

type BundledSkillOptions struct {
	UserType             string
	Features             map[string]bool
	AutoMemoryEnabled    bool
	KeybindingsEnabled   bool
	ClaudeInChromeEnable bool
}

var bundledSkillState = struct {
	sync.RWMutex
	skills []skillCommand
	byName map[string]skillCommand
}{
	byName: make(map[string]skillCommand),
}

var builtinPluginSkillState = struct {
	sync.RWMutex
	skills []skillCommand
}{
	skills: nil,
}

func InitClaudeBundledSkills(opts BundledSkillOptions) {
	if opts.UserType == "" {
		opts.UserType = os.Getenv("USER_TYPE")
	}
	ClearBundledSkills()

	RegisterBundledSkill(bundledUpdateConfigSkill())
	RegisterBundledSkill(bundledKeybindingsHelpSkill(opts))
	if opts.UserType == "ant" {
		RegisterBundledSkill(bundledVerifySkill())
	}
	RegisterBundledSkill(bundledDebugSkill(opts))
	if opts.UserType == "ant" {
		RegisterBundledSkill(bundledLoremIpsumSkill())
		RegisterBundledSkill(bundledSkillifySkill())
	}
	RegisterBundledSkill(bundledRememberSkill(opts))
	RegisterBundledSkill(bundledSimplifySkill())
	RegisterBundledSkill(bundledBatchSkill())
	if opts.UserType == "ant" {
		RegisterBundledSkill(bundledStuckSkill())
	}
	if bundledFeatureEnabled(opts, "AGENT_TRIGGERS") {
		RegisterBundledSkill(bundledLoopSkill())
	}
	if bundledFeatureEnabled(opts, "AGENT_TRIGGERS_REMOTE") {
		RegisterBundledSkill(bundledScheduleSkill())
	}
	if bundledFeatureEnabled(opts, "BUILDING_CLAUDE_APPS") {
		RegisterBundledSkill(bundledClaudeAPISkill())
	}
	if opts.ClaudeInChromeEnable {
		RegisterBundledSkill(bundledClaudeInChromeSkill())
	}
}

func RegisterBundledSkill(command SkillCommand) {
	command = normalizeBundledSkill(command)
	if command.Name == "" {
		return
	}
	bundledSkillState.Lock()
	defer bundledSkillState.Unlock()
	if _, exists := bundledSkillState.byName[command.Name]; exists {
		return
	}
	bundledSkillState.skills = append(bundledSkillState.skills, command)
	bundledSkillState.byName[command.Name] = command
}

func GetBundledSkills() []SkillCommand {
	bundledSkillState.RLock()
	defer bundledSkillState.RUnlock()
	out := make([]SkillCommand, 0, len(bundledSkillState.skills))
	for _, skill := range bundledSkillState.skills {
		if !skillIsEnabled(skill) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func ClearBundledSkills() {
	bundledSkillState.Lock()
	defer bundledSkillState.Unlock()
	bundledSkillState.skills = nil
	bundledSkillState.byName = make(map[string]skillCommand)
}

func lookupBundledSkill(name string) (skillCommand, bool) {
	bundledSkillState.RLock()
	defer bundledSkillState.RUnlock()
	command, ok := bundledSkillState.byName[name]
	if !ok || !skillIsEnabled(command) {
		return skillCommand{}, false
	}
	return command, true
}

func RegisterBuiltinPluginSkill(pluginName string, command SkillCommand) {
	_ = pluginName
	command = normalizeBundledSkill(command)
	if command.Name == "" {
		return
	}
	builtinPluginSkillState.Lock()
	defer builtinPluginSkillState.Unlock()
	builtinPluginSkillState.skills = append(builtinPluginSkillState.skills, command)
}

func GetBuiltinPluginSkillCommands() []SkillCommand {
	builtinPluginSkillState.RLock()
	defer builtinPluginSkillState.RUnlock()
	out := make([]SkillCommand, 0, len(builtinPluginSkillState.skills))
	for _, skill := range builtinPluginSkillState.skills {
		if !skillIsEnabled(skill) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func ClearBuiltinPluginSkills() {
	builtinPluginSkillState.Lock()
	defer builtinPluginSkillState.Unlock()
	builtinPluginSkillState.skills = nil
}

func lookupBuiltinPluginSkill(name string) (skillCommand, bool) {
	builtinPluginSkillState.RLock()
	defer builtinPluginSkillState.RUnlock()
	for _, skill := range builtinPluginSkillState.skills {
		if skill.Name == name && skillIsEnabled(skill) {
			return skill, true
		}
	}
	return skillCommand{}, false
}

func normalizeBundledSkill(command skillCommand) skillCommand {
	command.Source = "bundled"
	command.LoadedFrom = "bundled"
	command.HasUserSpecifiedDescription = true
	if command.UserInvocable == false && command.Name != "keybindings-help" {
		command.UserInvocable = true
	}
	return command
}

func skillIsEnabled(command skillCommand) bool {
	if command.IsEnabled == nil {
		return true
	}
	return command.IsEnabled()
}

func bundledFeatureEnabled(opts BundledSkillOptions, name string) bool {
	if opts.Features != nil {
		return opts.Features[name]
	}
	features := splitList(os.Getenv("MYCLAW_FEATURES"))
	for _, feature := range features {
		if strings.EqualFold(feature, name) {
			return true
		}
	}
	return false
}

func bundledUpdateConfigSkill() skillCommand {
	return skillCommand{
		Name:          "update-config",
		Description:   "Use this skill to configure the Claude Code harness via settings.json. Automated behaviors require hooks configured in settings.json. Also use for permissions, env vars, hook troubleshooting, or settings.json changes.",
		AllowedTools:  []string{"Read"},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Update Config\n\nConfigure Claude Code settings, permissions, hooks, environment variables, and settings files. Read the relevant settings file before proposing or applying changes."
			if strings.HasPrefix(args, "[hooks-only]") {
				prompt = "# Hooks Configuration\n\nConfigure Claude Code hooks in settings.json. Hooks are executed by the harness, not by memory or preferences."
				args = strings.TrimSpace(strings.TrimPrefix(args, "[hooks-only]"))
			}
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User Request\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledKeybindingsHelpSkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:          "keybindings-help",
		Description:   "Use when the user wants to customize keyboard shortcuts, rebind keys, add chord bindings, or modify ~/.claude/keybindings.json.",
		AllowedTools:  []string{"Read"},
		UserInvocable: false,
		IsEnabled: func() bool {
			return opts.KeybindingsEnabled
		},
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Keybindings Help\n\nHelp customize ~/.claude/keybindings.json, including key syntax, contexts, actions, unbinding, reserved shortcuts, and doctor-style validation."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User Request\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledVerifySkill() skillCommand {
	return skillCommand{
		Name:          "verify",
		Description:   "Verify a code change does what it should by running the app.",
		UserInvocable: true,
		Files: map[string]string{
			"examples/cli.md":    "CLI verification examples.",
			"examples/server.md": "Server verification examples.",
		},
		Content: "# Verify\n\nVerify that the code change does what it should. Run the application or focused tests, inspect failures, fix issues, and report the exact verification performed.",
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Verify\n\nVerify that the code change does what it should. Run the app or tests, inspect failures, fix issues, and summarize evidence."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User Request\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledDebugSkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:                   "debug",
		Description:            map[bool]string{true: "Debug your current Claude Code session by reading the session debug log. Includes all event logging", false: "Enable debug logging for this session and help diagnose issues"}[opts.UserType == "ant"],
		AllowedTools:           []string{"Read", "Grep", "Glob"},
		ArgumentHint:           "[issue description]",
		DisableModelInvocation: true,
		UserInvocable:          true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Debug Skill\n\nHelp the user debug an issue in the current Claude Code session. Inspect debug logs, settings, warnings, errors, and stack traces."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## Issue Description\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledLoremIpsumSkill() skillCommand {
	return skillCommand{
		Name:          "lorem-ipsum",
		Description:   "Generate filler text for long context testing. Specify token count as argument (e.g., /lorem-ipsum 50000). Outputs approximately the requested number of tokens. Ant-only.",
		ArgumentHint:  "[token_count]",
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			count := 10000
			if strings.TrimSpace(args) != "" {
				parsed, err := strconv.Atoi(strings.TrimSpace(args))
				if err != nil || parsed <= 0 {
					return "Invalid token count. Please provide a positive number (e.g., /lorem-ipsum 10000).", nil
				}
				count = parsed
			}
			if count > 500000 {
				count = 500000
			}
			return generateBundledLoremIpsum(count), nil
		},
	}
}

func bundledSkillifySkill() skillCommand {
	return skillCommand{
		Name:                   "skillify",
		Description:            "Capture this session's repeatable process into a skill. Call at end of the process you want to capture with an optional description.",
		AllowedTools:           []string{"Read", "Write", "Edit", "Glob", "Grep", "AskUserQuestion", "Bash(mkdir:*)"},
		ArgumentHint:           "[description of the process you want to capture]",
		DisableModelInvocation: true,
		UserInvocable:          true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Skillify\n\nCapture this session's repeatable process into a Claude Code skill. Create or update a SKILL.md with clear trigger conditions, workflow, and supporting references."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\nThe user described this process as: \"" + args + "\""
			}
			return prompt, nil
		},
	}
}

func bundledRememberSkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:        "remember",
		Description: "Review auto-memory entries and propose promotions to CLAUDE.md, CLAUDE.local.md, or shared memory. Also detects outdated, conflicting, and duplicate entries across memory layers.",
		WhenToUse:   "Use when the user wants to review, organize, or promote their auto-memory entries. Also useful for cleaning up outdated or conflicting entries across CLAUDE.md, CLAUDE.local.md, and auto-memory.",
		IsEnabled: func() bool {
			return opts.AutoMemoryEnabled
		},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Remember\n\nReview auto-memory entries and propose promotions to CLAUDE.md, CLAUDE.local.md, or shared memory. Present all proposals before making changes."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## Additional context from user\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledSimplifySkill() skillCommand {
	return skillCommand{
		Name:          "simplify",
		Description:   "Review changed code for reuse, quality, and efficiency, then fix any issues found.",
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Simplify: Code Review and Cleanup\n\nReview all changed files for reuse, quality, and efficiency. Fix any issues found.\n\n## Phase 1: Identify Changes\n\nRun git diff or inspect recently modified files.\n\n## Phase 2: Launch Three Review Agents in Parallel\n\nUse agents for code reuse review, code quality review, and efficiency review.\n\n## Phase 3: Fix Issues\n\nAggregate findings and fix each issue directly."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## Additional Focus\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledBatchSkill() skillCommand {
	return skillCommand{
		Name:                   "batch",
		Description:            "Research and plan a large-scale change, then execute it in parallel across 5-30 isolated worktree agents that each open a PR.",
		WhenToUse:              "Use when the user wants to make a sweeping, mechanical change across many files that can be decomposed into independent parallel units.",
		ArgumentHint:           "<instruction>",
		DisableModelInvocation: true,
		UserInvocable:          true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			if strings.TrimSpace(args) == "" {
				return "Provide an instruction describing the batch change you want to make.", nil
			}
			return "# Batch\n\nResearch and plan this large-scale change, decompose it into independent worktree agents, execute in parallel, and prepare PRs.\n\n## Instruction\n\n" + args, nil
		},
	}
}

func bundledStuckSkill() skillCommand {
	return skillCommand{
		Name:          "stuck",
		Description:   "[ANT-ONLY] Investigate frozen/stuck/slow Claude Code sessions on this machine and post a diagnostic report to #claude-code-feedback.",
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# /stuck - diagnose frozen/slow Claude Code sessions\n\nInvestigate other Claude Code processes for high CPU, stuck states, zombie processes, memory leaks, or hung child processes. Do not kill processes."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User-provided context\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledLoopSkill() skillCommand {
	return skillCommand{
		Name:          "loop",
		Description:   "Run a prompt or slash command on a recurring interval (e.g. /loop 5m /foo, defaults to 10m)",
		WhenToUse:     "When the user wants to set up a recurring task, poll for status, or run something repeatedly on an interval. Do NOT invoke for one-off tasks.",
		ArgumentHint:  "[interval] <prompt>",
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			if strings.TrimSpace(args) == "" {
				return "Usage: /loop [interval] <prompt>", nil
			}
			return "# Loop\n\nSet up a recurring task for this prompt, then execute it now.\n\n## Input\n\n" + args, nil
		},
	}
}

func bundledScheduleSkill() skillCommand {
	return skillCommand{
		Name:          "schedule",
		Description:   "Create, update, list, or run scheduled remote agents (triggers) that execute on a cron schedule.",
		WhenToUse:     "When the user wants to schedule a recurring remote agent, set up automated tasks, create a cron job for Claude Code, or manage scheduled agents/triggers.",
		AllowedTools:  []string{"RemoteTrigger", "AskUserQuestion"},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			return "# Schedule\n\nCreate, update, list, or run scheduled remote agents. Work through the required environment, repository, cadence, and prompt details.\n\n## User Request\n\n" + args, nil
		},
	}
}

func bundledClaudeAPISkill() skillCommand {
	return skillCommand{
		Name:          "claude-api",
		Description:   "Build apps with the Claude API or Anthropic SDK.\nTRIGGER when: code imports `anthropic`/`@anthropic-ai/sdk`/`claude_agent_sdk`, or user asks to use Claude API, Anthropic SDKs, or Agent SDK.\nDO NOT TRIGGER when: code imports `openai`/other AI SDK, general programming, or ML/data-science tasks.",
		AllowedTools:  []string{"Read", "Grep", "Glob", "WebFetch"},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Claude API\n\nHelp build applications with the Claude API, Anthropic SDKs, and Agent SDK. Prefer official SDK patterns and current API semantics."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User Request\n\n" + args
			}
			return prompt, nil
		},
	}
}

func bundledClaudeInChromeSkill() skillCommand {
	return skillCommand{
		Name:        "claude-in-chrome",
		Description: "Automates your Chrome browser to interact with web pages - clicking elements, filling forms, capturing screenshots, reading console logs, and navigating sites.",
		WhenToUse:   "When the user wants to interact with web pages, automate browser tasks, capture screenshots, read console logs, or perform browser-based actions.",
		AllowedTools: []string{
			"mcp__claude-in-chrome__tabs_context_mcp",
			"mcp__claude-in-chrome__click",
			"mcp__claude-in-chrome__fill",
			"mcp__claude-in-chrome__screenshot",
		},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := "# Claude in Chrome\n\nYou have access to Chrome browser automation tools. Start by calling mcp__claude-in-chrome__tabs_context_mcp to inspect current browser tabs."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## Task\n\n" + args
			}
			return prompt, nil
		},
	}
}

func generateBundledLoremIpsum(tokens int) string {
	words := []string{"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit"}
	var builder strings.Builder
	for i := 0; i < tokens; i++ {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(words[i%len(words)])
	}
	return builder.String()
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
