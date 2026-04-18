package tui

import (
	"strings"
)

type slashCommandSpec struct {
	Name         string
	Description  string
	Aliases      []string
	ArgumentHint string
}

var localSlashCommandSpecs = []slashCommandSpec{
	{Name: "help", Description: "Show this command reference"},
	{Name: "keybindings", Description: "Show keybinding reference", Aliases: []string{"keys", "shortcuts"}},
	{Name: "open", Description: "Quick open commands, sessions, tasks, and MCP", Aliases: []string{"search"}},
	{Name: "clear", Description: "Clear visible conversation", Aliases: []string{"reset", "new"}},
	{Name: "model", Description: "Show model options"},
	{Name: "mcp", Description: "Show MCP server status"},
	{Name: "session", Description: "Show session details"},
	{Name: "tasks", Description: "Show delegated task workbench", Aliases: []string{"agents"}},
	{Name: "resume", Description: "Resume a previous session", Aliases: []string{"continue"}},
	{Name: "compact", Description: "Show compaction status", ArgumentHint: "<optional custom summarization instructions>"},
	{Name: "debug", Description: "Show diagnostics"},
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
