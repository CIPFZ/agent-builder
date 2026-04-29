package tools

import (
	"reflect"
	"testing"
)

func TestSkillExtensionInventoryIncludesFrontmatterControls(t *testing.T) {
	skill := ParseSkillFile("ops", "ops/SKILL.md", `---
name: Ops Skill
description: Operate safely.
allowed-tools:
  - Read
  - Grep
context: project
agent: explorer
effort: high
hooks:
  pre: check
---
# Ops
`)

	item := SkillExtensionInventoryItem(skill, "dynamic")
	if item.Name != "ops" || item.DisplayName != "Ops Skill" || item.Source != "dynamic" {
		t.Fatalf("item identity = %#v", item)
	}
	if !reflect.DeepEqual(item.AllowedTools, []string{"Grep", "Read"}) {
		t.Fatalf("allowed tools = %#v, want sorted frontmatter tools", item.AllowedTools)
	}
	if item.Context != "project" || item.Agent != "explorer" || item.Effort != "high" || item.Hooks == nil {
		t.Fatalf("frontmatter projection = %#v, want context/agent/effort/hooks", item)
	}
}
