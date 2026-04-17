package tools_test

import (
	"context"
	"encoding/json"
	"reflect"
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
