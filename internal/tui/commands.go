package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	runtimecommands "myclaw/internal/commands"
)

type slashCommandSpec struct {
	Name         string
	Description  string
	Aliases      []string
	ArgumentHint string
}

var localSlashCommandSpecs = buildLocalSlashCommandSpecs()

var tuiOnlySlashCommandSpecs = []slashCommandSpec{
	{Name: "keybindings", Description: "Show keybinding reference", Aliases: []string{"keys", "shortcuts"}},
	{Name: "open", Description: "Quick open commands, sessions, tasks, and MCP"},
	{Name: "search", Description: "Search workspace file contents", Aliases: []string{"grep", "find"}},
	{Name: "clear", Description: "Clear visible conversation", Aliases: []string{"reset", "new"}},
	{Name: "context", Description: "Show current context usage"},
	{Name: "session", Description: "Show session details"},
	{Name: "debug", Description: "Show diagnostics"},
}

func buildLocalSlashCommandSpecs() []slashCommandSpec {
	registry := runtimecommands.NewDefaultRegistry()
	metadata := registry.List(runtimecommands.Context{
		PermissionMode:       "default",
		HasMemory:            true,
		HasResumableSessions: true,
		HasTasks:             true,
		HasMCP:               true,
	})
	specs := make([]slashCommandSpec, 0, len(metadata)+len(tuiOnlySlashCommandSpecs))
	seen := make(map[string]struct{}, len(metadata)+len(tuiOnlySlashCommandSpecs))
	for _, command := range metadata {
		spec := slashCommandSpec{
			Name:         command.Name,
			Description:  command.Description,
			Aliases:      append([]string(nil), command.Aliases...),
			ArgumentHint: command.ArgumentHint,
		}
		if command.Name == "compact" {
			spec.Description = "Run manual compaction or microcompact tool output"
			spec.ArgumentHint = "<optional custom summarization instructions>"
		}
		specs = append(specs, spec)
		seen[spec.Name] = struct{}{}
	}
	for _, command := range tuiOnlySlashCommandSpecs {
		if _, ok := seen[command.Name]; ok {
			continue
		}
		specs = append(specs, command)
	}
	return specs
}

var slashCommands = slashCommandNames(localSlashCommandSpecs)

type parsedSlashCommand struct {
	Spec slashCommandSpec
	Args string
}

func slashCommandNames(commands []slashCommandSpec) []string {
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, "/"+command.Name)
	}
	return names
}

func parseLocalSlashCommand(text string) (parsedSlashCommand, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return parsedSlashCommand{}, false
	}
	body := strings.TrimPrefix(text, "/")
	name, args, _ := strings.Cut(body, " ")
	name = strings.ToLower(strings.TrimSpace(name))
	args = strings.TrimSpace(args)
	if name == "" {
		return parsedSlashCommand{}, false
	}
	for _, command := range localSlashCommandSpecs {
		if command.Name == name || stringInSlice(name, command.Aliases) {
			return parsedSlashCommand{Spec: command, Args: args}, true
		}
	}
	return parsedSlashCommand{}, false
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func sendUserMessageCmd(bridge Bridge, text string) tea.Cmd {
	return func() tea.Msg {
		if err := bridge.SendUserMessage(text); err != nil {
			return BridgeErrMsg{Err: err}
		}
		return nil
	}
}
