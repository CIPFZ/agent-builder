package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

var _ tools.Tool = (*RunTool)(nil)
var _ tools.AutoClassifyingTool = (*RunTool)(nil)
var _ tools.ContextualTool = (*RunTool)(nil)

const maxShellOutputBytes = 32 * 1024

type RunTool struct {
	router      *sandbox.Router
	name        string
	aliases     []string
	description string
	searchHint  string
	powershell  bool
}

func NewRunTool(router *sandbox.Router) *RunTool {
	return newShellTool(router, "system.run", nil, "Run a shell command on the host system and return stdout and stderr.", "shell command", false)
}

func NewBashTool(router *sandbox.Router) *RunTool {
	return newShellTool(router, "Bash", []string{"bash"}, "Executes a given bash command in a persistent shell session with optional timeout and background execution metadata.", "bash shell command", false)
}

func NewPowerShellTool(router *sandbox.Router) *RunTool {
	return newShellTool(router, "PowerShell", []string{"powershell", "pwsh"}, "Executes a given PowerShell command with optional timeout and background execution metadata.", "powershell command", true)
}

func newShellTool(router *sandbox.Router, name string, aliases []string, description, searchHint string, powershell bool) *RunTool {
	if router == nil {
		router = sandbox.NewRouter(nil, nil)
	}
	return &RunTool{router: router, name: name, aliases: aliases, description: description, searchHint: searchHint, powershell: powershell}
}

func (t *RunTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        t.name,
		Aliases:     append([]string(nil), t.aliases...),
		Description: t.description,
		InputSchema: shellInputSchema(),
		Enabled:     true,
		ReadOnly:    false,
		Destructive: true,
	}
}

func (t *RunTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	result, err := t.InvokeWithContext(ctx, tools.ToolUseContext{
		Session: sess,
		Input:   input,
	})
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (t *RunTool) InvokeWithContext(ctx context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	spec := parseShellInput(toolCtx.Input)
	if spec.Command == "" {
		return tools.ToolResult{}, fmt.Errorf("%s requires command", t.name)
	}
	if spec.RunInBackground && t.name != "system.run" {
		toolCtx.ReportProgress(tools.ToolProgress{
			ToolUseID: toolCtx.ToolUseID,
			Type:      "shell.background_started",
			Message:   spec.Command,
			Data:      map[string]any{"command": spec.Command, "tool": t.name},
		})
		go func() {
			bgCtx := context.Background()
			if spec.Timeout > 0 {
				var cancel context.CancelFunc
				bgCtx, cancel = context.WithTimeout(bgCtx, spec.Timeout)
				defer cancel()
			}
			_, _ = t.router.Run(bgCtx, toolCtx.Session, spec.Command)
		}()
		return tools.ToolResult{Output: fmt.Sprintf("%s command is running in the background: %s", t.name, spec.Command)}, nil
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	toolCtx.ReportProgress(tools.ToolProgress{
		ToolUseID: toolCtx.ToolUseID,
		Type:      "shell.started",
		Message:   spec.Command,
		Data:      map[string]any{"command": spec.Command, "tool": t.name},
	})
	output, err := t.router.Run(runCtx, toolCtx.Session, spec.Command)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	toolCtx.ReportProgress(tools.ToolProgress{
		ToolUseID: toolCtx.ToolUseID,
		Type:      "shell.finished",
		Message:   spec.Command,
		Data:      map[string]any{"command": spec.Command, "tool": t.name, "timed_out": timedOut},
	})
	output = truncateShellOutput(output)
	if err != nil {
		return tools.ToolResult{Output: output}, err
	}
	return tools.ToolResult{Output: output}, nil
}

func (t *RunTool) IsEnabled() bool {
	return true
}

func (t *RunTool) IsReadOnly(input string) bool {
	if t.name == "system.run" {
		return false
	}
	command := strings.ToLower(strings.TrimSpace(commandFromInput(input)))
	if command == "" {
		return false
	}
	segments := splitCompoundCommand(command)
	if len(segments) > 1 {
		for _, segment := range segments {
			if !isSimpleReadOnlyCommand(segment) {
				return false
			}
		}
		return true
	}
	return isSimpleReadOnlyCommand(command)
}

func isSimpleReadOnlyCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return false
	}
	destructivePrefixes := []string{
		"rm ", "del ", "erase ", "remove-item ", "ri ", "move-item ", "mv ", "cp ", "copy ", "copy-item ",
		"write ", "write-output >", "set-content ", "add-content ", "new-item ", "mkdir ", "rmdir ",
		"git commit", "git push", "git reset", "git checkout ", "git clean",
	}
	for _, prefix := range destructivePrefixes {
		if command == strings.TrimSpace(prefix) || strings.HasPrefix(command, prefix) {
			return false
		}
	}
	readPrefixes := []string{
		"cat ", "type ", "more ", "less ", "head ", "tail ", "grep ", "rg ", "find ", "ls", "dir",
		"pwd", "cd ", "git status", "git diff", "git log", "go test", "npm test",
	}
	for _, prefix := range readPrefixes {
		if command == strings.TrimSpace(prefix) || strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func splitCompoundCommand(command string) []string {
	parts := []string{command}
	for _, sep := range []string{"&&", "||", ";", "\n"} {
		var next []string
		for _, part := range parts {
			next = append(next, strings.Split(part, sep)...)
		}
		parts = next
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (t *RunTool) IsDestructive(input string) bool {
	return !t.IsReadOnly(input)
}

func (t *RunTool) ToAutoClassifierInput(input string) any {
	return strings.TrimSpace(commandFromInput(input))
}

func (t *RunTool) ShouldDefer() bool {
	return false
}

func (t *RunTool) AlwaysLoad() bool {
	return false
}

func (t *RunTool) PromptDescription() string {
	return t.description
}

func (t *RunTool) SearchHint() string {
	return t.searchHint
}

func shellInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":           map[string]any{"type": "string", "description": "The command to execute"},
			"timeout":           map[string]any{"type": "number", "description": "Optional timeout in milliseconds"},
			"description":       map[string]any{"type": "string", "description": "Clear, concise description of what this command does"},
			"run_in_background": map[string]any{"type": "boolean", "description": "Set true to run long-lived commands in the background"},
			"dangerouslyDisableSandbox": map[string]any{
				"type":        "boolean",
				"description": "Disable sandboxing only when the permission policy explicitly allows it.",
			},
		},
		"required": []string{"command"},
	}
}

type shellInput struct {
	Command         string
	Timeout         time.Duration
	RunInBackground bool
}

func parseShellInput(input string) shellInput {
	input = strings.TrimSpace(input)
	spec := shellInput{Command: commandFromInput(input)}
	if strings.HasPrefix(input, "{") {
		var object map[string]any
		if err := json.Unmarshal([]byte(input), &object); err == nil {
			spec.RunInBackground, _ = object["run_in_background"].(bool)
			if timeout := durationMillis(object["timeout"]); timeout > 0 {
				spec.Timeout = timeout
			}
		}
	}
	return spec
}

func durationMillis(value any) time.Duration {
	switch v := value.(type) {
	case float64:
		return time.Duration(v) * time.Millisecond
	case int:
		return time.Duration(v) * time.Millisecond
	case int64:
		return time.Duration(v) * time.Millisecond
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return time.Duration(parsed) * time.Millisecond
		}
	}
	return 0
}

func truncateShellOutput(output string) string {
	if len(output) <= maxShellOutputBytes {
		return output
	}
	return output[:maxShellOutputBytes] + fmt.Sprintf("\n\n[... output truncated after %d bytes ...]", maxShellOutputBytes)
}

func commandFromInput(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if strings.HasPrefix(input, "{") {
		var object map[string]any
		if err := json.Unmarshal([]byte(input), &object); err == nil {
			if command, ok := object["command"].(string); ok {
				return strings.TrimSpace(command)
			}
		}
	}
	return input
}
