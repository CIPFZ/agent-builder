package tools

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type SkillPromptBuilder func(args string, toolCtx ToolUseContext) (string, error)

type BundledSkillOptions struct {
	UserType                    string
	CWD                         string
	Features                    map[string]bool
	AutoMemoryEnabled           bool
	KeybindingsEnabled          bool
	ClaudeInChromeEnable        bool
	DebugInfo                   BundledDebugInfo
	ResolveDebugInfo            func(ToolUseContext) BundledDebugInfo
	SettingsSchemaJSON          string
	ResolveSettingsSchemaJSON   func(ToolUseContext) string
	KeybindingsReference        BundledKeybindingsReference
	ResolveKeybindingsReference func(ToolUseContext) BundledKeybindingsReference
	ScheduleInfo                BundledScheduleInfo
	ResolveScheduleInfo         func(ToolUseContext) BundledScheduleInfo
}

type BundledDebugInfo struct {
	WasAlreadyLogging bool
	DebugLogPath      string
	LogInfo           string
	SettingsPaths     map[string]string
}

type BundledKeybindingsReference struct {
	ReservedShortcuts string
	ContextsTable     string
	ActionsTable      string
}

type BundledScheduleInfo struct {
	Authenticated              bool
	EnvironmentNames           []string
	CurrentRepositoryURL       string
	Timezone                   string
	NeedsGitHubAccessReminder  bool
	RemoteSetupURL             string
	RemoteSetupInstructionName string
}

var bundledSkillState = struct {
	sync.RWMutex
	skills []skillCommand
	byName map[string]skillCommand
}{
	byName: make(map[string]skillCommand),
}

var bundledSkillExtractState = struct {
	sync.Mutex
	root string
}{
	root: "",
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

	RegisterBundledSkill(bundledUpdateConfigSkill(opts))
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
		RegisterBundledSkill(bundledScheduleSkill(opts))
	}
	if bundledFeatureEnabled(opts, "BUILDING_CLAUDE_APPS") {
		RegisterBundledSkill(bundledClaudeAPISkill(opts))
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
	if len(command.Files) > 0 && command.SkillRoot == "" && command.Name != "" {
		command.SkillRoot = bundledSkillExtractDir(command.Name)
	}
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

func bundledDebugInfoForContext(opts BundledSkillOptions, toolCtx ToolUseContext) BundledDebugInfo {
	info := opts.DebugInfo
	if opts.ResolveDebugInfo != nil {
		resolved := opts.ResolveDebugInfo(toolCtx)
		info = mergeBundledDebugInfo(info, resolved)
	}
	return info
}

func bundledSettingsSchemaJSONForContext(opts BundledSkillOptions, toolCtx ToolUseContext) string {
	if opts.ResolveSettingsSchemaJSON != nil {
		if resolved := strings.TrimSpace(opts.ResolveSettingsSchemaJSON(toolCtx)); resolved != "" {
			return resolved
		}
	}
	return strings.TrimSpace(opts.SettingsSchemaJSON)
}

func bundledKeybindingsReferenceForContext(opts BundledSkillOptions, toolCtx ToolUseContext) BundledKeybindingsReference {
	ref := opts.KeybindingsReference
	if opts.ResolveKeybindingsReference != nil {
		ref = mergeBundledKeybindingsReference(ref, opts.ResolveKeybindingsReference(toolCtx))
	}
	return ref
}

func bundledScheduleInfoForContext(opts BundledSkillOptions, toolCtx ToolUseContext) BundledScheduleInfo {
	info := opts.ScheduleInfo
	if opts.ResolveScheduleInfo != nil {
		info = mergeBundledScheduleInfo(info, opts.ResolveScheduleInfo(toolCtx))
	}
	return info
}

func mergeBundledDebugInfo(base, override BundledDebugInfo) BundledDebugInfo {
	if override.WasAlreadyLogging {
		base.WasAlreadyLogging = true
	}
	if strings.TrimSpace(override.DebugLogPath) != "" {
		base.DebugLogPath = override.DebugLogPath
	}
	if strings.TrimSpace(override.LogInfo) != "" {
		base.LogInfo = override.LogInfo
	}
	if len(override.SettingsPaths) > 0 {
		if base.SettingsPaths == nil {
			base.SettingsPaths = map[string]string{}
		}
		for key, value := range override.SettingsPaths {
			if strings.TrimSpace(value) != "" {
				base.SettingsPaths[key] = value
			}
		}
	}
	return base
}

func mergeBundledKeybindingsReference(base, override BundledKeybindingsReference) BundledKeybindingsReference {
	if strings.TrimSpace(override.ReservedShortcuts) != "" {
		base.ReservedShortcuts = override.ReservedShortcuts
	}
	if strings.TrimSpace(override.ContextsTable) != "" {
		base.ContextsTable = override.ContextsTable
	}
	if strings.TrimSpace(override.ActionsTable) != "" {
		base.ActionsTable = override.ActionsTable
	}
	return base
}

func mergeBundledScheduleInfo(base, override BundledScheduleInfo) BundledScheduleInfo {
	if override.Authenticated {
		base.Authenticated = true
	}
	if len(override.EnvironmentNames) > 0 {
		base.EnvironmentNames = append([]string(nil), override.EnvironmentNames...)
	}
	if strings.TrimSpace(override.CurrentRepositoryURL) != "" {
		base.CurrentRepositoryURL = override.CurrentRepositoryURL
	}
	if strings.TrimSpace(override.Timezone) != "" {
		base.Timezone = override.Timezone
	}
	if override.NeedsGitHubAccessReminder {
		base.NeedsGitHubAccessReminder = true
	}
	if strings.TrimSpace(override.RemoteSetupURL) != "" {
		base.RemoteSetupURL = override.RemoteSetupURL
	}
	if strings.TrimSpace(override.RemoteSetupInstructionName) != "" {
		base.RemoteSetupInstructionName = override.RemoteSetupInstructionName
	}
	return base
}

func bundledUpdateConfigSkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:          "update-config",
		Description:   "Use this skill to configure the Claude Code harness via settings.json. Automated behaviors require hooks configured in settings.json. Also use for permissions, env vars, hook troubleshooting, or settings.json changes.",
		AllowedTools:  []string{"Read"},
		UserInvocable: true,
		PromptBuilder: func(args string, toolCtx ToolUseContext) (string, error) {
			schema := bundledSettingsSchemaJSONForContext(opts, toolCtx)
			if schema == "" {
				schema = defaultBundledSettingsSchemaJSON
			}
			prompt := updateConfigPrompt + "\n\n## Full Settings JSON Schema\n\n```json\n" + schema + "\n```"
			if strings.HasPrefix(args, "[hooks-only]") {
				prompt = hooksDocsPrompt + "\n\n" + hookVerificationFlowPrompt
				args = strings.TrimSpace(strings.TrimPrefix(args, "[hooks-only]"))
			}
			if strings.TrimSpace(args) != "" {
				if strings.HasPrefix(prompt, hooksDocsPrompt) {
					prompt += "\n\n## Task\n\n" + args
				} else {
					prompt += "\n\n## User Request\n\n" + args
				}
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
		PromptBuilder: func(args string, toolCtx ToolUseContext) (string, error) {
			ref := bundledKeybindingsReferenceForContext(opts, toolCtx)
			if ref.ReservedShortcuts == "" {
				ref.ReservedShortcuts = "No reserved shortcut table is configured."
			}
			if ref.ContextsTable == "" {
				ref.ContextsTable = "| Context | Description |\n| --- | --- |\n| Global | Global keybindings |"
			}
			if ref.ActionsTable == "" {
				ref.ActionsTable = "| Action | Description |\n| --- | --- |\n| submit | Submit the current message |"
			}
			prompt := strings.Join([]string{
				keybindingsIntroPrompt,
				"## Reserved Shortcuts\n\n" + ref.ReservedShortcuts,
				"## Available Contexts\n\n" + ref.ContextsTable,
				"## Available Actions\n\n" + ref.ActionsTable,
			}, "\n\n")
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
		Files:         cloneStringMap(verifySkillFiles),
		Content:       verifySkillBody,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := strings.TrimLeft(verifySkillBody, "\n")
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
		PromptBuilder: func(args string, toolCtx ToolUseContext) (string, error) {
			info := bundledDebugInfoForContext(opts, toolCtx)
			if info.DebugLogPath == "" {
				info.DebugLogPath = filepath.Join(os.Getenv("USERPROFILE"), ".claude", "debug", "session.txt")
			}
			if info.LogInfo == "" {
				info.LogInfo = "No debug log exists yet - logging was just enabled."
			}
			justEnabledSection := ""
			if !info.WasAlreadyLogging {
				justEnabledSection = "\n\n## Debug Logging Just Enabled\n\nDebug logging was OFF for this session until now. Nothing prior to this /debug invocation was captured.\n\nTell the user that debug logging is now active at `" + info.DebugLogPath + "`, ask them to reproduce the issue, then re-read the log."
			}
			prompt := "# Debug Skill\n\nHelp the user debug an issue they're encountering in this current Claude Code session." +
				justEnabledSection +
				"\n\n## Session Debug Log\n\nThe debug log for the current session is at: `" + info.DebugLogPath + "`\n\n" + info.LogInfo +
				"\n\nFor additional context, grep for [ERROR] and [WARN] lines across the full file." +
				"\n\n## Issue Description\n\n" + firstNonEmpty(strings.TrimSpace(args), "The user did not describe a specific issue. Read the debug log and summarize any errors, warnings, or notable issues.") +
				"\n\n## Settings\n\nRemember that settings are in:\n* user - " + firstNonEmpty(info.SettingsPaths["user"], "~/.claude/settings.json") +
				"\n* project - " + firstNonEmpty(info.SettingsPaths["project"], ".claude/settings.json") +
				"\n* local - " + firstNonEmpty(info.SettingsPaths["local"], ".claude/settings.local.json") +
				"\n\n## Instructions\n\n1. Review the user's issue description\n2. Look for [ERROR] and [WARN] entries, stack traces, and failure patterns across the file\n3. Explain what you found in plain language\n4. Suggest concrete fixes or next steps"
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

func bundledScheduleSkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:          "schedule",
		Description:   "Create, update, list, or run scheduled remote agents (triggers) that execute on a cron schedule.",
		WhenToUse:     "When the user wants to schedule a recurring remote agent, set up automated tasks, create a cron job for Claude Code, or manage scheduled agents/triggers.",
		AllowedTools:  []string{"RemoteTrigger", "AskUserQuestion"},
		UserInvocable: true,
		PromptBuilder: func(args string, toolCtx ToolUseContext) (string, error) {
			info := bundledScheduleInfoForContext(opts, toolCtx)
			if !info.Authenticated {
				return "You need to authenticate with a claude.ai account first. API accounts are not supported. Run /login, then try /schedule again.", nil
			}
			if info.RemoteSetupURL == "" {
				info.RemoteSetupURL = "https://claude.ai/code/scheduled"
			}
			if info.RemoteSetupInstructionName == "" {
				info.RemoteSetupInstructionName = "/web-setup"
			}
			prompt := "# Schedule\n\nCreate, update, list, or run scheduled remote agents (triggers) that execute on a cron schedule."
			if len(info.EnvironmentNames) > 0 {
				prompt += "\n\n## Remote Environments\n\n- " + strings.Join(info.EnvironmentNames, "\n- ")
			}
			if info.CurrentRepositoryURL != "" {
				prompt += "\n\n## Current Repository\n\n" + info.CurrentRepositoryURL
			}
			if info.Timezone != "" {
				prompt += "\n\n## User Timezone\n\n" + info.Timezone
			}
			if info.NeedsGitHubAccessReminder {
				prompt += "\n\n## GitHub Access Reminder\n\nIf the user's request requires GitHub repo access, remind them to run " + info.RemoteSetupInstructionName + " or install the Claude GitHub App so the remote agent can access the repository."
			}
			prompt += "\n\nTo delete a trigger, direct users to " + info.RemoteSetupURL + "."
			if strings.TrimSpace(args) != "" {
				prompt += "\n\n## User Request\n\nThe user said: \"" + args + "\"\n\nStart by understanding their intent and working through the appropriate workflow above."
			}
			return prompt, nil
		},
	}
}

func bundledClaudeAPISkill(opts BundledSkillOptions) skillCommand {
	return skillCommand{
		Name:          "claude-api",
		Description:   "Build apps with the Claude API or Anthropic SDK.\nTRIGGER when: code imports `anthropic`/`@anthropic-ai/sdk`/`claude_agent_sdk`, or user asks to use Claude API, Anthropic SDKs, or Agent SDK.\nDO NOT TRIGGER when: code imports `openai`/other AI SDK, general programming, or ML/data-science tasks.",
		AllowedTools:  []string{"Read", "Grep", "Glob", "WebFetch"},
		UserInvocable: true,
		PromptBuilder: func(args string, _ ToolUseContext) (string, error) {
			prompt := buildClaudeAPIPrompt(detectBundledClaudeAPILanguage(opts.CWD), args)
			if strings.TrimSpace(args) != "" {
				// buildClaudeAPIPrompt already appends the user request.
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

const verifySkillBody = `# Verify

Verify a code change does what it should by running the app.

## Verification Strategy

1. Understand what behavior changed and what evidence would prove it works.
2. Prefer focused tests or direct app execution over broad unrelated checks.
3. If verification fails, inspect the failure, fix the issue, and run the relevant check again.
4. Report the exact command or workflow used and the observed result.`

var verifySkillFiles = map[string]string{
	"examples/cli.md": `# CLI Verification

Verification for command-line tools should run the smallest command that exercises the changed behavior. Prefer targeted unit tests, then a representative CLI invocation, and include the exact command and output summary in the final report.`,
	"examples/server.md": `# Server Verification

Verification for server changes should start the service or run the relevant handler/integration tests. Check status codes, response bodies, logs, and any persistent state touched by the change.`,
}

const updateConfigPrompt = `# Update Config

Use this skill to configure the Claude Code harness via settings.json. Automated behaviors ("from now on when X", "each time X", "whenever X", "before/after X") require hooks configured in settings.json - the harness executes these, not Claude, so memory/preferences cannot fulfill them.

Always read the relevant settings file before proposing or applying changes. Validate JSON syntax after edits. For simple settings like theme/model, use Config tool.`

const hooksDocsPrompt = `# Hooks Configuration

Configure Claude Code hooks in settings.json. Hooks can run commands, prompts, or agents before and after tool use. Memory and preferences cannot implement automated hook behavior; settings.json must be updated.`

const hookVerificationFlowPrompt = `## Hook Verification Flow

1. Read the target settings file.
2. Add or update the hook in valid JSON.
3. Explain the matcher, hook type, command or prompt, and destination settings layer.

## Troubleshooting Hooks

If a hook is not running, check the settings file, JSON syntax, matcher, hook type, command behavior, and debug logs.`

const defaultBundledSettingsSchemaJSON = `{"type":"object","properties":{"hooks":{"type":"object"},"permissions":{"type":"object"},"env":{"type":"object"}}}`

const keybindingsIntroPrompt = `# Keybindings Help

Help customize ~/.claude/keybindings.json. Cover file format, keystroke syntax, chords, unbinding, context-specific bindings, behavioral rules, doctor output, and validation errors.`

var claudeAPIModelVars = map[string]string{
	"OPUS_ID":        "claude-opus-4-6",
	"OPUS_NAME":      "Claude Opus 4.6",
	"SONNET_ID":      "claude-sonnet-4-6",
	"SONNET_NAME":    "Claude Sonnet 4.6",
	"HAIKU_ID":       "claude-haiku-4-5",
	"HAIKU_NAME":     "Claude Haiku 4.5",
	"PREV_SONNET_ID": "claude-sonnet-4-5",
}

const claudeAPISkillPrompt = `# Claude API

Build apps with the Claude API, Anthropic SDKs, and Agent SDK.

Current recommended models include {{SONNET_ID}}, {{OPUS_ID}}, and {{HAIKU_ID}}.

## Reading Guide

Use the language-specific and shared reference docs below to select the right examples.

## When to Use WebFetch

Use WebFetch for live documentation, pricing, changelog, or API behavior that may have changed.

## Common Pitfalls

Do not append date suffixes to model aliases unless the API documentation explicitly requires it.`

var claudeAPISkillFiles = map[string]string{
	"go/claude-api.md": `# Go Claude API

Use the Go SDK or an HTTP client to call the Messages API. Configure model IDs such as {{SONNET_ID}}, pass messages as structured content, and stream responses when the UI needs incremental updates.`,
	"typescript/claude-api/README.md": `# TypeScript Claude API

Use @anthropic-ai/sdk for TypeScript and JavaScript projects. Prefer structured messages, tool schemas, streaming helpers, and explicit error handling.`,
	"python/claude-api/README.md": `# Python Claude API

Use the anthropic Python SDK for Python projects. Prefer client.messages.create, streaming context managers, and explicit model IDs.`,
	"shared/models.md": `# Models

Current models: {{OPUS_NAME}} ({{OPUS_ID}}), {{SONNET_NAME}} ({{SONNET_ID}}), and {{HAIKU_NAME}} ({{HAIKU_ID}}).`,
	"shared/tool-use-concepts.md": `# Tool Use Concepts

Tools require schemas, tool_use blocks, matching tool_result blocks, and model continuation after tool results.`,
	"shared/prompt-caching.md": `# Prompt Caching

Keep stable context before cache breakpoints and avoid unnecessary churn in large prompts.`,
	"shared/error-codes.md": `# Error Codes

Handle authentication, rate limit, overloaded, invalid request, and permission errors explicitly.`,
	"shared/live-sources.md": `# Live Sources

Use WebFetch for current API reference pages, changelogs, model lists, and SDK documentation.`,
}

func buildClaudeAPIPrompt(lang string, args string) string {
	cleanPrompt := processClaudeAPIContent(claudeAPISkillPrompt)
	readingGuideIdx := strings.Index(cleanPrompt, "## Reading Guide")
	basePrompt := cleanPrompt
	if readingGuideIdx >= 0 {
		basePrompt = strings.TrimRight(cleanPrompt[:readingGuideIdx], "\n")
	}
	parts := []string{basePrompt}
	if lang != "" {
		filePaths := claudeAPIFilesForLanguage(lang)
		parts = append(parts, strings.ReplaceAll(claudeAPIInlineReadingGuide, "{lang}", lang))
		parts = append(parts, "---\n\n## Included Documentation\n\n"+buildClaudeAPIInlineReference(filePaths))
	} else {
		parts = append(parts, strings.ReplaceAll(claudeAPIInlineReadingGuide, "{lang}", "unknown"))
		parts = append(parts, "No project language was auto-detected. Ask the user which language they are using, then refer to the matching docs below.")
		parts = append(parts, "---\n\n## Included Documentation\n\n"+buildClaudeAPIInlineReference(mapKeys(claudeAPISkillFiles)))
	}
	if idx := strings.Index(cleanPrompt, "## When to Use WebFetch"); idx >= 0 {
		parts = append(parts, strings.TrimRight(cleanPrompt[idx:], "\n"))
	}
	if strings.TrimSpace(args) != "" {
		parts = append(parts, "## User Request\n\n"+args)
	}
	return strings.Join(parts, "\n\n")
}

const claudeAPIInlineReadingGuide = `## Reference Documentation

The relevant documentation for your detected language is included below in <doc> tags. Each tag has a path attribute showing its original file path.

### Quick Task Reference

Single text classification/summarization/extraction/Q&A:
Refer to {lang}/claude-api/README.md

Chat UI or real-time response display:
Refer to {lang}/claude-api/README.md plus {lang}/claude-api/streaming.md

Function calling / tool use / agents:
Refer to {lang}/claude-api/README.md plus shared/tool-use-concepts.md

Error handling:
Refer to shared/error-codes.md

Latest docs via WebFetch:
Refer to shared/live-sources.md for URLs`

func claudeAPIFilesForLanguage(lang string) []string {
	paths := make([]string, 0)
	for path := range claudeAPISkillFiles {
		if strings.HasPrefix(path, lang+"/") || strings.HasPrefix(path, "shared/") {
			paths = append(paths, path)
		}
	}
	sortStrings(paths)
	return paths
}

func buildClaudeAPIInlineReference(filePaths []string) string {
	sortStrings(filePaths)
	sections := make([]string, 0, len(filePaths))
	for _, path := range filePaths {
		md := claudeAPISkillFiles[path]
		if md == "" {
			continue
		}
		sections = append(sections, `<doc path="`+path+`">`+"\n"+strings.TrimSpace(processClaudeAPIContent(md))+"\n</doc>")
	}
	return strings.Join(sections, "\n\n")
}

func processClaudeAPIContent(md string) string {
	out := md
	for key, value := range claudeAPIModelVars {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
	}
	return out
}

func detectBundledClaudeAPILanguage(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	indicators := map[string][]string{
		"python":     {".py", "requirements.txt", "pyproject.toml", "setup.py", "Pipfile"},
		"typescript": {".ts", ".tsx", "tsconfig.json", "package.json"},
		"java":       {".java", "pom.xml", "build.gradle"},
		"go":         {".go", "go.mod"},
		"ruby":       {".rb", "Gemfile"},
		"csharp":     {".cs", ".csproj"},
		"php":        {".php", "composer.json"},
	}
	for _, lang := range []string{"python", "typescript", "java", "go", "ruby", "csharp", "php"} {
		for _, indicator := range indicators[lang] {
			for _, name := range names {
				if strings.HasPrefix(indicator, ".") {
					if strings.HasSuffix(name, indicator) {
						return lang
					}
				} else if name == indicator {
					return lang
				}
			}
		}
	}
	return ""
}

func mapKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func bundledSkillExtractDir(skillName string) string {
	root := bundledSkillsRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, skillName)
}

func bundledSkillsRoot() string {
	bundledSkillExtractState.Lock()
	defer bundledSkillExtractState.Unlock()
	if bundledSkillExtractState.root != "" {
		return bundledSkillExtractState.root
	}
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		configDir = os.TempDir()
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		bundledSkillExtractState.root = filepath.Join(configDir, "myclaw", "bundled-skills", "dev")
		return bundledSkillExtractState.root
	}
	bundledSkillExtractState.root = filepath.Join(configDir, "myclaw", "bundled-skills", "dev", hex.EncodeToString(nonce))
	return bundledSkillExtractState.root
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
