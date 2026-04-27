package commands

import (
	"fmt"
	"sort"
	"strings"
)

type Visibility string

const (
	VisibilityAlways     Visibility = "always"
	VisibilityWhenMemory Visibility = "when_memory"
	VisibilityWhenResume Visibility = "when_resume"
	VisibilityWhenTasks  Visibility = "when_tasks"
	VisibilityWhenMCP    Visibility = "when_mcp"
)

type Behavior string

const (
	BehaviorImmediate Behavior = "immediate"
	BehaviorQuery     Behavior = "query"
)

type Metadata struct {
	Name         string
	Aliases      []string
	Description  string
	ArgumentHint string
	Category     string
	Visibility   Visibility
	Behavior     Behavior
}

type Context struct {
	PermissionMode       string
	Model                string
	HasMemory            bool
	HasResumableSessions bool
	HasTasks             bool
	HasMCP               bool
}

type Result struct {
	CommandName     string
	Output          string
	NormalizedInput string
	ShouldQuery     bool
}

type Registry struct {
	commands map[string]Metadata
	aliases  map[string]string
	order    []string
}

func NewRegistry(commands ...Metadata) *Registry {
	registry := &Registry{commands: map[string]Metadata{}, aliases: map[string]string{}}
	for _, command := range commands {
		registry.Register(command)
	}
	return registry
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(
		Metadata{Name: "help", Description: "Show this command reference", Category: "system", Visibility: VisibilityAlways, Behavior: BehaviorImmediate},
		Metadata{Name: "permissions", Description: "Show permission mode and approval guidance", Category: "runtime", Visibility: VisibilityAlways, Behavior: BehaviorImmediate},
		Metadata{Name: "model", Description: "Show or change model settings", Category: "runtime", Visibility: VisibilityAlways, Behavior: BehaviorImmediate},
		Metadata{Name: "memory", Description: "Show memory context", Category: "context", Visibility: VisibilityWhenMemory, Behavior: BehaviorImmediate},
		Metadata{Name: "resume", Aliases: []string{"continue"}, Description: "Resume a previous session", Category: "session", Visibility: VisibilityWhenResume, Behavior: BehaviorImmediate},
		Metadata{Name: "compact", Description: "Run manual compaction", ArgumentHint: "<optional instructions>", Category: "context", Visibility: VisibilityAlways, Behavior: BehaviorImmediate},
		Metadata{Name: "tasks", Aliases: []string{"agents"}, Description: "Show delegated tasks", Category: "agent", Visibility: VisibilityWhenTasks, Behavior: BehaviorImmediate},
		Metadata{Name: "mcp", Description: "Show MCP server status", Category: "mcp", Visibility: VisibilityWhenMCP, Behavior: BehaviorImmediate},
		Metadata{Name: "status", Description: "Ask for runtime status", Category: "runtime", Visibility: VisibilityAlways, Behavior: BehaviorQuery},
	)
}

func (r *Registry) Register(command Metadata) {
	command = normalize(command)
	if command.Name == "" {
		return
	}
	if _, ok := r.commands[command.Name]; !ok {
		r.order = append(r.order, command.Name)
	}
	r.commands[command.Name] = command
	for alias, canonical := range r.aliases {
		if canonical == command.Name {
			delete(r.aliases, alias)
		}
	}
	for _, alias := range command.Aliases {
		r.aliases[alias] = command.Name
	}
}

func (r *Registry) Resolve(name string) (Metadata, bool) {
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "/")
	if command, ok := r.commands[name]; ok {
		return cloneMetadata(command), true
	}
	if canonical, ok := r.aliases[name]; ok {
		return cloneMetadata(r.commands[canonical]), true
	}
	return Metadata{}, false
}

func (r *Registry) List(ctx Context) []Metadata {
	out := make([]Metadata, 0, len(r.order))
	for _, name := range r.order {
		command := r.commands[name]
		if command.visible(ctx) {
			out = append(out, cloneMetadata(command))
		}
	}
	return out
}

func (r *Registry) Execute(ctx Context, input string) (Result, error) {
	name, args := parse(input)
	if name == "" {
		return Result{}, fmt.Errorf("command input is empty")
	}
	command, ok := r.Resolve(name)
	if !ok {
		return Result{}, fmt.Errorf("slash command %q is not registered", name)
	}
	if !command.visible(ctx) {
		return Result{}, fmt.Errorf("slash command %q is not visible in the current runtime state", command.Name)
	}
	result := Result{CommandName: command.Name}
	if command.Behavior == BehaviorQuery {
		result.ShouldQuery = true
		result.NormalizedInput = strings.TrimSpace(args)
		if result.NormalizedInput == "" {
			result.NormalizedInput = command.Description
		}
		return result, nil
	}
	result.Output = command.render(ctx, args, r.List(ctx))
	return result, nil
}

func parse(input string) (string, string) {
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/"))
	name, args, _ := strings.Cut(body, " ")
	return strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(args)
}

func normalize(command Metadata) Metadata {
	command.Name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(command.Name)), "/")
	aliases := make([]string, 0, len(command.Aliases))
	for _, alias := range command.Aliases {
		alias = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(alias)), "/")
		if alias != "" && alias != command.Name {
			aliases = append(aliases, alias)
		}
	}
	command.Aliases = aliases
	command.Description = strings.TrimSpace(command.Description)
	command.ArgumentHint = strings.TrimSpace(command.ArgumentHint)
	command.Category = strings.TrimSpace(command.Category)
	if command.Visibility == "" {
		command.Visibility = VisibilityAlways
	}
	if command.Behavior == "" {
		command.Behavior = BehaviorImmediate
	}
	return command
}

func (m Metadata) visible(ctx Context) bool {
	switch m.Visibility {
	case VisibilityWhenMemory:
		return ctx.HasMemory
	case VisibilityWhenResume:
		return ctx.HasResumableSessions
	case VisibilityWhenTasks:
		return ctx.HasTasks
	case VisibilityWhenMCP:
		return ctx.HasMCP
	default:
		return true
	}
}

func (m Metadata) render(ctx Context, args string, visible []Metadata) string {
	switch m.Name {
	case "help":
		lines := []string{"Commands"}
		for _, command := range visible {
			label := "/" + command.Name
			if command.ArgumentHint != "" {
				label += " " + command.ArgumentHint
			}
			lines = append(lines, fmt.Sprintf("%s - %s", label, command.Description))
		}
		return strings.Join(lines, "\n")
	case "permissions":
		mode := strings.TrimSpace(ctx.PermissionMode)
		if mode == "" {
			mode = "default"
		}
		return "Permissions\nMode: " + mode
	case "model":
		model := strings.TrimSpace(ctx.Model)
		if model == "" {
			model = "default"
		}
		return "Model\nCurrent: " + model
	case "compact":
		if strings.TrimSpace(args) != "" {
			return "Compaction\nInstructions: " + strings.TrimSpace(args)
		}
		return "Compaction\nManual compaction available"
	case "memory":
		return "Memory\nRuntime memory is available"
	case "resume":
		return "Resume session\nResumable sessions are available"
	case "tasks":
		return "Tasks\nDelegated tasks are available"
	case "mcp":
		return "MCP\nMCP servers are available"
	default:
		return strings.Title(m.Name)
	}
}

func cloneMetadata(input Metadata) Metadata {
	out := input
	out.Aliases = append([]string(nil), input.Aliases...)
	return out
}

func Sort(commands []Metadata) {
	sort.SliceStable(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
}
