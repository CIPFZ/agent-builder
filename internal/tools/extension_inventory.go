package tools

import (
	"sort"
	"strings"
)

type SkillExtensionInventory struct {
	Name                   string
	DisplayName            string
	Description            string
	WhenToUse              string
	Version                string
	Source                 string
	LoadedFrom             string
	UserInvocable          bool
	ArgumentHint           string
	Path                   string
	ArgumentNames          []string
	AllowedTools           []string
	Model                  string
	Context                string
	Agent                  string
	Effort                 string
	Paths                  []string
	Shell                  string
	Hooks                  any
	DisableModelInvocation bool
	MCPPrompt              bool
	MCPServer              string
	MCPPromptName          string
	RemoteCanonical        bool
}

func SkillExtensionInventoryItem(skill SkillCommand, source string) SkillExtensionInventory {
	source = strings.TrimSpace(source)
	if source == "" {
		source = strings.TrimSpace(skill.Source)
	}
	if source == "" && strings.TrimSpace(skill.MCPServer) != "" {
		source = "mcp"
	}
	if source == "" {
		source = "dynamic"
	}
	return SkillExtensionInventory{
		Name:                   strings.TrimSpace(skill.Name),
		DisplayName:            strings.TrimSpace(skill.DisplayName),
		Description:            strings.TrimSpace(skill.Description),
		WhenToUse:              strings.TrimSpace(skill.WhenToUse),
		Version:                strings.TrimSpace(skill.Version),
		Source:                 source,
		LoadedFrom:             strings.TrimSpace(skill.LoadedFrom),
		UserInvocable:          skill.UserInvocable,
		ArgumentHint:           strings.TrimSpace(skill.ArgumentHint),
		Path:                   strings.TrimSpace(skill.Path),
		ArgumentNames:          compactSortedStrings(skill.ArgumentNames),
		AllowedTools:           compactSortedStrings(skill.AllowedTools),
		Model:                  strings.TrimSpace(skill.Model),
		Context:                strings.TrimSpace(skill.Context),
		Agent:                  strings.TrimSpace(skill.Agent),
		Effort:                 strings.TrimSpace(skill.Effort),
		Paths:                  compactSortedStrings(skill.Paths),
		Shell:                  string(skill.Shell),
		Hooks:                  skill.Hooks,
		DisableModelInvocation: skill.DisableModelInvocation,
		MCPPrompt:              skill.MCPPrompt,
		MCPServer:              strings.TrimSpace(skill.MCPServer),
		MCPPromptName:          strings.TrimSpace(skill.MCPPromptName),
		RemoteCanonical:        skill.RemoteCanonical,
	}
}

func compactSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
