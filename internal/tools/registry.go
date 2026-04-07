package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

type Definition struct {
	Name        string
	Aliases     []string
	Description string
	Source      string
	SearchHint  string
	Enabled     bool
	ReadOnly    bool
	Destructive bool
	ShouldDefer bool
	AlwaysLoad  bool
}

type Tool interface {
	Definition() Definition
	Invoke(context.Context, session.Session, string) (string, error)
	IsEnabled() bool
	IsReadOnly(string) bool
	IsDestructive(string) bool
	ShouldDefer() bool
	AlwaysLoad() bool
	PromptDescription() string
	SearchHint() string
}

type Registry struct {
	tools   map[string]Tool
	aliases map[string]string
	order   []string
}

type AssembleOptions struct {
	Policy permissions.Policy
}

type ExposeOptions struct {
	IncludeDeferred bool
	Policy          permissions.Policy
}

type SearchOptions struct {
	Policy permissions.Policy
}

func NewRegistry(toolList ...Tool) *Registry {
	registry := &Registry{
		tools:   make(map[string]Tool),
		aliases: make(map[string]string),
	}
	for _, tool := range toolList {
		registry.Register(tool)
	}
	return registry
}

func (r *Registry) Register(tool Tool) {
	def := normalizeDefinition(tool.Definition())
	if _, exists := r.tools[def.Name]; !exists {
		r.order = append(r.order, def.Name)
	}
	r.tools[def.Name] = tool
	for alias, canonical := range r.aliases {
		if canonical == def.Name {
			delete(r.aliases, alias)
		}
	}
	for _, alias := range def.Aliases {
		r.aliases[alias] = def.Name
	}
}

func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		defs = append(defs, normalizeDefinition(toolMetadata(tool)))
	}
	return defs
}

func (r *Registry) Assemble(opts AssembleOptions) []Definition {
	defs := r.Expose(ExposeOptions{
		IncludeDeferred: true,
		Policy:          opts.Policy,
	})
	out := make([]Definition, 0, len(defs))
	return append(out, defs...)
}

func (r *Registry) Expose(opts ExposeOptions) []Definition {
	defs := r.Definitions()
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		if isBlanketDenied(def.Name, opts.Policy.Rules) {
			continue
		}
		if def.ShouldDefer && !def.AlwaysLoad && !opts.IncludeDeferred {
			continue
		}
		out = append(out, def)
	}
	return out
}

func (r *Registry) Invoke(ctx context.Context, sess session.Session, name, input string) (string, error) {
	tool, ok := r.resolve(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	return tool.Invoke(ctx, sess, input)
}

func (r *Registry) Inspect(name, input string) (Definition, bool) {
	tool, ok := r.resolve(name)
	if !ok {
		return Definition{}, false
	}
	def := normalizeDefinition(tool.Definition())
	if promptText := strings.TrimSpace(tool.PromptDescription()); promptText != "" {
		def.Description = promptText
	}
	if searchHint := strings.TrimSpace(tool.SearchHint()); searchHint != "" {
		def.SearchHint = searchHint
	}
	def.Enabled = tool.IsEnabled()
	def.ReadOnly = tool.IsReadOnly(input)
	def.Destructive = tool.IsDestructive(input)
	def.ShouldDefer = tool.ShouldDefer()
	def.AlwaysLoad = tool.AlwaysLoad()
	return def, true
}

func (r *Registry) Search(query string, opts SearchOptions) []Definition {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	type scored struct {
		def   Definition
		score int
		index int
	}

	var matches []scored
	for idx, name := range r.order {
		tool := r.tools[name]
		def := normalizeDefinition(toolMetadata(tool))
		if !def.Enabled {
			continue
		}
		if isBlanketDenied(def.Name, opts.Policy.Rules) {
			continue
		}
		score := searchScore(query, def)
		if score == 0 {
			continue
		}
		matches = append(matches, scored{def: def, score: score, index: idx})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].index < matches[j].index
	})

	out := make([]Definition, 0, len(matches))
	for _, item := range matches {
		out = append(out, item.def)
	}
	return out
}

func (r *Registry) resolve(name string) (Tool, bool) {
	name = strings.TrimSpace(name)
	if tool, ok := r.tools[name]; ok {
		return tool, true
	}
	if canonical, ok := r.aliases[name]; ok {
		tool, ok := r.tools[canonical]
		return tool, ok
	}
	return nil, false
}

func normalizeDefinition(def Definition) Definition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	def.Source = strings.TrimSpace(def.Source)
	def.SearchHint = strings.TrimSpace(def.SearchHint)
	if def.Source == "" {
		def.Source = "builtin"
	}
	aliases := make([]string, 0, len(def.Aliases))
	for _, alias := range def.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == def.Name {
			continue
		}
		aliases = append(aliases, alias)
	}
	def.Aliases = aliases
	return def
}

func toolMetadata(tool Tool) Definition {
	def := tool.Definition()
	if promptText := strings.TrimSpace(tool.PromptDescription()); promptText != "" {
		def.Description = promptText
	}
	if searchHint := strings.TrimSpace(tool.SearchHint()); searchHint != "" {
		def.SearchHint = searchHint
	}
	def.Enabled = tool.IsEnabled()
	def.ReadOnly = tool.IsReadOnly("")
	def.Destructive = tool.IsDestructive("")
	def.ShouldDefer = tool.ShouldDefer()
	def.AlwaysLoad = tool.AlwaysLoad()
	return def
}

func isBlanketDenied(toolName string, rules []permissions.Rule) bool {
	for _, rule := range rules {
		if rule.ToolName != toolName {
			continue
		}
		if rule.Action != permissions.ActionDeny {
			continue
		}
		if len(rule.Match.CommandContains) > 0 || len(rule.Match.WorkDirPrefixes) > 0 {
			continue
		}
		return true
	}
	return false
}

func searchScore(query string, def Definition) int {
	score := 0
	if strings.Contains(strings.ToLower(def.Name), query) {
		score += 10
	}
	for _, alias := range def.Aliases {
		if strings.Contains(strings.ToLower(alias), query) {
			score += 8
			break
		}
	}
	if strings.Contains(strings.ToLower(def.Description), query) {
		score += 5
	}
	if strings.Contains(strings.ToLower(def.SearchHint), query) {
		score += 7
	}
	return score
}
