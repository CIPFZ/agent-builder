package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestInitClaudeBundledSkillsRegistersDefaultAntDefinitionsInClaudeOrder(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)

	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		UserType:             "ant",
		AutoMemoryEnabled:    true,
		KeybindingsEnabled:   true,
		ClaudeInChromeEnable: false,
	})

	skills := tools.GetBundledSkills()
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	want := []string{
		"update-config",
		"keybindings-help",
		"verify",
		"debug",
		"lorem-ipsum",
		"skillify",
		"remember",
		"simplify",
		"batch",
		"stuck",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("bundled skill names = %#v, want Claude order %#v", names, want)
	}
	byName := map[string]tools.SkillCommand{}
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	if byName["verify"].LoadedFrom != "bundled" || byName["verify"].Source != "bundled" || len(byName["verify"].Files) == 0 {
		t.Fatalf("verify skill = %#v, want bundled source metadata and reference files", byName["verify"])
	}
	if !byName["debug"].DisableModelInvocation {
		t.Fatalf("debug skill = %#v, want disable-model-invocation", byName["debug"])
	}
	if byName["keybindings-help"].UserInvocable {
		t.Fatalf("keybindings skill = %#v, want hidden/userInvocable false", byName["keybindings-help"])
	}
}

func TestBundledSkillsRespectClaudeUserTypeAndFeatureGates(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)

	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		UserType:           "external",
		AutoMemoryEnabled:  true,
		KeybindingsEnabled: true,
		Features: map[string]bool{
			"AGENT_TRIGGERS":       true,
			"BUILDING_CLAUDE_APPS": true,
		},
	})

	byName := map[string]tools.SkillCommand{}
	for _, skill := range tools.GetBundledSkills() {
		byName[skill.Name] = skill
	}
	for _, antOnly := range []string{"verify", "lorem-ipsum", "skillify", "stuck"} {
		if _, ok := byName[antOnly]; ok {
			t.Fatalf("bundled skills = %#v, did not want ant-only skill %q for external user", byName, antOnly)
		}
	}
	if _, ok := byName["loop"]; !ok {
		t.Fatalf("bundled skills = %#v, want AGENT_TRIGGERS loop skill", byName)
	}
	if _, ok := byName["claude-api"]; !ok {
		t.Fatalf("bundled skills = %#v, want BUILDING_CLAUDE_APPS claude-api skill", byName)
	}
}

func TestSkillToolInvokesBundledSkillPromptBuilder(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)
	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{})

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "sess-bundled", AgentID: "agent-bundled"},
		Input:   `{"skill":"simplify","args":"focus on tests"}`,
	})
	if err != nil {
		t.Fatalf("invoke bundled skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["commandName"] != "simplify" || parsed["status"] != "inline" {
		t.Fatalf("output = %#v, want bundled simplify inline output", parsed)
	}
	if len(result.NewMessages) != 1 || !strings.Contains(result.NewMessages[0].Content, "Simplify: Code Review and Cleanup") || !strings.Contains(result.NewMessages[0].Content, "focus on tests") {
		t.Fatalf("new messages = %#v, want bundled prompt builder output with args", result.NewMessages)
	}
}

func TestBuiltinPluginSkillCommandsUseBundledSourceSemantics(t *testing.T) {
	tools.ClearBuiltinPluginSkills()
	t.Cleanup(tools.ClearBuiltinPluginSkills)

	tools.RegisterBuiltinPluginSkill("tester", tools.SkillCommand{
		Name:        "plugin-review",
		Description: "Review from built-in plugin",
		Content:     "Plugin skill body.",
	})

	skills := tools.GetBuiltinPluginSkillCommands()
	if len(skills) != 1 {
		t.Fatalf("builtin plugin skills = %#v, want one skill", skills)
	}
	if skills[0].Source != "bundled" || skills[0].LoadedFrom != "bundled" {
		t.Fatalf("builtin plugin skill = %#v, want Claude bundled source semantics", skills[0])
	}
	if skills[0].PluginInfo != nil {
		t.Fatalf("builtin plugin skill = %#v, did not want plugin info for built-in plugin skills", skills[0])
	}
}

func TestVerifyBundledSkillUsesEmbeddedSkillBodyFilesAndUserRequestSection(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("XDG_CONFIG_HOME", appData)
	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{UserType: "ant"})

	var verify tools.SkillCommand
	for _, skill := range tools.GetBundledSkills() {
		if skill.Name == "verify" {
			verify = skill
			break
		}
	}
	if verify.Name == "" {
		t.Fatal("verify bundled skill not registered")
	}
	if strings.Contains(verify.Content, "description:") || !strings.Contains(verify.Content, "Verification Strategy") {
		t.Fatalf("verify content = %q, want parsed SKILL.md body without frontmatter", verify.Content)
	}
	for _, path := range []string{"examples/cli.md", "examples/server.md"} {
		if !strings.Contains(verify.Files[path], "Verification") {
			t.Fatalf("verify file %q = %q, want embedded reference content", path, verify.Files[path])
		}
	}

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "sess-verify", AgentID: "agent-verify"},
		Input:   `{"skill":"verify","args":"check login flow"}`,
	})
	if err != nil {
		t.Fatalf("invoke verify: %v", err)
	}
	content := result.NewMessages[0].Content
	if strings.Contains(content, "---") || !strings.Contains(content, "Verification Strategy") {
		t.Fatalf("verify prompt = %q, want parsed markdown body", content)
	}
	if !strings.Contains(content, "## User Request\n\ncheck login flow") {
		t.Fatalf("verify prompt = %q, want Claude user request append", content)
	}
	prefix := regexp.MustCompile(`Base directory for this skill: ([^\r\n]+)`)
	match := prefix.FindStringSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("verify prompt = %q, want bundled skill base directory prefix", content)
	}
	skillDir := strings.TrimSpace(match[1])
	for _, rel := range []string{"examples/cli.md", "examples/server.md"} {
		path := filepath.Join(skillDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read bundled skill file %q: %v", path, err)
		}
		if !strings.Contains(string(data), "Verification") {
			t.Fatalf("bundled skill file %q = %q, want extracted reference content", path, string(data))
		}
	}
}

func TestClaudeAPIBundledSkillDetectsLanguageAndInlinesReferenceDocs(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		Features: map[string]bool{"BUILDING_CLAUDE_APPS": true},
		CWD:      cwd,
	})

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "sess-api", AgentID: "agent-api"},
		Input:   `{"skill":"claude-api","args":"stream tool calls"}`,
	})
	if err != nil {
		t.Fatalf("invoke claude-api: %v", err)
	}
	content := result.NewMessages[0].Content
	for _, want := range []string{
		"claude-sonnet-4-6",
		"## Reference Documentation",
		`<doc path="go/claude-api.md">`,
		`<doc path="shared/models.md">`,
		"## User Request\n\nstream tool calls",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("claude-api prompt missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "{{SONNET_ID}}") || strings.Contains(content, `<doc path="typescript/claude-api/README.md">`) {
		t.Fatalf("claude-api prompt = %q, want model vars replaced and only Go/shared docs", content)
	}
}

func TestBundledDynamicPromptProvidersMirrorClaudeSections(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)
	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		UserType:           "ant",
		KeybindingsEnabled: true,
		DebugInfo: tools.BundledDebugInfo{
			WasAlreadyLogging: false,
			DebugLogPath:      filepath.Join(t.TempDir(), "debug.log"),
			LogInfo:           "Log size: 42B\n\n### Last 20 lines\n\n```\n[WARN] sample\n```",
			SettingsPaths: map[string]string{
				"user":    "user-settings.json",
				"project": "project-settings.json",
				"local":   "local-settings.json",
			},
		},
		SettingsSchemaJSON: `{"type":"object","properties":{"hooks":{}}}`,
		KeybindingsReference: tools.BundledKeybindingsReference{
			ReservedShortcuts: "| Key | Reason |\n| --- | --- |\n| ctrl+c | Terminal interrupt |",
			ContextsTable:     "| Context | Description |\n| --- | --- |\n| Global | Everywhere |",
			ActionsTable:      "| Action | Description |\n| --- | --- |\n| submit | Send message |",
		},
	})

	tests := []struct {
		skill string
		args  string
		want  []string
	}{
		{
			skill: "debug",
			args:  "login fails",
			want:  []string{"Debug Logging Just Enabled", "Log size: 42B", "user-settings.json", "login fails"},
		},
		{
			skill: "update-config",
			args:  "allow npm test",
			want:  []string{"Full Settings JSON Schema", `"hooks"`, "allow npm test"},
		},
		{
			skill: "update-config",
			args:  "[hooks-only] before Bash run formatter",
			want:  []string{"Troubleshooting Hooks", "Hook Verification Flow", "before Bash run formatter"},
		},
		{
			skill: "keybindings-help",
			args:  "rebind submit",
			want:  []string{"Reserved Shortcuts", "Available Contexts", "Available Actions", "ctrl+c", "submit", "rebind submit"},
		},
	}

	for _, tc := range tests {
		skill := bundledSkillByName(t, tc.skill)
		if skill.PromptBuilder == nil {
			t.Fatalf("%s has no prompt builder", tc.skill)
		}
		content, err := skill.PromptBuilder(tc.args, tools.ToolUseContext{
			Session: session.Session{ID: "sess-" + tc.skill, AgentID: "agent"},
		})
		if err != nil {
			t.Fatalf("build %s prompt: %v", tc.skill, err)
		}
		for _, want := range tc.want {
			if !strings.Contains(content, want) {
				t.Fatalf("%s prompt missing %q:\n%s", tc.skill, want, content)
			}
		}
	}
}

func bundledSkillByName(t *testing.T, name string) tools.SkillCommand {
	t.Helper()
	for _, skill := range tools.GetBundledSkills() {
		if skill.Name == name {
			return skill
		}
	}
	t.Fatalf("bundled skill %q not found in %#v", name, tools.GetBundledSkills())
	return tools.SkillCommand{}
}

func TestScheduleBundledSkillUsesProviderStates(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)
	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		Features: map[string]bool{"AGENT_TRIGGERS_REMOTE": true},
		ScheduleInfo: tools.BundledScheduleInfo{
			Authenticated:              true,
			EnvironmentNames:           []string{"claude-code-default"},
			CurrentRepositoryURL:       "https://github.com/acme/project",
			Timezone:                   "Asia/Shanghai",
			NeedsGitHubAccessReminder:  true,
			RemoteSetupURL:             "https://claude.ai/code/scheduled",
			RemoteSetupInstructionName: "/web-setup",
		},
	})

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "sess-schedule", AgentID: "agent"},
		Input:   `{"skill":"schedule","args":"run tests every day"}`,
	})
	if err != nil {
		t.Fatalf("invoke schedule: %v", err)
	}
	content := result.NewMessages[0].Content
	for _, want := range []string{"claude-code-default", "https://github.com/acme/project", "Asia/Shanghai", "/web-setup", "run tests every day"} {
		if !strings.Contains(content, want) {
			t.Fatalf("schedule prompt missing %q:\n%s", want, content)
		}
	}
}

func TestBundledPromptBuildersResolveDynamicStateAtInvocationTime(t *testing.T) {
	tools.ClearBundledSkills()
	t.Cleanup(tools.ClearBundledSkills)

	debugLogPath := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(debugLogPath, []byte("[WARN] dynamic\n"), 0o644); err != nil {
		t.Fatalf("write debug log: %v", err)
	}

	var schemaValue = `{"type":"object","properties":{"hooks":{}}}`
	var reserved = "| Key | Reason |\n| --- | --- |\n| ctrl+c | interrupt |"
	var environments = []string{"dynamic-env"}

	tools.InitClaudeBundledSkills(tools.BundledSkillOptions{
		UserType:           "ant",
		KeybindingsEnabled: true,
		ResolveDebugInfo: func(toolCtx tools.ToolUseContext) tools.BundledDebugInfo {
			return tools.BundledDebugInfo{
				WasAlreadyLogging: true,
				DebugLogPath:      debugLogPath,
				LogInfo:           "dynamic tail",
				SettingsPaths: map[string]string{
					"user":    "dynamic-user-settings.json",
					"project": "dynamic-project-settings.json",
					"local":   "dynamic-local-settings.json",
				},
			}
		},
		ResolveSettingsSchemaJSON: func(toolCtx tools.ToolUseContext) string {
			return schemaValue
		},
		ResolveKeybindingsReference: func(toolCtx tools.ToolUseContext) tools.BundledKeybindingsReference {
			return tools.BundledKeybindingsReference{
				ReservedShortcuts: reserved,
				ContextsTable:     "| Context | Description |\n| --- | --- |\n| Global | Dynamic |",
				ActionsTable:      "| Action | Description |\n| --- | --- |\n| submit | Dynamic |",
			}
		},
		Features: map[string]bool{"AGENT_TRIGGERS_REMOTE": true},
		ResolveScheduleInfo: func(toolCtx tools.ToolUseContext) tools.BundledScheduleInfo {
			return tools.BundledScheduleInfo{
				Authenticated:             true,
				EnvironmentNames:          append([]string(nil), environments...),
				CurrentRepositoryURL:      "https://github.com/acme/dynamic",
				Timezone:                  "Asia/Shanghai",
				NeedsGitHubAccessReminder: true,
			}
		},
	})

	schemaValue = `{"type":"object","properties":{"env":{}}}`
	reserved = "| Key | Reason |\n| --- | --- |\n| ctrl+x | dynamic |"
	environments = []string{"changed-env"}

	tests := []struct {
		skill string
		args  string
		want  []string
	}{
		{skill: "debug", args: "why", want: []string{"dynamic tail", "dynamic-user-settings.json"}},
		{skill: "update-config", args: "allow npm", want: []string{`"env"`, "allow npm"}},
		{skill: "keybindings-help", args: "rebind", want: []string{"ctrl+x", "Dynamic"}},
		{skill: "schedule", args: "run nightly", want: []string{"changed-env", "https://github.com/acme/dynamic"}},
	}

	for _, tc := range tests {
		skill := bundledSkillByName(t, tc.skill)
		content, err := skill.PromptBuilder(tc.args, tools.ToolUseContext{
			Session: session.Session{ID: "sess-" + tc.skill, AgentID: "agent"},
		})
		if err != nil {
			t.Fatalf("build %s prompt: %v", tc.skill, err)
		}
		for _, want := range tc.want {
			if !strings.Contains(content, want) {
				t.Fatalf("%s prompt missing %q:\n%s", tc.skill, want, content)
			}
		}
	}
}
